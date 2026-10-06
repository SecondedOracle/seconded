package client

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"math/big"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

type Answer struct {
	PriceComparison      *StockPriceComparison `json:"price_comparison,omitempty"`
	TradePreview         *TradePreview         `json:"trade_preview,omitempty"`
	DataAsOf             map[string]*string    `json:"data_as_of,omitzero"`
	DataFreshness        string                `json:"data_freshness,omitempty"`
	CheckObservations    *CheckObservations    `json:"check_observations,omitempty"`
	PaidReviewers        *PaidReviewers        `json:"paid_reviewers,omitempty"`
	PortfolioReport      *PortfolioReport      `json:"portfolio_report,omitempty"`
	LendingObservations  *LendingObservations  `json:"lending_observations,omitempty"`
	Comparison           *Comparison           `json:"comparison,omitempty"`
	RegistryObservations *RegistryObservations `json:"registry_observations,omitempty"`
	Option               int                   `json:"option"`
	LabelID              string                `json:"label_id"`
	Parity               *StockParity          `json:"parity,omitempty"`
	MarketStatus         *StockMarketStatus    `json:"market_status,omitempty"`
}

// NotVerifiedMessage is the text no_agreement receipts carried before the server could veto; those still verify.
const NotVerifiedMessage = "NOT VERIFIED: our two models disagreed. Do not act on this automatically; pause or ask a human."

// Current no_agreement receipts can also veto a verified fact, so the server says so. These must equal
// NOT_VERIFIED_MESSAGE and REASONS in server/payments/disagreement.py; tests/receipts enforces it.
const CurrentNotVerifiedMessage = "NOT VERIFIED: no usable, verified agreement was reached. Do not act on this automatically; pause or ask a human."

var legacyNotVerifiedReasons = []string{"disagreed", "model_declined", "timed_out"}
var notVerifiedReasons = []string{"agreed_abstain_label", "bridge_classification_unverified", "contradicts_verified_fact", "counterparty_list_future_or_inconsistent_source_date", "counterparty_list_partial_snapshot", "counterparty_list_snapshot_coverage_unsupported", "counterparty_list_snapshot_kind_unsupported", "counterparty_list_snapshot_malformed", "counterparty_list_snapshot_unavailable", "counterparty_list_stale_list", "counterparty_list_unavailable", "detected_hazard", "disagreed", "evidence_unbound", "funding_history_cross_chain_endpoints", "funding_history_native_history_unmeasured", "funding_history_transfer_log_limit", "funding_history_unavailable", "hidden_text_unresolved", "model_declined", "response_incomplete", "timed_out", "unsupported_oracle"}

// Mirrors server/payments/runner.py PUBLIC_SERVICE_FAILURE_REASONS; server fixtures cover every value.
var serviceFailureReasons = []string{
	"engine_error", "prefetch_unavailable", "provider_unavailable",
}

type Billing struct {
	Mode       string  `json:"mode"`
	Charged    string  `json:"charged"`
	Network    string  `json:"network"`
	Asset      string  `json:"asset"`
	Amount     string  `json:"amount_atomic"`
	Payer      string  `json:"payer"`
	PayTo      string  `json:"pay_to"`
	Tx         *string `json:"tx"`
	Settlement string  `json:"settlement"`
	// Refund names the refund state ("refund_owed" or "refunded") and is null otherwise. The server has always
	// signed it as that string (server/api/openapi.py Billing); it carries no refund transaction.
	Refund *string `json:"refund"`
}
type ReceiptEnvelope struct {
	PurchaseAssociation     *PurchaseAssociation `json:"purchase_association,omitempty"`
	CheckedBy               []string             `json:"checked_by"`
	ModelConfigFingerprint  string               `json:"model_config_fingerprint"`
	ModelConfigFingerprints map[string]string    `json:"model_config_fingerprints"`
	Version                 int                  `json:"v"`
	Kind                    string               `json:"kind"`
	CheckID                 string               `json:"check_id"`
	Seq                     int64                `json:"seq"`
	IssuedAt                string               `json:"issued_at"`
	OutcomeAt               *string              `json:"outcome_at"`
	Product                 string               `json:"product"`
	SchemaVersion           string               `json:"product_schema_version"`
	PredicateVersion        string               `json:"predicate_version"`
	TemplateVersion         string               `json:"template_version"`
	ModelPairID             string               `json:"model_pair_id"`
	Tier                    string               `json:"tier"`
	Digest                  string               `json:"request_digest" seconded:"nullable"`
	Commitment              string               `json:"request_commitment"`
	State                   string               `json:"state"`
	Outcome                 string               `json:"outcome" seconded:"nullable"`
	Status                  string               `json:"status,omitempty"`
	Message                 string               `json:"message,omitempty"`
	Reason                  string               `json:"reason,omitempty"`
	Answer                  *Answer              `json:"answer,omitempty"`
	Disclaimer              string               `json:"disclaimer,omitempty" seconded:"nullable"`
	Billing                 Billing              `json:"billing"`
	Lane                    string               `json:"lane"`
	UnfreezeAt              *string              `json:"unfreeze_at"`
	KeyID                   string               `json:"key_id"`
	Verification            *Verification        `json:"verification,omitempty"`
}

// Verification is receipt v3's signed block: measured facts and verified source spans, per-fact coverage,
// and the SHA-256 of the canonical fact sheet the models saw. Server home: server/checks/findings.py.
type Verification struct {
	Schema         string            `json:"schema"`
	EvidenceSHA256 string            `json:"evidence_sha256"`
	Block          *ReceiptBlock     `json:"block"`
	Findings       []ReceiptFinding  `json:"findings"`
	Coverage       []ReceiptCoverage `json:"coverage"`
}
type ReceiptBlock struct {
	ChainID int64  `json:"chain_id"`
	Number  int64  `json:"number"`
	Hash    string `json:"hash"`
}

// Values hold the measured numbers, e.g. {"tip_multiple":"57.2","excess_usd":"0.81"}. Non-integers are
// decimal strings, so the signed bytes never depend on float formatting.
type ReceiptFinding struct {
	Code     string         `json:"code"`
	Severity string         `json:"severity"`
	Values   map[string]any `json:"values"`
}
type ReceiptCoverage struct {
	Fact   string  `json:"fact"`
	Status string  `json:"status"`
	Reason *string `json:"reason"`
}

const VerificationSchema = "seconded-verification/v1"

// Bounds mirror server/checks/findings.py; the golden vectors in tests/client/verified-v3.json pin both.
const (
	maxSafeInteger     = 9007199254740991
	maxFindings        = 48
	maxCoverage        = 32
	maxFindingValues   = 12
	maxFindingList     = 8
	maxFindingText     = 128
	maxCoverageReason  = 256
	maxVerificationLen = 16384
)

var (
	findingName    = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)
	coverageFact   = regexp.MustCompile(`^[a-z][a-z0-9_]{0,31}(\.[a-z][a-z0-9_]{0,31}){0,2}$`)
	coverageReason = regexp.MustCompile(`^[a-z][a-z0-9_.:-]{0,63}(,[a-z][a-z0-9_.:-]{0,63}){0,7}$`)
	safeInteger    = regexp.MustCompile(`^-?(0|[1-9][0-9]{0,15})$`)
)

func findingText(s string) bool {
	if len(s) > maxFindingText {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < 32 || s[i] > 126 || s[i] == '"' || s[i] == '\\' {
			return false
		}
	}
	return true
}

func findingScalar(raw json.RawMessage) bool {
	switch s := string(raw); {
	case s == "null" || s == "true" || s == "false":
		return true
	case strings.HasPrefix(s, `"`):
		var text string
		return json.Unmarshal(raw, &text) == nil && findingText(text)
	default:
		n, err := strconv.ParseInt(s, 10, 64)
		return safeInteger.MatchString(s) && err == nil && n >= -maxSafeInteger && n <= maxSafeInteger
	}
}

// Values arrive as json.Number, strings, bools, nil and slices; judge their JSON form, not Go types.
func findingValue(value any) bool {
	raw, err := json.Marshal(value)
	if err != nil {
		return false
	}
	if !strings.HasPrefix(string(raw), "[") {
		return findingScalar(raw)
	}
	var items []json.RawMessage
	if json.Unmarshal(raw, &items) != nil || len(items) > maxFindingList {
		return false
	}
	for _, item := range items {
		if !findingScalar(item) {
			return false
		}
	}
	return true
}

func validVerification(v Verification) bool {
	if v.Schema != VerificationSchema || !configFingerprint.MatchString(v.EvidenceSHA256) || v.Findings == nil ||
		len(v.Findings) > maxFindings || len(v.Coverage) == 0 || len(v.Coverage) > maxCoverage {
		return false
	}
	if b := v.Block; b != nil && (b.ChainID < 1 || b.ChainID > maxSafeInteger || b.Number < 0 || b.Number > maxSafeInteger || !hexWord.MatchString(b.Hash)) {
		return false
	}
	seen := map[string]bool{}
	for _, f := range v.Findings {
		if !findingName.MatchString(f.Code) || !oneOf(f.Severity, "hard", "soft") || f.Values == nil || len(f.Values) > maxFindingValues {
			return false
		}
		for name, value := range f.Values {
			validValue := findingValue(value)
			if strings.HasPrefix(f.Code, "hidden_") && name == "quote" {
				quote, ok := value.(string)
				validValue = ok && utf8.ValidString(quote) && len(quote) > 0 && len(quote) <= maxFindingText
			}
			if !findingName.MatchString(name) || !validValue {
				return false
			}
		}
		key, err := canonicalValue(f)
		if err != nil || seen[string(key)] {
			return false
		}
		seen[string(key)] = true
	}
	facts := map[string]bool{}
	for _, c := range v.Coverage {
		if !coverageFact.MatchString(c.Fact) || facts[c.Fact] || !oneOf(c.Status, "checked", "partial", "not_checked", "unavailable") {
			return false
		}
		// Anything short of checked must say why; a checked fact with a caveat would be partial.
		if (c.Status == "checked") != (c.Reason == nil) || (c.Reason != nil && (len(*c.Reason) > maxCoverageReason || !coverageReason.MatchString(*c.Reason))) {
			return false
		}
		facts[c.Fact] = true
	}
	body, err := canonicalValue(v)
	return err == nil && len(body) <= maxVerificationLen
}

func sameVerification(a, b *Verification) bool {
	if a == nil || b == nil {
		return a == b
	}
	x, errA := canonicalValue(a)
	y, errB := canonicalValue(b)
	return errA == nil && errB == nil && string(x) == string(y)
}

type SignedReceipt struct {
	Envelope    ReceiptEnvelope `json:"envelope"`
	Signature   string          `json:"sig"`
	KeyID       string          `json:"key_id"`
	rawEnvelope json.RawMessage
}

func (r *SignedReceipt) UnmarshalJSON(data []byte) error {
	var wire struct {
		Envelope  json.RawMessage `json:"envelope"`
		Signature string          `json:"sig"`
		KeyID     string          `json:"key_id"`
	}
	if DecodeStrict(data, &wire, ResponseLimit) != nil || DecodeStrict(wire.Envelope, &r.Envelope, ResponseLimit) != nil || !comparisonWirePresent(wire.Envelope) {
		return ErrInvalid
	}
	r.Signature = wire.Signature
	r.KeyID = wire.KeyID
	r.rawEnvelope = append([]byte{}, wire.Envelope...)
	return nil
}
func (r SignedReceipt) MarshalJSON() ([]byte, error) {
	b := r.rawEnvelope
	if len(b) == 0 {
		var e error
		b, e = json.Marshal(r.Envelope)
		if e != nil {
			return nil, e
		}
	}
	return json.Marshal(struct {
		Envelope  json.RawMessage `json:"envelope"`
		Signature string          `json:"sig"`
		KeyID     string          `json:"key_id"`
	}{b, r.Signature, r.KeyID})
}

type ReceiptVerifier struct {
	keys   map[string]ed25519.PublicKey
	labels map[string]map[int]string
}

// Compiled receipt key pin for supported testnets.
// /v1/keys is advisory and cannot add or replace it.
const testnetReceiptKeyHex = "438d9301c477c27fecc3f56da0a7d5a6b3894cef69815bae63b5349dc2cddf0b"

var releaseReceiptKeys = map[string]ed25519.PublicKey{
	"rk-2026-09-a": receiptPublicKey(testnetReceiptKeyHex),
}

// The existing service key signs network-bound receipts in either deployment profile.
// A server key document cannot replace this compiled public trust anchor.
var mainnetReceiptKeys = map[string]ed25519.PublicKey{
	"rk-2026-09-a": receiptPublicKey(testnetReceiptKeyHex),
}

func receiptPublicKey(value string) ed25519.PublicKey {
	key, err := hex.DecodeString(value)
	if err != nil || len(key) != ed25519.PublicKeySize {
		panic("invalid release receipt public key")
	}
	return ed25519.PublicKey(key)
}

func productionReceiptKeysFor(network string) map[string]ed25519.PublicKey {
	switch normalizedNetwork(network) {
	case Network, "eip155:5042002", "eip155:46630":
		return releaseReceiptKeys
	case "eip155:8453", "eip155:5042", "eip155:4663":
		return mainnetReceiptKeys
	default:
		return nil
	}
}

// releaseLabels is each launch product's label catalogue in the server template's order (server/checks); an
// answer's option is its 1-based index. tests/contract/client-fixtures.json pins this table to the server.
var releaseLabels = currentReceiptLabels()

func currentReceiptLabels() map[string]map[int]string {
	labels := map[string]map[int]string{
		"portfolio_check":      {1: "portfolio_attention_required", 2: "portfolio_no_additional_attention", 3: "cannot_verify"},
		"lending_check":        {1: "cannot_verify", 2: "no_debt", 3: "within_lltv_at_block", 4: "liquidatable_at_block"},
		"cross_chain_compare":  {1: "comparison_available", 2: "comparison_invalid", 3: "cannot_verify"},
		"counterparty_check":   {1: "listed_warning", 2: "deployment_mismatch", 3: "cannot_verify", 4: "configured_deployment_match", 5: "no_list_match"},
		"trade_check":          {1: "do_not_proceed", 2: "cannot_verify", 3: "proceed"},
		"stock_token_check":    {1: "not_official_or_out_of_line", 2: "cannot_verify", 3: "official_stock_token"},
		"token_check":          {1: "warning_no_contract", 2: "warning_impersonation", 3: "warning_owner_control", 4: "no_checked_warning_signs", 5: "issuer_controlled_expected", 6: "warning_sell_blocked", 7: "warning_high_sell_tax", 8: "warning_sanctioned", 9: "warning_no_token_interface"},
		"agent_registry_check": {1: "cannot_verify", 2: "not_registered", 3: "owner_mismatch", 4: "wallet_mismatch", 5: "feedback_revocations_present", 6: "registered_with_feedback", 7: "registered_without_feedback"},
		"hidden_prompt_check":  {1: "no_injection_found", 2: "injection_found", 3: "injection_quoted"},
		"scam_check":           {1: "benign", 2: "suspicious", 3: "malicious"},
	}
	// The generated catalogue arrives in the server integration commit. Keep
	// the current product set exact until that additive entry is available.
	if !productOffered(comparisonProduct) {
		delete(labels, comparisonProduct)
	}
	return deploymentReceiptLabels(labels)
}

// storedReceiptLabels are catalogues that only receipts from earlier builds carry: the retired products, and
// Legacy Trade Check receipts used a different template (options 1 and 2 meant proceed and do_not_proceed).
// engine.go re-verifies every stored receipt when listing receipts, so dropping these would fail that listing
// for any ledger that holds one. A freshly fetched receipt for a product still sold must use releaseLabels.
var storedReceiptLabels = map[string]map[int]string{
	"cross_chain_compare":     {1: "comparison_available", 2: "comparison_invalid", 3: "cannot_verify"},
	"owner_instruction_check": {1: "contradicts_instruction", 2: "exceeds_instruction", 3: "breaks_owner_rule", 4: "matches_instruction_and_rules", 5: "cannot_verify"},
	"trade_check":             {1: "proceed", 2: "do_not_proceed"},
	"transaction_check":       {1: "matches_intent", 2: "exceeds_intent", 3: "contradicts_intent"},
	"full_stock_check":        {1: "proceed", 2: "do_not_proceed"},
}

func (v ReceiptVerifier) knownAnswer(product string, a Answer, stored bool) bool {
	if !validDataFreshness(a) {
		return false
	}
	if !validTradeAnswer(product, a) || !valid040Answer(product, a) || !validPortfolioAnswer(product, a) || !validLendingAnswer(product, a) || a.Option < 1 || !validComparisonAnswer(product, a) || !validStockFacts(product, a) || (a.RegistryObservations != nil && product != "agent_registry_check") {
		return false
	}
	if label := v.labels[product][a.Option]; label != "" && label == a.LabelID {
		return true
	}
	if stored || retiredProducts[product] {
		label := storedReceiptLabels[product][a.Option]
		return label != "" && label == a.LabelID
	}
	return false
}

// Historical receipts may omit provenance. An absent or unknown date never
// becomes current merely because the receipt signature verifies.
func validDataFreshness(a Answer) bool {
	if a.DataFreshness == "" {
		return a.DataAsOf == nil
	}
	if !oneOf(a.DataFreshness, "current", "not_verified") || a.DataAsOf == nil || len(a.DataAsOf) > 128 {
		return false
	}
	if a.DataFreshness == "current" && len(a.DataAsOf) == 0 {
		return false
	}
	for name, date := range a.DataAsOf {
		if !datasetIdentifier.MatchString(name) {
			return false
		}
		if date == nil {
			if a.DataFreshness == "current" {
				return false
			}
			continue
		}
		if _, err := time.Parse(time.RFC3339Nano, *date); err != nil {
			return false
		}
	}
	return true
}

var datasetIdentifier = regexp.MustCompile(`^[a-z][a-z0-9_]{0,79}$`)

var configFingerprint = regexp.MustCompile(`^[0-9a-f]{64}$`)

// Released receipts name the lane; every other receipt carries the admission's paid lane code
// (server/payments/lanes.py lane_for_payer). Recovery and trial lanes (R, T) are not sold at launch.
var receiptLanes = []string{"new", "proven", "N0", "N1", "P"}

var identifier = regexp.MustCompile(`^[a-z][a-z0-9-]{0,63}$`)

// disclaimer mirrors DISCLAIMED_PRODUCTS in server/payments/receipts.py, which quote previews also use.
func disclaimer(product string) string {
	if oneOf(product, "lending_check", comparisonProduct, "trade_check", "token_check", "stock_token_check", "full_stock_check") {
		return "not_financial_advice_ai_generated"
	}
	return ""
}

// Verify checks a receipt the server just sent.
func (v ReceiptVerifier) Verify(r SignedReceipt, e Entry, now time.Time) error {
	return v.verify(r, e, now, false)
}

// VerifyStored re-checks a receipt this client already accepted and saved, which may predate a label change.
func (v ReceiptVerifier) VerifyStored(r SignedReceipt, e Entry, now time.Time) error {
	return v.verify(r, e, now, true)
}

// sameRevision reports whether fresh is e's stored receipt signed again. The server stamps every GET with a new
// issued_at, so an unchanged check returns at the same seq under a new signature, which Verify refuses.
func (v ReceiptVerifier) sameRevision(fresh SignedReceipt, e Entry, now time.Time) bool {
	if e.Receipt == nil || fresh.Envelope.Seq != e.Receipt.Envelope.Seq {
		return false
	}
	stored := e.Receipt.Envelope
	// Keep the stored receipt for durable recovery validation. Relax only the
	// same-sequence signature comparison; Verify still authenticates fresh and
	// the complete envelopes below must match except their issuance timestamp.
	prior := *e.Receipt
	prior.Signature = fresh.Signature
	e.Receipt = &prior
	if v.Verify(fresh, e, now) != nil {
		return false
	}
	again := fresh.Envelope
	again.IssuedAt, stored.IssuedAt = "", ""
	again.Answer = answerBeforeExpiry(stored.Answer, again.Answer)
	x, errA := canonicalValue(again)
	y, errB := canonicalValue(stored)
	return errA == nil && errB == nil && string(x) == string(y)
}

// answerBeforeExpiry normalizes only an allowed freshness downgrade for
// immutable-answer comparisons. Dates and substantive answer fields still match
// byte-for-byte, and the received receipt is authenticated before comparison.
func answerBeforeExpiry(previous, next *Answer) *Answer {
	if previous == nil || next == nil {
		return next
	}
	answer := *next
	if answer.DataFreshness == "not_verified" && previous.DataFreshness == "current" {
		answer.DataFreshness = "current"
	}
	if answer.DataFreshness == "not_verified" && previous.DataFreshness == "" && len(answer.DataAsOf) == 0 {
		answer.DataFreshness = ""
		answer.DataAsOf = nil
	}
	return &answer
}

func (v ReceiptVerifier) verify(r SignedReceipt, e Entry, now time.Time, stored bool) error {
	s := r.Envelope
	b := s.Billing
	keys := v.keys
	if keys == nil {
		keys = receiptKeysFor(e.Network)
	}
	key, ok := keys[r.KeyID]
	if !ok || len(key) != ed25519.PublicKeySize || r.KeyID != s.KeyID {
		return ErrInvalid
	}
	sig, err := base64.RawURLEncoding.DecodeString(r.Signature)
	if err != nil || len(sig) != ed25519.SignatureSize {
		return ErrInvalid
	}
	canonical, err := canonicalValue(s)
	if len(r.rawEnvelope) > 0 {
		var original ReceiptEnvelope
		if DecodeStrict(r.rawEnvelope, &original, ResponseLimit) != nil || !comparisonWirePresent(r.rawEnvelope) {
			return ErrInvalid
		}
		normalized, e := canonicalValue(original)
		if e != nil || string(normalized) != string(canonical) {
			return ErrInvalid
		}
		canonical, err = Canonical(r.rawEnvelope)
	}
	if err != nil {
		return ErrInvalid
	}
	// Each version signs under its own prefix, so no envelope verifies as another version.
	prefix := "SECONDED-RECEIPT/v1\x00"
	switch s.Version {
	case 2:
		prefix = "SECONDED-RECEIPT/v2\x00"
	case 3:
		prefix = "SECONDED-RECEIPT/v3\x00"
	}
	msg := append([]byte(prefix), canonical...)
	if !ed25519.Verify(key, msg, sig) {
		return ErrInvalid
	}
	if e.Recovery != nil {
		if oneOf(s.State, "service_failed", "refused") && e.State == "SPENT" {
			return ErrInvalid
		}
		if e.Recovery.Validate(e) != nil || s.PurchaseAssociation == nil || *s.PurchaseAssociation != e.Recovery.Association || !hexID.MatchString(s.CheckID) {
			return ErrInvalid
		}
		if e.Recovery.ServerCheckID != "" && (s.CheckID != e.Recovery.ServerCheckID || s.Commitment != e.Commitment) {
			return ErrInvalid
		}
		// Public terms deliberately contain no server ID or salt. Only the signed
		// full purchase association permits adopting these fields on first receipt.
		e.CheckID, e.Commitment = s.CheckID, s.Commitment
	}
	// The challenge commitment was recomputed from the input and salt before payment.
	// New receipts carry a null digest; legacy receipts must still match both bindings.
	if s.Digest == "" {
		var fields map[string]json.RawMessage
		if json.Unmarshal(r.rawEnvelope, &fields) != nil || string(fields["request_digest"]) != "null" {
			return ErrInvalid
		}
	}
	if s.Version < 1 || s.Version > 3 || (s.State == "released" && s.Version == 1) || s.Kind != "seconded-receipt" || s.CheckID != e.CheckID || s.Seq < 1 || s.Product != e.Product || s.Tier != e.Tier || (s.Digest != "" && s.Digest != e.InputDigest) || !configFingerprint.MatchString(s.Commitment) || s.Commitment != e.Commitment || s.SchemaVersion != "1" || s.PredicateVersion != "1" || !oneOf(s.TemplateVersion, "1", "tpl-v1") || !identifier.MatchString(s.ModelPairID) || !oneOf(s.Lane, receiptLanes...) || s.Disclaimer != disclaimer(s.Product) {
		return ErrInvalid
	}
	if (s.Version == 3) != (s.Verification != nil) || (s.Verification != nil && !validVerification(*s.Verification)) {
		return ErrInvalid
	}
	if s.CheckedBy == nil || s.ModelConfigFingerprints == nil || !configFingerprint.MatchString(s.ModelConfigFingerprint) {
		return ErrInvalid
	}
	seenLabs := map[string]bool{}
	for _, lab := range s.CheckedBy {
		if !oneOf(lab, "OpenAI", "Anthropic") || seenLabs[lab] {
			return ErrInvalid
		}
		seenLabs[lab] = true
	}
	for part, hash := range s.ModelConfigFingerprints {
		if !configFingerprint.MatchString(hash) || (s.Product == "full_stock_check" && !oneOf(part, "order", "rules")) || (s.Product != "full_stock_check" && part != "check") {
			return ErrInvalid
		}
	}
	if len(s.CheckedBy) > 0 {
		part := "check"
		if s.Product == "full_stock_check" {
			part = "order"
		}
		if !configFingerprint.MatchString(s.ModelConfigFingerprints[part]) {
			return ErrInvalid
		}
	}
	issued, err := time.Parse(time.RFC3339, s.IssuedAt)
	if err != nil || issued.After(now.Add(30*time.Second)) || issued.Unix() < e.SignedAt-300 {
		return ErrInvalid
	}
	if s.OutcomeAt != nil {
		o, err := time.Parse(time.RFC3339, *s.OutcomeAt)
		if err != nil || o.After(issued) || o.Unix() < e.SignedAt-300 {
			return ErrInvalid
		}
	}
	if s.UnfreezeAt != nil {
		if _, err := time.Parse(time.RFC3339, *s.UnfreezeAt); err != nil {
			return ErrInvalid
		}
	}
	if !oneOf(b.Mode, "paid", "free-trial") || b.Network != normalizedNetwork(e.Network) || strings.ToLower(b.Asset) != normalizedNetwork(e.Network)+"/erc20:"+pins(e.Network).Asset || strings.ToLower(b.Payer) != e.Payer || strings.ToLower(b.PayTo) != PayTo || b.Amount != strconv.FormatInt(e.Amount, 10) || !oneOf(b.Charged, "yes", "no", "pending", "refund_owed", "refunded") || !oneOf(b.Settlement, "none", "unknown", "included", "final", "nonpayment_certified") {
		return ErrInvalid
	}
	if b.Tx != nil && !hexWord.MatchString(*b.Tx) {
		return ErrInvalid
	}
	// billing.refund is the refund state on a refund receipt and null on every other (server/api/openapi.py Billing).
	refund := oneOf(s.State, "refund_owed", "refunded")
	if refund != (b.Refund != nil) || (refund && *b.Refund != s.State) {
		return ErrInvalid
	}
	if s.State != "trial_delivered" && b.Mode != "paid" {
		return ErrInvalid
	}
	if s.Answer != nil && !v.knownAnswer(s.Product, *s.Answer, stored) {
		return ErrInvalid
	}
	if s.Answer != nil && s.Answer.DataFreshness == "current" {
		for _, date := range s.Answer.DataAsOf {
			observed, err := time.Parse(time.RFC3339Nano, *date)
			if err != nil || observed.After(issued) {
				return ErrInvalid
			}
		}
	}
	if s.Answer != nil && !validRegistryBinding(s.Answer.RegistryObservations, s.Verification, e) {
		return ErrInvalid
	}
	if !validTradeBinding(s) || !valid040Binding(s) || !validPortfolioBinding(s, e, now) || !validLendingBinding(s, e) || !validComparisonBinding(s, e, now) {
		return ErrInvalid
	}
	if s.State == "refused" && s.Reason == "" {
		return ErrInvalid
	}
	if s.State != "no_agreement" {
		if s.Status != "" || s.Message != "" || (s.Reason != "" &&
			!((s.State == "service_failed" && oneOf(s.Reason, serviceFailureReasons...)) ||
				(s.State == "refused" && s.PurchaseAssociation != nil && findingName.MatchString(s.Reason)))) {
			return ErrInvalid
		}
	}
	if s.State == "service_failed" && s.Reason == "" && len(r.rawEnvelope) > 0 {
		var fields map[string]json.RawMessage
		if json.Unmarshal(r.rawEnvelope, &fields) != nil {
			return ErrInvalid
		}
		if _, present := fields["reason"]; present {
			return ErrInvalid
		}
	}
	switch s.State {
	case "released":
		if s.Outcome != "agreed" || s.Answer == nil || s.OutcomeAt == nil || b.Tx != nil || b.Refund != nil ||
			!((b.Charged == "pending" && b.Settlement == "unknown") || (b.Charged == "no" && b.Settlement == "nonpayment_certified")) {
			return ErrInvalid
		}
	case "included", "final":
		if s.Outcome != "agreed" || s.Answer == nil || s.OutcomeAt == nil || b.Charged != "yes" || b.Tx == nil || b.Settlement != s.State {
			return ErrInvalid
		}
	case "no_agreement":
		legacy := s.Message == NotVerifiedMessage && oneOf(s.Reason, legacyNotVerifiedReasons...)
		current := s.Message == CurrentNotVerifiedMessage && oneOf(s.Reason, notVerifiedReasons...)
		if s.Status != "not_verified" || !(legacy || current) {
			return ErrInvalid
		}
		if s.Answer != nil || s.Outcome != "no_agreement" || s.OutcomeAt == nil || b.Charged != "pending" || b.Tx != nil || b.Settlement != "none" {
			return ErrInvalid
		}
	case "trial_delivered":
		if b.Mode != "free-trial" || e.Amount != 0 || b.Amount != "0" || b.Charged != "no" || b.Settlement != "none" || b.Tx != nil || b.Refund != nil || s.Outcome != "agreed" || s.Answer == nil || s.OutcomeAt == nil {
			return ErrInvalid
		}
	case "refund_owed", "refunded":
		// A refund receipt asserts no settlement of its own. billing.tx is the original payment when the server
		// recorded one: a paid check whose answer was lost has it, a payment found for an uncharged check may not.
		if s.Answer != nil || s.Outcome != "" || b.Charged != s.State || b.Refund == nil || b.Settlement != "none" {
			return ErrInvalid
		}
	case "content_refused", "service_failed":
		// The signed outcome repeats the state (server/payments/receipts.py OUTCOME); nothing was charged.
		if s.Answer != nil || s.Outcome != s.State || b.Charged != "no" || b.Tx != nil || b.Settlement != "none" {
			return ErrInvalid
		}
	case "closed_no_charge", "frozen_unsettled", "refused":
		// A cancelled or expired authorization is certified nonpayment; the other two never reached settlement.
		settlement := "none"
		if s.State == "closed_no_charge" {
			settlement = "nonpayment_certified"
		}
		if s.Answer != nil || s.Outcome != "" || b.Charged != "no" || b.Tx != nil || b.Settlement != settlement {
			return ErrInvalid
		}
	case "running", "settling", "delayed", "unavailable":
		if s.Answer != nil || !oneOf(s.Outcome, "", "pending", "agreed", "none") || b.Charged != "pending" || !oneOf(b.Settlement, "none", "unknown") {
			return ErrInvalid
		}
	default:
		return ErrInvalid
	}
	if e.Receipt != nil {
		old := e.Receipt.Envelope
		if s.Seq < old.Seq || (old.OutcomeAt != nil && (s.OutcomeAt == nil || *old.OutcomeAt != *s.OutcomeAt)) {
			return ErrInvalid
		}
		if s.Seq == old.Seq && r.Signature != e.Receipt.Signature {
			return ErrInvalid
		}
		if (e.Product == portfolioProduct || e.Product == comparisonProduct || e.Product == "lending_check") && old.Answer != nil {
			before, errA := canonicalValue(old.Answer)
			after, errB := canonicalValue(answerBeforeExpiry(old.Answer, s.Answer))
			if errA != nil || errB != nil || string(before) != string(after) {
				return ErrInvalid
			}
		}
		// Findings are fixed once published: a later receipt may not drop or change them.
		if old.Verification != nil && !sameVerification(old.Verification, s.Verification) {
			return ErrInvalid
		}
	}
	return nil
}
func validateReplyState(r CheckReply) error {
	state := r.Receipt.Envelope.State
	if state == "service_failed" {
		// Service reasons are envelope-only; tolerate a body copy only when it matches.
		if r.Status != "" || r.Message != "" || (r.Reason != "" && r.Reason != r.Receipt.Envelope.Reason) {
			return ErrInvalid
		}
		if r.Reason != "" && !oneOf(r.Reason, serviceFailureReasons...) {
			return ErrInvalid
		}
	}
	if r.Status != "" || r.Message != "" || r.Reason != "" {
		s := r.Receipt.Envelope
		if !oneOf(state, "no_agreement", "service_failed", "refused") || r.Status != s.Status || r.Message != s.Message || r.Reason != s.Reason {
			return ErrInvalid
		}
	}
	// Verified service_failed/charged=no is terminal even on an unusual HTTP status.
	if state == "service_failed" {
		return nil
	}
	if r.httpStatus == 202 {
		if !oneOf(state, "running", "settling", "delayed", "unavailable") {
			return ErrInvalid
		}
	} else if oneOf(state, "running", "settling", "delayed", "unavailable") {
		return ErrInvalid
	}
	if oneOf(state, "no_agreement", "included", "final", "released", "trial_delivered") && r.httpStatus != 200 {
		return ErrInvalid
	}
	return nil
}

// Lending observations bind one Morpho position. Exact quantities are decimal strings.
type LendingInput struct {
	Network  string `json:"network"`
	Account  string `json:"account"`
	MarketID string `json:"market_id"`
}
type LendingSource struct {
	ChainID       int64    `json:"chain_id"`
	BlockNumber   int64    `json:"block_number"`
	BlockHash     string   `json:"block_hash"`
	Readers       []string `json:"readers"`
	Timestamp     int64    `json:"timestamp"`
	ObservedAt    int64    `json:"observed_at"`
	CompletedAt   int64    `json:"completed_at"`
	BaseFeePerGas *string  `json:"baseFeePerGas"`
}
type LendingAsset struct {
	Address  string `json:"address"`
	Decimals int    `json:"decimals"`
}
type LendingOracle struct {
	Address       string  `json:"address"`
	Price         *string `json:"price"`
	Scale         string  `json:"scale"`
	Coverage      string  `json:"coverage"`
	FeedUpdatedAt *int64  `json:"feed_updated_at"`
}
type LendingIRM struct {
	Address  string  `json:"address"`
	Rate     *string `json:"rate_per_second_wad"`
	Coverage string  `json:"coverage"`
}
type LendingStored struct {
	Supply     string `json:"supply_assets_down"`
	Borrow     string `json:"borrow_assets_up"`
	LastUpdate string `json:"last_update"`
}
type LendingAccrued struct {
	Supply       string `json:"supply_assets_down"`
	Borrow       string `json:"borrow_assets_up"`
	SupplyShares string `json:"supply_shares"`
	FeeShares    string `json:"fee_shares_minted"`
	Timestamp    string `json:"timestamp"`
}
type LendingPosition struct {
	State        string          `json:"position_state"`
	SupplyShares string          `json:"supply_shares"`
	BorrowShares string          `json:"borrow_shares"`
	Collateral   string          `json:"collateral_atomic"`
	Stored       LendingStored   `json:"stored"`
	Accrued      *LendingAccrued `json:"accrued"`
	Relationship string          `json:"relationship"`
	Maximum      *string         `json:"max_borrow_assets_down"`
}
type LendingCoverage struct {
	MarketCount  int    `json:"market_count"`
	AccountCount int    `json:"account_count"`
	ReaderPolicy string `json:"reader_policy"`
	MaxAge       int64  `json:"max_block_age_seconds"`
	Age          int64  `json:"block_age_seconds"`
}
type LendingFacts struct {
	Version          string            `json:"version"`
	Network          string            `json:"network"`
	Account          string            `json:"account"`
	MarketID         string            `json:"market_id"`
	Core             string            `json:"core"`
	Source           LendingSource     `json:"source"`
	DeploymentSource string            `json:"deployment_source"`
	Loan             LendingAsset      `json:"loan_asset"`
	Collateral       LendingAsset      `json:"collateral_asset"`
	Units            string            `json:"units"`
	LLTV             string            `json:"lltv"`
	LLTVScale        string            `json:"lltv_scale"`
	Oracle           LendingOracle     `json:"oracle"`
	IRM              LendingIRM        `json:"irm"`
	MarketStored     map[string]string `json:"market_stored"`
	Position         LendingPosition   `json:"position"`
	Unknowns         []string          `json:"unknowns"`
	Coverage         LendingCoverage   `json:"coverage"`
	Reads            int               `json:"reads_used"`
}
type LendingSheet struct {
	Version  string                       `json:"version"`
	Input    LendingInput                 `json:"input"`
	Facts    LendingFacts                 `json:"facts"`
	Source   LendingSource                `json:"source"`
	Coverage map[string]map[string]string `json:"coverage"`
}
type LendingObservations struct {
	Sheet          LendingSheet `json:"sheet"`
	EvidenceSHA256 string       `json:"evidence_sha256"`
}

func validLendingInput(i LendingInput) bool {
	return oneOf(i.Network, "eip155:8453", "eip155:5042") && addressPattern.MatchString(i.Account) && hexWord.MatchString(strings.ToLower(i.MarketID))
}
func lendingDigest(v any) string {
	b, err := canonicalValue(v)
	if err != nil {
		return ""
	}
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func validLendingAnswer(product string, a Answer) bool {
	if product != "lending_check" {
		return a.LendingObservations == nil
	}
	o := a.LendingObservations
	if o == nil || o.Sheet.Version != "lending-check/v1" || !validLendingInput(o.Sheet.Input) || o.EvidenceSHA256 != lendingDigest(o.Sheet) {
		return false
	}
	f, i := o.Sheet.Facts, o.Sheet.Input
	if f.Version != "lending-facts/v1" || f.Network != i.Network || f.Account != strings.ToLower(i.Account) || f.MarketID != strings.ToLower(i.MarketID) || a.LabelID != f.Position.Relationship || !oneOf(a.LabelID, "no_debt", "within_lltv_at_block", "liquidatable_at_block") {
		return false
	}
	if lendingDigest(o.Sheet.Source) != lendingDigest(f.Source) || f.Source.ChainID != map[string]int64{"eip155:8453": 8453, "eip155:5042": 5042}[i.Network] || !hexWord.MatchString(f.Source.BlockHash) || f.Source.BlockNumber < 0 || f.Source.BlockNumber > maxSafeInteger || len(f.Source.Readers) != 2 || f.Source.Readers[0] == "" || f.Source.Readers[1] == "" || f.Source.Readers[0] == f.Source.Readers[1] {
		return false
	}
	if f.Coverage.MarketCount != 1 || f.Coverage.AccountCount != 1 || f.Coverage.ReaderPolicy != "existing_two_reader_session" || f.Reads < 1 || f.Reads > 30 || f.Coverage.MaxAge < 1 || f.Coverage.MaxAge > 120 || f.Source.Timestamp <= 0 || f.Source.ObservedAt < f.Source.Timestamp || f.Source.CompletedAt < f.Source.ObservedAt || f.Source.CompletedAt-f.Source.Timestamp > f.Coverage.MaxAge || f.Coverage.Age != f.Source.CompletedAt-f.Source.Timestamp {
		return false
	}
	if f.Units != "atomic_assets_and_unscaled_shares" || f.LLTVScale != "1000000000000000000" || f.Oracle.Scale != "1000000000000000000000000000000000000" || f.Oracle.FeedUpdatedAt != nil || !validRegistryID(f.LLTV) {
		return false
	}
	expectedCoverage := map[string]map[string]string{"lending": {"status": "checked"}, "lending.oracle_accuracy": {"status": "not_checked", "reason": "oracle_quality_not_assessed"}, "lending.oracle_age": {"status": "not_checked", "reason": "oracle_feed_freshness_unavailable"}, "lending.future_safety": {"status": "not_checked", "reason": "future_liquidation_not_guaranteed"}}
	if lendingDigest(o.Sheet.Coverage) != lendingDigest(expectedCoverage) {
		return false
	}
	for _, limit := range []string{"oracle_feed_freshness_unavailable", "oracle_quality_not_assessed", "future_liquidation_not_guaranteed", "other_markets_not_checked"} {
		if !oneOf(limit, f.Unknowns...) {
			return false
		}
	}
	for _, asset := range []LendingAsset{f.Loan, f.Collateral} {
		if !registryAddress.MatchString(asset.Address) || asset.Decimals < 0 || asset.Decimals > 255 {
			return false
		}
	}
	expectedCore := map[string]string{"eip155:8453": "0xbbbbbbbbbb9cc5e90e3b3af64bdaf62c37eeffcb", "eip155:5042": "0x34cd04070dd72b14e241112f6d83812df5af7fcd"}
	if f.Core != expectedCore[f.Network] || f.DeploymentSource != "https://docs.morpho.org/developers/contracts/addresses/" {
		return false
	}
	p := f.Position
	for _, n := range []string{p.SupplyShares, p.BorrowShares, p.Collateral, p.Stored.Supply, p.Stored.Borrow, p.Stored.LastUpdate} {
		if !validRegistryID(n) {
			return false
		}
	}
	if len(f.MarketStored) != 6 {
		return false
	}
	for _, key := range []string{"supply_assets", "supply_shares", "borrow_assets", "borrow_shares", "last_update", "fee_wad"} {
		if !validRegistryID(f.MarketStored[key]) {
			return false
		}
	}
	if (p.State == "zero_position") != (p.SupplyShares == "0" && p.BorrowShares == "0" && p.Collateral == "0") || !oneOf(p.State, "zero_position", "nonzero_position") {
		return false
	}
	for _, n := range []*string{f.Source.BaseFeePerGas, f.Oracle.Price, f.IRM.Rate, p.Maximum} {
		if n != nil && !validRegistryID(*n) {
			return false
		}
	}
	if p.Accrued != nil {
		for _, n := range []string{p.Accrued.Supply, p.Accrued.Borrow, p.Accrued.SupplyShares, p.Accrued.FeeShares, p.Accrued.Timestamp} {
			if !validRegistryID(n) {
				return false
			}
		}
		if p.Accrued.Timestamp != strconv.FormatInt(f.Source.Timestamp, 10) {
			return false
		}
	}
	lltv, _ := new(big.Int).SetString(f.LLTV, 10)
	wad := new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil)
	if lltv.Cmp(wad) >= 0 {
		return false
	}
	if p.BorrowShares == "0" {
		return a.LabelID == "no_debt" && p.Maximum == nil && (p.Accrued == nil || p.Accrued.Borrow == "0")
	}
	if p.Accrued == nil || p.Maximum == nil || f.Oracle.Price == nil || *f.Oracle.Price == "0" || f.Oracle.Coverage != "pinned_interface" {
		return false
	}
	collateral, _ := new(big.Int).SetString(p.Collateral, 10)
	price, _ := new(big.Int).SetString(*f.Oracle.Price, 10)
	max := new(big.Int).Mul(collateral, price)
	if max.BitLen() > 256 {
		return false
	}
	max.Div(max, new(big.Int).Mul(wad, wad))
	max.Mul(max, lltv)
	if max.BitLen() > 256 {
		return false
	}
	max.Div(max, wad)
	debt, _ := new(big.Int).SetString(p.Accrued.Borrow, 10)
	if max.String() != *p.Maximum || debt.Sign() == 0 {
		return false
	}
	return (a.LabelID == "within_lltv_at_block" && debt.Cmp(max) <= 0) || (a.LabelID == "liquidatable_at_block" && debt.Cmp(max) > 0)
}

func validLendingBinding(s ReceiptEnvelope, e Entry) bool {
	if s.Answer == nil {
		return true
	}
	if s.Product != "lending_check" {
		return s.Answer.LendingObservations == nil
	}
	if !validLendingAnswer(s.Product, *s.Answer) || s.Verification == nil || s.Verification.Block == nil || s.OutcomeAt == nil {
		return false
	}
	o := s.Answer.LendingObservations
	source := o.Sheet.Source
	block := s.Verification.Block
	if s.Verification.EvidenceSHA256 != o.EvidenceSHA256 || block.ChainID != source.ChainID || block.Number != source.BlockNumber || block.Hash != source.BlockHash || e.InputDigest != "sha256:"+lendingDigest(o.Sheet.Input) {
		return false
	}
	at, err := time.Parse(time.RFC3339, *s.OutcomeAt)
	return err == nil && at.Unix() >= source.CompletedAt && !at.After(time.Unix(source.Timestamp+o.Sheet.Facts.Coverage.MaxAge, 0))
}

func (o *LendingObservations) UnmarshalJSON(data []byte) error {
	type wire LendingObservations
	var v wire
	if DecodeStrict(data, &v, ResponseLimit) != nil {
		return ErrInvalid
	}
	original, e1 := Canonical(data)
	normalized, e2 := canonicalValue(v)
	if e1 != nil || e2 != nil || string(original) != string(normalized) {
		return ErrInvalid
	}
	*o = LendingObservations(v)
	return nil
}
