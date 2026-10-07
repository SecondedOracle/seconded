package client

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const standardDoor = "/v1/x402/checks"

// Standard terms are intentionally separate from original-door quote bindings.
// Preserve the complete accepted object; extensions cannot select payment pins.
type StandardTerms struct {
	Tier         string `json:"tier"`
	Schema       string `json:"product_schema_version"`
	Predicate    string `json:"predicate_version"`
	Expires      int64  `json:"expires_at"`
	MaxBefore    int64  `json:"max_valid_before"`
	MinRemaining int64  `json:"min_remaining_s"`
}
type StandardRequirement struct {
	Scheme  string `json:"scheme"`
	Network string `json:"network"`
	Amount  string `json:"amount"`
	Asset   string `json:"asset"`
	PayTo   string `json:"payTo"`
	Timeout int64  `json:"maxTimeoutSeconds"`
	Extra   struct {
		Name     string        `json:"name"`
		Version  string        `json:"version"`
		Ticket   string        `json:"ticket"`
		Seconded StandardTerms `json:"seconded"`
	} `json:"extra"`
}
type StandardChallenge struct {
	Version  int `json:"x402Version"`
	Resource struct {
		URL         string `json:"url"`
		Description string `json:"description,omitempty"`
		MimeType    string `json:"mimeType,omitempty"`
	} `json:"resource"`
	Accepts    []StandardRequirement `json:"accepts"`
	Extensions json.RawMessage       `json:"extensions,omitempty"`
	Seconded   struct {
		Price   string `json:"price_usd"`
		Expires int64  `json:"expires_at"`
		Binding string `json:"binding"`
	} `json:"seconded"`
}

func (a StandardRequirement) validatePins() error {
	p := pins(a.Network)
	amount, err := atomic(a.Amount)
	if err != nil || p.Network == "" || a.Scheme != "exact" || amount <= 0 || amount > MaxAuthorization || strings.ToLower(a.Asset) != p.Asset || strings.ToLower(a.PayTo) != PayTo || a.Extra.Name != p.Name || a.Extra.Version != p.Version || !hexDigest.MatchString(a.Extra.Ticket) {
		return errors.New("policy_mismatch")
	}
	q := a.Extra.Seconded
	// The qualified v1 cells all publish R=260, T=380, W=120, skew=30.
	if a.Timeout != 380 || q.MinRemaining != 260 || q.MaxBefore != q.Expires+a.Timeout+30 || q.Schema != "1" || q.Predicate != "1" {
		return ErrInvalid
	}
	return nil
}
func (c StandardChallenge) Validate(r Request, now int64) error {
	size, err := r.Size()
	if err != nil {
		return err
	}
	tier, price, err := ProductPriceOnNetwork(r.Product, size, r.Options.Network)
	if err != nil {
		return err
	}
	if c.Version != 2 || len(c.Accepts) != 1 || c.Seconded.Binding != "first_presentation" {
		return ErrInvalid
	}
	a := c.Accepts[0]
	q := a.Extra.Seconded
	if err = a.validatePins(); err != nil {
		return err
	}
	if a.Network != r.Options.Network || a.Amount != strconv.FormatInt(price, 10) || q.Tier != tier {
		return errors.New("policy_mismatch")
	}
	if q.Expires <= now || q.Expires > now+120 || c.Seconded.Expires != q.Expires || c.Seconded.Price != Dollars(price) {
		return ErrInvalid
	}
	return nil
}
func (a *API) standardChallenge(ctx context.Context, r Request) (Challenge, error) {
	status, b, h, err := a.request(ctx, "POST", standardDoor, r, nil)
	if err != nil {
		return Challenge{}, err
	}
	if status != 402 {
		return Challenge{}, publicAPIError(status, b)
	}
	var c StandardChallenge
	if DecodeStrict(b, &c, ResponseLimit) != nil {
		return Challenge{}, ErrInvalid
	}
	header, err := base64.StdEncoding.DecodeString(h.Get("PAYMENT-REQUIRED"))
	x, e1 := Canonical(header)
	y, e2 := Canonical(b)
	if err != nil || e1 != nil || e2 != nil || string(x) != string(y) || c.Resource.URL != a.url+standardDoor {
		return Challenge{}, ErrInvalid
	}
	if err = c.Validate(r, time.Now().Unix()); err != nil {
		return Challenge{}, err
	}
	nonce := make([]byte, 32)
	if _, err = rand.Read(nonce); err != nil {
		return Challenge{}, err
	}
	id := make([]byte, 16)
	if _, err = rand.Read(id); err != nil {
		return Challenge{}, err
	}
	terms := c.Accepts[0]
	q := terms.Extra.Seconded
	// A local recovery handle exists even when the first response is lost.
	return Challenge{Version: 2, Accepts: []Requirement{{Scheme: terms.Scheme, Network: terms.Network, Amount: terms.Amount, Asset: terms.Asset, PayTo: terms.PayTo, Timeout: terms.Timeout, Extra: Extra{terms.Extra.Name, terms.Extra.Version, "0x" + hex.EncodeToString(nonce)}}}, Seconded: QuoteBinding{CheckID: hex.EncodeToString(id), Tier: q.Tier, MaxBefore: q.MaxBefore, MinRemaining: q.MinRemaining}, standard: &c}, nil
}

type StandardPayload struct {
	Version  int                 `json:"x402Version"`
	Accepted StandardRequirement `json:"accepted"`
	Payload  struct {
		Signature     string        `json:"signature"`
		Authorization Authorization `json:"authorization"`
	} `json:"payload"`
}
type PurchaseAssociation struct {
	Version int    `json:"v"`
	SHA256  string `json:"sha256"`
}

// RecoveryRecord stays inside the private atomic ledger. It never enters MCP
// output. Raw body bytes and the encoded credential are reused exactly.
type RecoveryRecord struct {
	BodySHA256             string              `json:"body_sha256,omitempty"`
	CanonicalRequestSHA256 string              `json:"canonical_request_sha256,omitempty"`
	Format                 string              `json:"format"`
	URL                    string              `json:"url"`
	Body                   json.RawMessage     `json:"body,omitempty"`
	BodyBytes              string              `json:"body_bytes,omitempty"`
	PaymentSignatureSHA256 string              `json:"payment_signature_sha256"`
	PaymentSignature       string              `json:"payment_signature,omitempty"`
	Accepted               StandardRequirement `json:"accepted"`
	Quote                  StandardTerms       `json:"quote"`
	CanonicalRequest       string              `json:"canonical_request,omitempty"`
	Association            PurchaseAssociation `json:"purchase_association"`
	State                  string              `json:"state"`
	ServerCheckID          string              `json:"server_check_id,omitempty"`
	NextAttempt            int64               `json:"next_attempt,omitempty"`
	NewPurchaseAllowed     bool                `json:"new_purchase_allowed"`
}

func standardCanonical(r Request, a StandardRequirement) ([]byte, error) {
	return canonicalValue(map[string]any{"v": 1, "input": r.Input, "product": r.Product, "product_schema_version": a.Extra.Seconded.Schema, "predicate_version": a.Extra.Seconded.Predicate, "tier": a.Extra.Seconded.Tier, "price_atomic": a.Amount, "asset": a.Network + "/erc20:" + a.Asset, "network": a.Network, "scheme": a.Scheme, "pay_to": a.PayTo, "billing_mode": "paid"})
}
func association(canonical []byte, accepted StandardRequirement, auth Authorization) (PurchaseAssociation, error) {
	// The opaque public ticket is omitted by the service's association contract.
	raw, err := json.Marshal(accepted)
	if err != nil {
		return PurchaseAssociation{}, err
	}
	var terms map[string]any
	if json.Unmarshal(raw, &terms) != nil {
		return PurchaseAssociation{}, ErrInvalid
	}
	delete(terms["extra"].(map[string]any), "ticket")
	auth.From, auth.To, auth.Nonce = strings.ToLower(auth.From), strings.ToLower(auth.To), strings.ToLower(auth.Nonce)
	value, err := canonicalValue(map[string]any{"authorization": auth, "accepted": terms, "request": json.RawMessage(canonical)})
	if err != nil {
		return PurchaseAssociation{}, err
	}
	hash := sha256.Sum256(append([]byte("seconded-purchase-association/v1\x00"), value...))
	return PurchaseAssociation{1, hex.EncodeToString(hash[:])}, nil
}
func newRecovery(origin string, r Request, a StandardRequirement, auth Authorization, sig string) (*RecoveryRecord, error) {
	body, err := canonicalValue(r)
	if err != nil {
		return nil, err
	}
	canon, err := standardCanonical(r, a)
	if err != nil {
		return nil, err
	}
	assoc, err := association(canon, a, auth)
	if err != nil {
		return nil, err
	}
	payload := StandardPayload{Version: 2, Accepted: a}
	payload.Payload.Authorization = auth
	payload.Payload.Signature = sig
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	header := base64.StdEncoding.EncodeToString(raw)
	headerHash := sha256.Sum256([]byte(header))
	return &RecoveryRecord{BodyBytes: string(body), PaymentSignatureSHA256: hex.EncodeToString(headerHash[:]), Format: "seconded-door-recovery/v2", URL: origin + standardDoor, Body: body, PaymentSignature: header, Accepted: a, Quote: a.Extra.Seconded, CanonicalRequest: string(canon), Association: assoc, State: "unresolved"}, nil
}
func (r RecoveryRecord) Validate(e Entry) error {
	if !validComparisonEntry(e) || !validPortfolioEntry(e) {
		return ErrStorage
	}
	if !oneOf(r.Format, "seconded-door-recovery/v2", "seconded-door-archive/v1") || !oneOf(r.State, "unresolved", "resolved") || r.NextAttempt < 0 || (r.ServerCheckID != "" && !hexID.MatchString(r.ServerCheckID)) || r.Accepted.validatePins() != nil || r.Quote != r.Accepted.Extra.Seconded {
		return ErrStorage
	}
	if r.State == "resolved" && (e.Receipt == nil || e.RecoveryRequired) {
		return ErrStorage
	}
	if r.Format == "seconded-door-archive/v1" {
		if e.Receipt == nil || e.Payload != "" || len(r.Body) != 0 || r.BodyBytes != "" || r.CanonicalRequest != "" || r.PaymentSignature != "" ||
			!hexDigest.MatchString(r.BodySHA256) || !hexDigest.MatchString(r.CanonicalRequestSHA256) || !hexDigest.MatchString(r.PaymentSignatureSHA256) ||
			r.Association.Version != 1 || !hexDigest.MatchString(r.Association.SHA256) || r.ServerCheckID == "" ||
			r.Accepted.Network != e.Network || r.Accepted.Amount != strconv.FormatInt(e.Amount, 10) || r.Quote.Tier != e.Tier {
			return ErrStorage
		}
		state := e.Receipt.Envelope.State
		pending := oneOf(state, "running", "settling", "delayed", "unavailable", "refund_owed") ||
			(r.State == "unresolved" && oneOf(state, "refused", "content_refused", "frozen_unsettled"))
		if (r.State == "unresolved") != pending || e.RecoveryRequired != pending {
			return ErrStorage
		}
		// acceptReply checks expiry before archiving refusals. A later clock
		// rollback must not invalidate that stored decision or the whole ledger.
		if !pending && !oneOf(state, "included", "final", "released", "trial_delivered", "no_agreement", "closed_no_charge", "refunded", "service_failed", "refused", "content_refused", "frozen_unsettled") {
			return ErrStorage
		}
		return nil
	}
	// Validate the exact retained request and credential before replay.
	canonicalBody, bodyErr := Canonical(r.Body)
	headerHash := sha256.Sum256([]byte(r.PaymentSignature))
	if bodyErr != nil || string(canonicalBody) != r.BodyBytes || hex.EncodeToString(headerHash[:]) != r.PaymentSignatureSHA256 {
		return ErrStorage
	}
	var req Request
	if DecodeStrict(r.Body, &req, MessageLimit) != nil || req.Product != e.Product || req.Digest() != e.InputDigest || req.Options.Network != e.Network || req.Options.Payer != e.Payer {
		return ErrStorage
	}
	portfolioBinding, portfolioErr := portfolioRequestScope(req)
	if portfolioErr != nil || portfolioBinding != e.PortfolioScopeSHA256 {
		return ErrStorage
	}
	comparisonBinding, bindingErr := comparisonRequestHash(req)
	if bindingErr != nil || comparisonBinding != e.ComparisonRequestSHA256 {
		return ErrStorage
	}
	canon, err := standardCanonical(req, r.Accepted)
	if err != nil || string(canon) != r.CanonicalRequest {
		return ErrStorage
	}
	raw, err := base64.StdEncoding.DecodeString(r.PaymentSignature)
	if err != nil {
		return ErrStorage
	}
	var payload StandardPayload
	if DecodeStrict(raw, &payload, ResponseLimit) != nil || payload.Version != 2 || payload.Accepted != r.Accepted {
		return ErrStorage
	}
	a := payload.Payload.Authorization
	if a.From != e.Payer || a.To != PayTo || a.Value != strconv.FormatInt(e.Amount, 10) || a.ValidBefore != strconv.FormatInt(e.ValidBefore, 10) || a.Nonce != e.Nonce || r.Accepted.Network != e.Network || r.Accepted.Amount != a.Value || r.Quote.Tier != e.Tier {
		return ErrStorage
	}
	digest, err := a.standardDigestFor(e.Network)
	if err != nil {
		return ErrStorage
	}
	// Recover the payment signature without a wallet or a signing operation.
	signature, err := hex.DecodeString(strings.TrimPrefix(payload.Payload.Signature, "0x"))
	if err != nil {
		return ErrStorage
	}
	if verifyPaymentSigner(digest, signature, e.Payer) != nil {
		return ErrStorage
	}
	assoc, err := association(canon, r.Accepted, a)
	if err != nil || assoc != r.Association {
		return ErrStorage
	}
	return nil
}
func (l Ledger) requireRecovery() error {
	for _, e := range l.Entries {
		if e.RecoveryRequired || (e.Recovery != nil && (e.Recovery.State != "resolved" || !e.Recovery.NewPurchaseAllowed)) {
			return errors.New("presentation_indeterminate")
		}
	}
	return nil
}

// retryExpiry saturates before adding clock skew, including for old local data.
func retryExpiry(validBefore int64) int64 {
	return max(0, min(validBefore, int64(1<<63-1)-30)+30)
}

func (e Entry) retryAt() int64 {
	if e.Recovery == nil {
		return 0
	}
	expiry := retryExpiry(e.ValidBefore)
	next := e.Recovery.NextAttempt
	// Keep modest post-expiry backoff, including the writer's second round-up.
	// Excessive legacy deadlines fall back to the fixed expiry boundary, never
	// a moving now+60 deadline that would postpone recovery on every read.
	now := time.Now().Unix()
	if next > expiry && next > min(now, int64(1<<63-1)-61)+61 {
		return expiry
	}
	return next
}

func (a *API) replay(ctx context.Context, e Entry, wait int) (CheckReply, error) {
	r := e.Recovery
	if r == nil || r.Validate(e) != nil || r.URL != a.url+standardDoor {
		return CheckReply{}, ErrStorage
	}
	if r.Format == "seconded-door-archive/v1" || (r.State == "resolved" && !receiptNeedsCollection(e.Receipt)) || time.Now().Unix() < e.retryAt() {
		return CheckReply{}, errors.New("presentation_indeterminate")
	}
	status, b, h, err := a.request(ctx, "POST", standardDoor, json.RawMessage(r.BodyBytes), map[string]string{"PAYMENT-SIGNATURE": r.PaymentSignature, "Prefer": "wait=" + strconv.Itoa(wait)})
	return r.recoveryReply(status, b, h, err, e.ValidBefore)
}

func (r *RecoveryRecord) recoveryReply(status int, b []byte, h http.Header, err error, validBefore int64) (CheckReply, error) {
	// One attempt per tool call is the bounded recovery budget. Persist a lower
	// bound for the next attempt so restarts cannot evade bounded server backoff.
	// Round up because persisted deadlines have second precision: truncating
	// could allow a retry almost one second before the requested interval.
	now := time.Now().Unix() + 1
	r.NextAttempt = now + 1
	delay := int64(1)
	if raw := h.Get("Retry-After"); raw != "" {
		n, e := strconv.ParseInt(raw, 10, 64)
		if e != nil {
			when, e := http.ParseTime(raw)
			if e != nil {
				return CheckReply{}, ErrInvalid
			}
			n = when.Unix() - now
		}
		if n > delay {
			delay = n
		}
	}
	var reply CheckReply
	parseErr := err
	if err == nil {
		reply, parseErr = parseReply(status, b)
	}
	// Unsigned diagnostic hints affect only backoff, never payment resolution.
	var fields map[string]json.RawMessage
	if json.Unmarshal(b, &fields) == nil && fields["hints"] != nil {
		var hints Hints
		if DecodeStrict(fields["hints"], &hints, ResponseLimit) == nil && int64(hints.PollAfter) > delay {
			delay = int64(hints.PollAfter)
		}
	}
	if int64(reply.Hints.PollAfter) > delay {
		delay = int64(reply.Hints.PollAfter)
	}
	// Retain up to a minute of server backoff after expiry to avoid hammering an
	// overloaded service. Only verified receipts can resolve the payment or
	// permit a new purchase; unsigned timing only schedules the next attempt.
	deadline := max(retryExpiry(validBefore), min(now, int64(1<<63-1)-60)+60)
	delay = min(delay, deadline-now)
	r.NextAttempt = now + delay
	return reply, parseErr
}
func (e *Entry) acceptReply(reply CheckReply) {
	e.Receipt = &reply.Receipt
	e.RecoveryRequired = false
	if e.Recovery == nil {
		return
	}
	r := e.Recovery
	r.ServerCheckID = reply.Receipt.Envelope.CheckID
	r.NewPurchaseAllowed = reply.Hints.NewQuoteAllowed
	e.Commitment = reply.Receipt.Envelope.Commitment
	state := reply.Receipt.Envelope.State
	pending := oneOf(state, "running", "settling", "delayed", "unavailable", "refund_owed")
	// A refusal leaves the authorization usable until expiry; keep it private.
	if oneOf(state, "refused", "content_refused", "frozen_unsettled") && time.Now().Unix() <= e.ValidBefore {
		pending = true
	}
	e.RecoveryRequired = pending
	if pending {
		r.State = "unresolved"
	} else {
		r.State = "resolved"
		r.NewPurchaseAllowed = state != "service_failed" || reply.Hints.NewQuoteAllowed
		e.archiveRecovery()
	}
}

// An answer can be released before settlement is final. Keep collecting those
// revisions without treating a delivered answer as an indeterminate purchase.
func receiptNeedsCollection(receipt *SignedReceipt) bool {
	if receipt == nil {
		return true
	}
	s := receipt.Envelope
	return oneOf(s.State, "running", "settling", "delayed", "unavailable", "refund_owed", "included") ||
		(s.State == "released" && s.Billing.Settlement == "unknown")
}

// archiveRecovery is called only after receipt verification. The next atomic
// ledger save commits the answer and removal together; a failed write leaves
// the previous replayable record on disk. Copy before clearing shared state.
func (e *Entry) archiveRecovery() {
	if e.Recovery == nil || e.Recovery.State != "resolved" || e.Recovery.Format == "seconded-door-archive/v1" || receiptNeedsCollection(e.Receipt) {
		return
	}
	r := *e.Recovery
	bodyHash := sha256.Sum256([]byte(r.BodyBytes))
	canonicalHash := sha256.Sum256([]byte(r.CanonicalRequest))
	r.BodySHA256, r.CanonicalRequestSHA256 = hex.EncodeToString(bodyHash[:]), hex.EncodeToString(canonicalHash[:])
	r.Format = "seconded-door-archive/v1"
	r.Body, r.BodyBytes, r.CanonicalRequest, r.PaymentSignature = nil, "", "", ""
	e.Payload = ""
	e.Recovery = &r
}

// Legacy archives have erased the payment credential and need the same owner
// proof as original-door entries. Retained standard records need no signer.
func (g *Engine) recoverySigner(e Entry) (*LocalSigner, error) {
	if e.Recovery != nil && e.Recovery.Format != "seconded-door-archive/v1" {
		return nil, nil
	}
	if g.Store == nil {
		return nil, errors.New("wallet_recovery_required")
	}
	v, err := LoadVault(g.Store)
	if err != nil {
		return nil, err
	}
	return signerFrom(v)
}

// UseOriginalDoorRollback is a launch-time rollback selector, never an MCP
// argument and never an automatic response to a standard-door failure.
func (a *API) UseOriginalDoorRollback() { a.originalDoor = true }
