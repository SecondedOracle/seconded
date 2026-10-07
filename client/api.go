package client

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type Trial struct {
	Eligible  bool `json:"eligible"`
	Remaining int  `json:"remaining"`
}
type FairUse struct {
	State       string  `json:"state"`
	Lane        string  `json:"lane,omitempty"`
	Headroom    string  `json:"headroom_usd"`
	WouldAdmit  bool    `json:"would_admit"`
	AdmittedVia *string `json:"admitted_via"`
	Reason      string  `json:"reason,omitempty"`
	Graduation  struct {
		Count      int    `json:"paid_final_count"`
		Revenue    string `json:"settled_revenue_usd"`
		EligibleAt *int64 `json:"eligible_at,omitempty"`
	} `json:"graduation,omitempty"`
	Trial        Trial    `json:"trial,omitempty"`
	NextRecovery *int64   `json:"next_recovery_at,omitempty"`
	RetryAfter   *int     `json:"retry_after,omitempty"`
	Remedies     []Remedy `json:"remedies"`
}

func oneOf(s string, values ...string) bool {
	for _, v := range values {
		if s == v {
			return true
		}
	}
	return false
}
func (t Trial) validate() bool {
	return t.Remaining >= 0 && t.Remaining <= 2 && (!t.Eligible || t.Remaining > 0)
}
func (f FairUse) validate() bool {
	_, e := USD(f.Headroom)
	_, e2 := USD(f.Graduation.Revenue)
	if f.Graduation.Revenue == "" {
		e2 = nil
	}
	if e != nil || e2 != nil || !oneOf(f.State, "ok", "recovery_only", "exhausted", "service_exhausted") || !oneOf(f.Lane, "", "new", "proven") || !oneOf(f.Reason, "", "unpaid_work_budget_exhausted", "service_budget_exhausted") || f.Graduation.Count < 0 || !f.Trial.validate() {
		return false
	}
	if f.AdmittedVia != nil && !oneOf(*f.AdmittedVia, "normal", "recovery", "withheld_result") {
		return false
	}
	if f.RetryAfter != nil && *f.RetryAfter < 0 {
		return false
	}
	for _, r := range f.Remedies {
		if !r.validate() || r.ID == "pay_withheld_result" {
			return false
		}
	}
	if f.State == "service_exhausted" && (f.Reason != "service_budget_exhausted" || len(f.Remedies) != 0) {
		return false
	}
	for _, r := range f.Remedies {
		if r.ID == "recovery_check" && (f.NextRecovery == nil || *r.AvailableAt != *f.NextRecovery) {
			return false
		}
	}
	return true
}

type Extra struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Nonce   string `json:"authNonce"`
}
type Requirement struct {
	Scheme  string `json:"scheme"`
	Network string `json:"network"`
	Amount  string `json:"amount"`
	Asset   string `json:"asset"`
	PayTo   string `json:"payTo"`
	Timeout int64  `json:"maxTimeoutSeconds"`
	Extra   Extra  `json:"extra"`
}

// QuoteBinding is the "seconded" object server/payments/quotes.py mint_quote() returns;
// tests/contract/client-fixtures.json pins it to the server's output.
const ClientVersion = "0.4.2"

var semanticVersion = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?(?:\+[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?$`)

// minimumVersionOK validates both SemVer strings, then compares precedence.
// Decimal components are compared by length to avoid integer overflow. Build
// metadata has no precedence; numeric prerelease identifiers cannot have zeros
// at the start unless the identifier is exactly "0".
func minimumVersionOK(current, minimum string) bool {
	parse := func(s string) []string {
		if len(s) > 256 {
			return nil
		}
		m := semanticVersion.FindStringSubmatch(s)
		if m == nil {
			return nil
		}
		for _, part := range strings.Split(m[4], ".") {
			if numericVersionPart(part) && len(part) > 1 && part[0] == '0' {
				return nil
			}
		}
		return m
	}
	a, b := parse(current), parse(minimum)
	if a == nil || b == nil {
		return false
	}
	compareNumber := func(x, y string) int {
		if len(x) < len(y) {
			return -1
		}
		if len(x) > len(y) {
			return 1
		}
		return strings.Compare(x, y)
	}
	for i := 1; i <= 3; i++ {
		if cmp := compareNumber(a[i], b[i]); cmp != 0 {
			return cmp > 0
		}
	}
	if a[4] == "" || b[4] == "" {
		return a[4] == ""
	}
	ap, bp := strings.Split(a[4], "."), strings.Split(b[4], ".")
	for i := 0; i < len(ap) && i < len(bp); i++ {
		x, y := ap[i], bp[i]
		xn, yn := numericVersionPart(x), numericVersionPart(y)
		cmp := strings.Compare(x, y)
		if xn && yn {
			cmp = compareNumber(x, y)
		} else if xn != yn {
			return !xn
		}
		if cmp != 0 {
			return cmp > 0
		}
	}
	return len(ap) >= len(bp)
}

func numericVersionPart(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

type QuoteBinding struct {
	QuoteID       string `json:"quote_id"`
	CheckID       string `json:"check_id"`
	Tier          string `json:"tier"`
	Bytes         int    `json:"billable_bytes"`
	Price         string `json:"price_usd"`
	Salt          string `json:"commitment_salt"`
	Commitment    string `json:"request_commitment"`
	MaxBefore     int64  `json:"max_valid_before"`
	MinRemaining  int64  `json:"min_remaining_s"`
	Expires       int64  `json:"expires_at"`
	RecoveryNonce string `json:"recovery_nonce"`
	Binding       string `json:"binding"`
}

// The API adds x402 discovery metadata to every challenge (server/api/app.py _to_response). It is descriptive
// only: nothing in it selects an amount, network or destination, so it is decoded and otherwise ignored.
type Challenge struct {
	Version    int             `json:"x402Version"`
	Accepts    []Requirement   `json:"accepts"`
	Seconded   QuoteBinding    `json:"seconded"`
	Extensions json.RawMessage `json:"extensions,omitempty"`
	standard   *StandardChallenge
}

func (c Challenge) Validate(req Request, now int64) error {
	if c.standard != nil {
		return c.standard.Validate(req, now)
	}
	size, e := req.Size()
	if e != nil {
		return e
	}
	tier, amount, e := ProductPriceOnNetwork(req.Product, size, req.Options.Network)
	if e != nil {
		return e
	}
	if c.Version != 2 || len(c.Accepts) != 1 {
		return ErrInvalid
	}
	a := c.Accepts[0]
	q := c.Seconded
	if a.Scheme != "exact" || a.Network != req.Options.Network || a.Amount != strconv.FormatInt(amount, 10) || strings.ToLower(a.Asset) != pins(req.Options.Network).Asset || strings.ToLower(a.PayTo) != PayTo || a.Timeout != ValiditySeconds || a.Extra.Name != pins(req.Options.Network).Name || a.Extra.Version != pins(req.Options.Network).Version || !hexWord.MatchString(a.Extra.Nonce) {
		return errors.New("policy_mismatch")
	}
	commit, e := req.Commitment(q.Salt)
	if e != nil || q.Commitment != commit || q.Tier != tier || q.Bytes != size || q.Price != Dollars(amount) || !hexID.MatchString(q.QuoteID) || !hexID.MatchString(q.CheckID) || !hexID.MatchString(q.RecoveryNonce) || q.Binding != "quote_nonce" || q.Expires <= now || q.Expires > now+120 || q.MaxBefore != q.Expires+ValiditySeconds || q.MinRemaining <= 0 || q.MinRemaining > 285 {
		return ErrInvalid
	}
	return nil
}

type Quote struct {
	Product    string `json:"product"`
	Tier       string `json:"tier"`
	Bytes      int    `json:"billable_bytes"`
	Price      string `json:"price_usd"`
	TokenCheck string `json:"token_check"`
	Networks   []struct {
		Network string `json:"network"`
		Asset   string `json:"asset"`
		Amount  string `json:"amount_atomic"`
	} `json:"networks"`
	FairUse        *FairUse `json:"fair_use,omitempty"`
	Trial          Trial    `json:"trial,omitempty"`
	BillingVersion string   `json:"billing_contract_version,omitempty"`
	Disclaimer     string   `json:"disclaimer,omitempty"`
}

func (q Quote) Validate(req Request) error {
	size, e := req.Size()
	if e != nil {
		return e
	}
	tier, amount, e := ProductPriceOnNetwork(req.Product, size, req.Options.Network)
	if e != nil {
		return e
	}
	// The current preview omits legacy trial/version metadata. Validate it when
	// present; all price, network and asset fields remain mandatory and pinned.
	if q.Product != req.Product || q.Tier != tier || q.Bytes != size || q.Price != Dollars(amount) || q.TokenCheck != "at_challenge" || !oneOf(q.BillingVersion, "", "2026-09-23.1") || len(q.Networks) != 1 || !q.Trial.validate() {
		return ErrInvalid
	}
	n := q.Networks[0]
	if n.Network != req.Options.Network || strings.ToLower(n.Asset) != req.Options.Network+"/erc20:"+pins(req.Options.Network).Asset || n.Amount != strconv.FormatInt(amount, 10) {
		return ErrInvalid
	}
	if q.FairUse != nil && !q.FairUse.validate() {
		return ErrInvalid
	}
	if q.Disclaimer != disclaimer(req.Product) {
		return ErrInvalid
	}
	return nil
}

type PaymentPayload struct {
	Version  int         `json:"x402Version"`
	Accepted Requirement `json:"accepted"`
	Payload  struct {
		Signature     string        `json:"signature"`
		Authorization Authorization `json:"authorization"`
	} `json:"payload"`
}
type Hints struct {
	PollAfter       int  `json:"poll_after_s,omitempty" seconded:"nullable"`
	DoNotResign     bool `json:"do_not_resign,omitempty"`
	NewQuoteAllowed bool `json:"new_quote_allowed,omitempty"`
}
type CheckReply struct {
	Error      string        `json:"error,omitempty"`
	RetryAfter int           `json:"retry_after,omitempty"`
	ServerTime int64         `json:"server_time,omitempty"`
	Receipt    SignedReceipt `json:"receipt"`
	Hints      Hints         `json:"hints"`
	Status     string        `json:"status,omitempty"`
	Message    string        `json:"message,omitempty"`
	Reason     string        `json:"reason,omitempty"`
	httpStatus int
}
type API struct {
	url            string
	http           *http.Client
	verifier       ReceiptVerifier
	verifyIdentity bool
	// Original door is an explicit rollback only; zero value selects standard x402.
	originalDoor bool
}

// Web PKI authenticates transport across certificate renewals. Receipt keys are
// separate network-specific release pins; neither has a flag/env bypass.
const releaseAPIOrigin = "https://api.secondedoracle.xyz"

func releaseIdentityConfigured() bool {
	return apiOrigin() != "" && len(receiptKeysFor(Network)) > 0
}

func ReleaseAPI() (*API, error) {
	if !releaseIdentityConfigured() {
		return nil, errors.New("release_identity_not_configured")
	}
	origin := apiOrigin()
	h, e := pinnedHTTP(origin, nil)
	if e != nil {
		return nil, e
	}
	return &API{origin, h, ReceiptVerifier{nil, releaseLabels}, true, false}, nil
}

func (a *API) request(ctx context.Context, method, path string, body any, headers map[string]string) (int, []byte, http.Header, error) {
	// Check advertised key continuity before disclosing input or authorization.
	// This is not proof of key possession; signed receipts remain mandatory.
	if a.verifyIdentity && path != "/v1/keys" {
		if err := a.VerifyIdentity(ctx); err != nil {
			return 0, nil, nil, err
		}
	}
	var b []byte
	var e error
	if body != nil {
		b, e = canonicalValue(body)
		if e != nil {
			return 0, nil, nil, e
		}
	}
	req, e := http.NewRequestWithContext(ctx, method, a.url+path, jsonBody(b))
	if e != nil {
		return 0, nil, nil, ErrInvalid
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	// Apply identity last so request-specific headers cannot override it.
	req.Header.Set("SECONDED-CLIENT-VERSION", Version)
	resp, e := a.http.Do(req)
	if e != nil {
		if errors.Is(e, errAPIIdentityMismatch) {
			return 0, nil, nil, errAPIIdentityMismatch
		}
		return 0, nil, nil, errors.New("api_unreachable")
	}
	b, e = readResponse(resp)
	if e != nil {
		// Recovery must retain Retry-After even for a plain-text overload response.
		return resp.StatusCode, nil, resp.Header, e
	}
	observeLatestVersion(ctx, resp.Header)
	// Version signaling is checked before a challenge can reach the signing path.
	minimums := resp.Header.Values("SECONDED-MIN-CLIENT-VERSION")
	if resp.StatusCode == http.StatusUpgradeRequired || len(minimums) > 1 ||
		(len(minimums) == 1 && !minimumVersionOK(Version, minimums[0])) {
		return 0, nil, nil, errors.New("client_upgrade_required")
	}
	return resp.StatusCode, b, resp.Header, nil
}
func (a *API) Quote(ctx context.Context, r Request) (Quote, error) {
	var q Quote
	var normalizeErr error
	r, normalizeErr = normalizePortfolioRequest(r)
	if normalizeErr != nil {
		return q, normalizeErr
	}
	if _, err := r.Size(); err != nil {
		return q, err
	}
	status, b, _, e := a.request(ctx, "POST", "/v1/quote", r, nil)
	if e != nil {
		return q, e
	}
	return parseQuote(status, b, r)
}
func parseQuote(status int, b []byte, r Request) (Quote, error) {
	var q Quote
	if status != 200 {
		return q, publicAPIError(status, b)
	}
	if DecodeStrict(b, &q, ResponseLimit) != nil {
		return q, ErrInvalid
	}
	return q, q.Validate(r)
}
func (a *API) Challenge(ctx context.Context, r Request) (Challenge, error) {
	if _, err := r.Size(); err != nil {
		return Challenge{}, err
	}
	if r.Product == comparisonProduct && (a.originalDoor || !comparisonClientVersion(Version)) {
		return Challenge{}, errors.New("client_upgrade_required")
	}
	if !a.originalDoor {
		return a.standardChallenge(ctx, r)
	}
	status, b, h, e := a.request(ctx, "POST", "/v1/checks", r, nil)
	if e != nil {
		return Challenge{}, e
	}
	return parseChallenge(status, b, h.Get("PAYMENT-REQUIRED"), r, time.Now().Unix())
}
func parseChallenge(status int, b []byte, paymentRequired string, r Request, now int64) (Challenge, error) {
	var c Challenge
	if status != 402 {
		return c, publicAPIError(status, b)
	}
	if DecodeStrict(b, &c, ResponseLimit) != nil {
		return c, ErrInvalid
	}
	header, e := base64.StdEncoding.DecodeString(paymentRequired)
	if e != nil || len(header) > ResponseLimit {
		return c, ErrInvalid
	}
	hb, e := Canonical(header)
	bb, e2 := Canonical(b)
	if e != nil || e2 != nil || string(hb) != string(bb) {
		return c, ErrInvalid
	}
	return c, c.Validate(r, now)
}
func parseReply(status int, b []byte) (CheckReply, error) {
	var r CheckReply
	if DecodeStrict(b, &r, ResponseLimit) != nil || r.Hints.PollAfter < 0 || r.Hints.PollAfter > 30 ||
		(r.Receipt.Envelope.Product == portfolioProduct && len(b) >= ResponseLimit) {
		return r, ErrInvalid
	}
	// An authenticated terminal service failure is authoritative on any HTTP
	// status; the verifier still enforces its signature, association and billing.
	if status < 100 || status > 599 || (!oneOf(strconv.Itoa(status), "200", "202", "400", "403", "409", "413", "422", "423", "429", "503") && r.Receipt.Envelope.State != "service_failed") {
		return r, ErrInvalid
	}
	if status == 202 && r.Receipt.Envelope.State != "service_failed" && (!r.Hints.DoNotResign || r.Hints.PollAfter == 0) {
		return r, ErrInvalid
	}
	if r.Receipt.Envelope.State == "service_failed" {
		var fields map[string]json.RawMessage
		if json.Unmarshal(b, &fields) != nil {
			return r, ErrInvalid
		}
		if _, present := fields["reason"]; present && r.Reason == "" {
			return r, ErrInvalid
		}
	}
	r.httpStatus = status
	return r, nil
}
func (a *API) Present(ctx context.Context, r Request, payload string, wait int) (CheckReply, error) {
	status, b, _, e := a.request(ctx, "POST", "/v1/checks", r, map[string]string{"PAYMENT-SIGNATURE": payload, "Prefer": "wait=" + strconv.Itoa(wait)})
	if e != nil {
		return CheckReply{}, e
	}
	return parseReply(status, b)
}
func (a *API) Collect(ctx context.Context, e Entry, signer Signer) (CheckReply, error) {
	var archived *RecoveryRecord
	if e.Recovery != nil {
		if e.Recovery.Format != "seconded-door-archive/v1" {
			return a.replay(ctx, e, 0)
		}
		archived = e.Recovery
		if archived.Validate(e) != nil || archived.URL != a.url+standardDoor {
			return CheckReply{}, ErrStorage
		}
		if !receiptNeedsCollection(e.Receipt) || time.Now().Unix() < e.retryAt() {
			return CheckReply{}, errors.New("presentation_indeterminate")
		}
		// The local recovery handle differs from the signed server admission ID.
		e.CheckID = archived.ServerCheckID
		archived.NextAttempt = time.Now().Unix() + 2
	}
	if !hexID.MatchString(e.CheckID) || signer == nil || signer.Address() != e.Payer {
		return CheckReply{}, ErrInvalid
	}
	status, b, h, err := a.request(ctx, "GET", "/v1/checks/"+e.CheckID, nil, nil)
	if archived != nil && (err != nil || status != 401) {
		_, err = archived.recoveryReply(status, b, h, err, e.ValidBefore)
		if err == nil {
			err = ErrInvalid
		}
		return CheckReply{}, err
	}
	if err != nil {
		return CheckReply{}, err
	}
	if status != 401 {
		return CheckReply{}, ErrInvalid
	}
	var challenge OwnershipChallenge
	if DecodeStrict(b, &challenge, ResponseLimit) != nil || challenge.Validate(e.CheckID, pins(e.Network).ChainID, time.Now().Unix()) != nil || signer.Address() != e.Payer {
		return CheckReply{}, ErrInvalid
	}
	message := challenge.Ownership.Message
	d, err := challenge.Ownership.Digest()
	if err != nil {
		return CheckReply{}, err
	}
	sig, err := signChecked(signer, d)
	if err != nil {
		return CheckReply{}, err
	}
	proof, _ := json.Marshal(map[string]any{"check_id": e.CheckID, "nonce": message.Nonce, "expires_at": message.Expires, "signature": sig})
	status, b, h, err = a.request(ctx, "GET", "/v1/checks/"+e.CheckID, nil, map[string]string{"SECONDED-OWNERSHIP": base64.StdEncoding.EncodeToString(proof), "Prefer": "wait=0"})
	if archived != nil {
		return archived.recoveryReply(status, b, h, err, e.ValidBefore)
	}
	if err != nil {
		return CheckReply{}, err
	}
	return parseReply(status, b)
}

// PaymentDoorClosedError identifies an unavailable payment network before payment.
type PaymentDoorClosedError struct{}

func (*PaymentDoorClosedError) Error() string { return "payment door closed for this network" }

// Unsigned prepayment errors are diagnostic only; they cannot resolve a payment.
func publicAPIError(status int, body []byte) error {
	if status < 400 || status > 599 {
		return ErrInvalid
	}
	var wire struct {
		Error   string          `json:"error"`
		Receipt json.RawMessage `json:"receipt,omitempty"`
		Reason  string          `json:"reason,omitempty"`
		Hints   json.RawMessage `json:"hints,omitempty"`
	}
	if DecodeStrict(body, &wire, ResponseLimit) != nil {
		return ErrInvalid
	}
	if oneOf(wire.Error, "cell_unavailable", "no_open_cell") ||
		oneOf(wire.Reason, "cell_unavailable", "no_open_cell") {
		return &PaymentDoorClosedError{}
	}
	if wire.Error == "invalid_input" {
		if oneOf(wire.Reason, "input_not_object", "unknown_field", "missing_field", "invalid_field", "input_too_large", "input_too_deep", "input_tokens_exceed_cap", "network_not_enabled", "invalid_address", "product_unknown", "product_unpriced", "invalid_tier", "stock_registry_unverified_for_chain", "unknown_product", "input_not_canonicalizable", "max_price", "bad_content_length", "malformed_json", "body_not_object", "product_required", "input_required", "options_not_object", "unknown_option") {
			return errors.New(wire.Reason)
		}
		return errors.New("invalid_input")
	}
	if oneOf(wire.Error, "store_unavailable", "rate_limited", "input_too_large", "client_upgrade_required", "product_in_testing", "unsupported_network", "price_exceeds_max") {
		return errors.New(wire.Error)
	}
	return ErrInvalid
}
