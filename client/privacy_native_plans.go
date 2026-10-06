package client

import (
	"encoding/hex"
	"io"
	"strings"
	"time"

	secp "github.com/decred/dcrd/dcrec/secp256k1/v4"
)

const privacyAnnouncer = "0x55649E01B5Df198D18D95b5cc5051630cfD45564"
const privacyRegistry = "0x6538E6bf4B0eBd30A8Ea093027Ac2422ce5d6538"

func privacyChecksum(address string) string {
	lower := strings.ToLower(address[2:])
	digest := hex.EncodeToString(keccak([]byte(lower)))
	out := []byte(lower)
	for i, c := range out {
		if c >= 'a' && c <= 'f' && digest[i] >= '8' {
			out[i] = c - 32
		}
	}
	return "0x" + string(out)
}
func privacyReceiveAddress(v any) bool {
	if !privacyAddress(v, true) {
		return false
	}
	s := privacyString(v)
	body := s[2:]
	return body == strings.ToLower(body) || body == strings.ToUpper(body) || s == privacyChecksum(s)
}
func privacyDeployment(now time.Time) privacyMap {
	d := privacyDocument("stealth-base")
	stamp, e := time.Parse(time.RFC3339, privacyString(d["verified_at"]))
	if e != nil || privacyInt(d["chain_id"]) != 8453 || privacyInt(d["scheme_id"]) != 1 || privacyObject(d["announcer"])["address"] != privacyAnnouncer || privacyObject(d["registry"])["address"] != privacyRegistry || d["verification"] != "primary_source_deployment_table" || len(privacyArray(privacyObject(d["announcer"])["sources"])) == 0 || len(privacyArray(privacyObject(d["registry"])["sources"])) == 0 || now.Before(stamp) || now.Sub(stamp) > 30*24*time.Hour {
		return nil
	}
	return d
}
func (n *privacyNative) receiveKeys() bool {
	if n.spend != nil {
		return true
	}
	spend, e := secp.GeneratePrivateKeyFromRand(n.random)
	if e != nil {
		return false
	}
	view, e := secp.GeneratePrivateKeyFromRand(n.random)
	if e != nil {
		spend.Zero()
		return false
	}
	n.spend, n.view = spend, view
	return true
}
func (n *privacyNative) metaAddress() string {
	return "st:base:0x" + hex.EncodeToString(n.spend.PubKey().SerializeCompressed()) + hex.EncodeToString(n.view.PubKey().SerializeCompressed())
}
func (n *privacyNative) deriveReceiver() (string, string, string) {
	ephemeral, e := secp.GeneratePrivateKeyFromRand(n.random)
	if e != nil {
		return "", "", ""
	}
	defer ephemeral.Zero()
	var shared secp.ModNScalar
	shared.Mul2(&ephemeral.Key, &n.view.Key)
	defer shared.Zero()
	sharedKey := secp.NewPrivateKey(&shared)
	defer sharedKey.Zero()
	digest := keccak(sharedKey.PubKey().SerializeCompressed())
	defer clear(digest)
	address := n.receiverAddress(digest)
	if address == "" {
		return "", "", ""
	}
	return address, "0x" + hex.EncodeToString(ephemeral.PubKey().SerializeCompressed()), "0x" + hex.EncodeToString(digest[:1])
}

// Separating the tweak addition permits deterministic controls for the two
// degenerate scalars without weakening production entropy or hashing.
func (n *privacyNative) receiverAddress(digest []byte) string {
	if len(digest) != 32 || n.spend == nil {
		return ""
	}
	var derived secp.ModNScalar
	defer derived.Zero()
	derived.SetByteSlice(digest)
	if derived.IsZero() {
		return ""
	}
	derived.Add(&n.spend.Key)
	if derived.IsZero() {
		return ""
	}
	receiver := secp.NewPrivateKey(&derived)
	defer receiver.Zero()
	return privacyChecksum("0x" + hex.EncodeToString(keccak(receiver.PubKey().SerializeUncompressed()[1:])[12:]))
}

func (n *privacyNative) receive(input privacyMap) privacyMap {
	if n.receiveCalls >= 128 {
		return privacyMap{"status": "unknown", "reason": "local_session_capacity"}
	}
	r := privacyObject(privacyClone(input))
	for _, field := range []string{"registrant", "token", "gas_funder", "sweep_destination"} {
		if v, ok := r[field]; ok {
			if field == "token" && v == "native" {
				continue
			}
			if !privacyReceiveAddress(v) {
				return nil
			}
			r[field] = privacyChecksum(privacyString(v))
		}
	}
	for _, field := range []string{"linked_addresses", "used_addresses"} {
		a := []any{}
		for _, v := range privacyArray(r[field]) {
			if !privacyReceiveAddress(v) {
				return nil
			}
			a = append(a, privacyChecksum(privacyString(v)))
		}
		r[field] = a
	}
	if meta, ok := r["meta_address"]; ok {
		raw, e := hex.DecodeString(privacyString(meta)[10:])
		if e != nil || len(raw) != 66 {
			return nil
		}
		if _, e = secp.ParsePubKey(raw[:33]); e != nil {
			return nil
		}
		if _, e = secp.ParsePubKey(raw[33:]); e != nil {
			return nil
		}
	}
	now := n.now()
	d := privacyDeployment(now)
	if d == nil {
		n.receiveCalls++
		return privacyMap{"status": "unsupported", "reason": "deployment_evidence_unavailable_or_stale"}
	}
	if !n.receiveKeys() {
		return nil
	}
	meta := n.metaAddress()
	if v, ok := r["meta_address"]; ok && !strings.EqualFold(privacyString(v), meta) {
		return nil
	}
	if n.invoices[privacyString(r["invoice_id"])] {
		return nil
	}
	address, ephemeral, tag := n.deriveReceiver()
	if address == "" || n.used[address] || portfolioContains(r["used_addresses"], address) {
		return nil
	}
	linked := func(v any) bool {
		return v != nil && (v == r["registrant"] || portfolioContains(r["linked_addresses"], v))
	}
	plan := privacyText("receive")
	plan["invoice_id"] = r["invoice_id"]
	plan["token"] = r["token"]
	plan["meta_address"] = meta
	plan["stealth_address"] = address
	plan["expires_at"] = now.Unix() + 300
	ann := privacyObject(plan["announcement"])
	ann["stealth_address"] = address
	ann["ephemeral_public_key"] = ephemeral
	ann["metadata"] = tag
	scan := privacyObject(plan["scan"])
	scan["from_block"] = r["scan_start"]
	scan["topics"] = []any{"0x" + hex.EncodeToString(keccak([]byte("Announcement(uint256,address,address,bytes,bytes)"))), "0x" + strings.Repeat("0", 63) + "1"}
	links := privacyObject(plan["links"])
	links["gas_funding"] = "unknown"
	if linked(r["gas_funder"]) {
		links["gas_funding"] = "detected"
	}
	if v, ok := r["sweep_destination"]; ok {
		links["sweep_destination_linked"] = "unknown"
		if linked(v) {
			links["sweep_destination_linked"] = "detected"
		}
	}
	evidence := privacyObject(plan["evidence"])
	evidence["verified_at"] = d["verified_at"]
	evidence["basis"] = d["verification"]
	h := privacyHash(plan)
	plan["plan_hash"] = h
	raw, e := privacyCanonical(plan)
	if e != nil {
		return nil
	}
	// Prune plans, but keep invoice/address reservations for this bounded session.
	for hash, p := range n.receivePlans {
		if !now.Before(p.expires) {
			delete(n.receivePlans, hash)
		}
	}
	n.receivePlans[h] = privacySavedPlan{raw: raw, issued: now, expires: now.Add(300 * time.Second)}
	n.used[address] = true
	n.invoices[privacyString(r["invoice_id"])] = true
	n.receiveCalls++
	return plan
}
func privacySwapRequest(input privacyMap) privacyMap {
	guide, _ := privacyTool("seconded_private_swap_quote")
	if !privacySchema(guide.InputSchema, input, 0) {
		return nil
	}
	r := privacyObject(privacyClone(input))
	if !privacyAsset(r["asset_in"]) || !privacyAsset(r["asset_out"]) || strings.EqualFold(privacyString(r["asset_in"]), privacyString(r["asset_out"])) || !privacyAddress(r["recipient"], true) || !privacyAddress(r["refund_address"], true) || !privacyAmount(r["amount"], 120, false) || len(privacyString(r["amount"])) > 37 {
		return nil
	}
	for _, k := range []string{"asset_in", "asset_out", "recipient", "refund_address"} {
		r[k] = strings.ToLower(privacyString(r[k]))
	}
	return r
}
func privacySwapRoute(route string, r privacyMap) privacyMap {
	v := privacyArray(privacyText("swap_visibility")[route])
	deferred := oneOf(route, "custodial", "bridge", "rfq")
	status, reason, risk := "unknown", "no_verified_venue_quote", "unverified_contract_control"
	if deferred {
		status, reason, risk = "needs_review", "custodial_bridge_rfq_deferred", "operator_or_bridge_may_hold_funds"
	}
	dimension := any("unknown")
	switch r["privacy_goal"] {
	case "hide_intent":
		dimension = v[0]
	case "hide_amount":
		dimension = v[1]
	case "hide_sender":
		dimension = v[3]
	}
	assessment := "unknown"
	if dimension != "unknown" {
		assessment = "exposed"
	}
	bridge := "unknown"
	if route == "bridge" {
		bridge = "needs_review"
	}
	return privacyMap{
		"route": route, "status": status, "adapter_enabled": false, "reason": reason,
		"evidence":        privacyMap{"kind": "recorded_design_observation", "as_of": "2026-10-03", "source": "recorded_design_review", "current_state": "unknown", "deployment_verified": false},
		"visibility":      privacyMap{"public_intent": v[0], "public_settlement": v[1], "solver_rfq_visibility": v[2], "sender_linkage": v[3], "balance_privacy": "unknown", "timing_privacy": "unknown", "metadata_privacy": "unknown"},
		"goal_assessment": assessment,
		"quote":           privacyMap{"status": "unknown", "amount_out": nil, "minimum_out": nil, "fee": nil, "expires_at": nil, "binding": false},
		"spender":         privacyMap{"address": nil, "status": "unknown", "approval_required": "unknown"},
		"custody":         privacyMap{"seconded": "none", "venue": "unknown", "risk": risk},
		"refund":          privacyMap{"requested_address": r["refund_address"], "status": "unknown", "terms": "unknown", "controller": "unknown", "guaranteed": false},
		"risks":           privacyMap{"issuer_freeze": "not_removed_by_swap_or_wrapper", "bridge": bridge, "kyc": "unknown", "operator_hold": "unknown", "sanctions": "unknown"},
		"signing":         privacyMap{"status": "deferred", "typed_data": nil, "reason": "venue_domain_spender_limits_and_refund_not_verified", "binding_quote_requires_explicit_consent": true},
	}
}
func (n *privacyNative) swap(input privacyMap) privacyMap {
	r := privacySwapRequest(input)
	if r == nil {
		return nil
	}
	now := n.now()
	for hash, p := range n.swapPlans {
		if now.Before(p.issued) || !now.Before(p.expires) {
			delete(n.swapPlans, hash)
		}
	}
	if len(n.swapPlans) >= 128 {
		return nil
	}
	id := make([]byte, 16)
	if _, e := io.ReadFull(n.random, id); e != nil {
		return nil
	}
	id[6] = (id[6] & 15) | 64
	id[8] = (id[8] & 63) | 128
	plan := privacyText("swap")
	plan["plan_id"] = hex.EncodeToString(id)
	plan["request"] = r
	routes := []any{}
	for _, v := range privacyArray(r["routes"]) {
		routes = append(routes, privacySwapRoute(privacyString(v), r))
	}
	plan["routes"] = routes
	h := privacyHash(plan)
	plan["plan_hash"] = h
	raw, e := privacyCanonical(plan)
	if e != nil {
		return nil
	}
	request, _ := privacyCanonical(r)
	n.swapPlans[h] = privacySavedPlan{raw: raw, request: request, issued: now, expires: now.Add(300 * time.Second)}
	return plan
}
