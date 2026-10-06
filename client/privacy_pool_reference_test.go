package client

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/big"
	"sort"
	"strings"
	"time"
)

// The scan core accepts trusted paired public observations only. It has no URL,
// socket, default reader or model-controlled injection. The shipped MCP uses
// privacyLocalPool; the existing explicit hosted-neutral path owns networking.
type privacyPoolObservation struct {
	source  privacyMap
	state   []any
	logs    []any
	headers map[int64]privacyMap
	final   privacyMap
	reads   int
}
type privacyPoolEvent struct {
	kind                     string
	block, index, txIndex    int64
	hash, tx, box, recipient string
	amount                   *big.Int
	leaves                   int
}

func privacyHex(v any, size int) ([]byte, bool) {
	s := privacyString(v)
	if !strings.HasPrefix(s, "0x") || len(s)%2 != 0 {
		return nil, false
	}
	b, e := hex.DecodeString(s[2:])
	return b, e == nil && (size < 0 || len(b) == size)
}
func privacyQuantity(v any) (int64, bool) {
	if !privacyMatch(`0x(?:0|[1-9a-fA-F][0-9a-fA-F]*)`, v) {
		return 0, false
	}
	n, ok := new(big.Int).SetString(privacyString(v)[2:], 16)
	return n.Int64(), ok && n.IsInt64() && n.Sign() >= 0
}
func privacyABIAddress(v any) (string, bool) {
	b, ok := privacyHex(v, 32)
	if !ok {
		return "", false
	}
	for _, v := range b[:12] {
		if v != 0 {
			return "", false
		}
	}
	return "0x" + hex.EncodeToString(b[12:]), true
}
func privacyPoolTopic(signature string) string {
	return "0x" + hex.EncodeToString(keccak([]byte(signature)))
}
func privacyPoolEvents(raw []any, profile privacyMap, start, end int64) ([]privacyPoolEvent, bool) {
	if len(raw) > 256 {
		return nil, false
	}
	out := []privacyPoolEvent{}
	seen := map[string]bool{}
	trans := privacyPoolTopic("Transact(bytes32,bytes32,bytes32,bytes32,uint256,bytes32,address,int256,address,uint256,bytes,bytes)")
	funded := privacyPoolTopic("ReceiveFunded(address,uint256,uint16,uint256)")
	received := privacyPoolTopic("Received(address,uint256,uint256,uint256,uint256,uint256)")
	for _, v := range raw {
		m := privacyObject(v)
		block, ok := privacyQuantity(m["blockNumber"])
		index, ok2 := privacyQuantity(m["logIndex"])
		txIndex, ok3 := privacyQuantity(m["transactionIndex"])
		hash, ok4 := privacyHex(m["blockHash"], 32)
		tx, ok5 := privacyHex(m["transactionHash"], 32)
		address, ok6 := privacyHex(m["address"], 20)
		topics := privacyArray(m["topics"])
		data, ok7 := privacyHex(m["data"], -1)
		key := fmt.Sprintf("%d/%d", block, index)
		if !ok || !ok2 || !ok3 || !ok4 || !ok5 || !ok6 || !ok7 || m["removed"] != false || block < start || block > end || seen[key] || len(topics) == 0 || len(data) > 16384 || len(data)%32 != 0 {
			return nil, false
		}
		seen[key] = true
		for _, topic := range topics {
			if _, ok := privacyHex(topic, 32); !ok {
				return nil, false
			}
		}
		e := privacyPoolEvent{block: block, index: index, txIndex: txIndex, hash: "0x" + hex.EncodeToString(hash), tx: "0x" + hex.EncodeToString(tx)}
		addr := "0x" + hex.EncodeToString(address)
		word := func(i int) []byte { return data[32*i : 32*(i+1)] }
		switch {
		case topics[0] == trans && addr == profile["pool"] && len(topics) == 3:
			if len(data) < 384 {
				return nil, false
			}
			e.kind = "transact"
			// ABI round-trip equivalent: canonical addresses, contiguous dynamic tails,
			// zero padding and no trailing bytes. This rejects ambiguous offsets.
			for _, i := range []int{4, 6} {
				for _, v := range word(i)[:12] {
					if v != 0 {
						return nil, false
					}
				}
			}
			offset := 320
			for _, i := range []int{8, 9} {
				o := new(big.Int).SetBytes(word(i))
				if !o.IsInt64() || o.Int64() != int64(offset) || offset+32 > len(data) {
					return nil, false
				}
				length := new(big.Int).SetBytes(data[offset : offset+32])
				if !length.IsInt64() || length.Int64() > 16384 {
					return nil, false
				}
				l := int(length.Int64())
				last := offset + 32 + ((l+31)/32)*32
				if last > len(data) {
					return nil, false
				}
				for _, v := range data[offset+32+l : last] {
					if v != 0 {
						return nil, false
					}
				}
				offset = last
			}
			if offset != len(data) {
				return nil, false
			}
			e.amount = new(big.Int).SetBytes(word(5))
			if word(5)[0]&128 != 0 {
				e.amount.Sub(e.amount, new(big.Int).Lsh(big.NewInt(1), 256))
			}
			if new(big.Int).Abs(new(big.Int).Set(e.amount)).BitLen() > 120 {
				return nil, false
			}
			e.recipient = "0x" + hex.EncodeToString(word(4)[12:])
			for _, i := range []int{0, 1} {
				if new(big.Int).SetBytes(word(i)).Sign() != 0 {
					e.leaves++
				}
			}
		case topics[0] == funded && addr == profile["router"] && len(topics) == 2:
			if len(data) != 96 || new(big.Int).SetBytes(word(1)).BitLen() > 16 {
				return nil, false
			}
			e.kind = "funded"
			e.amount = new(big.Int).SetBytes(word(2))
			e.box, ok = privacyABIAddress(topics[1])
			if !ok {
				return nil, false
			}
		case topics[0] == received && addr == profile["router"] && len(topics) == 3:
			if len(data) != 128 {
				return nil, false
			}
			e.kind = "received"
			e.amount = new(big.Int).SetBytes(word(1))
			e.box, ok = privacyABIAddress(topics[1])
			if !ok {
				return nil, false
			}
		default:
			return nil, false
		}
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].block == out[j].block {
			return out[i].index < out[j].index
		}
		return out[i].block < out[j].block
	})
	return out, true
}
func privacyPoolDistribution(rows []privacyPoolEvent) privacyMap {
	counts := map[string]int{}
	values := map[string]*big.Int{}
	for _, r := range rows {
		v := new(big.Int).Abs(new(big.Int).Set(r.amount))
		k := v.String()
		counts[k]++
		values[k] = v
	}
	keys := make([]string, 0, len(counts))
	largest := 0
	for k, n := range counts {
		keys = append(keys, k)
		largest = max(largest, n)
	}
	sort.Slice(keys, func(i, j int) bool {
		if counts[keys[i]] != counts[keys[j]] {
			return counts[keys[i]] > counts[keys[j]]
		}
		return values[keys[i]].Cmp(values[keys[j]]) < 0
	})
	amounts := []any{}
	other := 0
	for i, k := range keys {
		if i < 16 {
			amounts = append(amounts, privacyMap{"amount": k, "count": counts[k]})
		} else {
			other += counts[k]
		}
	}
	return privacyMap{"amounts_wei": amounts, "other_event_count": other, "distinct_amounts": len(counts), "largest_amount_group_count": largest}
}
func privacyPoolActivity(rows []privacyPoolEvent, headers map[int64]privacyMap, low, high *big.Int, slots int64) privacyMap {
	deposits, withdrawals := []privacyPoolEvent{}, []privacyPoolEvent{}
	funded, swept := map[string]int{}, map[string]int{}
	earlier := map[string]bool{}
	blocks := map[int64]int{}
	times := []int64{}
	hazards, leaves, internal := 0, 0, 0
	for _, r := range rows {
		key := r.tx + "/" + r.box
		switch r.kind {
		case "funded":
			funded[r.box]++
			if earlier[key] {
				hazards++
			}
		case "received":
			swept[r.box]++
			earlier[key] = true
		case "transact":
			leaves += r.leaves
			blocks[r.block]++
			times = append(times, privacyInt(headers[r.block]["timestamp"]))
			if r.amount.Sign() > 0 {
				deposits = append(deposits, r)
			} else if r.amount.Sign() < 0 {
				withdrawals = append(withdrawals, r)
			} else {
				internal++
			}
		}
	}
	depAmounts, recipients := map[string]bool{}, map[string]bool{}
	linked, depBucket, withBucket := 0, 0, 0
	bucket := func(a *big.Int) bool {
		a = new(big.Int).Abs(new(big.Int).Set(a))
		return a.Cmp(low) >= 0 && a.Cmp(high) <= 0
	}
	for _, d := range deposits {
		depAmounts[d.amount.String()] = true
		if bucket(d.amount) {
			depBucket++
		}
	}
	for _, w := range withdrawals {
		recipients[w.recipient] = true
		if bucket(w.amount) {
			withBucket++
		}
		for _, d := range deposits {
			if d.amount.Cmp(new(big.Int).Neg(w.amount)) == 0 && (d.block < w.block || (d.block == w.block && d.index < w.index)) {
				linked++
				break
			}
		}
	}
	sort.Slice(times, func(i, j int) bool { return times[i] < times[j] })
	var first, last, gap any
	if len(times) > 0 {
		first, last = times[0], times[len(times)-1]
	}
	if len(times) > 1 {
		minimum := times[1] - times[0]
		for i := 1; i < len(times); i++ {
			minimum = min(minimum, times[i]-times[i-1])
		}
		gap = minimum
	}
	maxBlock := 0
	for _, c := range blocks {
		maxBlock = max(maxBlock, c)
	}
	fundTotal, sweepTotal, fundRepeat, sweepRepeat := 0, 0, 0, 0
	for _, c := range funded {
		fundTotal += c
		if c > 1 {
			fundRepeat++
		}
	}
	for _, c := range swept {
		sweepTotal += c
		if c > 1 {
			sweepRepeat++
		}
	}
	small := "not_established"
	if slots < 20 {
		small = "observed_upper_bound_below_20"
	}
	return privacyMap{"deposit_count": len(deposits), "withdrawal_count": len(withdrawals), "internal_transfer_count": internal, "deposit_distribution": privacyPoolDistribution(deposits), "withdrawal_distribution": privacyPoolDistribution(withdrawals),
		"amount_bucket":        privacyMap{"min_wei": low.String(), "max_wei": high.String(), "deposit_count": depBucket, "withdrawal_count": withBucket, "basis": "public boundary amounts only; cannot filter hidden note eligibility"},
		"anonymity_set_bounds": privacyMap{"eligible_notes_at_pin": privacyMap{"lower": 0, "upper": slots}, "eligible_notes_from_scanned_outputs": privacyMap{"lower": 0, "upper": leaves}, "basis": "Tree slots/output commitments are ceilings including spent/dummy notes, not users or entropy.", "independent_users": "unknown", "amount_eligible_notes": "unknown", "small_set": small},
		"timing":               privacyMap{"first_event_timestamp": first, "last_event_timestamp": last, "minimum_gap_seconds": gap, "maximum_same_block_events": maxBlock},
		"concentration":        privacyMap{"distinct_deposit_amounts": len(depAmounts), "distinct_withdrawal_recipients": len(recipients), "deposit_sender_concentration": "unknown; internal callers require traces", "withdrawals_with_prior_exact_amount_candidate": linked},
		"receive_boxes":        privacyMap{"funding_events": fundTotal, "sweep_events": sweepTotal, "boxes_with_repeated_funding": fundRepeat, "boxes_with_repeated_sweeps": sweepRepeat, "potential_post_sweep_payments_same_tx": hazards, "coverage": "router events only; direct payments and actual destruction require traces"}}
}

// Paired-reader collection stays with the existing hosted service. This local
// reducer is independently testable without granting an HTTP capability.
func privacyReducePool(input privacyMap, profile privacyMap, observation privacyPoolObservation, now time.Time) privacyMap {
	report := privacyLocalPool(input)
	if report == nil || report["status"] == "unsupported" {
		return report
	}
	report["reason"] = "evidence_unavailable_or_inconsistent"
	source := observation.source
	if !privacyExact(source, "chain_id", "block_number", "block_hash", "timestamp") {
		return report
	}
	if _, ok := privacyHex(source["block_hash"], 32); !ok {
		return report
	}
	for _, key := range []string{"chain_id", "block_number", "timestamp"} {
		schema := privacyMap{"type": "integer", "minimum": 0, "maximum": int64(9007199254740991)}
		if !privacySchema(schema, privacyClone(source[key]), 0) {
			return report
		}
	}
	chain := privacyInt(input["chain_id"])
	stamp := privacyInt(source["timestamp"])
	if privacyInt(source["chain_id"]) != chain || stamp <= 0 || now.Unix() < stamp || now.Unix()-stamp > 120 {
		return report
	}
	report["source"] = privacyClone(source)
	end := privacyInt(source["block_number"])
	if v, ok := input["to_block"]; ok {
		end = privacyInt(v)
	}
	start := max(privacyInt(profile["deployment_block"]), end-511)
	if v, ok := input["from_block"]; ok {
		start = privacyInt(v)
	}
	if start < privacyInt(profile["deployment_block"]) || end > privacyInt(source["block_number"]) || end < start || end-start+1 > 2048 || len(observation.state) != 8 {
		return report
	}
	code := privacyMap{}
	for i, k := range []string{"pool", "router", "verifier"} {
		raw, ok := privacyHex(observation.state[i], -1)
		if !ok {
			return report
		}
		var digest any
		if len(raw) > 0 {
			digest = fmt.Sprintf("%x", sha256.Sum256(raw))
		}
		code[k] = privacyMap{"present": len(raw) > 0, "sha256": digest, "matches_reviewed_runtime": digest == privacyObject(privacyObject(profile["runtime"])[k])["sha256"]}
	}
	report["code_state"] = code
	addresses := []string{}
	for _, v := range observation.state[3:7] {
		a, ok := privacyABIAddress(v)
		if !ok {
			return report
		}
		addresses = append(addresses, a)
	}
	zero := "0x" + strings.Repeat("0", 40)
	report["asset_state"] = privacyMap{"expected": zero, "pool": addresses[0], "router": addresses[3], "symbol": "ETH", "decimals": 18}
	for _, v := range code {
		if privacyObject(v)["matches_reviewed_runtime"] != true {
			return report
		}
	}
	raw, ok := privacyHex(observation.state[7], 32)
	slots := new(big.Int).SetBytes(raw)
	if !ok || !slots.IsInt64() || slots.Int64() > 1<<32 || slots.Bit(0) != 0 || addresses[0] != zero || addresses[1] != profile["verifier"] || addresses[2] != profile["pool"] || addresses[3] != zero {
		return report
	}
	rows, ok := privacyPoolEvents(observation.logs, profile, start, end)
	if !ok {
		return report
	}
	blocks := map[int64]bool{}
	leafCount := 0
	for _, r := range rows {
		blocks[r.block] = true
		header := observation.headers[r.block]
		for _, key := range []string{"number", "timestamp"} {
			if !privacySchema(privacyMap{"type": "integer", "minimum": 0, "maximum": int64(9007199254740991)}, privacyClone(header[key]), 0) {
				return report
			}
		}
		if privacyInt(header["number"]) != r.block || header["hash"] != r.hash || privacyInt(header["timestamp"]) > stamp {
			return report
		}
		leafCount += r.leaves
	}
	if len(blocks) > 64 || int64(leafCount) > slots.Int64() || observation.reads <= 0 || privacyInt(observation.final["number"]) != privacyInt(source["block_number"]) || observation.final["hash"] != source["block_hash"] || privacyInt(observation.final["timestamp"]) != stamp {
		return report
	}
	scan := privacyObject(report["scan"])
	ranges := []any{}
	for first := start; first <= end; first += 512 {
		ranges = append(ranges, privacyMap{"from_block": first, "to_block": min(first+511, end)})
	}
	scan["scanned_ranges"] = ranges
	scan["complete"] = true
	scan["lifetime_coverage"] = start == privacyInt(profile["deployment_block"]) && end == privacyInt(source["block_number"])
	scan["completeness_basis"] = "two readers agree; silent shared provider omissions cannot be excluded"
	low, high := big.NewInt(0), new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 120), big.NewInt(1))
	if v, ok := input["amount_min_wei"]; ok {
		low, _ = new(big.Int).SetString(privacyString(v), 10)
	}
	if v, ok := input["amount_max_wei"]; ok {
		high, _ = new(big.Int).SetString(privacyString(v), 10)
	}
	report["activity"] = privacyPoolActivity(rows, observation.headers, low, high, slots.Int64())
	report["status"] = "observed"
	report["reads_used"] = observation.reads
	delete(report, "reason")
	return report
}

// Both independent readers must supply identical decoded evidence. Neither
// caller hashes nor a single provider's assertion can establish agreement.
func privacyScanPool(input, profile privacyMap, first, second privacyPoolObservation, now time.Time) privacyMap {
	canonical := func(o privacyPoolObservation) string {
		headers := privacyMap{}
		for block, header := range o.headers {
			headers[fmt.Sprint(block)] = header
		}
		return privacyHash(privacyMap{"source": o.source, "state": o.state, "logs": o.logs, "headers": headers, "final": o.final})
	}
	a, b := canonical(first), canonical(second)
	if a == "" || a != b {
		report := privacyLocalPool(input)
		if report != nil && report["status"] != "unsupported" {
			report["reason"] = "evidence_unavailable_or_inconsistent"
		}
		return report
	}
	return privacyReducePool(input, profile, first, now)
}
