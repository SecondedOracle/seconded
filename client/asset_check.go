package client

import (
	"encoding/json"
	"regexp"
	"strconv"
	"unicode/utf8"
)

const assetCheckTool = "seconded_asset_check"
const assetCheckDescription = "Use when: checking token controls or stock-token identity. Send: kind=token|stock_token, network, address; optional fields follow that kind's schema. Answer → do: token warning_*: STOP; issuer_controlled_expected: issuer can freeze/upgrade. Stock identity is not a token-risk pass; review its original labels. NOT VERIFIED: pause. Cost: $0.10 small; $0.15 paid on Robinhood."

// Derive each closed branch from its retained product schema, preserving bounds
// and numeric unions. Product catalogs remain the raw quote/receipt contracts.
func assetCheckSchema() any {
	branches := []any{}
	for _, p := range productGuides() {
		if p.ID != "token_check" && p.ID != "stock_token_check" {
			continue
		}
		branch := p.InputSchema.(map[string]any)
		kind := "token"
		if p.ID == "stock_token_check" {
			kind = "stock_token"
		}
		branch["properties"].(map[string]any)["kind"] = map[string]any{"type": "string", "const": kind}
		branch["required"] = append(branch["required"].([]any), "kind")
		branches = append(branches, branch)
	}
	return map[string]any{"type": "object", "oneOf": branches}
}

func resolveAssetCheck(args json.RawMessage) (string, json.RawMessage, error) {
	var fields map[string]json.RawMessage
	if DecodeStrict(args, &fields, MessageLimit) != nil || fields == nil {
		return "", nil, inputError("arguments", "Supply an Asset Check argument object.")
	}
	var input map[string]json.RawMessage
	if DecodeStrict(fields["input"], &input, MessageLimit) != nil || input == nil {
		return "", nil, inputError("input", "Supply input with kind, network and address.")
	}
	var kind string
	if json.Unmarshal(input["kind"], &kind) != nil || !oneOf(kind, "token", "stock_token") {
		return "", nil, inputError("input.kind", "Choose token or stock_token explicitly.")
	}
	delete(input, "kind")
	product := kind + "_check"
	for _, p := range productGuides() {
		if p.ID != product {
			continue
		}
		props := p.InputSchema.(map[string]any)["properties"].(map[string]any)
		for key, value := range input {
			spec, exists := props[key]
			if !exists || !assetFieldValid(spec.(map[string]any), value) {
				return "", nil, inputError("input."+key, "Use only fields and values allowed by the selected Asset Check kind.")
			}
		}
	}
	fields["input"], _ = json.Marshal(input)
	resolved, err := json.Marshal(fields)
	if err != nil {
		return "", nil, err
	}
	name := "seconded_" + product
	if err := validateToolInput(name, resolved); err != nil {
		return "", nil, err
	}
	return name, resolved, nil
}

// Asset branches contain strings and one positive-number/string union. Validate
// their catalog constraints before any quote without coercing the original JSON.
func assetFieldValid(spec map[string]any, raw json.RawMessage) bool {
	if branches, ok := spec["oneOf"].([]any); ok {
		matches := 0
		for _, branch := range branches {
			if assetFieldValid(branch.(map[string]any), raw) {
				matches++
			}
		}
		return matches == 1
	}
	if spec["type"] == "number" {
		var value any
		if DecodeStrict(raw, &value, MessageLimit) != nil {
			return false
		}
		number, ok := value.(json.Number)
		if !ok {
			return false
		}
		v, err := strconv.ParseFloat(string(number), 64)
		return err == nil && v > 0
	}
	var value string
	if string(raw) == "null" || json.Unmarshal(raw, &value) != nil {
		return false
	}
	if choices, ok := spec["enum"].([]any); ok {
		found := false
		for _, choice := range choices {
			found = found || choice == value
		}
		if !found {
			return false
		}
	}
	if pattern, ok := spec["pattern"].(string); ok && !regexp.MustCompile(pattern).MatchString(value) {
		return false
	}
	length := utf8.RuneCountInString(value)
	if bound, ok := spec["minLength"].(json.Number); ok {
		minimum, _ := bound.Int64()
		if int64(length) < minimum {
			return false
		}
	}
	if bound, ok := spec["maxLength"].(json.Number); ok {
		maximum, _ := bound.Int64()
		if int64(length) > maximum {
			return false
		}
	}
	return true
}
