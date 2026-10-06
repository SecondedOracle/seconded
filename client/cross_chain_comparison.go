package client

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"regexp"
	"strings"
	"time"
)

const comparisonProduct = "cross_chain_compare"
const comparisonSchema = "seconded-cross-chain-comparison/v1"

type ComparisonRoute struct {
	Network  string `json:"network"`
	Venue    string `json:"venue"`
	TokenIn  string `json:"token_in"`
	TokenOut string `json:"token_out"`
}

type ComparisonInput struct {
	Mode     string            `json:"mode"`
	AmountIn string            `json:"amount_in"`
	AssetIn  string            `json:"asset_in"`
	AssetOut string            `json:"asset_out"`
	Routes   []ComparisonRoute `json:"routes"`
}

func comparisonRoutes() []ComparisonRoute {
	return []ComparisonRoute{
		{"eip155:8453", "uniswap_v3", "0x833589fcd6edb6e08f4c7c32d4f71b54bda02913", "0x60a3e35cc302bfa44cb288bc5a4f316fdb1adb42"},
		{"eip155:5042", "uniswap_v4", "0x3600000000000000000000000000000000000000", "0xbef5f6d51cb62b58e6a8f77868681825c6fe21c1"},
	}
}

var comparisonAmount = regexp.MustCompile(`^(0|[1-9][0-9]{0,4})(\.[0-9]{1,6})?$`)
var comparisonUint = regexp.MustCompile(`^[1-9][0-9]{0,77}$`)
var comparisonReader = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.:-]{0,127}$`)
var comparisonWord = regexp.MustCompile(`^0x[0-9a-f]{64}$`)

func comparisonAtomic(s string) (string, bool) {
	if !comparisonAmount.MatchString(s) {
		return "", false
	}
	whole, fraction, _ := strings.Cut(s, ".")
	n, ok := new(big.Int).SetString(whole+fraction+strings.Repeat("0", 6-len(fraction)), 10)
	if !ok || n.Sign() <= 0 || n.Cmp(big.NewInt(10000000000)) > 0 {
		return "", false
	}
	return n.String(), true
}

func comparisonHuman(atomic string) string {
	if len(atomic) <= 6 {
		atomic = strings.Repeat("0", 7-len(atomic)) + atomic
	}
	whole, fraction := atomic[:len(atomic)-6], strings.TrimRight(atomic[len(atomic)-6:], "0")
	if fraction == "" {
		return whole
	}
	return whole + "." + fraction
}

// Only the comparison digest is normalized. The purchase still binds the exact
// submitted body, including its original decimal spelling and route order.
func normalizeComparison(raw []byte) (ComparisonInput, error) {
	var input ComparisonInput
	if DecodeStrict(raw, &input, 8192) != nil {
		return input, ErrInvalid
	}
	atomic, ok := comparisonAtomic(input.AmountIn)
	routes := comparisonRoutes()
	if !ok || input.Mode != "exact_in" || input.AssetIn != "circle:usdc" || input.AssetOut != "circle:eurc" || len(input.Routes) != 2 {
		return input, ErrInvalid
	}
	if input.Routes[0] == routes[1] && input.Routes[1] == routes[0] {
		input.Routes[0], input.Routes[1] = input.Routes[1], input.Routes[0]
	}
	if input.Routes[0] != routes[0] || input.Routes[1] != routes[1] {
		return input, ErrInvalid
	}
	input.AmountIn = comparisonHuman(atomic)
	return input, nil
}

func comparisonHash(value any) string {
	raw, err := canonicalValue(value)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func comparisonRequestHash(req Request) (string, error) {
	if req.Product != comparisonProduct {
		return "", nil
	}
	input, err := normalizeComparison(req.Input)
	if err != nil {
		return "", err
	}
	return comparisonHash(input), nil
}

type Comparison struct {
	Schema          string              `json:"schema"`
	RequestSHA256   string              `json:"request_sha256"`
	EvidenceSHA256  string              `json:"evidence_sha256"`
	ProfileSHA256   string              `json:"profile_sha256"`
	ObservedAt      int64               `json:"observed_at"`
	Mode            string              `json:"mode"`
	AssetIn         string              `json:"asset_in"`
	AssetOut        string              `json:"asset_out"`
	AmountIn        string              `json:"amount_in"`
	Basis           string              `json:"basis"`
	Comparable      bool                `json:"comparable"`
	CostBasis       string              `json:"cost_basis"`
	TotalCostStatus string              `json:"total_cost_status"`
	Ranking         string              `json:"ranking"`
	Freshness       ComparisonFreshness `json:"freshness"`
	Rows            []ComparisonRow     `json:"rows"`
}

type ComparisonFreshness struct {
	MaxAgeSeconds         int64 `json:"max_age_seconds"`
	MaxBlockSkewSeconds   int64 `json:"max_block_skew_seconds"`
	MaxCaptureSkewSeconds int64 `json:"max_capture_skew_seconds"`
	ValidUntil            int64 `json:"valid_until"`
}

type ComparisonSource struct {
	ChainID        int64  `json:"chain_id"`
	BlockNumber    int64  `json:"block_number"`
	BlockHash      string `json:"block_hash"`
	BlockTimestamp int64  `json:"block_timestamp"`
	ObservedAt     int64  `json:"observed_at"`
	ReaderPairID   string `json:"reader_pair_id"`
	SourceSHA256   string `json:"source_sha256"`
}

type ComparisonImpact struct {
	Status   string  `json:"status"`
	ValuePPM *int64  `json:"value_ppm,omitempty"`
	Basis    *string `json:"basis,omitempty"`
	Reason   *string `json:"reason,omitempty"`
}

type ComparisonUnchecked struct {
	Status string `json:"status"`
	Reason string `json:"reason"`
}

type ComparisonCosts struct {
	ExecutionGas         string `json:"execution_gas"`
	L1Data               string `json:"l1_data"`
	Approval             string `json:"approval"`
	Bridge               string `json:"bridge"`
	DestinationExecution string `json:"destination_execution"`
	Total                string `json:"total"`
}

type ComparisonRow struct {
	Network                 string              `json:"network"`
	Venue                   string              `json:"venue"`
	TokenIn                 string              `json:"token_in"`
	TokenOut                string              `json:"token_out"`
	AssetIn                 string              `json:"asset_in"`
	AssetOut                string              `json:"asset_out"`
	DecimalsIn              int                 `json:"decimals_in"`
	DecimalsOut             int                 `json:"decimals_out"`
	AmountInAtomic          string              `json:"amount_in_atomic"`
	AmountOutAtomic         string              `json:"amount_out_atomic"`
	AmountOut               string              `json:"amount_out"`
	QuoteKind               string              `json:"quote_kind"`
	PoolID                  string              `json:"pool_id"`
	RouteProfileSHA256      string              `json:"route_profile_sha256"`
	Source                  ComparisonSource    `json:"source"`
	QuoteSHA256             string              `json:"quote_sha256"`
	PoolFeePPM              int64               `json:"pool_fee_ppm"`
	AdditionalPoolFeeStatus string              `json:"additional_pool_fee_status"`
	PriceImpact             ComparisonImpact    `json:"price_impact"`
	Slippage                ComparisonUnchecked `json:"slippage"`
	LiquidityDepth          ComparisonUnchecked `json:"liquidity_depth"`
	ReferencePrice          ComparisonUnchecked `json:"reference_price"`
	Costs                   ComparisonCosts     `json:"costs"`
}

func comparisonQuoteHash(row ComparisonRow) string {
	raw, err := json.Marshal(row)
	if err != nil {
		return ""
	}
	// RawMessage preserves integer tokens; no map[string]float64 round trip.
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil {
		return ""
	}
	delete(fields, "quote_sha256")
	return comparisonHash(fields)
}

func (c *Comparison) UnmarshalJSON(data []byte) error {
	type wire Comparison
	var decoded wire
	if DecodeStrict(data, &decoded, ResponseLimit) != nil {
		return ErrInvalid
	}
	candidate := Comparison(decoded)
	if !validComparison(&candidate) {
		return ErrInvalid
	}
	original, err := Canonical(data)
	normalized, err2 := canonicalValue(decoded)
	// This also distinguishes absent optional union fields from explicit nulls.
	if err != nil || err2 != nil || string(original) != string(normalized) {
		return ErrInvalid
	}
	*c = candidate
	return nil
}

func validComparison(c *Comparison) bool {
	if c == nil {
		return false
	}
	atomic, ok := comparisonAtomic(c.AmountIn)
	if !ok || c.AmountIn != comparisonHuman(atomic) || c.Schema != comparisonSchema || c.Mode != "exact_in" || c.AssetIn != "circle:usdc" || c.AssetOut != "circle:eurc" || !c.Comparable || c.Basis != "same_issuer_assets_equal_input_pool_quotes" || c.CostBasis != "pool_output_after_pool_fees" || c.TotalCostStatus != "unknown" || c.Ranking != "not_provided" || len(c.Rows) != 2 {
		return false
	}
	for _, digest := range []string{c.RequestSHA256, c.EvidenceSHA256, c.ProfileSHA256} {
		if !configFingerprint.MatchString(digest) {
			return false
		}
	}
	f := c.Freshness
	if f.MaxAgeSeconds != 30 || f.MaxBlockSkewSeconds != 10 || f.MaxCaptureSkewSeconds != 10 || c.ObservedAt <= 0 || c.ObservedAt > maxSafeInteger-30 {
		return false
	}
	until := int64(maxSafeInteger)
	routes := comparisonRoutes()
	for i, r := range c.Rows {
		if (ComparisonRoute{r.Network, r.Venue, r.TokenIn, r.TokenOut}) != routes[i] || r.AssetIn != c.AssetIn || r.AssetOut != c.AssetOut || r.DecimalsIn != 6 || r.DecimalsOut != 6 || r.AmountInAtomic != atomic || r.QuoteKind != "pool_simulation_exact_input" {
			return false
		}
		amount, ok := new(big.Int).SetString(r.AmountOutAtomic, 10)
		if !comparisonUint.MatchString(r.AmountOutAtomic) || !ok || amount.Sign() <= 0 || amount.BitLen() > 256 || r.AmountOut != comparisonHuman(r.AmountOutAtomic) {
			return false
		}
		if i == 0 && !registryNonzero(r.PoolID) || i == 1 && (!comparisonWord.MatchString(r.PoolID) || r.PoolID == "0x"+strings.Repeat("0", 64)) {
			return false
		}
		s := r.Source
		chain := int64(8453)
		if i == 1 {
			chain = 5042
		}
		if s.ChainID != chain || s.BlockNumber < 0 || s.BlockNumber > maxSafeInteger || !comparisonWord.MatchString(s.BlockHash) || !comparisonReader.MatchString(s.ReaderPairID) {
			return false
		}
		for _, stamp := range []int64{s.BlockTimestamp, s.ObservedAt} {
			if stamp <= 0 || stamp > c.ObservedAt || c.ObservedAt-stamp > 30 {
				return false
			}
			until = min(until, stamp+30)
		}
		if s.BlockTimestamp > s.ObservedAt {
			return false
		}
		for _, digest := range []string{r.RouteProfileSHA256, s.SourceSHA256, r.QuoteSHA256} {
			if !configFingerprint.MatchString(digest) {
				return false
			}
		}
		if r.PoolFeePPM < 0 || r.PoolFeePPM >= 1000000 || r.AdditionalPoolFeeStatus != "included_not_itemized" {
			return false
		}
		p := r.PriceImpact
		if p.Status == "checked" {
			if p.ValuePPM == nil || *p.ValuePPM < 0 || *p.ValuePPM > 1000000 || !valueIs(p.Basis, "mid_after_pool_fees") || p.Reason != nil {
				return false
			}
		} else if p.Status != "not_checked" || !valueIs(p.Reason, "pool_state_not_measured") || p.ValuePPM != nil || p.Basis != nil {
			return false
		}
		if r.Slippage != (ComparisonUnchecked{"not_checked", "no_transaction_limit_supplied"}) || r.LiquidityDepth != (ComparisonUnchecked{"not_checked", "no_depth_survey"}) || r.ReferencePrice != (ComparisonUnchecked{"not_checked", "not_requested"}) || r.Costs != (ComparisonCosts{"not_checked", "not_checked", "not_checked", "excluded", "not_checked", "unknown"}) {
			return false
		}
		if comparisonQuoteHash(r) != r.QuoteSHA256 {
			return false
		}
	}
	a, b := c.Rows[0].Source, c.Rows[1].Source
	if a.BlockTimestamp-b.BlockTimestamp > 10 || b.BlockTimestamp-a.BlockTimestamp > 10 || a.ObservedAt-b.ObservedAt > 10 || b.ObservedAt-a.ObservedAt > 10 || f.ValidUntil != until {
		return false
	}
	input := ComparisonInput{c.Mode, c.AmountIn, c.AssetIn, c.AssetOut, routes}
	return c.RequestSHA256 == comparisonHash(input)
}

func validComparisonEntry(e Entry) bool {
	if e.Product == comparisonProduct {
		return configFingerprint.MatchString(e.ComparisonRequestSHA256)
	}
	return e.ComparisonRequestSHA256 == ""
}

func validComparisonAnswer(product string, a Answer) bool {
	if product != comparisonProduct {
		return a.Comparison == nil
	}
	switch a.LabelID {
	case "comparison_available":
		return a.Option == 1 && validComparison(a.Comparison)
	case "comparison_invalid":
		return a.Option == 2 && a.Comparison == nil
	default:
		return false // Abstention uses the existing free no_agreement envelope.
	}
}

func validComparisonBinding(s ReceiptEnvelope, e Entry, now time.Time) bool {
	if !validComparisonEntry(e) {
		return false
	}
	if s.Answer == nil || s.Answer.Comparison == nil {
		return true
	}
	c := s.Answer.Comparison
	if s.Product != comparisonProduct || s.Verification == nil || s.Verification.Block != nil || c.EvidenceSHA256 != s.Verification.EvidenceSHA256 || c.RequestSHA256 != e.ComparisonRequestSHA256 || s.OutcomeAt == nil {
		return false
	}
	// The signed outcome fixes validity through agreement/pre-seal, not delivery
	// or issued_at. Initial delivery and recovery preserve that historical answer.
	// Compare exact timestamps: Unix-second truncation would admit a fractional
	// outcome after expiry or in the future.
	outcome, err := time.Parse(time.RFC3339, *s.OutcomeAt)
	if err != nil || outcome.Before(time.Unix(c.ObservedAt, 0)) || outcome.After(time.Unix(c.Freshness.ValidUntil, 0)) || outcome.After(now) {
		return false
	}
	checked := map[string]bool{}
	for _, coverage := range s.Verification.Coverage {
		checked[coverage.Fact] = coverage.Status == "checked"
	}
	for _, fact := range []string{"base_quote", "arc_quote", "asset_identity", "equal_trade_size", "freshness"} {
		if !checked[fact] {
			return false
		}
	}
	return true
}

func comparisonFresh(a *Answer, now int64) bool {
	return a == nil || a.Comparison == nil || (now >= a.Comparison.ObservedAt && now <= a.Comparison.Freshness.ValidUntil)
}

func comparisonText(a *Answer, now int64) ([]string, bool) {
	if a == nil || a.Comparison == nil {
		return nil, false
	}
	c := a.Comparison
	expired := now > c.Freshness.ValidUntil
	text := []string{fmt.Sprintf("Equal input: %s Circle USDC per chain; output: Circle EURC. Pool quotes include pool fees; do not subtract fees again.", c.AmountIn)}
	for i, row := range c.Rows {
		name := "Base"
		if i == 1 {
			name = "Arc"
		}
		text = append(text, fmt.Sprintf("%s: %s EURC (gross pool quote only, not total cost); pool fee %d ppm. Block %d at %s; observed at %s.", name, row.AmountOut, row.PoolFeePPM, row.Source.BlockNumber, time.Unix(row.Source.BlockTimestamp, 0).UTC().Format(time.RFC3339), time.Unix(row.Source.ObservedAt, 0).UTC().Format(time.RFC3339)))
	}
	state := "Quotes valid until "
	if expired {
		state = "Historical quotes expired at "
	}
	text = append(text, state+time.Unix(c.Freshness.ValidUntil, 0).UTC().Format(time.RFC3339)+".", "Gas, approval, destination execution and total costs are unknown; bridge costs are excluded. Liquidity depth and future slippage are not checked. No ranking or savings is provided.", "Inspect the quotes; check an exact transaction separately before signing.")
	return text, expired
}

// Reject explicit null on the new optional field without changing historical
// Answer decoding in the stock/registry modules.
func comparisonWirePresent(envelope []byte) bool {
	var fields struct {
		Answer map[string]json.RawMessage `json:"answer"`
	}
	if json.Unmarshal(envelope, &fields) != nil {
		return false
	}
	for _, key := range []string{"comparison", "portfolio_report", "lending_observations"} {
		if raw, present := fields.Answer[key]; present && strings.TrimSpace(string(raw)) == "null" {
			return false
		}
	}
	return true
}

func comparisonClientVersion(version string) bool {
	parts := semanticVersion.FindStringSubmatch(version)
	return parts != nil && parts[4] == "" && minimumVersionOK(version, "0.3.1")
}
