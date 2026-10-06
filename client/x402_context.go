package client

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"reflect"
	"regexp"
	"strings"
	"unicode/utf16"
)

// X402SigningRequest is the actual pending wallet request, not an authorization
// inferred from a seller's offer. No signature or private key is accepted.
type X402SigningRequest struct {
	Owner     string          `json:"owner"`
	TypedData json.RawMessage `json:"typed_data"`
	Allowance *string         `json:"allowance,omitempty"`
}

type x402TypedData struct {
	Types       map[string][]x402TypeField `json:"types"`
	Domain      map[string]any             `json:"domain"`
	PrimaryType string                     `json:"primaryType"`
	Message     map[string]any             `json:"message"`
}
type x402TypeField struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

var x402Address = regexp.MustCompile(`^0x[0-9a-fA-F]{40}$`)
var x402Hash = regexp.MustCompile(`^0x[0-9a-fA-F]{64}$`)
var x402Uint = regexp.MustCompile(`^(0|[1-9][0-9]*|0x[0-9a-fA-F]+)$`)

func x402Number(v any) (string, error) {
	s, ok := v.(string)
	if !ok {
		if n, yes := v.(json.Number); yes {
			s = string(n)
		} else {
			return "", ErrInvalid
		}
	}
	if len(s) > 78 || !x402Uint.MatchString(s) {
		return "", ErrInvalid
	}
	base := 10
	if strings.HasPrefix(s, "0x") {
		base = 16
		s = s[2:]
	}
	n, ok := new(big.Int).SetString(s, base)
	if !ok || n.Sign() < 0 || n.BitLen() > 256 {
		return "", ErrInvalid
	}
	return n.String(), nil
}
func x402Addr(v any) (string, error) {
	s, ok := v.(string)
	if !ok || !x402Address.MatchString(s) {
		return "", ErrInvalid
	}
	return strings.ToLower(s), nil
}
func x402Keys(m map[string]any, keys ...string) bool {
	if len(m) != len(keys) {
		return false
	}
	for _, k := range keys {
		if _, ok := m[k]; !ok {
			return false
		}
	}
	return true
}

// Python's closed fact-sheet hash uses sorted, compact ASCII JSON. RFC8785
// already orders this key vocabulary; escape non-ASCII resources identically.
func x402Digest(v any) (string, error) {
	raw, err := canonicalValue(v)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	for _, r := range string(raw) {
		if r < 0x7f {
			b.WriteRune(r)
		} else if r <= 0xffff {
			fmt.Fprintf(&b, `\u%04x`, r)
		} else {
			a, c := utf16.EncodeRune(r)
			fmt.Fprintf(&b, `\u%04x\u%04x`, a, c)
		}
	}
	h := sha256.Sum256([]byte(b.String()))
	return "0x" + hex.EncodeToString(h[:]), nil
}

func bindX402SigningContext(input json.RawMessage, signing *X402SigningRequest) (json.RawMessage, error) {
	var m map[string]any
	if DecodeStrict(input, &m, MessageLimit) != nil || m == nil {
		return nil, ErrInvalid
	}
	if _, exists := m["signing_context"]; exists {
		return nil, ErrInvalid
	}
	if signing == nil {
		return input, nil
	}
	rows, ok := m["accepts"].([]any)
	if !ok || len(rows) < 1 || len(rows) > 8 {
		return nil, ErrInvalid
	}
	index := int64(0)
	if n, exists := m["chosen_index"]; exists {
		x, ok := n.(json.Number)
		if !ok {
			return nil, ErrInvalid
		}
		var err error
		index, err = x.Int64()
		if err != nil {
			return nil, ErrInvalid
		}
	}
	if index < 0 || index >= int64(len(rows)) {
		return nil, ErrInvalid
	}
	m["chosen_index"] = index
	if _, ok := m["budget"]; !ok {
		m["budget"] = nil
	}
	if _, ok := m["prior_paytos"]; !ok {
		m["prior_paytos"] = []any{}
	}
	if m["settlement_tx"] == nil {
		delete(m, "settlement_tx")
	} else {
		s, ok := m["settlement_tx"].(string)
		if !ok || !x402Hash.MatchString(s) {
			return nil, ErrInvalid
		}
		m["settlement_tx"] = strings.ToLower(s)
	}
	prior, ok := m["prior_paytos"].([]any)
	if !ok {
		return nil, ErrInvalid
	}
	for i, a := range prior {
		s, e := x402Addr(a)
		if e != nil {
			return nil, e
		}
		prior[i] = s
	}
	for _, r := range rows {
		row, ok := r.(map[string]any)
		if !ok {
			return nil, ErrInvalid
		}
		if legacy, exists := row["maxAmountRequired"]; exists {
			if _, dup := row["amount"]; dup {
				return nil, ErrInvalid
			}
			row["amount"] = legacy
			delete(row, "maxAmountRequired")
		}
		for _, k := range []string{"asset", "payTo"} {
			a, e := x402Addr(row[k])
			if e != nil {
				return nil, e
			}
			row[k] = a
		}
		if _, ok := row["resource"]; !ok {
			row["resource"] = nil
		}
		if row["facilitator_url"] == nil {
			delete(row, "facilitator_url")
		}
	}
	offer := rows[index].(map[string]any)
	if offer["scheme"] != "exact" && offer["scheme"] != "upto" {
		return nil, ErrInvalid
	}
	binding, err := x402Digest(m)
	if err != nil {
		return nil, err
	}
	var td x402TypedData
	if DecodeStrict(signing.TypedData, &td, MessageLimit) != nil {
		return nil, ErrInvalid
	}
	owner, err := x402Addr(signing.Owner)
	if err != nil {
		return nil, err
	}
	chain, err := x402Number(td.Domain["chainId"])
	if err != nil {
		return nil, err
	}
	chainN, ok := new(big.Int).SetString(chain, 10)
	if !ok || chainN.BitLen() > 53 {
		return nil, ErrInvalid
	}
	token, err := x402Addr(td.Domain["verifyingContract"])
	if err != nil {
		return nil, err
	}
	context := map[string]any{"input_sha256": binding, "scheme": offer["scheme"], "facilitator_url": offer["facilitator_url"], "mechanism": "unknown"}
	if signing.Allowance != nil {
		a, e := x402Number(*signing.Allowance)
		if e != nil {
			return nil, e
		}
		context["allowance"] = a
	}
	auth := map[string]any{"chain_id": chainN.Int64(), "owner": owner}
	number := func(dst string, v any) error {
		s, e := x402Number(v)
		if e == nil {
			auth[dst] = s
		}
		return e
	}
	address := func(dst string, v any) error {
		s, e := x402Addr(v)
		if e == nil {
			auth[dst] = s
		}
		return e
	}
	domainType := []x402TypeField{{"name", "string"}, {"version", "string"}, {"chainId", "uint256"}, {"verifyingContract", "address"}}
	switch td.PrimaryType {
	case "TransferWithAuthorization", "ReceiveWithAuthorization":
		expected := []x402TypeField{{"from", "address"}, {"to", "address"}, {"value", "uint256"}, {"validAfter", "uint256"}, {"validBefore", "uint256"}, {"nonce", "bytes32"}}
		if !x402Keys(td.Domain, "name", "version", "chainId", "verifyingContract") || !x402Keys(td.Message, "from", "to", "value", "validAfter", "validBefore", "nonce") {
			return nil, ErrInvalid
		}
		if _, ok := td.Domain["name"].(string); !ok {
			return nil, ErrInvalid
		}
		if _, ok := td.Domain["version"].(string); !ok {
			return nil, ErrInvalid
		}
		if len(td.Types) != 1 && len(td.Types) != 2 || !reflect.DeepEqual(td.Types[td.PrimaryType], expected) {
			return nil, ErrInvalid
		}
		if d, exists := td.Types["EIP712Domain"]; exists {
			if !reflect.DeepEqual(d, domainType) {
				return nil, ErrInvalid
			}
		} else if len(td.Types) != 1 {
			return nil, ErrInvalid
		}
		from, e := x402Addr(td.Message["from"])
		if e != nil || from != owner {
			return nil, ErrInvalid
		}
		nonce, ok := td.Message["nonce"].(string)
		if !ok || !x402Hash.MatchString(nonce) {
			return nil, ErrInvalid
		}
		auth["nonce"] = strings.ToLower(nonce)
		auth["token"] = token
		if address("to", td.Message["to"]) != nil || number("value", td.Message["value"]) != nil || number("valid_after", td.Message["validAfter"]) != nil || number("valid_before", td.Message["validBefore"]) != nil {
			return nil, ErrInvalid
		}
		context["mechanism"] = "eip3009"
		context["eip3009"] = auth
	case "PermitWitnessTransferFrom":
		domainType = []x402TypeField{{"name", "string"}, {"chainId", "uint256"}, {"verifyingContract", "address"}}
		if token != "0x000000000022d473030f116ddee9f6b43ac78ba3" || td.Domain["name"] != "Permit2" || !x402Keys(td.Domain, "name", "chainId", "verifyingContract") || !x402Keys(td.Message, "permitted", "spender", "nonce", "deadline", "witness") {
			return nil, ErrInvalid
		}
		witness, ok := td.Message["witness"].(map[string]any)
		if !ok {
			return nil, ErrInvalid
		}
		permitted, ok := td.Message["permitted"].(map[string]any)
		if !ok || !x402Keys(permitted, "token", "amount") {
			return nil, ErrInvalid
		}
		fields := []x402TypeField{{"to", "address"}, {"validAfter", "uint256"}}
		if _, upto := witness["facilitator"]; upto {
			if !x402Keys(witness, "to", "validAfter", "facilitator") {
				return nil, ErrInvalid
			}
			fields = []x402TypeField{{"to", "address"}, {"facilitator", "address"}, {"validAfter", "uint256"}}
			if address("facilitator", witness["facilitator"]) != nil {
				return nil, ErrInvalid
			}
			context["facilitator"] = auth["facilitator"]
		} else if !x402Keys(witness, "to", "validAfter") {
			return nil, ErrInvalid
		}
		expected := map[string][]x402TypeField{"PermitWitnessTransferFrom": {{"permitted", "TokenPermissions"}, {"spender", "address"}, {"nonce", "uint256"}, {"deadline", "uint256"}, {"witness", "Witness"}}, "TokenPermissions": {{"token", "address"}, {"amount", "uint256"}}, "Witness": fields}
		if d, exists := td.Types["EIP712Domain"]; exists {
			if !reflect.DeepEqual(d, domainType) {
				return nil, ErrInvalid
			}
			delete(td.Types, "EIP712Domain")
		}
		if !reflect.DeepEqual(td.Types, expected) {
			return nil, ErrInvalid
		}
		if address("token", permitted["token"]) != nil || address("to", witness["to"]) != nil || address("spender", td.Message["spender"]) != nil || number("amount", permitted["amount"]) != nil || number("nonce", td.Message["nonce"]) != nil || number("deadline", td.Message["deadline"]) != nil || number("valid_after", witness["validAfter"]) != nil {
			return nil, ErrInvalid
		}
		context["mechanism"] = "permit2_witness"
		context["permit2"] = auth
	default:
		// Reusable Permit2 permits and naked approvals must never be presented as a
		// bounded witness transfer. Keep them explicit so the checker can refuse.
		if token == "0x000000000022d473030f116ddee9f6b43ac78ba3" {
			context["mechanism"] = "permit2"
		} else {
			context["mechanism"] = "erc20_approval"
		}
	}
	m["signing_context"] = context
	return canonicalValue(m)
}
