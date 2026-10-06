package client

import (
	"fmt"
	"math/big"
	"os"
	"strings"
	"sync"
	"time"
)

const privacyUSDC = "0x833589fcd6edb6e08f4c7c32d4f71b54bda02913"
const privacyRouteDisclaimer = "A clear result is not universal clearance. Absence of listing is neither legal clearance nor a safety guarantee. Address matching alone does not evaluate persons, ownership, or geography."

type privacySanctions struct {
	payload privacyMap
	index   map[string][]any
	reason  string
}

var privacySanctionsCache = sync.OnceValue(func() privacySanctions { return privacyDecodeSanctions(privacyDocument("sanctions")) })

func privacyLoadSanctions(snapshot privacyMap) privacySanctions {
	if snapshot != nil {
		return privacyDecodeSanctions(snapshot)
	}
	if os.Getenv("SECONDED_SNAPSHOT_DIR") != "" {
		return privacyDecodeSanctions(privacyDocument("sanctions"))
	}
	cached := privacySanctionsCache()
	result := privacySanctions{payload: privacyObject(privacyClone(cached.payload)), reason: cached.reason, index: map[string][]any{}}
	for key, rows := range cached.index {
		result.index[key] = privacyArray(privacyClone(rows))
	}
	return result
}
func privacyDecodeSanctions(snapshot privacyMap) privacySanctions {
	result := privacySanctions{index: map[string][]any{}, reason: "snapshot_unavailable"}
	p := privacyObject(snapshot["payload"])
	if p == nil || snapshot["schema"] != "seconded-public-data/v1" || snapshot["sha256"] != privacyHash(p) || p["kind"] != "sanctions" || !oneOf(privacyString(p["coverage"]), "complete_digital_currency_extract", "excerpt") || !privacyMatch(`[0-9a-f]{64}`, privacyObject(p["source"])["sha256"]) || len(privacyArray(p["addresses"])) == 0 {
		return result
	}
	if _, e := time.Parse("2006-01-02", privacyString(p["list_date"])); e != nil {
		return result
	}
	for _, v := range privacyArray(p["addresses"]) {
		r := privacyObject(v)
		a := strings.TrimSpace(privacyString(r["address"]))
		if privacyString(r["currency"]) == "" || privacyString(r["uid"]) == "" {
			return result
		}
		if privacyMatch(`0[xX][0-9a-fA-F]{40}`, a) {
			a = strings.ToLower(a)
		} else if strings.HasPrefix(strings.ToLower(a), "0x") || !privacyMatch(`[A-Za-z0-9:_-]{10,200}`, a) {
			return result
		}
		result.index[a] = append(result.index[a], r)
	}
	result.payload = p
	result.reason = ""
	return result
}
func privacyListAge(p privacyMap, now time.Time) int {
	day, _ := time.Parse("2006-01-02", privacyString(p["list_date"]))
	today := time.Date(now.UTC().Year(), now.UTC().Month(), now.UTC().Day(), 0, 0, 0, 0, time.UTC)
	return int(today.Sub(day) / (24 * time.Hour))
}
func privacyRouteScreen(addresses []string, snapshot privacyMap, now time.Time) privacyMap {
	r := privacyMap{"status": "unknown", "is_sanctioned": nil, "matches": []any{}, "list_date": nil, "list_version": nil, "scope": "Local OFAC SDN digital-currency snapshot; no oracle query", "disclaimer": privacyRouteDisclaimer}
	if len(addresses) == 0 {
		r["reason"] = "no_addresses_provided"
		return r
	}
	s := privacyLoadSanctions(snapshot)
	if s.payload == nil {
		r["reason"] = s.reason
		return r
	}
	r["list_date"] = s.payload["list_date"]
	r["list_version"] = privacyObject(s.payload["source"])["sha256"]
	age := privacyListAge(s.payload, now)
	if age < 0 || age > 7 {
		r["reason"] = "stale_list"
		if age < 0 {
			r["reason"] = "future_list"
		}
		r["scope"] = "Local OFAC SDN snapshot; no oracle query"
		return r
	}
	matches := []any{}
	for _, a := range addresses {
		matches = append(matches, s.index[strings.ToLower(a)]...)
	}
	if len(matches) > 0 {
		r["status"] = "listed"
		r["is_sanctioned"] = true
		r["matches"] = matches
	} else if s.payload["coverage"] == "excerpt" {
		r["reason"] = "partial_snapshot"
	} else {
		r["status"] = "not_listed"
		r["is_sanctioned"] = false
	}
	return r
}
func privacyPurchaseRequest(input privacyMap) privacyMap {
	guide, _ := privacyTool("seconded_private_purchase_prepare")
	if !privacySchema(guide.InputSchema, input, 0) {
		return nil
	}
	r := privacyObject(privacyClone(input))
	invoice := privacyObject(r["invoice"])
	address := func(m privacyMap, k string) bool {
		if !privacyAddress(m[k], true) {
			return false
		}
		m[k] = strings.ToLower(privacyString(m[k]))
		return true
	}
	amount := func(v any) bool { return privacyAmount(v, 256, false) && len(privacyString(v)) != 64 }
	for _, k := range []string{"merchant", "payee"} {
		if !address(invoice, k) {
			return nil
		}
	}
	if invoice["asset"] != "native" && !address(invoice, "asset") {
		return nil
	}
	if !address(r, "wallet_address") || !address(r, "refund_address") || !amount(r["max_amount"]) || !amount(invoice["amount"]) {
		return nil
	}
	known := []any{}
	for _, v := range privacyArray(r["known_addresses"]) {
		if !privacyAddress(v, true) {
			return nil
		}
		known = append(known, strings.ToLower(privacyString(v)))
	}
	r["known_addresses"] = known
	if portfolioContains(known, r["wallet_address"]) || r["wallet_address"] == invoice["merchant"] || r["wallet_address"] == invoice["payee"] || r["refund_address"] != r["wallet_address"] {
		return nil
	}
	a, _ := new(big.Int).SetString(privacyString(invoice["amount"]), 10)
	max, _ := new(big.Int).SetString(privacyString(r["max_amount"]), 10)
	if a.Cmp(max) > 0 {
		return nil
	}
	for _, v := range privacyArray(r["funding_route"]) {
		row := privacyObject(v)
		if !address(row, "source") || !address(row, "recipient") || !amount(row["amount"]) {
			return nil
		}
		if row["asset"] != "native" && !address(row, "asset") {
			return nil
		}
		if _, ok := row["observed_at"]; !ok {
			row["observed_at"] = nil
		}
	}
	return r
}
func privacyPurchaseScreen(r, snapshot privacyMap, now time.Time) privacyMap {
	out := privacyMap{"status": "unknown", "list_date": nil, "list_version": nil, "scope": "Local OFAC SDN digital-currency snapshot; no oracle query", "disclaimer": privacyDocument("text")["purchase_disclaimer"], "participants": []any{}}
	s := privacyLoadSanctions(snapshot)
	if s.payload == nil {
		out["reason"] = "snapshot_unavailable"
		return out
	}
	out["list_date"] = s.payload["list_date"]
	out["list_version"] = privacyObject(s.payload["source"])["sha256"]
	age := privacyListAge(s.payload, now)
	if age < 0 || age > 7 {
		out["reason"] = "stale_or_future_list"
		return out
	}
	rows := []any{}
	overall := "not_listed"
	add := func(role string, a any) {
		status := "not_listed"
		if s.payload["coverage"] != "complete_digital_currency_extract" {
			status = "unknown"
		}
		if len(s.index[privacyString(a)]) > 0 {
			status = "listed"
		}
		if status == "listed" || (status == "unknown" && overall != "listed") {
			overall = status
		}
		rows = append(rows, privacyMap{"role": role, "address": a, "status": status})
	}
	inv := privacyObject(r["invoice"])
	add("merchant", inv["merchant"])
	add("payee", inv["payee"])
	add("wallet", r["wallet_address"])
	add("refund", r["refund_address"])
	if inv["asset"] != "native" {
		add("asset", inv["asset"])
	}
	for i, v := range privacyArray(r["known_addresses"]) {
		add(fmt.Sprintf("known_%d", i), v)
	}
	for i, v := range privacyArray(r["funding_route"]) {
		row := privacyObject(v)
		for _, k := range []string{"source", "recipient", "asset"} {
			if row[k] != "native" {
				add(fmt.Sprintf("route_%d_%s", i, k), row[k])
			}
		}
	}
	out["status"] = overall
	out["participants"] = rows
	return out
}
func privacyFunding(r privacyMap, now time.Time) privacyMap {
	risks := map[string]bool{"public_funding_boundaries": true, "amount_timing_correlation": true, "rpc_ip_metadata": true, "consolidation_sweep_links": true}
	seen := map[string]bool{}
	fresh := len(privacyArray(r["funding_route"])) > 0
	for _, v := range privacyArray(r["funding_route"]) {
		row := privacyObject(v)
		for _, k := range []string{"source", "recipient"} {
			a := privacyString(row[k])
			if seen[a] {
				risks["repeated_route_address"] = true
			}
			seen[a] = true
		}
		if portfolioContains(r["known_addresses"], row["source"]) && row["recipient"] == r["wallet_address"] {
			risks["direct_funding_link"] = true
		}
		switch row["kind"] {
		case "gas_topup":
			risks["gas_funding_link"] = true
		case "public_swap":
			risks["public_settlement"] = true
		case "pool_withdrawal":
			for _, k := range []string{"pool_safety_unverified", "anonymity_set_unknown", "public_withdrawal"} {
				risks[k] = true
			}
		}
		age := privacyWallSeconds(now) - float64(privacyInt(row["observed_at"]))
		if row["observed_at"] == nil || age < 0 || age > 3600 {
			fresh = false
		}
	}
	freshness := "unknown"
	if fresh {
		freshness = "within_age_limit"
	}
	return privacyMap{"steps": r["funding_route"], "provenance": "caller_asserted", "unlinkability": "unknown", "freshness": freshness, "risks": privacySorted(risks), "execution": "guidance_only_no_funding_calls"}
}
func (n *privacyNative) purchase(input, authenticated, snapshot privacyMap) privacyMap {
	r := privacyPurchaseRequest(input)
	if r == nil {
		return nil
	}
	invoice := privacyObject(r["invoice"])
	if authenticated == nil {
		authenticated = privacyTrustedDocument("invoice-" + privacyString(invoice["nonce"]) + ".json")
	}
	if authenticated == nil {
		return privacyMap{"status": "unknown", "reason": "invoice_authentication_unavailable", "transaction": nil}
	}
	// Validate and normalize the independent invoice with the same request rules.
	trusted := privacyObject(privacyClone(input))
	trusted["invoice"] = authenticated
	clean := privacyPurchaseRequest(trusted)
	if clean == nil || privacyHash(invoice) != privacyHash(clean["invoice"]) {
		return nil
	}
	now := n.now()
	if privacyInt(invoice["expiry"]) <= now.Unix() {
		return nil
	}
	screening := privacyPurchaseScreen(r, snapshot, now)
	var tx any
	if privacyInt(invoice["chain_id"]) == 8453 && (invoice["asset"] == "native" || invoice["asset"] == privacyUSDC) {
		to, value, data := invoice["payee"], invoice["amount"], "0x"
		if invoice["asset"] != "native" {
			to, value = privacyUSDC, "0"
			amount, _ := new(big.Int).SetString(privacyString(invoice["amount"]), 10)
			data = "0xa9059cbb" + strings.Repeat("0", 24) + privacyString(invoice["payee"])[2:] + fmt.Sprintf("%064x", amount)
		}
		tx = privacyMap{"chain_id": 8453, "from": r["wallet_address"], "to": to, "value": value, "data": data}
	}
	status := "unsigned_plan"
	switch screening["status"] {
	case "listed":
		status, tx = "blocked", nil
	case "not_listed":
		if tx == nil || invoice["rail"] == "x402" {
			status, tx = "guidance_only", nil
		}
	default:
		status, tx = "unknown", nil
	}
	plan := privacyText("purchase")
	plan["status"] = status
	plan["invoice_binding"] = invoice
	plan["payment_id"] = privacyHash(privacyMap{"domain": "seconded-private-purchase/v1", "chain_id": invoice["chain_id"], "merchant": invoice["merchant"], "nonce": invoice["nonce"]})
	c := privacyObject(plan["compartmentalization"])
	c["fresh_recipient"] = r["wallet_address"]
	c["refund_address"] = r["refund_address"]
	plan["funding_route"] = privacyFunding(r, now)
	plan["sanctions_screen"] = screening
	plan["transaction"] = tx
	plan["prepared_at"] = privacyWallNumber(now)
	plan["valid_until"] = privacyWallNumber(now.Add(3600 * time.Second))
	if float64(privacyInt(invoice["expiry"])) < privacyWallSeconds(now)+3600 {
		plan["valid_until"] = invoice["expiry"]
	}
	privacyObject(plan["fulfillment"])["item_commitment"] = invoice["item_commitment"]
	plan["plan_hash"] = privacyHash(plan)
	return plan
}
