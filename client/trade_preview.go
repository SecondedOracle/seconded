package client

import (
	"encoding/json"
	"fmt"
	"math/big"
	"strings"
)

// TradePreview retains the sealed tables exactly, under a closed shared schema.
// The signed receipt binds the request; evidence_sha256 binds these observations
// to verification, including its chain and immutable block pin.
type TradePreview map[string]any

func (p *TradePreview) UnmarshalJSON(data []byte) error {
	var value map[string]any
	if DecodeStrict(data, &value, ResponseLimit) != nil || !validTradePreview(value) {
		return ErrInvalid
	}
	*p = value
	return nil
}

func tradeUint(value any, bits int) (*big.Int, bool) {
	s, ok := value.(string)
	if !ok {
		return nil, false
	}
	n, ok := new(big.Int).SetString(s, 10)
	return n, ok && n.Sign() >= 0 && n.BitLen() <= bits
}

func tradeHuman(n *big.Int, decimals any) any {
	if decimals == nil {
		return nil
	}
	d := int(factInt(decimals))
	sign := ""
	if n.Sign() < 0 {
		sign = "-"
	}
	s := new(big.Int).Abs(n).String()
	if d == 0 {
		return sign + s
	}
	if len(s) <= d {
		s = strings.Repeat("0", d+1-len(s)) + s
	}
	whole, fraction := s[:len(s)-d], strings.TrimRight(s[len(s)-d:], "0")
	if fraction == "" {
		return sign + whole
	}
	return sign + whole + "." + fraction
}

func validTradePreview(p map[string]any) bool {
	shape := observationShapes["trade_preview"]
	if !shape.valid(p) {
		return false
	}
	e, block := factMap(p["effects"]), factMap(p["block"])
	chain := factInt(block["chain_id"])
	if e["coverage"] == "not_checked" {
		return chain != 4663 || oneOf(e["reason"].(string), "nitro_transfer_accounting_unverified",
			"unsigned_typed_data_has_no_execution_envelope", "effects_sheet_budget", "effects_not_reported")
	}
	if chain != 8453 && chain != 5042 {
		return false
	}
	_, alias := e["native_token_alias"]
	if (chain == 5042) != alias {
		return false
	}
	parties, assets := factList(e["parties"]), factList(e["assets"])
	if factInt(e["sender"]) >= int64(len(parties)) {
		return false
	}
	spenders := map[any]bool{}
	for _, name := range []string{"recipients", "spenders"} {
		for _, index := range factList(e[name]) {
			i := factInt(index)
			if i >= int64(len(parties)) {
				return false
			}
			if name == "spenders" {
				spenders[parties[i]] = true
			}
		}
	}
	if len(factList(e["balances"]))+len(factList(e["approvals"])) > 20 {
		return false
	}
	partial := shape.Properties["effects"].AnyOf[1]
	for _, name := range []string{"assets", "balances", "approvals"} {
		columns := partial.Properties[name].Items.Items.AnyOf
		for _, value := range factList(e[name]) {
			for i, cell := range factList(value) {
				if !columns[i].valid(cell) {
					return false
				}
			}
		}
	}
	native := "ETH"
	if chain == 5042 {
		native = "USDC"
	}
	seen := map[any]bool{}
	for _, value := range assets {
		row := factList(value)
		if seen[row[0]] {
			return false
		}
		seen[row[0]] = true
		if row[0] == "native" && (row[1] != native || row[2] == nil || factInt(row[2]) != 18 || row[3] != "chain_native") {
			return false
		}
	}
	seen = map[any]bool{}
	for _, value := range factList(e["balances"]) {
		row := factList(value)
		a, party := factInt(row[0]), factInt(row[1])
		if a >= int64(len(assets)) || party >= int64(len(parties)) {
			return false
		}
		key := fmt.Sprintf("%d:%d", a, party)
		if seen[key] {
			return false
		}
		seen[key] = true
		asset := factList(assets[a])
		if chain == 5042 && asset[0] == "0x3600000000000000000000000000000000000000" {
			return false
		}
		before, ok1 := tradeUint(row[2], 256)
		after, ok2 := tradeUint(row[3], 256)
		if !ok1 || !ok2 {
			return false
		}
		delta := new(big.Int).Sub(after, before)
		if delta.Sign() == 0 || delta.String() != row[4] || tradeHuman(delta, asset[2]) != row[5] {
			return false
		}
	}
	seen = map[any]bool{}
	for _, value := range factList(e["approvals"]) {
		row := factList(value)
		a, owner := factInt(row[0]), factInt(row[1])
		if a >= int64(len(assets)) || owner >= int64(len(parties)) || !spenders[row[2]] {
			return false
		}
		key := fmt.Sprintf("%d:%d:%s:%s", a, owner, row[2], row[3])
		if seen[key] {
			return false
		}
		seen[key] = true
		asset := factList(assets[a])
		if asset[0] == "native" {
			return false
		}
		bits := 256
		if row[3] == "permit2" {
			bits = 160
		}
		_, ok1 := tradeUint(row[4], bits)
		after, ok2 := tradeUint(row[5], bits)
		if !ok1 || !ok2 || tradeHuman(after, asset[2]) != row[8] {
			return false
		}
		if row[3] == "permit2" {
			_, ok1 = tradeUint(row[6], 48)
			_, ok2 = tradeUint(row[7], 48)
			if !ok1 || !ok2 {
				return false
			}
		} else if row[6] != nil || row[7] != nil {
			return false
		}
	}
	gas := factMap(e["gas"])
	estimate, ok1 := tradeUint(gas["estimate"], 256)
	limit, ok2 := tradeUint(gas["limit"], 256)
	if !ok1 || !ok2 || estimate.Cmp(limit) > 0 || gas["native"] != native ||
		gas["limit_source"] == "simulation_default" && gas["limit"] != "10000000" {
		return false
	}
	_, excludes := gas["excludes"]
	if (chain == 8453) != excludes {
		return false
	}
	fee, feePresent := gas["execution_fee_base_units"]
	display, displayPresent := gas["execution_fee"]
	if feePresent != displayPresent {
		return false
	}
	if feePresent {
		n, ok := tradeUint(fee, 512)
		if !ok || tradeHuman(n, json.Number("18")) != display {
			return false
		}
	}
	unknowns := map[any]bool{}
	for _, value := range factList(e["unknowns"]) {
		unknowns[value] = true
	}
	return unknowns["native_fee_attribution"] && unknowns["undiscovered_allowance_keys"]
}

func validTradeAnswer(product string, a Answer) bool {
	return a.TradePreview == nil || product == "trade_check" && validTradePreview(*a.TradePreview)
}

func validTradeBinding(s ReceiptEnvelope) bool {
	if s.Answer == nil || s.Answer.TradePreview == nil {
		return true
	}
	if !validTradeAnswer(s.Product, *s.Answer) || s.Verification == nil || s.Verification.Block == nil {
		return false
	}
	p := *s.Answer.TradePreview
	b, v := factMap(p["block"]), s.Verification.Block
	return p["evidence_sha256"] == s.Verification.EvidenceSHA256 && factInt(b["chain_id"]) == v.ChainID &&
		factInt(b["number"]) == v.Number && b["hash"] == v.Hash
}

func tradeFactsText(a *Answer) []string {
	if a == nil || a.TradePreview == nil || !validTradePreview(*a.TradePreview) {
		return nil
	}
	p := *a.TradePreview
	b, e := factMap(p["block"]), factMap(p["effects"])
	facts := []string{fmt.Sprintf("Conditional Trade preview on chain %d at block %d (%s).", factInt(b["chain_id"]), factInt(b["number"]), b["hash"])}
	if e["coverage"] == "not_checked" {
		return append(facts, "Asset changes not checked: "+e["reason"].(string)+".")
	}
	facts = append(facts, "Partial coverage: bounded observers do not cover all assets or allowances.")
	gas := factMap(e["gas"])
	facts = append(facts, fmt.Sprintf("Execution gas estimate: %s; simulation limit: %s (%s).", gas["estimate"], gas["limit"], gas["limit_source"]))
	if fee, ok := gas["execution_fee"]; ok {
		facts = append(facts, fmt.Sprintf("Conditional execution fee: %s %s.", fee, gas["native"]))
	} else {
		facts = append(facts, "Execution fee not estimated: supplied fee inputs did not establish a price.")
	}
	if _, ok := gas["excludes"]; ok {
		facts = append(facts, "Execution fee excludes L1 data and operator fees.")
	}
	for _, value := range factList(e["balances"]) {
		row := factList(value)
		if row[1] != e["sender"] {
			continue
		}
		asset := factList(factList(e["assets"])[factInt(row[0])])
		facts = append(facts, fmt.Sprintf("Sender balance delta for %s: %s base units.", asset[0], row[4]))
	}
	facts = append(facts, "Native fee attribution and undiscovered allowance keys remain unchecked; inspect the signed recipients, spenders and approval tables.")
	return facts
}
