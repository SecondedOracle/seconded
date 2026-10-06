package client

import (
	"fmt"
	"math/big"
)

func privacyLocalPool(input privacyMap) privacyMap {
	guide, _ := privacyTool("seconded_privacy_pool_check")
	if !privacySchema(guide.InputSchema, privacyClone(input), 0) {
		return nil
	}
	if _, ok := input["evidence_source"]; ok {
		return nil
	}
	if lo, ok := input["from_block"]; ok {
		if hi, ok := input["to_block"]; ok && privacyInt(lo) > privacyInt(hi) {
			return nil
		}
	}
	low, high := big.NewInt(0), new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 120), big.NewInt(1))
	for _, k := range []string{"amount_min_wei", "amount_max_wei"} {
		if v, ok := input[k]; ok {
			if !privacyAmount(v, 120, true) {
				return nil
			}
			n, _ := new(big.Int).SetString(privacyString(v), 10)
			if k == "amount_min_wei" {
				low = n
			} else {
				high = n
			}
		}
	}
	if low.Cmp(high) > 0 {
		return nil
	}
	chain := privacyInt(input["chain_id"])
	registry := privacyDocument("tacit-pools")
	profile := privacyObject(privacyObject(registry["chains"])[fmt.Sprint(chain)])
	review := privacyMap{}
	for _, k := range []string{"reviewed_at", "human_audit", "ceremony", "verifier_license", "pool_immutable", "pool_pause"} {
		review[k] = registry[k]
	}
	r := privacyMap{"version": "privacy-pool-check/v1", "kind": "unsigned_report", "signed": false, "chain_id": input["chain_id"], "status": "unknown", "warnings": privacyText("pool_warnings"), "security_review": review, "source": nil, "code_state": nil, "asset_state": nil, "activity": nil,
		"scan":     privacyMap{"requested_from": input["from_block"], "requested_to": input["to_block"], "scanned_ranges": []any{}, "complete": false, "lifetime_coverage": false},
		"unknowns": []any{"unspent_note_eligibility", "independent_origins", "relay_liveness", "ASP_root_or_policy", "current_exit_success", "direct_box_payments"},
		"privacy":  privacyMap{"input_retention": "none in this library; caller and RPC retention unknown", "rpc_disclosure": "chain, public pool/router addresses and block ranges; no keys/notes", "transport": "injected paired-reader HTTPS transport; provider sees requester IP"}}
	if profile == nil {
		r["status"] = "unsupported"
		r["reason"] = "chain_unsupported"
		if chain == 5042 || chain == 5042002 {
			r["reason"] = "arc_unsupported"
		}
		return r
	}
	r["contracts"] = privacyMap{"pool": profile["pool"], "router": profile["router"], "verifier": profile["verifier"]}
	r["reason"] = "reader_unavailable"
	return r
}
