package client

import (
	"strings"
	"testing"
)

func TestPublicBurnRefusalProjection(t *testing.T) {
	for _, code := range []string{"pool_liquidity_below_minimum", "injected_unknown_refusal"} {
		err := publicAPIError(422, []byte(`{"error":"`+code+`","reason":"untrusted server text"}`))
		result := toolFailure(nil, err)
		want := code
		if code == "injected_unknown_refusal" {
			want = "invalid_response"
		}
		if result["reason"] != want || result["charged"] != "not_yet_known" {
			t.Fatal(result)
		}
		if code == "pool_liquidity_below_minimum" && !strings.Contains(result["next"].(string), "Wait and retry the same request; nothing was charged") {
			t.Fatal(result)
		}
		if strings.Contains(result["message"].(string), "untrusted server text") {
			t.Fatal(result)
		}
	}
}

func TestBurnRefusedResultGuidance(t *testing.T) {
	for _, code := range []string{"insufficient_credit", "fair_use_limited", "wallet_frozen", "admission_hold", "capacity_limited", "spend_limited", "sales_off", "daily_cap", "at_capacity", "door_disabled", "credit_network_mismatch", "credit_check_quote_expired"} {
		t.Run(code, func(t *testing.T) {
			entry := Entry{State: "RELEASED", Credit: &CreditRecovery{}, Receipt: &SignedReceipt{Envelope: ReceiptEnvelope{State: "refused", Reason: code}}}
			got := resultFor(entry)
			if got.Status != "refused" || got.Reason != code || got.Message != publicErrorMessages[code] || got.Next != burnErrorMessages[code].Next || got.Receipt != "verified" || got.Charged != "no" {
				t.Fatal(got)
			}
			entry.Receipt.Envelope.State = "running"
			if pending := resultFor(entry); pending.Status != "pending" || pending.Next != "call_receipt_after" {
				t.Fatal("pending recovery token changed", pending)
			}
		})
	}
	unknown := resultFor(Entry{Receipt: &SignedReceipt{Envelope: ReceiptEnvelope{State: "refused", Reason: "untrusted_unknown_reason"}}})
	if unknown.Reason != "operation_failed" || unknown.Next != "call_receipt_after" {
		t.Fatal("unknown refusal fallback changed", unknown)
	}
}

func TestBurnPreservesSharedPublicMessages(t *testing.T) {
	for code, want := range map[string]string{
		"client_upgrade_required": "Install a reviewed newer client release before checking again.",
		"input_too_large":         "Shorten the input to fit the product's size limit.",
		"invalid_input":           "Correct the input using the schema from seconded_products.",
		"price_exceeds_max":       "The service price exceeds the requested ceiling. Review the quote before changing max_price_usd.",
		"product_in_testing":      "This product is not available for paid checks. Choose a supported product from seconded_products.",
		"rate_limited":            "Pause before trying again; recover any existing check first.",
		"store_unavailable":       "The service store is unavailable. Wait and recover receipts before trying again.",
		"unsupported_network":     "The selected payment network is unavailable. Choose an enabled payment network; subject selection is separate.",
	} {
		if got := publicErrorMessages[code]; got != want {
			t.Errorf("%s: got %q, want %q", code, got, want)
		}
	}
}
