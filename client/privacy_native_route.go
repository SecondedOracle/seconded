package client

import (
	"fmt"
	"strings"
)

func privacyRouteRequest(input privacyMap) privacyMap {
	r := privacyObject(privacyClone(input))
	for _, k := range []string{"from_address", "to_address", "asset", "amount", "route_id"} {
		if _, ok := r[k]; !ok {
			r[k] = nil
		}
	}
	if _, ok := r["threat_models"]; !ok {
		r["threat_models"] = []any{"chain_observer", "rpc_provider", "solver_facilitator", "counterparty", "issuer"}
	}
	r["evidence"] = privacyMap{}
	for _, collection := range []string{"steps", "funding_graph"} {
		rows := privacyArray(r[collection])
		if rows == nil {
			rows = []any{}
		}
		for _, v := range rows {
			row := privacyObject(v)
			if row["kind"] == nil {
				row["kind"] = "transfer"
			}
			fields := []string{"amount", "asset", "observed_at", "timestamp"}
			if collection == "steps" {
				fields = append(fields, "from_address", "to_address", "contract")
			}
			for _, k := range fields {
				if _, ok := row[k]; !ok {
					row[k] = nil
				}
			}
			for _, k := range []string{"observed_at", "timestamp"} {
				if row[k] != nil && privacyInt(row[k]) > 100000000000 {
					if k == "timestamp" {
						return nil
					}
					row[k] = nil
				}
			}
		}
		r[collection] = rows
	}
	return r
}
func privacyContract(chain int64, address string) privacyMap {
	norm := strings.ToLower(address)
	r := privacyMap{"address": norm, "is_pinned": false, "is_paused": nil, "code_matches_pin": nil, "status": "unknown", "provenance": "caller_asserted", "observed_at": nil, "reason": "missing_invalid_or_stale_evidence", "verified_on_chain": false}
	pins := privacyDocument(fmt.Sprintf("pins-%d", chain))
	for _, v := range privacyArray(pins["contracts"]) {
		p := privacyObject(v)
		if strings.EqualFold(privacyString(p["address"]), norm) && p["status"] == "pinned" {
			r["is_pinned"] = true
			r["pin_label"] = p["label"]
			r["pin_kind"] = p["kind"]
			r["pinned_code_sha256"] = privacyObject(p["code"])["sha256"]
		}
	}
	profile := privacyObject(privacyText("chain_profiles")[fmt.Sprint(chain)])
	if profile["tacit_pool"] == norm {
		r["pool_check"] = privacyLocalPool(privacyMap{"chain_id": chain})
	}
	return r
}
func (n *privacyNative) route(input privacyMap) privacyMap {
	r := privacyRouteRequest(input)
	if r == nil {
		return nil
	}
	chain := privacyInt(r["chain_id"])
	dimensions := privacyMap{}
	links := []any{}
	warnings := []any{}
	contracts := privacyMap{}
	summary := privacyMap{"direct_funding_detected": false, "consolidation_detected": false, "gas_topups_detected": false, "address_reuse_detected": false, "public_settlement_detected": false, "amount_timing_correlation_detected": false, "links": links}
	report := privacyMap{"schema": "seconded-privacy-route-check/v1", "route_id": "route-" + privacyHash(r), "chain_id": r["chain_id"], "status": "stale_evidence", "overall_verdict": "unknown", "dimensions": dimensions, "linkage_analysis": summary, "chain_facts": nil, "contract_evidence": contracts, "warnings": warnings}
	bind := func() privacyMap {
		report["plan_hash"] = privacyHash(privacyMap{"request": r, "report": report})
		return report
	}
	if chain != 8453 && chain != 4663 && chain != 5042 {
		report["status"] = "unsupported_chain"
		report["chain_facts"] = privacyMap{"chain_id": r["chain_id"], "status": "unsupported_chain", "singleton": true}
		report["warnings"] = []any{fmt.Sprintf("Chain %d is not supported; privacy properties are unknown.", chain)}
		for _, d := range []string{"address_privacy", "amount_privacy", "balance_privacy", "timing_privacy", "intent_privacy", "metadata_privacy"} {
			dimensions[d] = privacyMap{"status": "unknown", "findings": []any{"Unsupported chain singleton: privacy rails unavailable."}, "reason": "unsupported_chain"}
		}
		report["sanctions_screen"] = privacyMap{"status": "not_checked", "is_sanctioned": nil, "matches": []any{}, "list_date": nil, "list_version": nil, "scope": "Local OFAC SDN digital-currency snapshot; no oracle query", "disclaimer": "Absence of listing is neither legal clearance nor a safety guarantee.", "reason": "unsupported_chain"}
		return bind()
	}
	profile := privacyObject(privacyText("chain_profiles")[fmt.Sprint(chain)])
	report["chain_facts"] = profile
	if chain == 5042 {
		warnings = append(warnings, "Arc native privacy (APS) is unavailable and on roadmap only. All account balances and USDC gas transfers are public.")
	}
	if chain == 8453 {
		profile["inco_provenance"] = "caller_asserted"
		profile["inco_snapshot"] = privacyMap{"paused": true, "as_of_block": 52062663, "source": "recorded_design_review"}
	}
	add := func(kind, confidence string, source, target any, description string) {
		links = append(links, privacyMap{"link_type": kind, "observer": "chain_observer", "confidence": confidence, "source": source, "target": target, "description": description})
	}
	type edge struct{ source, target, kind string }
	edges := []edge{}
	for _, v := range privacyArray(r["funding_graph"]) {
		row := privacyObject(v)
		edges = append(edges, edge{strings.ToLower(privacyString(row["source"])), strings.ToLower(privacyString(row["target"])), privacyString(row["kind"])})
	}
	for _, v := range privacyArray(r["steps"]) {
		row := privacyObject(v)
		if row["from_address"] != nil && row["to_address"] != nil {
			edges = append(edges, edge{strings.ToLower(privacyString(row["from_address"])), strings.ToLower(privacyString(row["to_address"])), privacyString(row["kind"])})
		}
	}
	from, to := strings.ToLower(privacyString(r["from_address"])), strings.ToLower(privacyString(r["to_address"]))
	incoming := map[string]map[string]bool{}
	targets := []string{}
	occurrences := map[string]int{}
	addresses := []string{}
	for _, e := range edges {
		if incoming[e.target] == nil {
			incoming[e.target] = map[string]bool{}
			targets = append(targets, e.target)
		}
		incoming[e.target][e.source] = true
		for _, a := range []string{e.source, e.target} {
			if occurrences[a] == 0 {
				addresses = append(addresses, a)
			}
			occurrences[a]++
		}
		if oneOf(e.kind, "funding", "direct_funding") || (from != "" && e.source == from) {
			add("direct_funding", "observed", e.source, e.target, fmt.Sprintf("Direct funding link observed from %s... to %s...", e.source[:10], e.target[:10]))
		}
		if oneOf(e.kind, "gas_topup", "gas_funding") {
			add("gas_topup", "observed", e.source, e.target, fmt.Sprintf("Gas top-up observed from %s... to ephemeral %s...", e.source[:10], e.target[:10]))
		}
	}
	for _, target := range targets {
		if count := len(incoming[target]); count >= 2 {
			add("consolidation", "observed", nil, target, fmt.Sprintf("Consolidation sweep detected: %d distinct sources funnel into target %s...", count, target[:10]))
		}
	}
	for _, address := range addresses {
		count := occurrences[address]
		if count >= 3 || (count >= 2 && address != from && address != to) {
			add("address_reuse", "observed", address, nil, fmt.Sprintf("Address reuse detected: %s... participates in %d observed edges/steps.", address[:10], count))
		}
	}
	public := false
	amounts := []string{}
	times := []int64{}
	for _, v := range privacyArray(r["steps"]) {
		row := privacyObject(v)
		kind := privacyString(row["kind"])
		if portfolioContains(privacyDocument("text")["public_settlement"], strings.ToLower(privacyString(row["contract"]))) || oneOf(kind, "swap", "public_dex", "public_settlement", "bridge") {
			public = true
			add("public_settlement", "observed", row["from_address"], row["to_address"], fmt.Sprintf("Public settlement on step '%s': settlement contract/DEX emits public on-chain transfer events linking sender, receiver, and amounts.", kind))
		}
		if row["amount"] != nil {
			amounts = append(amounts, privacyString(row["amount"]))
		}
		if row["timestamp"] != nil {
			times = append(times, privacyInt(row["timestamp"]))
		}
		if row["contract"] != nil {
			contracts[privacyString(row["contract"])] = privacyContract(chain, privacyString(row["contract"]))
		}
	}
	same := len(amounts) >= 2
	for _, a := range amounts {
		if a != amounts[0] {
			same = false
		}
	}
	if same {
		add("amount_correlation", "inferred", nil, nil, fmt.Sprintf("Exact amount correlation: multiple route steps operate on identical amount '%s', allowing observer clustering.", amounts[0]))
	}
	timing := ""
	if len(times) >= 2 {
		lo, hi := times[0], times[0]
		for _, v := range times {
			lo = min(lo, v)
			hi = max(hi, v)
		}
		if hi-lo < 3600 {
			timing = fmt.Sprintf("Timing correlation: intermediate hops execute within %ds delta.", hi-lo)
			add("timing_correlation", "inferred", nil, nil, timing)
		}
	}
	dimension := func(key string, findings ...string) {
		dimensions[key] = privacyMap{"status": "unknown", "findings": findings, "reason": "missing_invalid_or_stale_evidence"}
	}
	dimension("address_privacy", "Evidence is stale; cannot verify address unlinkability.")
	dimension("amount_privacy", "Evidence is stale; cannot verify amount confidentiality.")
	dimension("balance_privacy", "Standard account balances are public; confidential balance state is unverified.")
	if timing != "" {
		dimension("timing_privacy", timing)
	} else if len(times) == 0 {
		dimension("timing_privacy", "No step timestamps provided; timing correlation unmeasured.")
	} else {
		dimension("timing_privacy", "Hops are spread over time, but timing correlation cannot be refuted without global traffic analysis.")
	}
	if public {
		dimension("intent_privacy", "Public mempool and DEX settlement expose order intent to MEV searchers and solvers.")
	} else {
		dimension("intent_privacy", "Transaction intent before inclusion depends on RPC mempool policy and solver relay visibility.")
	}
	dimension("metadata_privacy", "RPC providers observe IP-to-address correlation unless anonymized via Tor/relay.", "Relayers and facilitators observe submission timestamps and client HTTP metadata.")
	mapping := map[string]string{"direct_funding": "direct_funding_detected", "consolidation": "consolidation_detected", "gas_topup": "gas_topups_detected", "address_reuse": "address_reuse_detected", "public_settlement": "public_settlement_detected", "amount_correlation": "amount_timing_correlation_detected", "timing_correlation": "amount_timing_correlation_detected"}
	for _, v := range links {
		summary[mapping[privacyString(privacyObject(v)["link_type"])]] = true
	}
	summary["links"] = links
	summary["provenance"] = "caller_asserted"
	summary["freshness"] = "unknown"
	summary["execution_boundary"] = "agent_local_only"
	parties := []string{}
	for _, k := range []string{"from_address", "to_address"} {
		if r[k] != nil {
			parties = append(parties, privacyString(r[k]))
		}
	}
	for _, collection := range []string{"steps", "funding_graph"} {
		for _, v := range privacyArray(r[collection]) {
			row := privacyObject(v)
			for _, k := range []string{"from_address", "to_address", "contract", "source", "target"} {
				if row[k] != nil {
					parties = append(parties, privacyString(row[k]))
				}
			}
			if privacyAddress(row["asset"], false) {
				parties = append(parties, privacyString(row["asset"]))
			}
		}
	}
	if privacyAddress(r["asset"], false) {
		parties = append(parties, privacyString(r["asset"]))
	}
	screen := privacyRouteScreen(parties, nil, n.now())
	report["sanctions_screen"] = screen
	if screen["is_sanctioned"] == true {
		warnings = append(warnings, fmt.Sprintf("SANCTIONED COUNTERPARTY DETECTED: %d match(es) in OFAC SDN list.", len(privacyArray(screen["matches"]))))
	}
	report["warnings"] = warnings
	return bind()
}
