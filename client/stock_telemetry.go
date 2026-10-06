package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/big"
	"regexp"
	"strings"
	"time"
)

type StockPriceComparison struct {
	Status string  `json:"status"`
	Reason *string `json:"reason"`
}

func validPriceComparison(p *StockPriceComparison) bool {
	if p == nil {
		return true
	}
	if p.Status == "checked" {
		return p.Reason == nil
	}
	if p.Reason == nil {
		return false
	}
	if p.Status == "unknown" {
		return oneOf(*p.Reason, "no_fresh_reference", "no_reviewed_price_feed_yet", "references_disagree", "price_evidence_unavailable")
	}
	return p.Status == "not_checked" && oneOf(*p.Reason, "no_intended_price", "no_fresh_reference", "official_list_unknown", "not_an_official_token", "identity_mismatch", "halted_paused_or_multiplier_change")
}

func (p *StockPriceComparison) UnmarshalJSON(data []byte) error {
	type wire StockPriceComparison
	var value wire
	if DecodeStrict(data, &value, ResponseLimit) != nil {
		return ErrInvalid
	}
	original, e1 := Canonical(data)
	normalized, e2 := canonicalValue(value)
	if e1 != nil || e2 != nil || !bytes.Equal(original, normalized) {
		return ErrInvalid
	}
	*p = StockPriceComparison(value)
	if !validPriceComparison(p) {
		return ErrInvalid
	}
	return nil
}

type StockParity struct {
	Status             string   `json:"status"`
	Reason             *string  `json:"reason"`
	Unit               string   `json:"unit"`
	Pool               *string  `json:"pool"`
	QuoteAsset         *string  `json:"quote_asset"`
	PoolPriceUSD       *string  `json:"pool_price_usd"`
	ReferencePriceUSD  *string  `json:"reference_price_usd"`
	PremiumDiscountBPS *string  `json:"premium_discount_bps"`
	PoolObservedAt     *int64   `json:"pool_observed_at"`
	ReferenceUpdatedAt *int64   `json:"reference_updated_at"`
	QuoteUSDUpdatedAt  *int64   `json:"quote_usd_updated_at"`
	ReferenceSource    *string  `json:"reference_source"`
	Unmeasured         []string `json:"unmeasured"`
}

type StockMarketStatus struct {
	RegularSession             string   `json:"regular_session"`
	Calendar                   string   `json:"calendar"`
	Timezone                   string   `json:"timezone"`
	ObservedAt                 int64    `json:"observed_at"`
	OraclePaused               *bool    `json:"oracle_paused"`
	CorporateActionPending     *bool    `json:"corporate_action_pending"`
	CorporateActionEffectiveAt *int64   `json:"corporate_action_effective_at"`
	TradingHalt                *bool    `json:"trading_halt"`
	Unmeasured                 []string `json:"unmeasured"`
}

var parityReasons = []string{
	"regular_session_closed", "regular_session_unknown", "identity_unverified", "oracle_paused", "oracle_pause_unknown",
	"trading_halted", "corporate_action_pending", "reference_unavailable", "reference_stale",
	"reference_basis_unknown", "multiplier_unknown", "pool_timestamp_unavailable", "pool_stale",
	"pool_scope_unavailable", "factory_code_mismatch", "no_supported_pool",
	"quote_reference_unavailable", "pool_state_unavailable",
}
var telemetryDecimal = regexp.MustCompile(`^-?(0|[1-9][0-9]*)\.[0-9]{8}$`)

func validTelemetryNumber(s *string, positive bool) bool {
	if s == nil || len(*s) > 160 || !telemetryDecimal.MatchString(*s) {
		return false
	}
	n, ok := new(big.Rat).SetString(*s)
	return ok && (!positive || n.Sign() > 0)
}
func validUnmeasured(items []string) bool {
	if items == nil || len(items) > 8 {
		return false
	}
	seen := map[string]bool{}
	for _, item := range items {
		if seen[item] || !oneOf(item, "exchange_holidays", "exchange_early_closes", "venue_execution", "other_pools", "pool_depth", "trading_halts", "scheduled_corporate_actions", "oracle_pause") {
			return false
		}
		seen[item] = true
	}
	return true
}
func validStockFacts(product string, a Answer) bool {
	if !validPriceComparison(a.PriceComparison) || (a.PriceComparison != nil && product != "stock_token_check") {
		return false
	}
	if a.Parity == nil && a.MarketStatus == nil {
		return true
	}
	if product != "stock_token_check" {
		return false
	}
	if m := a.MarketStatus; m != nil && m.Calendar != "weekday_hours_only" {
		p := a.Parity
		if p == nil {
			return false
		}
		unknown := m.RegularSession == "unknown"
		if unknown != (m.Calendar == "unavailable") {
			return false
		}
		if unknown && (p.Status != "not_checked" || !valueIs(p.Reason, "regular_session_unknown")) {
			return false
		}
		if m.RegularSession == "scheduled_closed" && (p.Status != "not_checked" || !valueIs(p.Reason, "regular_session_closed")) {
			return false
		}
		if m.RegularSession == "scheduled_open" && p.Reason != nil && oneOf(*p.Reason, "regular_session_unknown", "regular_session_closed") {
			return false
		}
		for _, items := range [][]string{m.Unmeasured, p.Unmeasured} {
			for _, fact := range []string{"exchange_holidays", "exchange_early_closes"} {
				if oneOf(fact, items...) != unknown {
					return false
				}
			}
		}
	} else if a.Parity != nil && valueIs(a.Parity.Reason, "regular_session_unknown") {
		return false
	}
	if p := a.Parity; p != nil {
		if p.Unit != "usd_per_token" || !oneOf(p.Status, "measured", "not_checked") || !validUnmeasured(p.Unmeasured) {
			return false
		}
		for _, stamp := range []*int64{p.PoolObservedAt, p.ReferenceUpdatedAt, p.QuoteUSDUpdatedAt} {
			if stamp != nil && *stamp <= 0 {
				return false
			}
		}
		if p.ReferenceSource != nil && !oneOf(*p.ReferenceSource, "chainlink_total_return", "issuer_token_quote") {
			return false
		}
		if p.Status == "measured" {
			if p.Reason != nil || p.Pool == nil || !addressPattern.MatchString(*p.Pool) || p.QuoteAsset == nil || !addressPattern.MatchString(*p.QuoteAsset) || p.PoolObservedAt == nil || p.ReferenceUpdatedAt == nil || p.QuoteUSDUpdatedAt == nil || p.ReferenceSource == nil || !validTelemetryNumber(p.PoolPriceUSD, true) || !validTelemetryNumber(p.ReferencePriceUSD, true) || !validTelemetryNumber(p.PremiumDiscountBPS, false) {
				return false
			}
		} else if p.Reason == nil || !oneOf(*p.Reason, parityReasons...) || p.PoolPriceUSD != nil || p.ReferencePriceUSD != nil || p.PremiumDiscountBPS != nil || p.Pool != nil || p.QuoteAsset != nil || p.QuoteUSDUpdatedAt != nil {
			return false
		}
	}
	if m := a.MarketStatus; m != nil {
		if !oneOf(m.RegularSession, "scheduled_open", "scheduled_closed", "unknown") || !validMarketCalendar(m) || m.Timezone != "America/New_York" || m.ObservedAt <= 0 || !validUnmeasured(m.Unmeasured) {
			return false
		}
		if m.CorporateActionEffectiveAt != nil && (m.CorporateActionPending == nil || !*m.CorporateActionPending || *m.CorporateActionEffectiveAt <= m.ObservedAt) {
			return false
		}
	}
	return true
}

func stockFactsText(a *Answer) []string {
	if a == nil {
		return nil
	}
	out := []string{}
	if p := a.PriceComparison; p != nil {
		text := "Intended price comparison: " + strings.ReplaceAll(p.Status, "_", " ")
		if p.Reason != nil {
			text += " (" + strings.ReplaceAll(*p.Reason, "_", " ") + ")"
		}
		out = append(out, text+".")
	}
	if p := a.Parity; p != nil {
		if p.Status == "measured" {
			out = append(out, fmt.Sprintf("Pool mid price: %s USD per token; reference: %s USD per token (%s). Premium/discount: %s basis points; positive means premium, negative means discount. Pool observed at %s; reference updated at %s.", *p.PoolPriceUSD, *p.ReferencePriceUSD, strings.ReplaceAll(*p.ReferenceSource, "_", " "), *p.PremiumDiscountBPS, time.Unix(*p.PoolObservedAt, 0).UTC().Format(time.RFC3339), time.Unix(*p.ReferenceUpdatedAt, 0).UTC().Format(time.RFC3339)))
		} else if p.Reason != nil {
			out = append(out, "Parity was not measured: "+strings.ReplaceAll(*p.Reason, "_", " ")+".")
		}
		out = append(out, "Pool coverage: pinned Uniswap V3 stablecoin pairs only, first liquid fee tier in order 100, 500, 3000, 10000. This is a mid price, not an execution quote.")
	}
	if m := a.MarketStatus; m != nil {
		session := "closed"
		if m.RegularSession == "scheduled_open" {
			session = "open"
		}
		if m.Calendar == "weekday_hours_only" {
			out = append(out, "Regular US equity session is scheduled "+session+" under weekday 09:30–16:00 New York hours. Holidays and early closes are not measured.")
		} else if m.RegularSession == "unknown" {
			out = append(out, "Regular US equity schedule is unknown: calendar evidence is unavailable.")
		} else if strings.HasPrefix(m.Calendar, "nyse_2027_v1:") {
			out = append(out, "Regular US equity session is scheduled "+session+" under the sourced 2027 NYSE calendar, including holidays and early closes. Nasdaq coverage and live venue availability are not established.")
		} else {
			out = append(out, "Regular US equity session is scheduled "+session+" under the sourced 2026 NYSE/Nasdaq calendar, including holidays and early closes. This is not live venue availability.")
		}
		state := func(value *bool) string {
			if value == nil {
				return "unknown"
			}
			if *value {
				return "yes"
			}
			return "no"
		}
		out = append(out, "Oracle paused: "+state(m.OraclePaused)+"; corporate action pending: "+state(m.CorporateActionPending)+"; issuer trading halt: "+state(m.TradingHalt)+".")
		if m.CorporateActionEffectiveAt != nil {
			out = append(out, "Scheduled multiplier change takes effect at "+time.Unix(*m.CorporateActionEffectiveAt, 0).UTC().Format(time.RFC3339)+".")
		}
		if len(m.Unmeasured) > 0 {
			out = append(out, "Unmeasured: "+strings.ReplaceAll(strings.Join(m.Unmeasured, ", "), "_", " ")+".")
		}
	}
	return out
}

// Optional facts must be objects when present. DecodeStrict recursively enforces
// the exact nested field names; the alias avoids recursing into this method.
func (a *Answer) UnmarshalJSON(data []byte) error {
	type wireAnswer Answer
	var wire wireAnswer
	if DecodeStrict(data, &wire, ResponseLimit) != nil {
		return ErrInvalid
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(data, &fields) != nil {
		return ErrInvalid
	}
	for _, key := range []string{"price_comparison", "parity", "market_status", "registry_observations", "check_observations", "paid_reviewers"} {
		if raw, present := fields[key]; present && bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return ErrInvalid
		}
	}
	candidate := Answer(wire)
	if !validStockFacts("stock_token_check", candidate) || !validRegistryObservations(candidate.RegistryObservations) {
		return ErrInvalid
	}
	if candidate.MarketStatus != nil && candidate.MarketStatus.Calendar != "weekday_hours_only" {
		for key, value := range map[string]any{"market_status": candidate.MarketStatus, "parity": candidate.Parity} {
			original, err := Canonical(fields[key])
			normalized, err2 := canonicalValue(value)
			if err != nil || err2 != nil || !bytes.Equal(original, normalized) {
				return ErrInvalid
			}
		}
	}
	*a = candidate
	return nil
}

func validMarketCalendar(m *StockMarketStatus) bool {
	if m.Calendar == "weekday_hours_only" {
		return m.RegularSession != "unknown"
	}
	if m.Calendar == "unavailable" {
		return m.RegularSession == "unknown"
	}
	return regexp.MustCompile(`^(us_equities_2026_v1|nyse_2027_v1):[0-9a-f]{64}$`).MatchString(m.Calendar) && m.RegularSession != "unknown"
}
