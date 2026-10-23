package client

// BurnCreditError carries a bounded server refusal code. It is diagnostic only:
// closing a spend still requires its authenticated terminal receipt.
type BurnCreditError struct {
	Code string
}

func (e *BurnCreditError) Error() string { return e.Code }

// Guidance is local, reviewed copy; server messages and hints are never displayed.
// Next describes this API attempt, not settlement of earlier burns or checks.
type burnErrorGuidance struct {
	Message  string
	Next     string
	Statuses []int
}

var burnErrorMessages = map[string]burnErrorGuidance{
	"admission_hold": {
		Message:  "The service has paused admission for this check.",
		Next:     "Recover the original request and wait for the operator to lift the hold before a new check; do not retry unchanged. Keep any existing burn and quote for recovery.",
		Statuses: []int{503},
	},
	"ambiguous_burn_events": {
		Message:  "The receipt contains multiple matching burn events.",
		Next:     "Ask the service operator to review the ambiguous original transaction without burning again; do not retry unchanged. Keep any existing burn and quote for recovery.",
		Statuses: []int{422},
	},
	"asset_scope_unavailable": {
		Message:  "The service cannot verify the requested asset scope.",
		Next:     "Ask the service operator to restore asset-scope verification and recover the original request; do not retry unchanged. Keep any existing burn and quote for recovery.",
		Statuses: []int{503},
	},
	"at_capacity": {
		Message:  "Degen Check capacity is full or could not be measured.",
		Next:     "Recover the original request and wait for capacity to recover before a new check; do not retry unchanged. Keep any existing burn and quote for recovery.",
		Statuses: []int{503},
	},
	"burn_already_claimed": {
		Message:  "The burn was already claimed under different quote terms.",
		Next:     "Recover the original credit grant with its original quote and transaction; do not retry unchanged. Keep any existing burn and quote for recovery.",
		Statuses: []int{422},
	},
	"burn_below_quote_minimum": {
		Message:  "The existing burn amount is below the quote minimum.",
		Next:     "Resume the original pack or review a fresh quote for the existing burn without swapping or burning again; do not retry unchanged. Keep any existing burn and quote for recovery.",
		Statuses: []int{422},
	},
	"burn_credits_unavailable": {
		Message:  "The credit service is disabled or could not complete this operation.",
		Next:     "Wait briefly and retry the same request once; nothing was charged by this API attempt (quote and status are read-only; an exact redeem replay returns the original claim). If it keeps failing, ask the service operator to restore the credit service. Keep any existing burn and quote; recover outstanding checks with seconded_receipt.",
		Statuses: []int{503},
	},
	"burn_event_missing": {
		Message:  "The receipt has no matching payer burn event for the configured token.",
		Next:     "Check the original payer, token and transaction and provide the correct existing burn; do not retry unchanged. Keep any existing burn and quote for recovery.",
		Statuses: []int{422},
	},
	"burn_log_block_mismatch": {
		Message:  "The burn log block differs from the receipt block.",
		Next:     "Ask the service operator to repair inconsistent receipt evidence for the original transaction; do not retry unchanged. Keep any existing burn and quote for recovery.",
		Statuses: []int{422},
	},
	"burn_log_transaction_mismatch": {
		Message:  "The burn log belongs to a different transaction.",
		Next:     "Ask the service operator to repair inconsistent receipt evidence for the original transaction; do not retry unchanged. Keep any existing burn and quote for recovery.",
		Statuses: []int{422},
	},
	"burn_not_redeemable": {
		Message:  "The recorded burn belongs to a different payer.",
		Next:     "Use the original payer and transaction and recover the original claim without burning again; do not retry unchanged. Keep any existing burn and quote for recovery.",
		Statuses: []int{422},
	},
	"burn_not_released": {
		Message:  "The burn has not reached the required confirmation depth.",
		Next:     "Wait and retry the same request; nothing was charged by this API attempt. Keep any existing burn and quote; recover outstanding checks with seconded_receipt.",
		Statuses: []int{422},
	},
	"burn_receipt_missing": {
		Message:  "The original burn receipt is not yet available from a reader.",
		Next:     "Wait and retry the same request; nothing was charged by this API attempt. Keep any existing burn and quote; recover outstanding checks with seconded_receipt.",
		Statuses: []int{422},
	},
	"burn_transaction_failed": {
		Message:  "The original burn transaction failed onchain.",
		Next:     "Inspect the failed transaction and any earlier pack stages before deciding how to recover; do not retry unchanged. Keep any existing burn and quote for recovery.",
		Statuses: []int{422},
	},
	"burn_value_below_credit_unit": {
		Message:  "The existing burn is worth less than one credit unit at the current price.",
		Next:     "Wait and retry the same request; nothing was charged by this API attempt. Keep any existing burn and quote; recover outstanding checks with seconded_receipt.",
		Statuses: []int{422},
	},
	"capacity_limited": {
		Message:  "The service has reached its running-check capacity.",
		Next:     "Recover the original refused request before starting a new check after capacity recovers; do not retry unchanged. Keep any existing burn and quote for recovery.",
		Statuses: []int{503},
	},
	"client_upgrade_required": {
		Message:  "The service requires a newer client release.",
		Next:     "Install a reviewed newer client and recover outstanding checks before starting another purchase; do not retry unchanged. Keep any existing burn and quote for recovery.",
		Statuses: []int{426},
	},
	"credit_authorization_expired": {
		Message:  "The credit authorization proof is expired or outside its allowed time window.",
		Next:     "Recover the original request with seconded_receipt using a fresh proof and the original nonce; do not retry unchanged. Keep any existing burn and quote for recovery.",
		Statuses: []int{422},
	},
	"credit_authorization_required": {
		Message:  "The credit authorization proof is missing or malformed.",
		Next:     "Supply a proof with nonce, expires_at and signature for the original request; do not retry unchanged. Keep any existing burn and quote for recovery.",
		Statuses: []int{401, 422},
	},
	"credit_check_quote_expired": {
		Message:  "The check quote expired before admission.",
		Next:     "Recover the original request with seconded_receipt before obtaining a fresh check quote; do not retry unchanged. Keep any existing burn and quote for recovery.",
		Statuses: []int{422},
	},
	"credit_max_price_required": {
		Message:  "The credit request lacks an explicit signed price ceiling.",
		Next:     "Set options.max_price and sign the exact request after recovering any outstanding check; do not retry unchanged. Keep any existing burn and quote for recovery.",
		Statuses: []int{422},
	},
	"credit_network_mismatch": {
		Message:  "Credit-paid checks require the configured Robinhood mainnet network.",
		Next:     "Recover outstanding checks before correcting the request network to eip155:4663; do not retry unchanged. Keep any existing burn and quote for recovery.",
		Statuses: []int{422},
	},
	"credit_payer_mismatch": {
		Message:  "The credit authorization signer differs from the request payer.",
		Next:     "Use the original payer wallet and exact original request for recovery; do not retry unchanged. Keep any existing burn and quote for recovery.",
		Statuses: []int{422},
	},
	"credit_request_conflict": {
		Message:  "The request nonce is already bound to different request contents.",
		Next:     "Restore the original request and nonce and recover it with seconded_receipt; do not retry unchanged. Keep any existing burn and quote for recovery.",
		Statuses: []int{422},
	},
	"daily_cap": {
		Message:  "The daily Degen Check admission limit has been reached.",
		Next:     "Wait until the next UTC day and recover the original request before a new check; do not retry unchanged. Keep any existing burn and quote for recovery.",
		Statuses: []int{503},
	},
	"door_disabled": {
		Message:  "This payment door is disabled on the service.",
		Next:     "Recover outstanding checks and use an enabled payment door; do not retry unchanged. Keep any existing burn and quote for recovery.",
		Statuses: []int{503},
	},
	"face_credit_above_cap": {
		Message:  "The requested credit face value exceeds the configured issuance cap.",
		Next:     "Reduce the requested face value to the advertised maximum; do not retry unchanged. Keep any existing burn and quote for recovery.",
		Statuses: []int{422},
	},
	"face_credit_below_minimum": {
		Message:  "The requested credit face value is below the configured minimum.",
		Next:     "Request at least the advertised minimum face value ($5 under the current policy); do not retry unchanged. Keep any existing burn and quote for recovery.",
		Statuses: []int{422},
	},
	"fair_use_limited": {
		Message:  "The check does not satisfy the service fair-use limits.",
		Next:     "Recover the original request and satisfy the returned fair-use policy, including the smallest first-paid tier where required; do not retry unchanged. Keep any existing burn and quote for recovery.",
		Statuses: []int{429},
	},
	"global_monthly_cap_exceeded": {
		Message:  "The global monthly credit issuance cap cannot accommodate this request.",
		Next:     "Reduce the requested face value or wait for the next UTC month before obtaining a fresh quote; do not retry unchanged. Keep any existing burn and quote for recovery.",
		Statuses: []int{422},
	},
	"history_reader_unavailable": {
		Message:  "The required official history reader is missing from the server configuration.",
		Next:     "Ask the service operator to configure the official Robinhood history reader; do not retry unchanged. Keep any existing burn and quote for recovery.",
		Statuses: []int{422},
	},
	"input_too_large": {
		Message:  "The request body exceeds the endpoint size limit.",
		Next:     "Reduce the request body to the endpoint size limit; do not retry unchanged. Keep any existing burn and quote for recovery.",
		Statuses: []int{413},
	},
	"insufficient_credit": {
		Message:  "The account has insufficient spendable credit for this check.",
		Next:     "Recover the original request and obtain sufficient credit before starting a new check; do not retry unchanged. Keep any existing burn and quote for recovery.",
		Statuses: []int{402},
	},
	"invalid_agent_id": {
		Message:  "The agent identifier is invalid.",
		Next:     "Supply 1 to 256 characters without surrounding whitespace or NUL; do not retry unchanged. Keep any existing burn and quote for recovery.",
		Statuses: []int{422},
	},
	"invalid_credit_nonce": {
		Message:  "The credit authorization nonce is not a valid 32-byte hexadecimal value.",
		Next:     "Restore the original request nonce and obtain a fresh recovery proof; do not retry unchanged. Keep any existing burn and quote for recovery.",
		Statuses: []int{422},
	},
	"invalid_credit_signature": {
		Message:  "The credit authorization signature is invalid.",
		Next:     "Use the payer wallet to sign the exact request, nonce, expiry and API origin; do not retry unchanged. Keep any existing burn and quote for recovery.",
		Statuses: []int{422},
	},
	"invalid_debit_amount": {
		Message:  "The server credit debit amount is invalid.",
		Next:     "Ask the operator to repair the priced admission and recover the original check; do not retry unchanged. Keep any existing burn and quote for recovery.",
		Statuses: []int{422},
	},
	"invalid_discount": {
		Message:  "The quote discount is not a valid integer percentage in basis points.",
		Next:     "Restore the original server-issued quote or obtain a fresh quote without editing its discount; do not retry unchanged. Keep any existing burn and quote for recovery.",
		Statuses: []int{422},
	},
	"invalid_face_credit": {
		Message:  "The requested credit face value is not a positive integer.",
		Next:     "Supply a positive integer USD-micros face value within the advertised limits; do not retry unchanged. Keep any existing burn and quote for recovery.",
		Statuses: []int{422},
	},
	"invalid_input": {
		Message:  "The request body or one of its fields is invalid.",
		Next:     "Correct the request JSON and fields; do not retry unchanged. Keep any existing burn and quote for recovery.",
		Statuses: []int{422},
	},
	"invalid_key_id": {
		Message:  "The quote signing-key identifier is invalid.",
		Next:     "Restore the key identifier from the original server-issued quote; do not retry unchanged. Keep any existing burn and quote for recovery.",
		Statuses: []int{422},
	},
	"invalid_minimum_burn": {
		Message:  "The quote minimum burn amount is not a valid positive integer.",
		Next:     "Use the untouched server-issued quote with its decimal minimum_burn_atomic string; do not retry unchanged. Keep any existing burn and quote for recovery.",
		Statuses: []int{422},
	},
	"invalid_payer": {
		Message:  "The payer is not a valid wallet address.",
		Next:     "Supply the original signing wallet address as 0x plus 40 hexadecimal digits; do not retry unchanged. Keep any existing burn and quote for recovery.",
		Statuses: []int{422},
	},
	"invalid_price_window": {
		Message:  "The sampled block timestamps do not form the required price observation window.",
		Next:     "Wait and retry the same request; nothing was charged by this API attempt. Keep any existing burn and quote; recover outstanding checks with seconded_receipt.",
		Statuses: []int{422},
	},
	"invalid_quote": {
		Message:  "The redemption quote has an invalid structure.",
		Next:     "Supply the complete original server-issued quote; do not retry unchanged. Keep any existing burn and quote for recovery.",
		Statuses: []int{422},
	},
	"invalid_quote_expiry": {
		Message:  "The quote timestamps violate the server expiry policy.",
		Next:     "Restore the original server-issued quote or obtain a fresh quote without editing its timestamps; do not retry unchanged. Keep any existing burn and quote for recovery.",
		Statuses: []int{422},
	},
	"invalid_quote_id": {
		Message:  "The quote identifier is invalid.",
		Next:     "Restore the identifier from the original server-issued quote; do not retry unchanged. Keep any existing burn and quote for recovery.",
		Statuses: []int{422},
	},
	"invalid_quote_request": {
		Message:  "The credit quote request has invalid fields.",
		Next:     "Supply exactly payer, agent_id and face_usd_micros; do not retry unchanged. Keep any existing burn and quote for recovery.",
		Statuses: []int{422},
	},
	"invalid_quote_signature": {
		Message:  "The server could not authenticate the quote signature.",
		Next:     "Restore the untouched original server-issued quote and ask the operator to check its signing-key configuration; do not retry unchanged. Keep any existing burn and quote for recovery.",
		Statuses: []int{422},
	},
	"invalid_redeem_request": {
		Message:  "The redemption request has invalid fields.",
		Next:     "Supply exactly tx_hash, quote and signature for the original burn; do not retry unchanged. Keep any existing burn and quote for recovery.",
		Statuses: []int{422},
	},
	"invalid_redemption_signature": {
		Message:  "The redemption signature is invalid.",
		Next:     "Sign the exact original quote and burn transaction with the payer wallet for this API origin; do not retry unchanged. Keep any existing burn and quote for recovery.",
		Statuses: []int{422},
	},
	"invalid_reservation_id": {
		Message:  "The server credit reservation identifier is invalid.",
		Next:     "Ask the operator to repair the admission reservation and recover the original check; do not retry unchanged. Keep any existing burn and quote for recovery.",
		Statuses: []int{422},
	},
	"invalid_transaction_hash": {
		Message:  "A transaction hash is not a valid 32-byte hexadecimal value.",
		Next:     "Check the original transaction hash and ask the operator to repair any malformed reader hashes; do not retry unchanged. Keep any existing burn and quote for recovery.",
		Statuses: []int{422},
	},
	"malformed_latest_block": {
		Message:  "A reader returned an invalid latest block.",
		Next:     "Wait briefly and retry the same request once; nothing was charged by this API attempt. If it persists, ask the service operator to repair the reader. Keep any existing burn and quote; recover outstanding checks with seconded_receipt.",
		Statuses: []int{422},
	},
	"malformed_price_block": {
		Message:  "A reader returned an invalid price block.",
		Next:     "Wait briefly and retry the same request once; nothing was charged by this API attempt. If it persists, ask the service operator to repair the reader. Keep any existing burn and quote; recover outstanding checks with seconded_receipt.",
		Statuses: []int{422},
	},
	"malformed_receipt_block": {
		Message:  "A reader returned an invalid receipt block identity.",
		Next:     "Wait briefly and retry the same request once; nothing was charged by this API attempt. If it persists, ask the service operator to repair the reader. Keep any existing burn and quote; recover outstanding checks with seconded_receipt.",
		Statuses: []int{422},
	},
	"malformed_receipt_from": {
		Message:  "A reader returned an invalid transaction sender address.",
		Next:     "Wait briefly and retry the same request once; nothing was charged by this API attempt. If it persists, ask the service operator to repair the reader. Keep any existing burn and quote; recover outstanding checks with seconded_receipt.",
		Statuses: []int{422},
	},
	"malformed_receipt_log": {
		Message:  "A reader returned a malformed receipt log.",
		Next:     "Wait briefly and retry the same request once; nothing was charged by this API attempt. If it persists, ask the service operator to repair the reader. Keep any existing burn and quote; recover outstanding checks with seconded_receipt.",
		Statuses: []int{422},
	},
	"malformed_receipt_logs": {
		Message:  "A reader returned an invalid receipt log list.",
		Next:     "Wait briefly and retry the same request once; nothing was charged by this API attempt. If it persists, ask the service operator to repair the reader. Keep any existing burn and quote; recover outstanding checks with seconded_receipt.",
		Statuses: []int{422},
	},
	"malformed_receipt_status": {
		Message:  "A reader returned an invalid transaction receipt status.",
		Next:     "Wait briefly and retry the same request once; nothing was charged by this API attempt. If it persists, ask the service operator to repair the reader. Keep any existing burn and quote; recover outstanding checks with seconded_receipt.",
		Statuses: []int{422},
	},
	"malformed_transfer_log": {
		Message:  "A reader returned an invalid token transfer log.",
		Next:     "Wait briefly and retry the same request once; nothing was charged by this API attempt. If it persists, ask the service operator to repair the reader. Keep any existing burn and quote; recover outstanding checks with seconded_receipt.",
		Statuses: []int{422},
	},
	"noncanonical_payer": {
		Message:  "The quote payer address is not in canonical lowercase form.",
		Next:     "Restore the original server-issued quote with its lowercase payer address; do not retry unchanged. Keep any existing burn and quote for recovery.",
		Statuses: []int{422},
	},
	"not_official_token": {
		Message:  "The requested token is not an official supported token.",
		Next:     "Use the official token address for the selected network; do not retry unchanged. Keep any existing burn and quote for recovery.",
		Statuses: []int{422},
	},
	"payer_agent_monthly_cap_exceeded": {
		Message:  "This payer and agent have insufficient monthly credit issuance capacity.",
		Next:     "Reduce the requested face value or wait for the next UTC month before obtaining a fresh quote; do not retry unchanged. Keep any existing burn and quote for recovery.",
		Statuses: []int{422},
	},
	"pool_key_mismatch": {
		Message:  "The configured pool keys do not match the pinned pool identifiers.",
		Next:     "Ask the service operator to correct the pinned pool configuration; do not retry unchanged. Keep any existing burn and quote for recovery.",
		Statuses: []int{422},
	},
	"pool_liquidity_below_minimum": {
		Message:  "A pricing pool has insufficient liquidity for a safe credit valuation.",
		Next:     "Wait and retry the same request; nothing was charged by this API attempt. Keep any existing burn and quote; recover outstanding checks with seconded_receipt.",
		Statuses: []int{422},
	},
	"price_block_changed": {
		Message:  "A sampled price block changed during verification.",
		Next:     "Wait and retry the same request; nothing was charged by this API attempt. Keep any existing burn and quote; recover outstanding checks with seconded_receipt.",
		Statuses: []int{422},
	},
	"price_block_unavailable": {
		Message:  "The common reader head is too early to provide the required price history.",
		Next:     "Wait and retry the same request; nothing was charged by this API attempt. Keep any existing burn and quote; recover outstanding checks with seconded_receipt.",
		Statuses: []int{422},
	},
	"price_deviation": {
		Message:  "The sampled prices exceed the allowed deviation from the current price.",
		Next:     "Wait and retry the same request; nothing was charged by this API attempt. Keep any existing burn and quote; recover outstanding checks with seconded_receipt.",
		Statuses: []int{422},
	},
	"price_exceeds_max": {
		Message:  "The check price exceeds the signed price ceiling.",
		Next:     "Review the current price and explicitly change the ceiling only after recovering the original request; do not retry unchanged. Keep any existing burn and quote for recovery.",
		Statuses: []int{409},
	},
	"price_temporarily_unavailable": {
		Message:  "Price lookup is cooling down after a failed observation.",
		Next:     "Wait and retry the same request; nothing was charged by this API attempt. Keep any existing burn and quote; recover outstanding checks with seconded_receipt.",
		Statuses: []int{422},
	},
	"product_in_testing": {
		Message:  "The requested product is still in testing.",
		Next:     "Select an available product after recovering any outstanding request; do not retry unchanged. Keep any existing burn and quote for recovery.",
		Statuses: []int{422},
	},
	"quote_already_used": {
		Message:  "The quote has already been used for a credit claim.",
		Next:     "Recover the original claim using its original burn transaction; do not retry unchanged. Keep any existing burn and quote for recovery.",
		Statuses: []int{422},
	},
	"quote_discount_mismatch": {
		Message:  "The quote discount differs from the current server policy.",
		Next:     "Resume the same pack with a fresh server quote for the existing burn; do not retry unchanged. Keep any existing burn and quote for recovery.",
		Statuses: []int{422},
	},
	"quote_expired": {
		Message:  "The burn quote has expired.",
		Next:     "Resume the same pack to obtain a fresh quote for the existing burn; do not retry unchanged. Keep any existing burn and quote for recovery.",
		Statuses: []int{422},
	},
	"rate_limited": {
		Message:  "The server request rate limit was reached.",
		Next:     "Wait and retry the same request; nothing was charged by this API attempt. Keep any existing burn and quote; recover outstanding checks with seconded_receipt.",
		Statuses: []int{429},
	},
	"reader_head_regressed": {
		Message:  "A reader head moved backwards during burn verification.",
		Next:     "Wait and retry the same request; nothing was charged by this API attempt. Keep any existing burn and quote; recover outstanding checks with seconded_receipt.",
		Statuses: []int{422},
	},
	"reader_head_skew": {
		Message:  "The reader heads are too far apart to establish a common price anchor.",
		Next:     "Wait and retry the same request; nothing was charged by this API attempt. Keep any existing burn and quote; recover outstanding checks with seconded_receipt.",
		Statuses: []int{422},
	},
	"reader_missing_receipt_method": {
		Message:  "A server reader cannot retrieve transaction receipts.",
		Next:     "Ask the service operator to configure a reader with receipt support; do not retry unchanged. Keep any existing burn and quote for recovery.",
		Statuses: []int{422},
	},
	"reader_price_disagreement": {
		Message:  "The readers disagree on the sampled price evidence.",
		Next:     "Wait and retry the same request; nothing was charged by this API attempt. Keep any existing burn and quote; recover outstanding checks with seconded_receipt.",
		Statuses: []int{422},
	},
	"reader_quorum_unavailable": {
		Message:  "The server lacks a quorum of distinct configured readers.",
		Next:     "Ask the service operator to configure at least two distinct valid reader hosts; do not retry unchanged. Keep any existing burn and quote for recovery.",
		Statuses: []int{422},
	},
	"reader_receipt_disagreement": {
		Message:  "The readers disagree on the burn receipt evidence.",
		Next:     "Wait and retry the same request; nothing was charged by this API attempt. Keep any existing burn and quote; recover outstanding checks with seconded_receipt.",
		Statuses: []int{422},
	},
	"receipt_block_changed": {
		Message:  "The burn receipt block changed during verification.",
		Next:     "Wait and retry the same request; nothing was charged by this API attempt. Keep any existing burn and quote; recover outstanding checks with seconded_receipt.",
		Statuses: []int{422},
	},
	"receipt_block_not_canonical": {
		Message:  "The burn receipt block is not currently canonical.",
		Next:     "Wait and retry the same request; nothing was charged by this API attempt. Keep any existing burn and quote; recover outstanding checks with seconded_receipt.",
		Statuses: []int{422},
	},
	"receipt_payer_mismatch": {
		Message:  "The burn transaction sender differs from the quote payer.",
		Next:     "Use the original payer and its matching existing burn transaction; do not retry unchanged. Keep any existing burn and quote for recovery.",
		Statuses: []int{422},
	},
	"receipt_transaction_mismatch": {
		Message:  "The reader returned a receipt for a different transaction.",
		Next:     "Ask the service operator to repair the reader and use the original transaction hash; do not retry unchanged. Keep any existing burn and quote for recovery.",
		Statuses: []int{422},
	},
	"redemption_payer_mismatch": {
		Message:  "The redemption signer does not match the quote payer.",
		Next:     "Sign the original quote and burn transaction with the original payer wallet; do not retry unchanged. Keep any existing burn and quote for recovery.",
		Statuses: []int{422},
	},
	"removed_burn_log": {
		Message:  "The burn log was removed from the canonical chain.",
		Next:     "Wait and retry the same request; nothing was charged by this API attempt. Keep any existing burn and quote; recover outstanding checks with seconded_receipt.",
		Statuses: []int{422},
	},
	"request_timeout": {
		Message:  "The server timed out while reading the request body.",
		Next:     "Wait and retry the same request; nothing was charged by this API attempt. Keep any existing burn and quote; recover outstanding checks with seconded_receipt.",
		Statuses: []int{408},
	},
	"reservation_id_conflict": {
		Message:  "The credit reservation identifier is already bound to different terms.",
		Next:     "Recover the original check and ask the operator to reconcile the reservation; do not retry unchanged. Keep any existing burn and quote for recovery.",
		Statuses: []int{422},
	},
	"sales_off": {
		Message:  "Degen Check sales are disabled.",
		Next:     "Wait for the operator to enable sales and recover the original request before a new check; do not retry unchanged. Keep any existing burn and quote for recovery.",
		Statuses: []int{503},
	},
	"sampled_head_changed": {
		Message:  "The sampled chain head changed during burn verification.",
		Next:     "Wait and retry the same request; nothing was charged by this API attempt. Keep any existing burn and quote; recover outstanding checks with seconded_receipt.",
		Statuses: []int{422},
	},
	"spend_limited": {
		Message:  "The service has no remaining admission budget for this check.",
		Next:     "Recover the original request and wait for the service budget to recover before a new check; do not retry unchanged. Keep any existing burn and quote for recovery.",
		Statuses: []int{429},
	},
	"stale_price_block": {
		Message:  "A price block has the wrong height or an out-of-window timestamp.",
		Next:     "Wait and retry the same request; nothing was charged by this API attempt. Keep any existing burn and quote; recover outstanding checks with seconded_receipt.",
		Statuses: []int{422},
	},
	"store_unavailable": {
		Message:  "The service could not determine or store the operation outcome.",
		Next:     "Recover the original request with seconded_receipt before any new purchase; retry recovery with the same request; do not retry unchanged. Keep any existing burn and quote for recovery.",
		Statuses: []int{503},
	},
	"unknown_quote_key_id": {
		Message:  "The server no longer recognizes the quote signing key.",
		Next:     "Keep the original quote and ask the operator to restore its verification key or guide recovery; do not retry unchanged. Keep any existing burn and quote for recovery.",
		Statuses: []int{422},
	},
	"unsupported_chain": {
		Message:  "The quote chain is invalid or unsupported by the server policy.",
		Next:     "Use a server-issued quote for the configured Robinhood mainnet chain; do not retry unchanged. Keep any existing burn and quote for recovery.",
		Statuses: []int{422},
	},
	"unsupported_network": {
		Message:  "The requested network is unsupported.",
		Next:     "Select an enabled network after recovering any outstanding request; do not retry unchanged. Keep any existing burn and quote for recovery.",
		Statuses: []int{422},
	},
	"unsupported_release_policy": {
		Message:  "The configured chain release policy cannot verify this burn.",
		Next:     "Ask the service operator to configure the supported chain and confirmation policy; do not retry unchanged. Keep any existing burn and quote for recovery.",
		Statuses: []int{422},
	},
	"wallet_frozen": {
		Message:  "The service has frozen check admission for this wallet.",
		Next:     "Ask the service operator to review the wallet freeze and recover outstanding checks; do not retry unchanged. Keep any existing burn and quote for recovery.",
		Statuses: []int{423},
	},
	"wrong_chain": {
		Message:  "A server reader is connected to the wrong chain.",
		Next:     "Ask the service operator to configure readers for Robinhood mainnet chain 4663; do not retry unchanged. Keep any existing burn and quote for recovery.",
		Statuses: []int{422},
	},
	"zero_burn": {
		Message:  "The token burn event has a zero amount.",
		Next:     "Inspect the original transaction and use evidence of a valid nonzero burn without burning again to retry; do not retry unchanged. Keep any existing burn and quote for recovery.",
		Statuses: []int{422},
	},
}

func knownBurnErrorStatus(code string, status int) bool {
	guidance, known := burnErrorMessages[code]
	if !known {
		return false
	}
	for _, allowed := range guidance.Statuses {
		if status == allowed {
			return true
		}
	}
	return false
}
