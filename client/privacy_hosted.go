package client

import "encoding/json"

// Each node reconstructs only declared fields. Objects are closed recursively;
// values, collections and the wire body are bounded before reaching the model.
type privacyResponseNode struct {
	fields   map[string]*privacyResponseNode
	optional map[string]bool
	item     *privacyResponseNode
	kind     string
	nullable bool
	limit    int
}

func (s *privacyResponseNode) reconstruct(value any) (any, bool) {
	if value == nil {
		return nil, s.nullable
	}
	switch s.kind {
	case "object":
		input, ok := value.(map[string]any)
		if !ok {
			return nil, false
		}
		for key := range s.fields {
			if _, present := input[key]; !present && !s.optional[key] {
				return nil, false
			}
		}
		output := privacyMap{}
		for key, value := range input {
			child, ok := s.fields[key]
			if !ok {
				return nil, false
			}
			clean, ok := child.reconstruct(value)
			if !ok {
				return nil, false
			}
			output[key] = clean
		}
		return output, true
	case "array":
		input, ok := value.([]any)
		if !ok || len(input) > s.limit {
			return nil, false
		}
		output := make([]any, 0, len(input))
		for _, value := range input {
			clean, ok := s.item.reconstruct(value)
			if !ok {
				return nil, false
			}
			output = append(output, clean)
		}
		return output, true
	case "string":
		v, ok := value.(string)
		return v, ok && len(v) <= s.limit
	case "integer":
		v, ok := value.(json.Number)
		if !ok {
			return nil, false
		}
		n, e := v.Int64()
		return v, e == nil && n >= 0 && n <= 9007199254740991
	case "boolean":
		v, ok := value.(bool)
		return v, ok
	}
	return nil, false
}

var privacyHostedShape = func() *privacyResponseNode {
	text := &privacyResponseNode{kind: "string", limit: 512}
	number := &privacyResponseNode{kind: "integer"}
	boolean := &privacyResponseNode{kind: "boolean"}
	nullable := func(s *privacyResponseNode) *privacyResponseNode { c := *s; c.nullable = true; return &c }
	object := func(fields map[string]*privacyResponseNode) *privacyResponseNode {
		return &privacyResponseNode{kind: "object", fields: fields}
	}
	array := func(item *privacyResponseNode, limit int) *privacyResponseNode {
		return &privacyResponseNode{kind: "array", item: item, limit: limit}
	}
	group := func(s *privacyResponseNode, keys ...string) *privacyResponseNode {
		m := map[string]*privacyResponseNode{}
		for _, k := range keys {
			m[k] = s
		}
		return object(m)
	}
	distribution := object(map[string]*privacyResponseNode{
		"amounts_wei":       array(object(map[string]*privacyResponseNode{"amount": text, "count": number}), 16),
		"other_event_count": number, "distinct_amounts": number, "largest_amount_group_count": number,
	})
	code := object(map[string]*privacyResponseNode{"present": boolean, "sha256": nullable(text), "matches_reviewed_runtime": boolean})
	activity := object(map[string]*privacyResponseNode{
		"deposit_count": number, "withdrawal_count": number, "internal_transfer_count": number,
		"deposit_distribution": distribution, "withdrawal_distribution": distribution,
		"amount_bucket": object(map[string]*privacyResponseNode{"min_wei": text, "max_wei": text, "deposit_count": number, "withdrawal_count": number, "basis": text}),
		"anonymity_set_bounds": object(map[string]*privacyResponseNode{
			"eligible_notes_at_pin": group(number, "lower", "upper"), "eligible_notes_from_scanned_outputs": group(number, "lower", "upper"),
			"basis": text, "independent_users": text, "amount_eligible_notes": text, "small_set": text,
		}),
		"timing":        object(map[string]*privacyResponseNode{"first_event_timestamp": nullable(number), "last_event_timestamp": nullable(number), "minimum_gap_seconds": nullable(number), "maximum_same_block_events": number}),
		"concentration": object(map[string]*privacyResponseNode{"distinct_deposit_amounts": number, "distinct_withdrawal_recipients": number, "deposit_sender_concentration": text, "withdrawals_with_prior_exact_amount_candidate": number}),
		"receive_boxes": object(map[string]*privacyResponseNode{"funding_events": number, "sweep_events": number, "boxes_with_repeated_funding": number, "boxes_with_repeated_sweeps": number, "potential_post_sweep_payments_same_tx": number, "coverage": text}),
	})
	root := object(map[string]*privacyResponseNode{
		"version": text, "kind": text, "signed": boolean, "chain_id": number, "status": text, "reason": text, "reads_used": number,
		"contracts":       group(text, "pool", "router", "verifier"),
		"warnings":        group(text, "exact_amount_linkage", "gpl_verifier", "immutable_no_pause", "no_human_audit", "post_sweep_destruction", "public_boundaries", "receive_box_reuse", "relay_dependency", "small_set", "unvalidated_ceremony"),
		"security_review": object(map[string]*privacyResponseNode{"reviewed_at": text, "human_audit": text, "ceremony": text, "verifier_license": text, "pool_immutable": boolean, "pool_pause": boolean}),
		"source":          nullable(object(map[string]*privacyResponseNode{"chain_id": number, "block_number": number, "block_hash": text, "timestamp": number})),
		"code_state":      nullable(group(code, "pool", "router", "verifier")),
		"asset_state":     nullable(object(map[string]*privacyResponseNode{"expected": text, "pool": text, "router": text, "symbol": text, "decimals": number})),
		"activity":        nullable(activity),
		"scan":            object(map[string]*privacyResponseNode{"requested_from": nullable(number), "requested_to": nullable(number), "scanned_ranges": array(group(number, "from_block", "to_block"), 4), "complete": boolean, "lifetime_coverage": boolean, "completeness_basis": text}),
		"unknowns":        array(text, 16), "privacy": group(text, "input_retention", "rpc_disclosure", "transport"),
	})
	root.optional = map[string]bool{}
	for key := range root.fields {
		root.optional[key] = !oneOf(key, "version", "kind", "signed", "chain_id", "status")
	}
	root.fields["scan"].optional = map[string]bool{"completeness_basis": true}
	return root
}()

func privacyHostedResponse(raw []byte, chain json.Number) privacyMap {
	var input privacyMap
	if DecodeStrict(raw, &input, 32*1024) != nil {
		return nil
	}
	value, ok := privacyHostedShape.reconstruct(input)
	if !ok {
		return nil
	}
	result := privacyObject(value)
	if result["version"] != "privacy-pool-check/v1" || result["kind"] != "unsigned_report" || result["signed"] != false || result["chain_id"] != chain || !oneOf(privacyString(result["status"]), "observed", "unknown", "unsupported") {
		return nil
	}
	if result["status"] == "observed" && (privacyObject(result["activity"]) == nil || privacyObject(result["scan"])["complete"] != true || privacyObject(result["source"])["chain_id"] != chain) {
		return nil
	}
	result["charged"] = "no"
	result["evidence_source"] = "hosted_neutral"
	return result
}
