package client

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestBurnRefusalInventory(t *testing.T) {
	for _, code := range strings.Fields(`
admission_hold
ambiguous_burn_events
asset_scope_unavailable
at_capacity
burn_already_claimed
burn_below_quote_minimum
burn_credits_unavailable
burn_event_missing
burn_log_block_mismatch
burn_log_transaction_mismatch
burn_not_redeemable
burn_not_released
burn_receipt_missing
burn_transaction_failed
burn_value_below_credit_unit
capacity_limited
client_upgrade_required
credit_authorization_expired
credit_authorization_required
credit_check_quote_expired
credit_max_price_required
credit_network_mismatch
credit_payer_mismatch
credit_request_conflict
daily_cap
door_disabled
face_credit_above_cap
face_credit_below_minimum
fair_use_limited
global_monthly_cap_exceeded
history_reader_unavailable
input_too_large
insufficient_credit
invalid_agent_id
invalid_credit_nonce
invalid_credit_signature
invalid_debit_amount
invalid_discount
invalid_face_credit
invalid_input
invalid_key_id
invalid_minimum_burn
invalid_payer
invalid_price_window
invalid_quote
invalid_quote_expiry
invalid_quote_id
invalid_quote_request
invalid_quote_signature
invalid_redeem_request
invalid_redemption_signature
invalid_reservation_id
invalid_transaction_hash
malformed_latest_block
malformed_price_block
malformed_receipt_block
malformed_receipt_from
malformed_receipt_log
malformed_receipt_logs
malformed_receipt_status
malformed_transfer_log
noncanonical_payer
not_official_token
payer_agent_monthly_cap_exceeded
pool_key_mismatch
pool_liquidity_below_minimum
price_block_changed
price_block_unavailable
price_deviation
price_exceeds_max
price_temporarily_unavailable
product_in_testing
quote_already_used
quote_discount_mismatch
quote_expired
rate_limited
reader_head_regressed
reader_head_skew
reader_missing_receipt_method
reader_price_disagreement
reader_quorum_unavailable
reader_receipt_disagreement
receipt_block_changed
receipt_block_not_canonical
receipt_payer_mismatch
receipt_transaction_mismatch
redemption_payer_mismatch
removed_burn_log
request_timeout
reservation_id_conflict
sales_off
sampled_head_changed
spend_limited
stale_price_block
store_unavailable
unknown_quote_key_id
unsupported_chain
unsupported_network
unsupported_release_policy
wallet_frozen
wrong_chain
zero_burn
`) {
		t.Run(code, func(t *testing.T) {
			guidance, ok := burnErrorMessages[code]
			if !ok || guidance.Message == "" || guidance.Next == "" || len(guidance.Statuses) == 0 {
				t.Fatal("missing refusal guidance", code)
			}
			for _, status := range guidance.Statuses {
				body, _ := json.Marshal(map[string]any{"error": code, "reason": "untrusted server text", "hints": map[string]string{"next": "untrusted server instruction"}})
				err := publicAPIError(status, body)
				result := failedResult(Result{}, err)
				if result.Reason != code || result.Message != publicErrorMessages[code] || result.Next != guidance.Next {
					t.Fatalf("status %d: %+v / %v", status, result, err)
				}
				if result.Charged != "not_yet_known" || strings.Contains(result.Message+result.Next, "untrusted server") {
					t.Fatal("unsigned diagnostic crossed trust boundary", result)
				}
			}
		})
	}
}

func TestBurnRetrySemantics(t *testing.T) {
	for _, code := range strings.Fields(`pool_liquidity_below_minimum invalid_price_window reader_head_skew price_block_changed price_block_unavailable stale_price_block price_deviation price_temporarily_unavailable reader_price_disagreement reader_receipt_disagreement burn_receipt_missing burn_not_released burn_value_below_credit_unit`) {
		next := burnErrorMessages[code].Next
		if !strings.Contains(next, "Wait and retry the same request; nothing was charged") {
			t.Errorf("transient %s: %s", code, next)
		}
	}
	for _, code := range strings.Fields(`history_reader_unavailable reader_quorum_unavailable pool_key_mismatch wrong_chain invalid_quote_signature quote_expired invalid_discount invalid_minimum_burn face_credit_above_cap`) {
		next := burnErrorMessages[code].Next
		if !strings.Contains(next, "do not retry unchanged") || strings.Contains(next, "nothing was charged") {
			t.Errorf("input/config/permanent %s: %s", code, next)
		}
	}
	for _, code := range strings.Fields(`burn_credits_unavailable malformed_latest_block malformed_price_block malformed_receipt_block malformed_receipt_from malformed_receipt_log malformed_receipt_logs malformed_receipt_status malformed_transfer_log`) {
		next := burnErrorMessages[code].Next
		if !strings.Contains(next, "retry the same request once") || !strings.Contains(next, "ask the service operator") || strings.Contains(next, "do not retry unchanged") {
			t.Errorf("bounded %s: %s", code, next)
		}
	}

}
