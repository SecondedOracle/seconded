package client

import (
	"encoding/json"
	"math/big"
	"reflect"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const degenProduct = "degen_check"
const degenDisclaimer = "Not financial advice. This check reports facts and model opinions as of the block shown; it is not a guarantee."
const degenConflictNote = "SECONDED has its own token, $SECONDED, which uses the same check."

// DegenAnswer is the signed page contract. Charged records a paid deliverable;
// Result.Charged remains the independently reconciled payment state.
type DegenAnswer struct {
	VerdictExpiresAt       *string               `json:"verdict_expires_at,omitempty"`
	JudgedSnapshot         *DegenSnapshot        `json:"judged_snapshot,omitempty"`
	OutcomeCause           string                `json:"outcome_cause,omitempty"`
	Stale                  bool                  `json:"stale,omitempty"`
	VerdictAt              string                `json:"verdict_at,omitempty" seconded:"nullable"`
	SupersededByHardChange bool                  `json:"superseded_by_hard_change,omitempty"`
	LastGoodVerdict        *DegenLastGoodVerdict `json:"last_good_verdict,omitempty"`
	AIVerdictUnavailable   string                `json:"ai_verdict_unavailable,omitempty"`
	Market                 *DegenMarket          `json:"market,omitempty"`
	Outcome                string                `json:"outcome,omitempty"`
	Schema                 string                `json:"schema"`
	Verdict                string                `json:"verdict"`
	Charged                bool                  `json:"charged"`
	Updating               bool                  `json:"updating,omitempty"`
	Token                  DegenToken            `json:"token"`
	AsOf                   DegenAsOf             `json:"as_of"`
	Fingerprint            string                `json:"fingerprint"`
	Versions               DegenVersions         `json:"versions"`
	HardRules              []string              `json:"hard_rules_fired"`
	Scorecard              []DegenRow            `json:"scorecard"`
	Holder                 DegenHolder           `json:"holder"`
	Burned                 *DegenBurned          `json:"burned"`
	PossibleBundle         *DegenBundle          `json:"possible_bundle"`
	Models                 []DegenModel          `json:"models"`
	History                []DegenHistory        `json:"history"`
	Coverage               DegenCoverage         `json:"coverage"`
	Disclaimer             string                `json:"disclaimer"`
	ConflictNote           string                `json:"conflict_note"`
}

type DegenSnapshot struct {
	EvidenceSHA256 string               `json:"evidence_sha256"`
	Facts          map[string]degenFact `json:"facts"`
}

// DegenLastGoodVerdict identifies history even when live hard changes supersede it.
type DegenLastGoodVerdict struct {
	Verdict   string `json:"verdict"`
	VerdictAt string `json:"verdict_at"`
}

// DegenMarket is refreshed on acquisition and signed, but never invalidates a verdict pin.
type DegenMarket struct {
	PriceBasis        string      `json:"price_basis,omitempty"`
	CapBasis          string      `json:"cap_basis,omitempty"`
	SupplyAsOf        *string     `json:"supply_as_of,omitempty"`
	PriceNative       DegenMetric `json:"price_native"`
	PriceUSD          DegenMetric `json:"price_usd"`
	MarketCapNative   DegenMetric `json:"market_cap_native"`
	MarketCapUSD      DegenMetric `json:"market_cap_usd"`
	VolumeNative      DegenMetric `json:"volume_24h_native"`
	VolumeUSD         DegenMetric `json:"volume_24h_usd"`
	CirculatingSupply DegenMetric `json:"circulating_supply"`
	SupplyBasis       string      `json:"supply_basis"`
	VolumeBasis       string      `json:"volume_basis"`
	WindowStart       *int64      `json:"volume_window_start"`
	WindowEnd         *int64      `json:"volume_window_end"`
}
type DegenMetric struct {
	Value  *string `json:"value"`
	Unit   string  `json:"unit"`
	Source string  `json:"source"`
	AsOf   *string `json:"as_of"`
	Reason *string `json:"reason"`
}

// The four basis sentences are signed fixed text. Each list holds the current
// wording first and the wording used before 2026-10-14 second, so answers signed
// before the rewrite still verify; any other text is rejected.
var degenSupplyBasis = []string{
	"Tokens in existence minus tokens sent to burn addresses. Tokens held by the launch curve, the trading pool and the locker are included.",
	"totalSupply minus zero/dead balances; hard burns already excluded; includes curve/pool/locked tokens",
}
var degenVolumeBasis = []string{
	"Total ETH traded, before fees, in buys and sells on the launch curve and its trading pool during the 24 hours ending at the checked block. The dollar figure uses the current ETH price.",
	"Gross ETH trade legs over the trailing 24h ending at the pinned block; USD uses the current ETH/USD reference",
}
var degenPriceBasis = []string{
	"The current price of one token for the smallest possible trade, before fees. It is not what a real sale would return.",
	"Marginal price per token before fees; not executable proceeds",
}
var degenCapBasis = []string{
	"The current price multiplied by the tokens in existence that were not sent to burn addresses, as of the supply time shown.",
	"Current marginal price multiplied by non-burned supply at supply_as_of",
}

func (m DegenMarket) valid(at time.Time) bool {
	if !oneOf(m.SupplyBasis, degenSupplyBasis...) || !oneOf(m.VolumeBasis, degenVolumeBasis...) {
		return false
	}
	metrics := []DegenMetric{m.PriceNative, m.PriceUSD, m.MarketCapNative, m.MarketCapUSD, m.VolumeNative, m.VolumeUSD, m.CirculatingSupply}
	units := []string{"ETH", "USD", "ETH", "USD", "ETH", "USD", "tokens"}
	suffixes := []string{"", "+eth_usd_oracle", "+circulating_supply", "+circulating_supply+eth_usd_oracle"}
	for i, metric := range metrics {
		if metric.Unit != units[i] || (metric.Value == nil) == (metric.Reason == nil) {
			return false
		}
		if i < 4 {
			if !oneOf(metric.Source, "curve_reserves"+suffixes[i], "v4_slot0"+suffixes[i], "unavailable"+suffixes[i]) {
				return false
			}
		} else if metric.Source != []string{"curve_and_pool_logs", "curve_and_pool_logs+eth_usd_oracle", "mint_burn_and_supply"}[i-4] {
			return false
		}
		if metric.Value != nil && (len(*metric.Value) > 192 || !degenDecimal.MatchString(*metric.Value) || metric.AsOf == nil) {
			return false
		}
		if metric.Reason != nil && !oneOf(*metric.Reason, "price_unavailable", "supply_or_price_unavailable", "supply_unavailable", "volume_unavailable", "usd_reference_unavailable", "dependency_unavailable") {
			return false
		}
		if metric.AsOf != nil {
			stamp, err := time.Parse(time.RFC3339, *metric.AsOf)
			if err != nil || stamp.After(at) {
				return false
			}
		}
	}
	if (m.WindowStart == nil) != (m.WindowEnd == nil) {
		return false
	}
	if m.WindowEnd != nil && (*m.WindowStart < 0 || *m.WindowEnd < *m.WindowStart || *m.WindowEnd-*m.WindowStart > 86400 || *m.WindowEnd > maxSafeInteger) {
		return false
	}
	if (m.VolumeNative.Value != nil || m.VolumeUSD.Value != nil) && m.WindowEnd == nil {
		return false
	}
	return true
}

type DegenToken struct {
	Network string  `json:"network"`
	Address string  `json:"address"`
	Name    string  `json:"name"`
	Symbol  string  `json:"symbol"`
	Phase   *string `json:"phase"`
}
type DegenAsOf struct {
	Block       *int64  `json:"block"`
	Time        string  `json:"time"`
	PinnedUntil *string `json:"pinned_until"`
}
type DegenVersions struct {
	Rules   string `json:"rules"`
	PromptA string `json:"prompt_a"`
	PromptB string `json:"prompt_b"`
}
type DegenRow struct {
	Block           *int64  `json:"block,omitempty"`
	ObservedAt      *string `json:"observed_at,omitempty"`
	MeasurementKind string  `json:"measurement_kind,omitempty"`
	Band            string  `json:"band,omitempty"`
	Coverage        string  `json:"coverage,omitempty"`
	LowerBound      bool    `json:"lower_bound,omitempty"`
	ID              string  `json:"id"`
	Label           string  `json:"label"`
	Status          string  `json:"status"`
	ValueText       string  `json:"value_text"`
	NAReason        *string `json:"na_reason"`
}
type DegenHolder struct {
	Coverage   string             `json:"coverage,omitempty"`
	LowerBound bool               `json:"lower_bound,omitempty"`
	Band       string             `json:"band"`
	Percent    *string            `json:"top10_excl_protocol_percent"`
	Labels     []DegenHolderLabel `json:"labels"`
}
type DegenHolderLabel struct {
	Label   string `json:"label"`
	Percent string `json:"percent"`
}
type DegenBurned struct {
	Percent string `json:"percent_of_minted"`
	Raw     string `json:"burned_raw"`
}
type DegenBundle struct {
	MeasurementKind string `json:"measurement_kind,omitempty"`
	Wallets         int64  `json:"wallets"`
	Percent         string `json:"combined_percent"`
	Note            string `json:"note"`
}
type DegenModel struct {
	VerdictAt          string        `json:"verdict_at,omitempty"`
	Lane               string        `json:"lane"`
	Verdict            string        `json:"verdict"`
	Reasons            []DegenReason `json:"reasons"`
	AddressedHardRules bool          `json:"addressed_hard_rules"`
}
type DegenReason struct {
	EvidenceScope string   `json:"evidence_scope,omitempty"`
	Code          string   `json:"code"`
	Text          string   `json:"text"`
	EvidenceIDs   []string `json:"evidence_ids"`
	Polarity      string   `json:"polarity"`
}
type DegenHistory struct {
	ComparisonAvailable bool          `json:"comparison_available,omitempty"`
	Time                string        `json:"time"`
	Verdict             string        `json:"verdict"`
	Changed             []DegenChange `json:"changed"`
}
type DegenChange struct {
	ID     string `json:"id"`
	Label  string `json:"label"`
	Before string `json:"before"`
	After  string `json:"after"`
}
type DegenCoverage struct {
	Description string             `json:"description,omitempty"`
	Checked     int                `json:"checked"`
	Total       int                `json:"total"`
	Unavailable []DegenUnavailable `json:"unavailable"`
}
type DegenUnavailable struct {
	ID     string `json:"id"`
	Label  string `json:"label"`
	Reason string `json:"reason"`
}

type degenObservations struct {
	JudgeDiagnostics map[string]DegenJudgeDiagnostic `json:"judge_diagnostics,omitempty"`
	Decision         string                          `json:"decision"`
	Charged          bool                            `json:"charged"`
	Facts            []degenFact                     `json:"facts"`
	HardRules        []string                        `json:"hard_rules_fired"`
	Holder           json.RawMessage                 `json:"holder"`
	Burned           json.RawMessage                 `json:"burned"`
	Models           []DegenModel                    `json:"models"`
	Fingerprint      string                          `json:"fingerprint"`
	Versions         DegenVersions                   `json:"versions"`
	Coverage         struct {
		Checked     int      `json:"checked"`
		Total       int      `json:"total"`
		Unavailable []string `json:"unavailable"`
	} `json:"coverage"`
	MissLog []string `json:"miss_log"`
	Token   struct {
		Network string  `json:"network"`
		Address string  `json:"address"`
		Name    *string `json:"name"`
		Symbol  *string `json:"symbol"`
		Phase   *string `json:"phase"`
	} `json:"token"`
	Answer         *DegenAnswer `json:"degen_answer"`
	EvidenceSHA256 string       `json:"evidence_sha256"`
}
type DegenJudgeDiagnostic struct {
	Cause           string   `json:"cause"`
	ValidationCodes []string `json:"validation_codes"`
}

type degenFact struct {
	ObservedAt *float64        `json:"observed_at,omitempty"`
	ID         string          `json:"id"`
	Label      string          `json:"label"`
	Value      json.RawMessage `json:"value"`
	Status     string          `json:"status"`
	Source     json.RawMessage `json:"source"`
	Block      *int64          `json:"block"`
	Reason     *string         `json:"reason"`
	Core       bool            `json:"core"`
}

var degenCoreIDs = []string{"E-SELL-SIM", "E-LP-CUSTODY", "E-TEMPLATE", "E-SELL-FEE", "E-CREATOR-SHARE", "E-HOLDERS", "E-FIRST-BUYERS", "E-LIQ-DEPTH"}
var degenFactIDs = append(append([]string{}, degenCoreIDs...), "E-SUPPLY", "E-BURNED", "E-CREATOR", "E-PHASE", "E-CURVE-RESERVES", "E-POOL", "E-HOLDER-BAND", "E-CREATOR-HISTORY", "E-AGE")
var degenReasonFacts = map[string]string{
	"LIQUIDITY_LOCKED": "E-LP-CUSTODY", "LIQUIDITY_NOT_LOCKED": "E-LP-CUSTODY",
	"CREATOR_HOLDS_NOTHING": "E-CREATOR-SHARE", "CREATOR_HOLDS_LARGE_SHARE": "E-CREATOR-SHARE",
	"HOLDER_CONCENTRATION_LOW": "E-HOLDER-BAND", "HOLDER_CONCENTRATION_MODERATE": "E-HOLDER-BAND", "HOLDER_CONCENTRATION_HIGH": "E-HOLDER-BAND",
	"SAME_BLOCK_BUYERS": "E-FIRST-BUYERS", "POSSIBLE_BUNDLE": "E-FIRST-BUYERS", "SERIAL_LAUNCHER": "E-CREATOR-HISTORY", "FEES_HIGH": "E-SELL-FEE", "THIN_LIQUIDITY": "E-LIQ-DEPTH", "SELL_SIMULATION_FAILED": "E-SELL-SIM", "TEMPLATE_MISMATCH": "E-TEMPLATE", "BURN_SIGNIFICANT": "E-BURNED",
}
var degenDecimal = regexp.MustCompile(`^(0|[1-9][0-9]*)(\.[0-9]+)?$`)

func degenPercent(s string) bool {
	if len(s) > 192 || !degenDecimal.MatchString(s) {
		return false
	}
	n, ok := new(big.Rat).SetString(s)
	return ok && n.Sign() >= 0 && n.Cmp(big.NewRat(100, 1)) <= 0
}
func degenText(s string, limit int) bool {
	if !utf8.ValidString(s) || utf8.RuneCountInString(s) > limit || strings.ContainsAny(s, "<>") {
		return false
	}
	for _, r := range s {
		if unicode.IsControl(r) || unicode.In(r, unicode.Cf) {
			return false
		}
	}
	return true
}

// Bound every display string without allowing maps, arrays or raw HTML to stand
// in for text. Exact field presence and primitive types are checked separately.
func degenTexts(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.Pointer:
		return v.IsNil() || degenTexts(v.Elem())
	case reflect.String:
		return degenText(v.String(), 1024)
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			if !degenTexts(v.Field(i)) {
				return false
			}
		}
	case reflect.Slice:
		for i := 0; i < v.Len(); i++ {
			if !degenTexts(v.Index(i)) {
				return false
			}
		}
	}
	return true
}
func (a *DegenAnswer) UnmarshalJSON(data []byte) error {
	type wire DegenAnswer
	var value wire
	if DecodeStrict(data, &value, 48<<10) != nil {
		return ErrInvalid
	}
	answer := DegenAnswer(value)
	if !answer.valid() {
		return ErrInvalid
	}
	*a = answer
	return nil
}
func (a DegenAnswer) valid() bool {
	if !a.validMeaning() || a.Schema != "degen-answer/v1" || !oneOf(a.Verdict, "no_red_flags", "red_flags", "models_disagree", "insufficient_evidence") || !configFingerprint.MatchString(a.Fingerprint) || !degenTexts(reflect.ValueOf(a)) {
		return false
	}
	if a.Charged && !oneOf(a.Verdict, "no_red_flags", "red_flags", "models_disagree") {
		return false
	}
	if a.Outcome != "" && (a.Outcome != "models_disagree" || !a.Charged) {
		return false
	}
	if a.Token.Network != "eip155:4663" || !addressPattern.MatchString(a.Token.Address) || a.Token.Address == "0x"+strings.Repeat("0", 40) || !degenText(a.Token.Name, 32) || !degenText(a.Token.Symbol, 32) || (a.Token.Phase != nil && !oneOf(*a.Token.Phase, "curve", "graduated")) {
		return false
	}
	at, err := time.Parse(time.RFC3339, a.AsOf.Time)
	if err != nil || (a.AsOf.Block == nil) != (a.AsOf.PinnedUntil == nil) || (a.Market != nil && !a.Market.valid(at)) {
		return false
	}
	if a.AIVerdictUnavailable != "" && !oneOf(a.AIVerdictUnavailable, "daily_limit", "budget_unavailable", "pin_unavailable", "read_budget_unavailable", "cancelled") {
		return false
	}
	if a.Stale {
		stamp, err := time.Parse(time.RFC3339, a.VerdictAt)
		if err != nil || stamp.After(at) || a.Charged || a.LastGoodVerdict == nil ||
			a.LastGoodVerdict.VerdictAt != a.VerdictAt ||
			!oneOf(a.LastGoodVerdict.Verdict, "no_red_flags", "red_flags", "models_disagree") {
			return false
		}
		if a.SupersededByHardChange {
			expected := "insufficient_evidence"
			if len(a.HardRules) > 0 {
				expected = "red_flags"
			}
			if a.Verdict != expected || len(a.Models) != 0 {
				return false
			}
		} else if a.Verdict != a.LastGoodVerdict.Verdict {
			return false
		}
	} else if a.SupersededByHardChange || a.LastGoodVerdict != nil {
		return false
	}
	if a.AsOf.Block != nil {
		until, err := time.Parse(time.RFC3339, *a.AsOf.PinnedUntil)
		if *a.AsOf.Block < 0 || *a.AsOf.Block > maxSafeInteger || err != nil || !until.Equal(at.Add(30*time.Minute)) {
			return false
		}
	}
	// Keep stored v1/v2 pins replayable alongside v3 answers; mixed pairs remain invalid.
	switch a.Versions {
	case DegenVersions{"degen-rules/v1", "degen-a-v1", "degen-b-v1"},
		DegenVersions{"degen-rules/v1", "degen-a-v2", "degen-b-v2"},
		DegenVersions{"degen-rules/v1", "degen-a-v3", "degen-b-v3"}:
	default:
		return false
	}
	if a.Disclaimer != degenDisclaimer || a.ConflictNote != degenConflictNote || len(a.HardRules) > 5 || len(a.Scorecard) != 8 || len(a.History) > 20 {
		return false
	}
	if (a.Verdict == "insufficient_evidence" && len(a.Models) != 0) || (a.Verdict != "insufficient_evidence" && len(a.Models) != 2 && !a.SupersededByHardChange) {
		return false
	}
	rules := map[string]bool{}
	for _, rule := range a.HardRules {
		if !oneOf(rule, "H1", "H2", "H3", "H4", "H5") || rules[rule] {
			return false
		}
		rules[rule] = true
	}
	if len(rules) > 0 && !oneOf(a.Verdict, "red_flags", "insufficient_evidence") {
		return false
	}
	missing := map[string]DegenRow{}
	seen := map[string]bool{}
	for _, row := range a.Scorecard {
		if !oneOf(row.ID, degenCoreIDs...) || seen[row.ID] || !oneOf(row.Status, "ok", "warn", "flag", "na", "observed") || (row.Status == "na") != (row.NAReason != nil) || row.Label == "" || row.ValueText == "" {
			return false
		}
		seen[row.ID] = true
		if row.NAReason != nil {
			if *row.NAReason == "" {
				return false
			}
			missing[row.ID] = row
		}
	}
	if (len(missing) > 3 && a.Verdict != "insufficient_evidence" && !a.Stale) || a.Coverage.Total != 8 || a.Coverage.Checked != 8-len(missing) || len(a.Coverage.Unavailable) != len(missing) {
		return false
	}
	_, sellMissing := missing["E-SELL-SIM"]
	_, feeMissing := missing["E-SELL-FEE"]
	if sellMissing && feeMissing && len(rules) == 0 && a.Verdict != "insufficient_evidence" && !a.Stale {
		return false
	}
	for _, row := range a.Coverage.Unavailable {
		fact, ok := missing[row.ID]
		if !ok || row.Label != fact.Label || row.Reason != *fact.NAReason {
			return false
		}
		delete(missing, row.ID)
	}
	if !oneOf(a.Holder.Band, "low", "moderate", "high", "na") || (a.Holder.Band == "na") != (a.Holder.Percent == nil) || len(a.Holder.Labels) > 5 {
		return false
	}
	if a.Holder.Percent != nil {
		if !degenPercent(*a.Holder.Percent) {
			return false
		}
		n, _ := new(big.Rat).SetString(*a.Holder.Percent)
		band := "moderate"
		if n.Cmp(big.NewRat(25, 1)) < 0 {
			band = "low"
		}
		if n.Cmp(big.NewRat(50, 1)) > 0 {
			band = "high"
		}
		if band != a.Holder.Band {
			return false
		}
	}
	labels := map[string]bool{}
	for _, label := range a.Holder.Labels {
		if !oneOf(label.Label, "pool", "locker", "curve", "factory", "burn") || labels[label.Label] || !degenPercent(label.Percent) {
			return false
		}
		labels[label.Label] = true
	}
	if a.Burned != nil && (!degenPercent(a.Burned.Percent) || len(a.Burned.Raw) > 192 || !degenDecimal.MatchString(a.Burned.Raw) || strings.Contains(a.Burned.Raw, ".")) {
		return false
	}
	if b := a.PossibleBundle; b != nil && (b.Wallets < 2 || b.Wallets > maxSafeInteger || !degenPercent(b.Percent) || !((b.MeasurementKind == "" && b.Note == "Same-block first trades. Common control is not determined.") || (b.MeasurementKind == "same_block_recipients" && b.Note == "Current share held by launch-block recipient addresses. Shared funding or control is not established."))) {
		return false
	}
	lanes := map[string]bool{}
	for _, model := range a.Models {
		if !oneOf(model.Lane, "A", "B") || lanes[model.Lane] || !oneOf(model.Verdict, "no_red_flags", "red_flags") || len(model.Reasons) < 1 || len(model.Reasons) > 6 {
			return false
		}
		lanes[model.Lane] = true
		for _, reason := range model.Reasons {
			fact, ok := degenReasonFacts[reason.Code]
			polarity := "flag"
			if oneOf(reason.Code, "LIQUIDITY_LOCKED", "CREATOR_HOLDS_NOTHING", "HOLDER_CONCENTRATION_LOW", "BURN_SIGNIFICANT") {
				polarity = "clean"
			}
			if !ok || reason.Polarity != polarity || reason.Text == "" || len(reason.EvidenceIDs) < 1 || len(reason.EvidenceIDs) > 4 {
				return false
			}
			for _, id := range reason.EvidenceIDs {
				if id != fact {
					return false
				}
			}
		}
	}
	if len(a.Models) == 2 {
		agrees := a.Models[0].Verdict == a.Models[1].Verdict
		if (a.Outcome == "models_disagree") != (a.Charged && !agrees) {
			return false
		}
		if len(rules) == 0 {
			expected := "models_disagree"
			if agrees {
				expected = a.Models[0].Verdict
			}
			if a.Verdict != expected {
				return false
			}
		}
	}
	previous := at
	for _, history := range a.History {
		stamp, err := time.Parse(time.RFC3339, history.Time)
		if err != nil || stamp.After(previous) || !oneOf(history.Verdict, "no_red_flags", "red_flags", "models_disagree", "insufficient_evidence") || len(history.Changed) > len(degenFactIDs) {
			return false
		}
		previous = stamp
		for _, change := range history.Changed {
			if !oneOf(change.ID, degenFactIDs...) {
				return false
			}
		}
	}
	return true
}

func decodeDegenObservations(data []byte) (*degenObservations, error) {
	var o degenObservations
	if DecodeStrict(data, &o, 48<<10) != nil || o.Answer == nil || !o.Charged || !o.Answer.Charged || o.Decision != o.Answer.Verdict || o.Fingerprint != o.Answer.Fingerprint || o.Versions != o.Answer.Versions || !configFingerprint.MatchString(o.EvidenceSHA256) || len(o.Facts) > 32 || len(o.MissLog) > 10 || !reflect.DeepEqual(o.HardRules, o.Answer.HardRules) || !reflect.DeepEqual(o.Models, o.Answer.Models) || o.Token.Network != o.Answer.Token.Network || o.Token.Address != o.Answer.Token.Address {
		return nil, ErrInvalid
	}
	seen := map[string]bool{}
	for _, fact := range o.Facts {
		if fact.ID == "" || seen[fact.ID] || !oneOf(fact.Status, "observed", "not_available") || (fact.Block != nil && (*fact.Block < 0 || *fact.Block > maxSafeInteger)) {
			return nil, ErrInvalid
		}
		seen[fact.ID] = true
	}
	if o.Coverage.Total != 8 || o.Coverage.Checked < 0 || o.Coverage.Checked > 8 || len(o.Coverage.Unavailable) != 8-o.Coverage.Checked {
		return nil, ErrInvalid
	}
	return &o, nil
}

// DegenAnswer returns the typed page object without changing legacy observation maps.
func (a *Answer) DegenAnswer() *DegenAnswer {
	if a == nil || a.CheckObservations == nil {
		return nil
	}
	page, _ := (*a.CheckObservations)["degen_answer"].(*DegenAnswer)
	return page
}
func validDegenAnswer(product string, a Answer) bool {
	if product != degenProduct {
		return a.DegenAnswer() == nil
	}
	if a.CheckObservations == nil {
		return false
	}
	data, err := json.Marshal(a.CheckObservations)
	if err != nil {
		return false
	}
	o, err := decodeDegenObservations(data)
	if err != nil {
		return false
	}
	if o.Answer.Outcome == "models_disagree" {
		return a.Option == 0 && a.LabelID == "models_disagree"
	}
	return o.Decision == a.LabelID && map[int]string{1: "no_red_flags", 2: "red_flags"}[a.Option] == a.LabelID
}
func validDegenBinding(s ReceiptEnvelope, e Entry) bool {
	if s.Answer == nil || s.Product != degenProduct {
		return true
	}
	data, err := json.Marshal(s.Answer.CheckObservations)
	if err != nil {
		return false
	}
	o, err := decodeDegenObservations(data)
	if err != nil || !oneOf(s.Outcome, "agreed", "models_disagree") || !oneOf(s.State, "released", "included", "final") || s.Verification == nil || s.Verification.EvidenceSHA256 != o.EvidenceSHA256 {
		return false
	}
	if (s.Outcome == "models_disagree") != (o.Answer.Outcome == "models_disagree") {
		return false
	}
	if subject := e.CheckedSubject; subject != nil {
		return subject.InputSHA256 == e.InputDigest && subject.Identifiers["/network"] == o.Answer.Token.Network &&
			subject.Identifiers["/address"] == strings.ToLower(o.Answer.Token.Address)
	}
	input := map[string]string{"network": o.Answer.Token.Network, "address": o.Answer.Token.Address}
	return e.InputDigest == "sha256:"+lendingDigest(input)
}

func validDegenProduct(s ReceiptEnvelope) bool {
	if s.Answer == nil || s.Answer.CheckObservations == nil {
		return true
	}
	_, present := (*s.Answer.CheckObservations)["degen_answer"]
	return !present || s.Product == degenProduct
}
