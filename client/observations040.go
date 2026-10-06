package client

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"reflect"
)

//go:embed observations040-schema.json
var observations040SchemaJSON []byte

var observationShapes = func() map[string]*portfolioShape {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(observations040SchemaJSON, &raw); err != nil {
		panic(err)
	}
	shapes := map[string]*portfolioShape{}
	for product, schema := range raw {
		wrapped, err := json.Marshal(map[string]json.RawMessage{"report": schema})
		if err != nil {
			panic(err)
		}
		shapes[product] = loadPortfolioContract(wrapped).Report
	}
	return shapes
}()

// Named maps preserve every sealed byte's value. Only the generated closed
// schemas may admit a shape, and receipt verification selects its product.
type CheckObservations map[string]any
type PaidReviewers map[string]any

func (o *CheckObservations) UnmarshalJSON(data []byte) error {
	var value map[string]any
	if DecodeStrict(data, &value, ResponseLimit) != nil {
		return ErrInvalid
	}
	for _, product := range registered040Products {
		if observationShapes[product].valid(value) {
			*o = value
			return nil
		}
	}
	return ErrInvalid
}
func (p *PaidReviewers) UnmarshalJSON(data []byte) error {
	var value map[string]any
	if DecodeStrict(data, &value, ResponseLimit) != nil || !validPaidReviewers(value) {
		return ErrInvalid
	}
	*p = value
	return nil
}
func factInt(v any) int64          { n, _ := v.(json.Number); i, _ := n.Int64(); return i }
func factMap(v any) map[string]any { m, _ := v.(map[string]any); return m }
func factList(v any) []any         { a, _ := v.([]any); return a }

func validPaidReviewers(value map[string]any) bool {
	if !observationShapes["paid_reviewers"].valid(value) {
		return false
	}
	f := factMap(value["facts"])
	if f["status"] == "not_checked" {
		return true
	}
	c := factMap(f["classification"])
	count := factInt(f["reviewer_count"])
	if factInt(c["payment_observed"])+factInt(c["payment_not_observed_in_scan"])+factInt(c["reviewer_is_payee"]) != count ||
		int64(len(factList(f["reviewers"])))+factInt(f["omitted_reviewer_rows"]) > count {
		return false
	}
	payees := map[any]bool{}
	for _, item := range factList(f["payees"]) {
		p := factMap(item)
		if payees[p["address"]] || len(factList(p["roles"])) == 0 {
			return false
		}
		payees[p["address"]] = true
	}
	displayed := map[string]int64{}
	seen := map[any]bool{}
	for _, item := range factList(f["reviewers"]) {
		row := factMap(item)
		address := row["address"]
		if seen[address] {
			return false
		}
		seen[address] = true
		transfers, orders := factInt(row["usdc_transfers"]), factInt(row["termix_settled_orders"])
		observed := transfers > 0 || orders > 0
		expected := "not_observed_in_scan"
		if observed {
			expected = "observed"
		}
		if payees[address] {
			expected = "reviewer_is_payee"
		}
		displayed[expected]++
		if (transfers > 0 && row["usdc_atomic"] == "0") || (orders > 0 && row["termix_provider_atomic"] == "0") {
			return false
		}
		if row["payment"] != expected || (row["last_payment"] != nil) != observed ||
			(transfers == 0 && row["usdc_atomic"] != "0") || (orders == 0 && row["termix_provider_atomic"] != "0") {
			return false
		}
		if !validRegistryID(row["usdc_atomic"].(string)) || !validRegistryID(row["termix_provider_atomic"].(string)) {
			return false
		}
	}
	for category, field := range map[string]string{"observed": "payment_observed", "not_observed_in_scan": "payment_not_observed_in_scan", "reviewer_is_payee": "reviewer_is_payee"} {
		if displayed[category] > factInt(c[field]) {
			return false
		}
	}
	if factInt(f["omitted_reviewer_rows"]) > 0 && len(factList(f["reviewers"])) != 12 {
		return false
	}
	for _, item := range factList(factMap(f["scan"])["windows"]) {
		w := factMap(item)
		if factInt(w["to_block"]) < factInt(w["from_block"]) || factInt(w["to_block"])-factInt(w["from_block"]) >= 1000 {
			return false
		}
	}
	return true
}

func valid040Answer(product string, a Answer) bool {
	if a.PaidReviewers != nil && (product != "agent_registry_check" || !validPaidReviewers(*a.PaidReviewers)) {
		return false
	}
	if !oneOf(product, registered040Products...) {
		return a.CheckObservations == nil
	}
	if a.CheckObservations == nil {
		return false
	}
	o := map[string]any(*a.CheckObservations)
	if !observationShapes[product].valid(o) || o["decision"] != a.LabelID || map[int]string{1: "proceed", 2: "do_not_proceed", 3: "not_verified"}[a.Option] != a.LabelID {
		return false
	}
	evidence := factMap(o["evidence"])
	if lendingDigest(evidence) != o["evidence_sha256"] {
		return false
	}
	if product != "shielded_route_check" {
		risks, unknowns := factList(evidence["risks"]), factList(evidence["unknown"])
		if !reflect.DeepEqual(o["unknowns"], evidence["unknown"]) {
			return false
		}
		if a.LabelID == "proceed" && (len(risks) != 0 || len(unknowns) != 0) {
			return false
		}
		if a.LabelID == "do_not_proceed" && (len(risks) == 0 || !reflect.DeepEqual(o["reasons"], evidence["risks"])) {
			return false
		}
	}
	if product == "route_check" && o["linkability_risk"] != evidence["linkability_risk"] {
		return false
	}
	if product == "private_receive_scan" {
		facts, subject := factMap(evidence["facts"]), factMap(evidence["subject"])
		if !validAnnouncementSet(factList(o["announcements"]), facts, subject) {
			return false
		}
		umbra := factMap(facts["umbra"])
		if (subject["network"] == "eip155:8453") != (umbra != nil) {
			return false
		}
		if umbra == nil && len(factList(o["umbra_announcements"])) != 0 {
			return false
		}
		if umbra != nil && !validAnnouncementSet(factList(o["umbra_announcements"]), umbra, subject) {
			return false
		}
	}
	if product == "shielded_route_check" {
		if o["execution_allowed"] != false || !reflect.DeepEqual(o["ranked"], evidence["rankings"]) {
			return false
		}
		options := map[int64]map[string]any{}
		for _, item := range append(append([]any{}, factList(o["options"])...), factList(o["not_private"])...) {
			row := factMap(item)
			id := factInt(row["id"])
			if options[id] != nil {
				return false
			}
			options[id] = row
		}
		// Candidate and exclusion lists must each match their sealed counterparts.
		for _, pair := range [][2]string{{"options", "routes"}, {"not_private", "not_private"}} {
			display, sealed := factList(o[pair[0]]), factList(evidence[pair[1]])
			if len(display) != len(sealed) {
				return false
			}
			for i, item := range display {
				row := factMap(item)
				if row["shielded_destination"] != (pair[0] == "options") || row["id"] != factMap(sealed[i])["id"] {
					return false
				}
			}
		}
		routes := append(append([]any{}, factList(evidence["routes"])...), factList(evidence["not_private"])...)
		if len(routes) != len(options) {
			return false
		}
		routeIDs := map[int64]bool{}
		for _, item := range routes {
			compact := factMap(item)
			id := factInt(compact["id"])
			option := options[id]
			if option == nil || routeIDs[id] {
				return false
			}
			routeIDs[id] = true
			for source, target := range map[string]string{"chain": "chain", "total": "total_cost_atomic", "seconds": "time_seconds", "balance": "custody_balance_atomic", "depositors": "eligible_depositors", "thin": "thin_crowd", "reviewed": "security_review_complete", "risks": "risks", "unknowns": "unknowns", "shielded_destination": "shielded_destination", "privacy_endpoint": "privacy_endpoint", "missing_legs": "missing_legs"} {
				if !reflect.DeepEqual(compact[source], option[target]) {
					return false
				}
			}
			compliance := factMap(option["compliance"])
			for source, target := range map[string]string{"asp": "asp", "ppoi": "ppoi", "screening": "currently_verified"} {
				if !reflect.DeepEqual(compact[source], compliance[target]) {
					return false
				}
			}
			// Mirror the reviewed destination catalogue; live flags cannot promote a venue.
			chain := factInt(option["chain"])
			private := (id == 1 && (chain == 8453 || chain == 4663)) || ((id == 12 || id == 13) && chain == 1)
			endpoint := "unverified"
			if id == 14 {
				endpoint = "transparent_zcash"
			} else if id == 12 || id == 13 {
				endpoint = "screened_pool"
			} else if private {
				endpoint = "pool"
			}
			eligible := private && len(factList(option["missing_legs"])) == 0 && len(factList(option["risks"])) == 0 && len(factList(option["unknowns"])) == 0 && option["total_cost_atomic"] != nil
			if option["shielded_destination"] != private || option["privacy_endpoint"] != endpoint || compact["eligible"] != eligible || option["eligible"] != (eligible && option["quote_expired"] == false) {
				return false
			}
			bridge := option["bridge"] != "none" || id == 14
			if compact["public_bridge"] != bridge || option["bridge_public"] != bridge || compact["transparent_zcash"] != (id == 14) {
				return false
			}
			_, quoted := compact["quote_hash"]
			if quoted != (option["quote_basis"] != nil) || (quoted && !reflect.DeepEqual(compact["expires"], option["quote_expires_at"])) {
				return false
			}
		}
		excluded := []any{}
		for _, item := range factList(o["options"]) {
			if len(factList(factMap(item)["risks"])) > 0 {
				excluded = append(excluded, item)
			}
		}
		if !reflect.DeepEqual(excluded, o["do_not_route"]) {
			return false
		}
		priority, _ := factMap(evidence["subject"])["priority"].(string)
		if a.LabelID == "proceed" && len(factList(factMap(o["ranked"])[priority])) == 0 {
			return false
		}
		if a.LabelID == "do_not_proceed" {
			if len(options) == 0 {
				return false
			}
			for _, row := range options {
				if len(factList(row["risks"])) == 0 {
					return false
				}
			}
		}
		for _, ranking := range factMap(o["ranked"]) {
			seen := map[int64]bool{}
			for _, id := range factList(ranking) {
				key := factInt(id)
				row := options[key]
				if row == nil || seen[key] || row["shielded_destination"] != true || row["eligible"] != true || len(factList(row["missing_legs"])) != 0 || row["total_cost_atomic"] == nil || row["time_seconds"] == nil || row["quote_expired"] == true || len(factList(row["risks"])) != 0 || len(factList(row["unknowns"])) != 0 {
					return false
				}
				seen[key] = true
			}
		}
		for _, item := range factList(o["do_not_route"]) {
			row := factMap(item)
			if !reflect.DeepEqual(row, options[factInt(row["id"])]) || len(factList(row["risks"])) == 0 {
				return false
			}
		}
	}
	return true
}
func validAnnouncementSet(rows []any, facts, subject map[string]any) bool {
	if facts["announcement_count"] == nil {
		return len(rows) == 0 && facts["announcement_set_sha256"] == nil
	}
	if factInt(facts["announcement_count"]) != int64(len(rows)) {
		return false
	}
	if len(rows) == 0 {
		return facts["announcement_set_sha256"] == nil || facts["announcement_set_sha256"] == lendingDigest(rows)
	}
	if facts["announcement_set_sha256"] != lendingDigest(rows) {
		return false
	}
	seen := map[string]bool{}
	for _, item := range rows {
		row := factMap(item)
		block := factInt(row["block_number"])
		if block < factInt(subject["from_block"]) || block > factInt(subject["to_block"]) {
			return false
		}
		key := fmt.Sprint(row["block_hash"], row["transaction_hash"], "/", row["log_index"])
		if seen[key] {
			return false
		}
		seen[key] = true
	}
	return true
}
func valid040Binding(s ReceiptEnvelope) bool {
	if s.Answer == nil {
		return true
	}
	a := *s.Answer
	if !valid040Answer(s.Product, a) {
		return false
	}
	for _, value := range []map[string]any{func() map[string]any {
		if a.CheckObservations != nil {
			return *a.CheckObservations
		}
		return nil
	}(), func() map[string]any {
		if a.PaidReviewers != nil {
			return *a.PaidReviewers
		}
		return nil
	}()} {
		if value != nil && (s.Verification == nil || value["evidence_sha256"] != s.Verification.EvidenceSHA256) {
			return false
		}
	}
	if a.CheckObservations != nil {
		source := factMap(factMap((*a.CheckObservations)["evidence"])["source"])
		if source != nil {
			block := s.Verification.Block
			if block == nil || factInt(source["chain_id"]) != block.ChainID || factInt(source["block_number"]) != block.Number || source["block_hash"] != block.Hash {
				return false
			}
		}
	}
	return true
}
func observationFactsText(a *Answer) []string {
	if a == nil {
		return nil
	}
	out := []string{}
	if a.CheckObservations != nil {
		out = append(out, "Signed observations describe only the submitted check and its stated coverage. Review unknowns before acting.")
		if (*a.CheckObservations)["execution_allowed"] == false {
			out = append(out, "Route estimates are advisory; execution is not authorized.")
		}
	}
	if a.PaidReviewers != nil {
		out = append(out, "Observed reviewer payments are not proof of genuine reviews. Missing payments only describe the scanned windows.")
	}
	return out
}
