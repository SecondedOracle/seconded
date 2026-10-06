package client

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"math/big"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const portfolioProduct = "portfolio_check"

//go:embed portfolio-schema.json
var portfolioSchemaJSON []byte

// The generated, closed server schema is the shape authority. This evaluator is
// deliberately limited to its vocabulary; it never resolves external schemas.
type portfolioShape struct {
	Schema               string                     `json:"$schema"`
	Type                 string                     `json:"type"`
	Properties           map[string]*portfolioShape `json:"properties"`
	Required             []string                   `json:"required"`
	AdditionalProperties bool                       `json:"additionalProperties"`
	AnyOf                []*portfolioShape          `json:"anyOf"`
	Enum                 []any                      `json:"enum"`
	Const                json.RawMessage            `json:"const"`
	Items                *portfolioShape            `json:"items"`
	MinItems             int                        `json:"minItems"`
	MaxItems             int                        `json:"maxItems"`
	Unique               bool                       `json:"uniqueItems"`
	Minimum              int64                      `json:"minimum"`
	Maximum              int64                      `json:"maximum"`
	Pattern              string                     `json:"pattern"`
	MaxLength            int                        `json:"maxLength"`
	pattern              *regexp.Regexp
	constant             any
}

var portfolioContract = loadPortfolioContract(portfolioSchemaJSON)

func loadPortfolioContract(data []byte) struct {
	Request      *portfolioShape   `json:"request"`
	Report       *portfolioShape   `json:"report"`
	Common       []string          `json:"common"`
	ReasonPolicy map[string][]any  `json:"reason_policy"`
	Cores        map[string]string `json:"cores"`
} {
	var c struct {
		Request      *portfolioShape   `json:"request"`
		Report       *portfolioShape   `json:"report"`
		Common       []string          `json:"common"`
		ReasonPolicy map[string][]any  `json:"reason_policy"`
		Cores        map[string]string `json:"cores"`
	}
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.UseNumber()
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&c); err != nil {
		panic(err)
	}
	var compile func(*portfolioShape)
	compile = func(s *portfolioShape) {
		if s == nil {
			return
		}
		if s.Schema != "" && s.Schema != "https://json-schema.org/draft/2020-12/schema" {
			panic(fmt.Sprintf("unsupported Portfolio schema dialect %q", s.Schema))
		}
		switch s.Type {
		case "", "object", "array", "string", "integer", "boolean":
		default:
			panic(fmt.Sprintf("unsupported Portfolio schema type %q", s.Type))
		}
		if s.Pattern != "" {
			s.pattern = regexp.MustCompile(s.Pattern)
		}
		if s.Const != nil {
			d := json.NewDecoder(strings.NewReader(string(s.Const)))
			d.UseNumber()
			if err := d.Decode(&s.constant); err != nil {
				panic(err)
			}
		}
		for _, p := range s.Properties {
			compile(p)
		}
		for _, p := range s.AnyOf {
			compile(p)
		}
		compile(s.Items)
	}
	compile(c.Request)
	compile(c.Report)
	return c
}

func (s *portfolioShape) valid(v any) bool {
	if len(s.AnyOf) > 0 {
		for _, branch := range s.AnyOf {
			if branch.valid(v) {
				return true
			}
		}
		return false
	}
	if s.Const != nil && !reflect.DeepEqual(v, s.constant) {
		return false
	}
	if s.Enum != nil {
		found := false
		for _, choice := range s.Enum {
			found = found || reflect.DeepEqual(v, choice)
		}
		if !found {
			return false
		}
	}
	switch s.Type {
	case "object":
		m, ok := v.(map[string]any)
		if !ok {
			return false
		}
		for _, key := range s.Required {
			if _, ok := m[key]; !ok {
				return false
			}
		}
		for key, value := range m {
			field, ok := s.Properties[key]
			if !ok || !field.valid(value) {
				return false
			}
		}
	case "array":
		a, ok := v.([]any)
		if !ok || len(a) < s.MinItems || len(a) > s.MaxItems {
			return false
		}
		seen := map[string]bool{}
		for _, value := range a {
			if !s.Items.valid(value) {
				return false
			}
			if s.Unique {
				b, err := canonicalValue(value)
				if err != nil || seen[string(b)] {
					return false
				}
				seen[string(b)] = true
			}
		}
	case "string":
		t, ok := v.(string)
		if !ok || len(t) > s.MaxLength || !s.pattern.MatchString(t) {
			return false
		}
	case "integer":
		n, ok := v.(json.Number)
		if !ok {
			return false
		}
		i, err := n.Int64()
		if err != nil || i < s.Minimum || i > s.Maximum {
			return false
		}
	case "boolean":
		if _, ok := v.(bool); !ok {
			return false
		}
	}
	return true
}

// These accessors are used only after the generated schema accepts the tree.
type portfolioObject = map[string]any

func po(v any) portfolioObject { return v.(map[string]any) }
func pa(v any) []any           { return v.([]any) }
func ps(v any) string          { return v.(string) }
func pn(v any) int64           { n, _ := v.(json.Number).Int64(); return n }
func pi(n int64) json.Number   { return json.Number(strconv.FormatInt(n, 10)) }
func portfolioIndex(rows any, key string) (map[string]portfolioObject, bool) {
	out := map[string]portfolioObject{}
	for _, row := range pa(rows) {
		m := po(row)
		id := ps(m[key])
		if out[id] != nil {
			return nil, false
		}
		out[id] = m
	}
	return out, true
}
func portfolioRefs(refs any, index map[string]portfolioObject) bool {
	for _, ref := range pa(refs) {
		if index[ps(ref)] == nil {
			return false
		}
	}
	return true
}
func portfolioContains(values any, v any) bool {
	items, _ := values.([]any)
	for _, x := range items {
		if reflect.DeepEqual(x, v) {
			return true
		}
	}
	return false
}

func normalizePortfolio(input json.RawMessage) (json.RawMessage, error) {
	var m map[string]any
	if DecodeStrict(input, &m, MessageLimit) != nil || m == nil {
		return nil, ErrInvalid
	}
	original, err := canonicalValue(m)
	if err != nil || len(original) > 8192 {
		return nil, ErrInvalid
	}
	defaults := map[string]any{"networks": []any{"eip155:8453", "eip155:5042", "eip155:4663"}, "token_hints": []any{}, "market_hints": []any{}, "dust_usd": "1.00"}
	for key, value := range defaults {
		if _, ok := m[key]; !ok {
			m[key] = value
		}
	}
	rows := []any{m}
	for _, key := range []string{"token_hints", "market_hints"} {
		if a, ok := m[key].([]any); ok {
			rows = append(rows, a...)
		}
	}
	for _, row := range rows {
		if o, ok := row.(map[string]any); ok {
			for _, key := range []string{"account", "address", "core", "market_id"} {
				if s, ok := o[key].(string); ok {
					o[key] = strings.ToLower(s)
				}
			}
		}
	}
	if !portfolioContract.Request.valid(m) {
		return nil, ErrInvalid
	}
	for _, key := range []string{"token_hints", "market_hints"} {
		for _, value := range pa(m[key]) {
			hint := po(value)
			if !portfolioContains(m["networks"], hint["network"]) {
				return nil, ErrInvalid
			}
			if key == "market_hints" && hint["core"] != portfolioContract.Cores[strings.TrimPrefix(ps(hint["network"]), "eip155:")] {
				return nil, ErrInvalid
			}
		}
	}
	b, err := canonicalValue(m)
	if err != nil || len(b) > 8192 {
		return nil, ErrInvalid
	}
	return b, nil
}

func normalizePortfolioRequest(r Request) (Request, error) {
	if r.Product != portfolioProduct {
		return r, nil
	}
	b, err := normalizePortfolio(r.Input)
	r.Input = b
	return r, err
}

func portfolioRequestScope(r Request) (string, error) {
	if r.Product != portfolioProduct {
		return "", nil
	}
	normalized, err := normalizePortfolio(r.Input)
	original, err2 := Canonical(r.Input)
	if err != nil || err2 != nil || string(normalized) != string(original) {
		return "", ErrInvalid
	}
	var input map[string]any
	if DecodeStrict(normalized, &input, 8192) != nil {
		return "", ErrInvalid
	}
	networks := []any{}
	for _, n := range pa(input["networks"]) {
		networks = append(networks, json.Number(strings.TrimPrefix(ps(n), "eip155:")))
	}
	scope := map[string]any{"account": input["account"], "networks": networks, "position_limit": pi(25), "dust_usd": input["dust_usd"]}
	return comparisonHash(scope), nil
}

func validPortfolioEntry(e Entry) bool {
	if e.Product == portfolioProduct {
		return hexDigest.MatchString(e.PortfolioScopeSHA256)
	}
	return e.PortfolioScopeSHA256 == ""
}

// Keep the exact validated tree, including required nulls, without a lossy
// float64 round trip. The receipt signature authenticates these report bytes.
type PortfolioReport struct{ fields portfolioObject }

func (r PortfolioReport) MarshalJSON() ([]byte, error) { return json.Marshal(r.fields) }
func (r *PortfolioReport) UnmarshalJSON(data []byte) error {
	var m map[string]any
	if DecodeStrict(data, &m, ResponseLimit) != nil || !validPortfolioReport(m) {
		return ErrInvalid
	}
	r.fields = m
	return nil
}

func validPortfolioReport(r portfolioObject) bool {
	if !portfolioContract.Report.valid(r) {
		return false
	}
	body, err := canonicalValue(r)
	if err != nil || len(body) > 48<<10 {
		return false
	}
	scope := po(r["scope"])
	evidence := map[string]any{"schema": "portfolio-evidence/v1", "subject": "subject", "networks": scope["networks"]}
	for _, key := range portfolioContract.Common {
		evidence[key] = r[key]
	}
	body, err = canonicalValue(evidence)
	// Schema strings are all ASCII, so this is the server's ASCII canonical hash.
	if err != nil || len(body) > 24<<10 || comparisonHash(evidence) != r["evidence_sha256"] {
		return false
	}
	observed, until := pn(r["observed_at"]), pn(r["valid_until"])
	pins, ok := portfolioIndex(r["sources"], "id")
	if !ok {
		return false
	}
	chains := map[int64]portfolioObject{}
	for _, pin := range pins {
		chain := pn(pin["chain_id"])
		if chains[chain] != nil || !portfolioContains(scope["networks"], pin["chain_id"]) || pn(pin["timestamp"]) > observed || observed >= until || until > pn(pin["timestamp"])+120 {
			return false
		}
		chains[chain] = pin
	}
	if len(chains) != len(pa(scope["networks"])) {
		return false
	}
	positions, ok := portfolioIndex(r["positions"], "id")
	if !ok {
		return false
	}
	economic := map[string]bool{}
	for _, p := range positions {
		pin := pins[ps(p["pin_ref"])]
		if pin == nil || pin["chain_id"] != p["chain_id"] {
			return false
		}
		roles := map[string]bool{}
		nonzero := false
		for _, item := range pa(p["legs"]) {
			leg := po(item)
			role := ps(leg["role"])
			if roles[role] || (leg["status"] == "known") != (leg["raw_amount"] != nil) {
				return false
			}
			roles[role] = true
			if leg["raw_amount"] != nil {
				if !validRegistryID(ps(leg["raw_amount"])) {
					return false
				}
				nonzero = nonzero || leg["raw_amount"] != "0"
			}
		}
		key := fmt.Sprint(p["chain_id"], "/", p["address"])
		if p["kind"] == "lending" {
			if p["core"] != portfolioContract.Cores[p["chain_id"].(json.Number).String()] {
				return false
			}
			if len(roles) != 3 || !roles["supply"] || !roles["collateral"] || !roles["debt"] || p["display_dust"] == true {
				return false
			}
			for _, field := range []string{"supply_shares", "borrow_shares"} {
				if !validRegistryID(ps(p[field])) {
					return false
				}
				nonzero = nonzero || p[field] != "0"
			}
			for _, leg := range pa(p["legs"]) {
				if po(leg)["asset"] == nil {
					return false
				}
			}
			if !nonzero {
				return false
			}
			key = fmt.Sprint(p["chain_id"], "/lending/", p["core"], "/", p["market_id"])
		} else {
			if len(roles) != 1 || !roles["wallet"] || (p["kind"] == "native") != (p["address"] == nil) || p["kind"] == "stock" && pn(p["chain_id"]) == 5042 {
				return false
			}
			leg := po(pa(p["legs"])[0])
			if leg["asset"] != p["address"] || leg["status"] != "known" || leg["raw_amount"] == "0" {
				return false
			}
		}
		if economic[key] || scope["dust_usd"] == "0.00" && p["display_dust"] == true {
			return false
		}
		economic[key] = true
	}
	if pn(po(r["budget"])["nonzero_positions"]) != int64(len(positions)) {
		return false
	}
	coverage := po(r["coverage"])
	entries, ok := portfolioIndex(coverage["entries"], "id")
	if !ok || !portfolioContains(coverage["limitations"], "bounded_catalog_and_logs") {
		return false
	}
	components := map[string]bool{}
	for _, c := range entries {
		chain, component, status := pn(c["chain_id"]), ps(c["component"]), ps(c["status"])
		key := fmt.Sprint(chain, "/", component)
		if chains[chain] == nil || components[key] || (status == "checked") != (c["reason"] == nil) {
			return false
		}
		components[key] = true
		candidates, checked, unknown := pn(c["candidates"]), pn(c["checked"]), pn(c["unknown"])
		if checked+unknown > candidates || component == "lending" && candidates > 64 || oneOf(status, "not_checked", "unavailable") && checked != 0 {
			return false
		}
		if status == "checked" && (unknown != 0 || checked != candidates || oneOf(component, "tokens", "stocks", "lending") && c["catalog_sha256"] == nil) {
			return false
		}
		if chain == 4663 && component == "lending" && (status != "not_checked" || c["reason"] != "protocol_unqualified") || chain == 5042 && component == "stocks" && (status != "not_checked" || c["reason"] != "chain_unqualified") {
			return false
		}
		if c["window"] != nil {
			w, pin := po(c["window"]), chains[chain]
			if pn(w["from_block"]) > pn(w["to_block"]) || w["to_block"] != pin["number"] || w["end_hash"] != pin["hash"] {
				return false
			}
		} else if component == "logs" && oneOf(status, "checked", "partial") {
			return false
		}
	}
	if len(components) != len(chains)*5 {
		return false
	}
	prices, ok := portfolioIndex(r["prices"], "id")
	if !ok {
		return false
	}
	for _, price := range prices {
		if chains[pn(price["chain_id"])] == nil || pn(price["observed_at"]) > observed || until > pn(price["valid_until"]) {
			return false
		}
	}
	findings, ok := portfolioIndex(r["findings"], "id")
	if !ok {
		return false
	}
	dependencies, ok := portfolioIndex(r["dependencies"], "id")
	if !ok {
		return false
	}
	candidates, ok := portfolioIndex(r["candidates"], "id")
	if !ok {
		return false
	}
	bounds, ok := portfolioIndex(r["validity_bounds"], "id")
	if !ok {
		return false
	}
	sources, refs, all := map[string]portfolioObject{}, map[string]portfolioObject{}, map[string]bool{}
	for _, index := range []map[string]portfolioObject{pins, prices, entries, findings, dependencies, candidates, positions, bounds} {
		for id, row := range index {
			if all[id] {
				return false
			}
			all[id] = true
			refs[id] = row
		}
	}
	for _, index := range []map[string]portfolioObject{pins, prices, entries} {
		for id, row := range index {
			sources[id] = row
		}
	}
	for id := range candidates {
		delete(refs, id)
	}
	for id := range positions {
		delete(refs, id)
	}
	for id := range bounds {
		delete(refs, id)
	}
	for _, index := range []map[string]portfolioObject{findings, dependencies} {
		for _, item := range index {
			if !portfolioRefs(item["position_ids"], positions) || !portfolioRefs(item["source_refs"], sources) || len(pa(item["source_refs"])) == 0 {
				return false
			}
			for _, ref := range pa(item["source_refs"]) {
				if c := entries[ps(ref)]; c != nil && c["status"] != "checked" && item["status"] == "clear" {
					return false
				}
			}
		}
	}
	for _, b := range bounds {
		if sources[ps(b["source_ref"])] == nil || until > pn(b["valid_until"]) {
			return false
		}
	}
	covered := map[string]bool{}
	for _, c := range candidates {
		policy := portfolioContract.ReasonPolicy[ps(c["reason_code"])]
		if c["kind"] != policy[0] || pn(c["priority_floor"]) > pn(policy[1]) || !portfolioRefs(c["position_ids"], positions) || !portfolioRefs(c["finding_ids"], findings) || !portfolioRefs(c["evidence_refs"], refs) {
			return false
		}
		linkedPositions := map[string]bool{}
		for _, id := range pa(c["finding_ids"]) {
			f := findings[ps(id)]
			covered[ps(id)] = true
			if f["status"] == "clear" || f["status"] == "hard" && (c["status"] != "hard" || pn(c["priority_floor"]) != 0) {
				return false
			}
			for _, pid := range pa(f["position_ids"]) {
				linkedPositions[ps(pid)] = true
			}
		}
		if len(linkedPositions) != len(pa(c["position_ids"])) {
			return false
		}
		for _, pid := range pa(c["position_ids"]) {
			if !linkedPositions[ps(pid)] {
				return false
			}
		}
	}
	for id, f := range findings {
		if f["status"] == "hard" && !covered[id] {
			return false
		}
	}
	if !validPortfolioValuations(r, positions, prices, findings) {
		return false
	}
	actions, ok := portfolioIndex(r["actions"], "id")
	if !ok || len(actions) != len(candidates) || pn(po(r["agreement"])["item_count"]) != int64(len(candidates)) {
		return false
	}
	for id, action := range actions {
		c := candidates[id]
		if c == nil || pn(action["priority"]) > pn(c["priority_floor"]) {
			return false
		}
		for _, key := range []string{"kind", "position_ids", "finding_ids"} {
			if !reflect.DeepEqual(action[key], c[key]) {
				return false
			}
		}
		if po(r["agreement"])["overall"] == "no_additional_attention" && oneOf(ps(c["status"]), "hard", "unknown") {
			return false
		}
	}
	return sort.SliceIsSorted(pa(r["actions"]), func(i, j int) bool {
		a, b := po(pa(r["actions"])[i]), po(pa(r["actions"])[j])
		if pn(a["priority"]) != pn(b["priority"]) {
			return pn(a["priority"]) < pn(b["priority"])
		}
		x, y := candidates[ps(a["id"])], candidates[ps(b["id"])]
		if pn(x["priority_floor"]) != pn(y["priority_floor"]) {
			return pn(x["priority_floor"]) < pn(y["priority_floor"])
		}
		return ps(a["id"]) < ps(b["id"])
	})
}

func portfolioDecimal(v any) *big.Rat {
	if v == nil {
		return new(big.Rat)
	}
	n, _ := new(big.Rat).SetString(ps(v))
	return n
}
func validPortfolioValuations(r portfolioObject, positions, prices, findings map[string]portfolioObject) bool {
	vals := po(r["valuations"])
	items, ok := portfolioIndex(vals["items"], "position_id")
	if !ok || len(items) != len(positions) {
		return false
	}
	assets, debt := new(big.Rat), new(big.Rat)
	priced, unpricedDebt := int64(0), int64(0)
	for id, item := range items {
		p := positions[id]
		if p == nil || !portfolioRefs(item["price_refs"], prices) {
			return false
		}
		known := item["assets_usd"] != nil && item["liabilities_usd"] != nil
		if known {
			priced++
		}
		if p["kind"] == "lending" && item["liabilities_usd"] == nil {
			unpricedDebt++
		}
		a, d := portfolioDecimal(item["assets_usd"]), portfolioDecimal(item["liabilities_usd"])
		assets.Add(assets, a)
		debt.Add(debt, d)
		if p["economic_alias"] == "unknown" && item["assets_usd"] != nil {
			return false
		}
		for _, leg := range pa(p["legs"]) {
			l := po(leg)
			side := "assets_usd"
			if l["role"] == "debt" {
				side = "liabilities_usd"
			}
			if l["status"] == "unknown" && item[side] != nil {
				return false
			}
		}
		if p["kind"] != "lending" && item["liabilities_usd"] != nil && item["liabilities_usd"] != "0" {
			return false
		}
		if (a.Sign() != 0 || d.Sign() != 0) && len(pa(item["price_refs"])) == 0 {
			return false
		}
		if p["display_dust"] == true {
			if !known || a.Cmp(big.NewRat(1, 1)) >= 0 {
				return false
			}
			for _, f := range findings {
				if f["status"] != "clear" && portfolioContains(f["position_ids"], id) {
					return false
				}
			}
		}
	}
	if assets.Cmp(portfolioDecimal(vals["gross_assets_usd"])) != 0 || debt.Cmp(portfolioDecimal(vals["liabilities_usd"])) != 0 || new(big.Rat).Sub(assets, debt).Cmp(portfolioDecimal(vals["net_usd"])) != 0 {
		return false
	}
	return pn(vals["priced_positions"]) == priced && pn(vals["unpriced_positions"]) == int64(len(positions))-priced && pn(vals["unpriced_debt"]) == unpricedDebt && (vals["status"] == "priced") == (priced == int64(len(positions)))
}

func portfolioVerification(r portfolioObject) *Verification {
	reason := "bounded_catalog_and_logs"
	v := &Verification{Schema: VerificationSchema, EvidenceSHA256: ps(r["evidence_sha256"]), Findings: []ReceiptFinding{}, Coverage: []ReceiptCoverage{{Fact: "portfolio.inventory", Status: "partial", Reason: &reason}}}
	for _, item := range pa(po(r["coverage"])["entries"]) {
		c := po(item)
		var reason *string
		if c["reason"] != nil {
			s := ps(c["reason"])
			reason = &s
		}
		v.Coverage = append(v.Coverage, ReceiptCoverage{Fact: fmt.Sprintf("portfolio.chain%d.%s", pn(c["chain_id"]), ps(c["component"])), Status: ps(c["status"]), Reason: reason})
	}
	for i, item := range pa(r["findings"]) {
		f := po(item)
		if oneOf(ps(f["status"]), "hard", "soft") {
			v.Findings = append(v.Findings, ReceiptFinding{Code: ps(f["code"]), Severity: ps(f["status"]), Values: map[string]any{"finding_index": i}})
		}
	}
	sort.Slice(v.Coverage, func(i, j int) bool { return v.Coverage[i].Fact < v.Coverage[j].Fact })
	sort.Slice(v.Findings, func(i, j int) bool {
		a, b := v.Findings[i], v.Findings[j]
		if a.Severity != b.Severity {
			return a.Severity == "hard"
		}
		if a.Code != b.Code {
			return a.Code < b.Code
		}
		x, _ := canonicalValue(a)
		y, _ := canonicalValue(b)
		return string(x) < string(y)
	})
	return v
}

func validPortfolioAnswer(product string, a Answer) bool {
	if product != portfolioProduct {
		return a.PortfolioReport == nil
	}
	if a.PortfolioReport == nil || !validPortfolioReport(a.PortfolioReport.fields) {
		return false
	}
	overall := po(a.PortfolioReport.fields["agreement"])["overall"]
	return overall == "review" && a.Option == 1 && a.LabelID == "portfolio_attention_required" || overall == "no_additional_attention" && a.Option == 2 && a.LabelID == "portfolio_no_additional_attention"
}
func validPortfolioBinding(s ReceiptEnvelope, e Entry, now time.Time) bool {
	if !validPortfolioEntry(e) {
		return false
	}
	if s.Answer == nil || s.Answer.PortfolioReport == nil {
		return true
	}
	r := s.Answer.PortfolioReport.fields
	if s.Version != 3 || s.Product != portfolioProduct || s.OutcomeAt == nil || comparisonHash(r["scope"]) != e.PortfolioScopeSHA256 || !sameVerification(s.Verification, portfolioVerification(r)) {
		return false
	}
	b, err := canonicalValue(s.Verification)
	if err != nil || len(b) > 8<<10 {
		return false
	}
	at, err := time.Parse(time.RFC3339, *s.OutcomeAt)
	return err == nil && !at.Before(time.Unix(pn(r["observed_at"]), 0)) && !at.After(time.Unix(pn(r["valid_until"])-15, 0)) && !at.After(now)
}

func portfolioText(r *PortfolioReport, now int64) ([]string, bool) {
	m := r.fields
	expired := now > pn(m["valid_until"])
	text := []string{fmt.Sprintf("Bounded portfolio coverage: partial inventory; %d discovered positions. This does not prove a complete or empty wallet.", len(pa(m["positions"])))}
	for _, item := range pa(po(m["coverage"])["entries"]) {
		c := po(item)
		line := fmt.Sprintf("Chain %d %s: %s (%d checked, %d unknown of %d candidates).", pn(c["chain_id"]), ps(c["component"]), ps(c["status"]), pn(c["checked"]), pn(c["unknown"]), pn(c["candidates"]))
		if c["reason"] != nil {
			line += " Reason: " + ps(c["reason"]) + "."
		}
		text = append(text, line)
	}
	for _, item := range pa(m["actions"]) {
		a := po(item)
		text = append(text, fmt.Sprintf("Priority %d: %s; positions %v; findings %v.", pn(a["priority"]), ps(a["kind"]), a["position_ids"], a["finding_ids"]))
	}
	state := "Report valid until "
	if expired {
		state = "Historical report expired at "
	}
	text = append(text, state+time.Unix(pn(m["valid_until"]), 0).UTC().Format(time.RFC3339)+". Observations are not atomic across chains; changes can invalidate them sooner.", "Review attention items within the declared coverage. Actions do not authorize transactions or allocations.")
	return text, expired
}
