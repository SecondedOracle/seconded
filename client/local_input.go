package client

import (
	"encoding/json"
	"fmt"
	"strings"
)

// This type is only constructed before quote/payment or receipt recovery begins.
// Service and evidence failures must never acquire local no-charge semantics.
type localInputError struct{ field, message string }

func (e *localInputError) Error() string     { return "invalid_input" }
func (e *localInputError) Unwrap() error     { return ErrInvalid }
func inputError(field, message string) error { return &localInputError{field, message} }

func validateToolInput(name string, args json.RawMessage) error {
	if isPrivacyName(name) {
		return nil
	}
	if len(args) == 0 {
		args = json.RawMessage(`{}`)
	}
	var fields map[string]json.RawMessage
	if DecodeStrict(args, &fields, MessageLimit) != nil || fields == nil {
		return inputError("arguments", "Supply a JSON object matching the tool schema.")
	}
	product := toolProduct(name)
	if name == "seconded_receipt" {
		if raw, ok := fields["check_id"]; ok {
			var id string
			if json.Unmarshal(raw, &id) != nil || !hexID.MatchString(id) {
				return inputError("check_id", "Use 32 lowercase hexadecimal characters, or omit check_id to list recent checks.")
			}
		}
	}
	if name != "seconded_quote" && product == "" {
		return nil
	}
	if raw, ok := fields["network"]; ok {
		var network string
		if json.Unmarshal(raw, &network) != nil || !oneOf(network, "base", "arc", "robinhood", "base_sepolia", "arc_testnet", "robinhood_testnet") {
			return inputError("network", "Choose base, arc, robinhood, base_sepolia, arc_testnet, or robinhood_testnet; omit network for the default.")
		}
	}
	if raw, ok := fields["wait_seconds"]; ok {
		var wait *int
		if json.Unmarshal(raw, &wait) != nil || wait == nil || *wait < 0 || *wait > 25 || name == "seconded_quote" {
			return inputError("wait_seconds", "Use an integer from 0 to 25 for a check; omit wait_seconds for a quote.")
		}
	}
	if _, ok := fields["input"]; !ok {
		return inputError("input", "Supply the required input object from seconded_products.")
	}
	var a struct {
		Product string              `json:"product,omitempty"`
		Input   json.RawMessage     `json:"input"`
		Max     string              `json:"max_price_usd,omitempty"`
		Network string              `json:"network,omitempty"`
		Wait    *int                `json:"wait_seconds,omitempty"`
		Signing *X402SigningRequest `json:"signing,omitempty"`
	}
	if DecodeStrict(args, &a, MessageLimit) != nil {
		return inputError("arguments", "Use the field names and types shown in seconded_products.")
	}
	if _, ok := fields["max_price_usd"]; ok {
		if _, err := USD(a.Max); err != nil || name == "seconded_quote" {
			return inputError("max_price_usd", "Use a nonnegative decimal USD string for a check; omit max_price_usd for a quote.")
		}
	}
	if name == "seconded_quote" {
		product = a.Product
	} else if _, ok := fields["product"]; ok {
		return inputError("product", "The check tool selects its product; omit product.")
	}
	if _, ok := fields["signing"]; ok && (name != "seconded_x402_payment_check" || a.Signing == nil) {
		return inputError("signing", "Supply signing only with seconded_x402_payment_check.")
	}
	return validateProductInput(product, a.Input)
}

// Enforce required/nonempty fields from the compiled catalog, including nested
// objects and arrays. Semantic product validation remains authoritative server-side.
func validateProductInput(product string, raw json.RawMessage) error {
	if !productOffered(product) {
		return inputError("product", "Choose an available product from seconded_products.")
	}
	var value any
	if DecodeStrict(raw, &value, MessageLimit) != nil {
		return inputError("input", "Supply the required input object using valid JSON.")
	}
	if obj, ok := value.(map[string]any); !ok || obj == nil {
		return inputError("input", "Supply a nonempty input object matching the product schema.")
	}
	for _, p := range productGuides() {
		if p.ID == product {
			return requiredInput(p.InputSchema.(map[string]any), value, "input")
		}
	}
	return nil
}

func requiredInput(spec map[string]any, value any, path string) error {
	switch spec["type"] {
	case "object":
		if _, ok := value.(map[string]any); !ok {
			return inputError(path, "Supply an object matching the product schema.")
		}
	case "array":
		if _, ok := value.([]any); !ok {
			return inputError(path, "Supply an array matching the product schema.")
		}
	case "string":
		if _, ok := value.(string); !ok {
			return inputError(path, "Supply a string matching the product schema.")
		}
	}

	if obj, ok := value.(map[string]any); ok {
		if required, ok := spec["required"].([]any); ok {
			for _, item := range required {
				key := item.(string)
				v, exists := obj[key]
				empty := !exists || v == nil
				switch v := v.(type) {
				case string:
					empty = strings.TrimSpace(v) == ""
				case map[string]any:
					empty = len(v) == 0
				case []any:
					empty = len(v) == 0
				}
				if empty {
					return inputError(path+"."+key, "Supply a nonempty required field using the schema from seconded_products.")
				}
			}
		}
		if variants, ok := spec["oneOf"].([]any); ok {
			matches := 0
			for _, variant := range variants {
				branch := variant.(map[string]any)
				if _, ok := branch["required"]; ok && requiredInput(branch, value, path) == nil {
					matches++
				}
			}
			if matches != 1 {
				return inputError(path, "Supply exactly one of the alternatives required by the product schema.")
			}
		}
		if props, ok := spec["properties"].(map[string]any); ok {
			for key, sub := range props {
				if v, exists := obj[key]; exists && v != nil {
					if err := requiredInput(sub.(map[string]any), v, path+"."+key); err != nil {
						return err
					}
				}
			}
		}
	}
	if items, ok := value.([]any); ok {
		if sub, ok := spec["items"].(map[string]any); ok {
			for i, v := range items {
				if err := requiredInput(sub, v, fmt.Sprintf("%s[%d]", path, i)); err != nil {
					return err
				}
			}
		}
	}
	return nil
}
