package client

import (
	"bufio"
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"strconv"
	"strings"
	"sync"
	syncatomic "sync/atomic"
	"time"
)

// Version is stamped by the release builder and reported during MCP initialization.
var Version = ClientVersion

const billingGuarantee = "You are charged only after both models agree and the answer is saved; if delivery fails you can always fetch it with seconded_receipt; no answer → no charge."

const instructions = "SECONDED runs paid two-model checks and returns a verdict label. The verdict binds only to the input you sent, not to any action. Each purchase is signed once; recovery re-posts the identical durable credential and body through the standard door. A 202 or timeout stays pending. For invalid_input with next=correct_input, correct the named field before retrying. After other check tool errors or timeouts, call seconded_receipt with no arguments before checking again: a paid check may still be running. Never ask the user for a private key or seed phrase. Checks pay on Base mainnet (USDC) unless a check names another network; they pay on Arc or Robinhood mainnet, or on a testnet (base_sepolia, arc_testnet, robinhood_testnet), only when the check names it. When you have the unsigned transaction, send it as structured transaction data, not prose. Unconfigured network identities refuse payment."

type Tool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema any             `json:"inputSchema"`
	Annotations map[string]bool `json:"annotations"`
}

func schema(properties map[string]any, required ...string) any {
	s := map[string]any{"type": "object", "properties": properties, "additionalProperties": false}
	if len(required) > 0 {
		s["required"] = required
	}
	return s
}
func stringSchema() any { return map[string]any{"type": "string"} }

// Each deployment embeds its own complete catalogue at compile time.
// Release builds cannot discover staging products through runtime settings.
var productGuideJSON = deploymentProductGuideJSON

type ProductGuide struct {
	ID          string `json:"id"`
	Status      string `json:"status"`
	Tool        string `json:"tool"`
	Description string `json:"description"`
	InputSchema any    `json:"input_schema"`
}

func productGuides() []ProductGuide {
	var guide struct {
		Products []ProductGuide `json:"products"`
	}
	decoder := json.NewDecoder(bytes.NewReader(productGuideJSON))
	decoder.UseNumber() // Schema bounds include uint256 values that float64 cannot preserve.
	if err := decoder.Decode(&guide); err != nil {
		panic(err)
	}
	return guide.Products
}

func Tools() []Tool {
	ids := []string{}
	for _, p := range productGuides() {
		if p.Status == "available" && productOffered(p.ID) {
			ids = append(ids, p.ID)
		}
	}
	product := map[string]any{"type": "string", "enum": ids}
	input := map[string]any{"type": "object"}
	ro := func() map[string]bool { return map[string]bool{"readOnlyHint": true, "openWorldHint": true} }
	tools := []Tool{
		{"seconded_get_limits", "Use when the owner asks about limits or affordability needs missing budget information. Shows current limits and safety switches; null means no user limit. Not a required preflight; every check enforces limits.", schema(map[string]any{}), ro()},
		{"seconded_set_limits", "Change spending limits at the user's request, without a password. For 'set my daily limit to $20', pass {\"daily_usd\":20}. Chat can only tighten limits or freeze. Any increase, removal, safety-switch weakening or unfreeze requires the human terminal. These changes require the owner to run the terminal command returned on refusal. On Macs with Touch ID, owner changes also require biometrics. Otherwise an installed administrator approval key requires an offline signature. Without either, the six-character TTY confirmation does not stop an agent with shell access. There is an absolute $25/day chat ceiling, even when the owner has authorized a higher daily limit in the terminal. Omitted fields stay unchanged. The hard $2.50 per-authorization cap always applies, even if per_check_usd is higher or none. Also changes value_gate, dedupe, loop_brake, alerts and frozen switches.", limitToolSchema(), map[string]bool{"readOnlyHint": false, "destructiveHint": false, "idempotentHint": true, "openWorldHint": false}},
		{"seconded_products", "Use when tool descriptions do not identify the needed check or input shape. With complete input for a matching check, call it directly. Lists prices, examples and answer meanings.", schema(map[string]any{}), ro()},
		{"seconded_quote", "Use when a price preview is requested or needed to decide affordability. Quotes price only; does not assess the action. A fully specified bounded check can be called directly.", schema(map[string]any{"product": product, "input": input, "network": map[string]any{"type": "string", "enum": []string{"base_sepolia", "arc_testnet", "robinhood_testnet", "base", "arc", "robinhood"}, "description": "Payment network to quote; omitted uses the default."}}, "product", "input"), ro()},
		{"seconded_receipt", "Recover a check or recent checks by fetching status only. With no check_id, lists the " + strconv.Itoa(recentReceipts) + " newest checks and any older check still awaiting its receipt, newest first. A result's verification holds the receipt's signed findings and coverage. Never creates a new payment authorization; standard recovery may re-post the identical credential, while legacy recovery signs a read-only ownership proof. After any check error or timeout, call this before checking again. The answer wait can end while payment is pending: continue receipt-only recovery separately until final payment, certified nonpayment, or manual review. Certified nonpayment says not charged and permits retrying the check.", schema(map[string]any{"check_id": map[string]any{"type": "string", "pattern": "^[0-9a-f]{32}$"}}), ro()},
		{"seconded_wallet", "Show the dedicated check wallet address, balances per network and how to fund it, or freeze new authorizations. Wallet switching is terminal-only: seconded-mcp wallet --human --switch back (or newer). Both keys are preserved. Base funding normally becomes available after about a minute, when both readers agree on recent state; Robinhood also uses recent agreement. Final payment confirmation is separate. Pending amounts cannot fund checks yet. Unavailable balances are null, never assumed zero. Never returns a private key; export is a separate human-run CLI command.", schema(map[string]any{"action": map[string]any{"type": "string", "enum": []string{"status", "freeze"}}}), map[string]bool{"readOnlyHint": false, "destructiveHint": false, "openWorldHint": true}},
	}
	for _, p := range productGuides() {
		if p.Status != "available" || !productOffered(p.ID) {
			continue
		}
		properties := map[string]any{
			"input": p.InputSchema, "max_price_usd": map[string]any{"type": "string", "description": "Optional price ceiling in USD; required only when value gate is on (off for new setups). Cannot raise the owner-configured per-check limit (default $2.50), enforced before signing."},
			"network":      map[string]any{"type": "string", "enum": []string{"base_sepolia", "arc_testnet", "robinhood_testnet", "base", "arc", "robinhood"}, "description": "Chain you pay on. Omitted means base (Base mainnet). Other networks only when named: arc, robinhood, or testnets base_sepolia, arc_testnet, robinhood_testnet."},
			"wait_seconds": map[string]any{"type": "integer", "minimum": 0, "maximum": 25},
		}
		if p.ID == "x402_payment_check" {
			properties["signing"] = schema(map[string]any{
				"owner":      map[string]any{"type": "string", "pattern": "^0x[0-9a-fA-F]{40}$"},
				"typed_data": map[string]any{"type": "object", "description": "Exact pending eth_signTypedData_v4 object; no signature or secret. Client decodes and binds it to input."},
				"allowance":  map[string]any{"type": "string", "description": "Any additional approval granted by the pending signing operation, in atomic units."},
			}, "owner", "typed_data")
		}
		tools = append(tools, Tool{p.Tool, p.Description + " New setups: $2.50 max per check, $25 a day; chat only tightens or freezes.", schema(properties, "input"),
			map[string]bool{"readOnlyHint": false, "destructiveHint": false, "idempotentHint": false, "openWorldHint": true}})
	}
	return describeTools(append(tools, privacyTools()...))
}
func catalogue() any {
	var guide map[string]any
	decoder := json.NewDecoder(bytes.NewReader(productGuideJSON))
	decoder.UseNumber()
	if err := decoder.Decode(&guide); err != nil {
		panic(err)
	}
	guide["privacy_tools"] = privacyGuides()
	guide["network"], guide["asset"] = DefaultPaymentNetwork, pins(DefaultPaymentNetwork).Asset
	guide["live_api_configured"] = releaseIdentityConfigured()
	guide["api_transport_identity"] = "web_pki_and_receipt_key_pin"
	return guide
}
func toolProduct(name string) string {
	for _, p := range productGuides() {
		if p.Tool == name && p.Status == "available" && productOffered(p.ID) {
			return p.ID
		}
	}
	return ""
}
func (g *Engine) attachUpdateNotice(result any, now time.Time) any {
	fields := map[string]any{}
	raw, err := json.Marshal(result)
	if err != nil {
		return result
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if decoder.Decode(&fields) != nil || fields == nil {
		return result
	}
	if notice := g.takeUpdateNotice(now); notice != "" {
		fields["update_notice"] = notice
		return fields
	}
	return result
}

func (g *Engine) Call(ctx context.Context, name string, args json.RawMessage) (result any, err error) {
	if err := validateToolInput(name, args); err != nil {
		return toolFailure(nil, err), err
	}
	if name == "seconded_receipt" || toolProduct(name) != "" {
		var cancel context.CancelFunc
		ctx, cancel = checkContext(ctx)
		defer cancel()
	}
	if isPrivacyName(name) {
		return callPrivacy(ctx, g, name, args)
	}
	signal := &syncatomic.Bool{}
	ctx = context.WithValue(ctx, updateSignalKey{}, signal)
	defer func() {
		if !signal.Load() {
			return
		}
		result = g.attachUpdateNotice(result, time.Now())
	}()
	// Receipt recovery must remain usable when the wallet secret is unavailable.
	// Defer policy/discovery notices to the next control call instead of opening
	// the key store merely to attach an announcement to a recovery response.
	if name == "seconded_receipt" {
		result, err = g.call(ctx, name, args)
		if err != nil {
			result = toolFailure(result, err)
		}
		return result, err
	}
	defer func() {
		notices, noticeErr := g.takePolicyNotices()
		if noticeErr != nil && err == nil {
			err = noticeErr
		}
		if len(notices) != 0 {
			fields := map[string]any{}
			if result != nil {
				raw, _ := json.Marshal(result)
				decoder := json.NewDecoder(bytes.NewReader(raw))
				decoder.UseNumber()
				_ = decoder.Decode(&fields)
			}
			if fields == nil {
				fields = map[string]any{}
			}
			fields["policy_changes"] = notices
			result = fields
		}
		if err != nil {
			result = toolFailure(result, err)
		}
	}()
	discoveryErr := g.discoverEarlierWallet(ctx)
	result, err = g.call(ctx, name, args)
	notice, noticeErr := g.takeEarlierWalletNotice(ctx)
	if discoveryErr != nil {
		notice = StorageErrorMessage(discoveryErr)
	}
	if noticeErr != nil {
		if err != nil {
			return result, err
		}
		return result, noticeErr
	}
	if notice == "" {
		return result, err
	}
	fields := map[string]any{}
	if err != nil {
		fields = toolFailure(result, err)
	} else {
		raw, marshalErr := json.Marshal(result)
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.UseNumber()
		if marshalErr != nil || decoder.Decode(&fields) != nil {
			return nil, ErrInvalid
		}
	}
	fields["earlier_wallet_notice"] = notice
	return fields, err
}

func (g *Engine) call(ctx context.Context, name string, args json.RawMessage) (any, error) {
	if g == nil && name != "seconded_products" {
		return nil, ErrSetupRequired
	}
	if name != "seconded_products" && name != "seconded_receipt" && name != "seconded_get_limits" && name != "seconded_set_limits" {
		if err := g.Refresh(ctx); err != nil {
			return nil, err
		}
	}
	if len(args) == 0 {
		args = []byte("{}")
	}
	checkTool := toolProduct(name) != ""
	switch {
	case name == "seconded_get_limits" || name == "seconded_set_limits":
		return g.Limits(args, name == "seconded_set_limits")
	case name == "seconded_products":
		var a struct{}
		if e := DecodeStrict(args, &a, MessageLimit); e != nil {
			return nil, e
		}
		return catalogue(), nil
	case name == "seconded_quote" || checkTool:
		var a struct {
			Product string              `json:"product,omitempty"`
			Input   json.RawMessage     `json:"input"`
			Max     string              `json:"max_price_usd,omitempty"`
			Network string              `json:"network,omitempty"`
			Wait    *int                `json:"wait_seconds,omitempty"`
			Signing *X402SigningRequest `json:"signing,omitempty"`
		}
		if e := DecodeStrict(args, &a, MessageLimit); e != nil {
			return nil, e
		}
		network, known := map[string]string{"": DefaultPaymentNetwork, "base_sepolia": Network, "arc_testnet": "eip155:5042002", "robinhood_testnet": "eip155:46630", "base": "eip155:8453", "arc": "eip155:5042", "robinhood": "eip155:4663"}[a.Network]
		if !known {
			return nil, ErrInvalid
		}
		var fields map[string]json.RawMessage
		if json.Unmarshal(args, &fields) != nil {
			return nil, ErrInvalid
		}
		if name != "seconded_quote" {
			if _, supplied := fields["product"]; supplied {
				return nil, ErrInvalid
			}
			a.Product = toolProduct(name)
		}
		if err := validateAddressCharacters(args); err != nil {
			return nil, err
		}
		if _, supplied := fields["signing"]; supplied {
			if name != "seconded_x402_payment_check" || a.Signing == nil {
				return nil, ErrInvalid
			}
		}
		if a.Product == "x402_payment_check" {
			// Only this check call may derive a context from the raw wallet request.
			var err error
			a.Input, err = bindX402SigningContext(a.Input, nil)
			if err != nil {
				return nil, err
			}
		}
		req := Request{a.Product, a.Input, Options{Network: network}}
		var normalizeErr error
		req, normalizeErr = normalizePortfolioRequest(req)
		if normalizeErr != nil {
			return nil, normalizeErr
		}
		if _, e := req.Size(); e != nil {
			return nil, inputError("input", "Use valid input matching the product schema from seconded_products.")
		}
		if name == "seconded_quote" {
			if a.Max != "" || a.Wait != nil {
				return nil, ErrInvalid
			}
			return g.Quote(ctx, req)
		}
		wait := 8
		if a.Wait != nil {
			if *a.Wait < 0 || *a.Wait > 25 {
				return nil, ErrInvalid
			}
			wait = *a.Wait
			if wait > 8 {
				wait = 8
			}
		}
		if a.Max != "" {
			req.Options.MaxPrice = a.Max
		}
		return g.check(ctx, req, a.Max, wait, a.Signing)
	case name == "seconded_receipt":
		var a struct {
			ID string `json:"check_id,omitempty"`
		}
		if e := DecodeStrict(args, &a, MessageLimit); e != nil {
			return nil, e
		}
		out, e := g.Receipts(ctx, a.ID)
		if e != nil {
			return nil, e
		}
		return map[string]any{"checks": out}, nil
	case name == "seconded_wallet":
		var a struct {
			Action string `json:"action,omitempty"`
		}
		if e := DecodeStrict(args, &a, MessageLimit); e != nil {
			return nil, e
		}
		if a.Action == "switch_back" || a.Action == "switch_newer" {
			return nil, ErrTerminalPolicyRequired
		}
		return g.Wallet(ctx, a.Action)
	default:
		return nil, ErrInvalid
	}
}

// Only fixed public codes and locally authored guidance may cross the error boundary.
var publicErrorMessages = map[string]string{
	"nonpayment_limited": "This wallet has a certified unpaid check in the last 24 hours. Wait for the returned retry interval and recover the receipt before trying again.",
	"first_paid_small_required": "Complete one paid check at this product's small tier before buying a larger tier. Recover the receipt before trying again.",
	"agreed_abstain_label":                                 "The judges agreed that the check could not be verified. Pause and ask a human before acting.",
	"bridge_classification_unverified":                     "A route contract has no verified bridge classification. Pause and ask a human before acting.",
	"counterparty_list_future_or_inconsistent_source_date": "The counterparty list source dates are inconsistent. Pause and ask a human before acting.",
	"counterparty_list_partial_snapshot":                   "The counterparty list snapshot has only partial coverage. Pause and ask a human before acting.",
	"counterparty_list_snapshot_coverage_unsupported":      "The counterparty list snapshot coverage is unsupported. Pause and ask a human before acting.",
	"counterparty_list_snapshot_kind_unsupported":          "The counterparty list snapshot kind is unsupported. Pause and ask a human before acting.",
	"counterparty_list_snapshot_malformed":                 "The counterparty list snapshot is malformed. Pause and ask a human before acting.",
	"counterparty_list_snapshot_unavailable":               "The counterparty list snapshot is unavailable. Pause and ask a human before acting.",
	"counterparty_list_stale_list":                         "The counterparty list is stale. Pause and ask a human before acting.",
	"counterparty_list_unavailable":                        "The counterparty list is unavailable. Pause and ask a human before acting.",
	"detected_hazard":                                      "A checked hazard was detected, but no agreed warning answer was produced. Pause and ask a human before acting.",
	"funding_history_cross_chain_endpoints":                "Funding history across different chains is not checked. Pause and ask a human before acting.",
	"funding_history_native_history_unmeasured":            "Native-asset funding history is not measured. Pause and ask a human before acting.",
	"funding_history_transfer_log_limit":                   "Funding history exceeded the bounded transfer-log limit. Pause and ask a human before acting.",
	"funding_history_unavailable":                          "Funding history is unavailable. Pause and ask a human before acting.",
	"response_incomplete":                                  "A provider response was truncated or incomplete. Pause and ask a human before acting.",
	"unsupported_oracle":                                   "The lending market uses an oracle outside the reviewed coverage. Pause and ask a human before acting.",

	"non_ascii_address":         "Address and identifier inputs must contain ASCII characters only. Remove invisible or confusable characters and supply the exact original address; do not substitute a lookalike.",
	"insufficient_balance":      "The service refused the check before charging because the available balance was insufficient. Recover the receipt after the authorization expires before trying again.",
	"disagreement_limited":      "The service refused the check before charging because the daily disagreement limit was reached. Wait for the limit and the authorization to expire before trying again.",
	"disagreed":                 "The models disagreed. Pause and ask a human before acting.",
	"model_declined":            "A model declined the check. Pause and ask a human before acting.",
	"timed_out":                 "The check timed out without verified agreement. Recover its receipt and pause before acting.",
	"contradicts_verified_fact": "The proposed conclusion contradicts a verified fact. Pause and ask a human to review it.",
	"evidence_unbound":          "The evidence could not be bound to this input. Pause and review the subject with a human.",
	"hidden_text_unresolved":    "Hidden text could not be resolved. Pause and ask a human to review the input.",

	"stock_registry_unverified_for_chain": "The stock registry is not verified for this subject chain. Choose a supported mainnet subject from seconded_products.",
	"unknown_product":                     "Choose a supported product from seconded_products.",
	"input_not_canonicalizable":           "Use valid JSON input matching the product schema.",
	"max_price":                           "Supply a nonnegative decimal max_price_usd after reviewing the quote.",
	"bad_content_length":                  "The request length was invalid. Check the client installation and retry after receipt recovery.",
	"malformed_json":                      "Supply valid JSON matching the tool schema.",
	"body_not_object":                     "Supply a JSON object matching the tool schema.",
	"product_required":                    "Choose a product from seconded_products.",
	"input_required":                      "Supply the required input object from the product schema.",
	"options_not_object":                  "Supply options as a JSON object matching the request schema.",
	"unknown_option":                      "Remove unsupported request options.",
	"product_in_testing":                  "This product is not available for paid checks. Choose a supported product from seconded_products.",
	"unsupported_network":                 "The selected payment network is unavailable. Choose an enabled payment network; subject selection is separate.",
	"price_exceeds_max":                   "The service price exceeds the requested ceiling. Review the quote before changing max_price_usd.",

	"invalid_response":                          "The input or service response could not be validated. Check the tool input and obtain a reviewed client update if this persists.",
	"input_too_large":                           "Shorten the input to fit the product's size limit.",
	"frozen_local":                              "New payments are frozen. The owner must unfreeze them in a terminal; seconded_set_limits with frozen=false returns the exact command.",
	"terminal_policy_required":                  "This policy change requires the owner to run seconded-mcp limits --human --patch in a controlling terminal.",
	"policy_mismatch":                           "The payment terms do not match the local policy. Stop and obtain a reviewed client or service update.",
	"limit_per_check":                           "The price exceeds the per-check limit. Ask the owner to review the limit; the hard authorization cap still applies.",
	"limit_hour":                                "The hourly spending limit is reached. Wait for headroom or ask the owner to review the limit.",
	"limit_day":                                 "The daily spending limit is reached. Wait for headroom or ask the owner to review the limit.",
	"limit_outstanding":                         "Outstanding payments reached the limit. Recover receipts and wait for finalized evidence.",
	"loop_brake":                                "The repeated-check safety limit is reached. Pause and review the checks before continuing.",
	"price_above_max":                           "The price exceeds max_price_usd. Review a quote and supply an acceptable price ceiling.",
	"value_gate_requires_max_price":             "Supply max_price_usd after reviewing the quoted price.",
	"release_identity_not_configured":           "Install a reviewed client release with configured service and receipt identities.",
	"budget_blocked_on_chain_evidence":          "Reliable chain evidence is unavailable. Check seconded_wallet again shortly. If this continues, update the client or contact support before retrying.",
	"wallet_unfunded":                           "Fund the displayed wallet on the selected payment network, then check its balance again. Base funding normally becomes available after about a minute through agreement from both readers; final payment confirmation is separate.",
	"api_unreachable":                           "The service could not be reached. Check connectivity and recover receipts before trying again.",
	"api_identity_mismatch":                     "The service identity could not be verified. Stop and obtain a reviewed client update.",
	"receipt_key_pin_mismatch":                  "The receipt key does not match this client. Stop and obtain a reviewed key-rotation update.",
	"unknown_check":                             "Use a check_id from this wallet, or call seconded_receipt without arguments to list recent checks.",
	"duplicate_authorization":                   "A payment authorization already exists. Call seconded_receipt; do not sign another payment.",
	"local_storage_unavailable":                 "Check the private profile's permissions and installation metadata. Do not share wallet files or keys.",
	"payer_has_code_eip7702_or_contract_wallet": "The payer is not a supported plain wallet on this network. Use a dedicated supported check wallet.",
	"payer_code_check_unavailable":              "The wallet type could not be verified. Wait for chain readers to recover before trying again.",
	"earlier_wallet_not_found":                  "No earlier wallet was found. Check the active wallet address before funding it.",
	"earlier_wallet_changed":                    "The earlier wallet changed. Stop and review the retained wallet profiles with the owner.",
	"keystore_unavailable":                      "Restore access to the operating system credential store and retry. Do not paste a private key into chat.",
	"keystore_data_invalid":                     "The stored credential is invalid. Use owner-managed wallet recovery; do not overwrite or share the key.",
	"setup_required":                            SetupRequiredMessage,
	"client_upgrade_required":                   "Install a reviewed newer client release before checking again.",
	"signing_failed":                            "Local signing failed. Check the wallet installation with self-check; do not share the key.",
	"evidence_identity_not_configured":          "Install a reviewed client with independent chain readers configured.",
	"chain_evidence_disagreement":               "The chain readers disagree. Wait for agreement; pending funds are not spendable evidence.",
	"chain_unavailable":                         "A chain reader is unavailable. Wait and check the wallet again.",
	"chain_stale":                               "The chain evidence is too old. Wait for fresh evidence from both readers; a valid finalized anchor is still required.",
	"chain_history_unavailable":                 "Required chain history is unavailable. Recover receipts and ask the operator to review the reader history.",
	"network_not_enabled":                       "Checks inspect mainnet transactions; pass network=base|arc|robinhood for the subject inside input. You can still pay on a testnet using the tool's outer network field.",
	"input_not_object":                          "Pass input as a JSON object matching the product schema.",
	"unknown_field":                             "Remove unknown input fields using the schema from seconded_products.",
	"missing_field":                             "Supply the required input fields listed by seconded_products.",
	"invalid_field":                             "Correct the input fields using the schema from seconded_products.",
	"input_too_deep":                            "Simplify the nested input to fit the product schema.",
	"input_tokens_exceed_cap":                   "Shorten the input to fit the product's token limit.",
	"invalid_address":                           "Supply a valid public address on the subject network.",
	"product_unknown":                           "Choose a supported product from seconded_products.",
	"product_unpriced":                          "Choose a priced product from seconded_products.",
	"invalid_tier":                              "Use a supported input size and product tier from seconded_products.",
	"invalid_input":                             "Correct the input using the schema from seconded_products.",
	"store_unavailable":                         "The service store is unavailable. Wait and recover receipts before trying again.",
	"rate_limited":                              "Pause before trying again; recover any existing check first.",
	"operation_failed":                          "The operation failed. Call seconded_receipt before another check, then review setup and connectivity if the problem persists.",
	"presentation_indeterminate":                "Payment may have been sent. Call seconded_receipt to recover; do not submit another check.",
	"engine_error":                              "The service could not finish the check. Recover its receipt before deciding whether to try again.",
	"prefetch_unavailable":                      "Required evidence could not be fetched. Recover the receipt and wait for evidence availability.",
	"provider_unavailable":                      "A model provider was unavailable. Recover the receipt before trying again.",
	"service_failed":                            "The service could not finish the check. Recover its receipt before trying again.",
	"refused":                                   "The service refused the check. Review the input and ask a human before continuing.",
	"content_refused":                           "The service refused this content. Review the input and ask a human before continuing.",
	"frozen_unsettled":                          "The check is frozen with settlement unresolved. Recover the receipt; do not send another payment.",
	"closed_no_charge":                          "Not charged. Retry the check when ready.",
}

func safeError(e error) string {
	if e == nil {
		return ""
	}
	for _, known := range []error{ErrKeystoreData, ErrStorage, ErrSetupRequired, ErrTerminalPolicyRequired} {
		if errors.Is(e, known) {
			return known.Error()
		}
	}
	if _, ok := publicErrorMessages[e.Error()]; ok {
		return e.Error()
	}
	return "operation_failed"
}

func failedResult(r Result, err error) Result {
	r.Status = "failed"
	r.Reason = safeError(err)
	r.Message = publicErrorMessages[r.Reason]
	if message := evidenceFailureMessage(err); message != "" {
		r.Message = message
	}
	r.Next = "call_receipt_after"
	var inputErr *localInputError
	if errors.As(err, &inputErr) {
		r.Message, r.Next, r.Charged = inputErr.message, "correct_input", "no"
	}
	if r.Reason == "setup_required" {
		r.Next = "run_setup"
	}
	if r.Charged == "" {
		r.Charged = "not_yet_known"
	}
	if r.Receipt == "" {
		r.Receipt = "none"
	}
	if r.CheckedBy == nil {
		r.CheckedBy = []string{}
	}
	if r.Notices == nil {
		r.Notices = []string{}
	}
	return r
}

func toolFailure(result any, err error) map[string]any {
	fields := map[string]any{}
	// Preserve check IDs and verified billing facts even if a later operation failed.
	if result != nil {
		if b, e := json.Marshal(result); e == nil {
			_ = json.Unmarshal(b, &fields)
		}
	}
	if fields == nil {
		fields = map[string]any{}
	}
	r := failedResult(Result{}, err)
	fields["status"], fields["reason"], fields["message"], fields["next"] = r.Status, r.Reason, r.Message, r.Next
	var inputErr *localInputError
	if errors.As(err, &inputErr) {
		fields["field"] = inputErr.field
	}
	var policyErr *terminalPolicyError
	if errors.As(err, &policyErr) {
		fields["message"], fields["terminal_command"], fields["next"] = policyErr.Error(), policyErr.command, "run_in_terminal"
	}
	for name, fallback := range map[string]string{"charged": r.Charged, "receipt": r.Receipt} {
		if value, ok := fields[name].(string); !ok || value == "" {
			fields[name] = fallback
		}
	}
	return fields
}

type rpcRequest struct {
	Version string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// MCP envelopes are extensible; payment and tool argument schemas are closed.
// Validate the entire wire value before projecting known fields so extensions
// cannot bypass duplicate-key, Unicode, nesting or size checks.
func decodeMCP(data []byte, dst any, limit int) error {
	if err := checkJSON(data, limit); err != nil {
		return err
	}
	projected, err := mcpKnownFields(data, reflect.TypeOf(dst).Elem())
	if err != nil {
		return err
	}
	return DecodeStrict(projected, dst, limit)
}

func mcpKnownFields(data []byte, shape reflect.Type) ([]byte, error) {
	if shape.Kind() != reflect.Struct {
		return data, nil
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(data, &fields) != nil || fields == nil {
		return nil, ErrInvalid
	}
	known := make(map[string]json.RawMessage)
	for i := 0; i < shape.NumField(); i++ {
		field := shape.Field(i)
		name, _, _ := strings.Cut(field.Tag.Get("json"), ",")
		if value, ok := fields[name]; ok {
			projected, err := mcpKnownFields(value, field.Type)
			if err != nil {
				return nil, err
			}
			known[name] = projected
		}
	}
	var projected bytes.Buffer
	encoder := json.NewEncoder(&projected)
	// HTML escaping would inflate valid host messages before the next size check.
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(known); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(projected.Bytes(), []byte("\n")), nil
}

func validID(id json.RawMessage) bool {
	if len(id) == 0 || string(id) == "null" {
		return false
	}
	var v any
	if DecodeStrict(id, &v, 1024) != nil {
		return false
	}
	switch v.(type) {
	case string, json.Number:
		return true
	}
	return false
}

func Serve(ctx context.Context, in io.Reader, out io.Writer, g *Engine) error {
	return ServeWithSetup(ctx, in, out, g, nil)
}

// ServeWithSetup defers first-run wallet creation until a wallet-dependent tool is called.
// The first call returns funding instructions only, even if it requested a check.
// Initialization, tool discovery and the free catalog never create or fund a wallet.
func ServeWithSetup(ctx context.Context, in io.Reader, out io.Writer, g *Engine, setup func() (*Engine, Installation, error)) error {
	ctx, cancelAll := context.WithCancel(ctx)
	defer cancelAll()
	privacy := &privacyProcess{}
	ctx = context.WithValue(ctx, privacySessionKey{}, privacy)
	defer privacy.close()
	var refreshWG sync.WaitGroup
	startRefresh := func(engine *Engine, immediately bool) {
		if engine == nil {
			return
		}
		refreshWG.Add(1)
		go func() {
			defer refreshWG.Done()
			ticker := time.NewTicker(30 * time.Second)
			defer ticker.Stop()
			for {
				if !immediately {
					select {
					case <-ctx.Done():
						return
					case <-ticker.C:
					}
				}
				pollCtx, done := context.WithTimeout(ctx, 8*time.Second)
				_ = engine.Refresh(pollCtx)
				done()
				immediately = false
			}
		}()
	}
	startRefresh(g, true)

	var writeMu sync.Mutex
	var wg sync.WaitGroup
	var cancelMu sync.Mutex
	cancels := map[string]context.CancelFunc{}
	slots := make(chan struct{}, 4)
	send := func(id json.RawMessage, result any, code int, message string) {
		writeMu.Lock()
		defer writeMu.Unlock()
		resp := map[string]any{"jsonrpc": "2.0", "id": id}
		if code != 0 {
			resp["error"] = map[string]any{"code": code, "message": message}
		} else {
			resp["result"] = result
		}
		_ = json.NewEncoder(out).Encode(resp)
	}
	initialized := false
	ready := false
	firstTool := true
	version := ""
	scan := bufio.NewScanner(in)
	scan.Buffer(make([]byte, 4096), MessageLimit+1)
	for scan.Scan() {
		line := append([]byte{}, scan.Bytes()...)
		if len(line) > MessageLimit {
			send(json.RawMessage("null"), nil, -32700, "Invalid JSON")
			continue
		}
		var req rpcRequest
		if decodeMCP(line, &req, MessageLimit) != nil || req.Version != "2.0" || req.Method == "" {
			send(json.RawMessage("null"), nil, -32600, "Invalid request")
			continue
		}
		if len(req.ID) == 0 {
			switch req.Method {
			case "notifications/initialized":
				if initialized {
					ready = true
				}
			case "notifications/cancelled":
				var p struct {
					ID     json.RawMessage `json:"requestId"`
					Reason string          `json:"reason,omitempty"`
				}
				if decodeMCP(req.Params, &p, ResponseLimit) == nil && validID(p.ID) {
					cancelMu.Lock()
					if c := cancels[string(p.ID)]; c != nil {
						c()
					}
					cancelMu.Unlock()
				}
			}
			continue
		}
		if !validID(req.ID) {
			send(json.RawMessage("null"), nil, -32600, "Invalid request ID")
			continue
		}
		switch req.Method {
		case "initialize":
			var p struct {
				Protocol     string                     `json:"protocolVersion"`
				Capabilities map[string]json.RawMessage `json:"capabilities"`
				Client       struct {
					Name    string `json:"name"`
					Version string `json:"version"`
					Title   string `json:"title,omitempty"`
				} `json:"clientInfo"`
			}
			if initialized || decodeMCP(req.Params, &p, ResponseLimit) != nil || p.Client.Name == "" || p.Protocol == "" {
				send(req.ID, nil, -32602, "Invalid initialize parameters")
				continue
			}
			version = p.Protocol
			if !oneOf(version, "2024-11-05", "2025-03-26", "2025-06-18", "2025-11-25") {
				version = "2025-11-25"
			}
			initialized = true
			guidance := instructions + " " + billingGuarantee
			if g == nil {
				guidance = SetupRequiredMessage + " " + guidance
			}
			send(req.ID, map[string]any{"protocolVersion": version, "capabilities": map[string]any{"tools": map[string]any{}}, "serverInfo": map[string]string{"name": "seconded-mcp", "version": Version}, "instructions": guidance}, 0, "")
		case "ping":
			send(req.ID, map[string]any{}, 0, "")
		case "tools/list":
			if !ready {
				send(req.ID, nil, -32002, "Initialize first")
				continue
			}
			if len(req.Params) > 0 {
				var p struct {
					Cursor string `json:"cursor,omitempty"`
				}
				if decodeMCP(req.Params, &p, ResponseLimit) != nil || p.Cursor != "" {
					send(req.ID, nil, -32602, "Invalid parameters")
					continue
				}
			}
			send(req.ID, map[string]any{"tools": Tools()}, 0, "")
		case "tools/call":
			if !ready {
				send(req.ID, nil, -32002, "Initialize first")
				continue
			}
			var p struct {
				Name      string          `json:"name"`
				Arguments json.RawMessage `json:"arguments,omitempty"`
				Meta      json.RawMessage `json:"_meta,omitempty"`
			}
			if decodeMCP(req.Params, &p, MessageLimit) != nil {
				send(req.ID, nil, -32602, "Invalid parameters")
				continue
			}
			if len(p.Arguments) > 0 {
				var arguments map[string]json.RawMessage
				if json.Unmarshal(p.Arguments, &arguments) != nil || arguments == nil {
					send(req.ID, nil, -32602, "Tool arguments must be an object")
					continue
				}
			}
			known := false
			for _, t := range Tools() {
				if t.Name == p.Name {
					known = true
				}
			}
			if !known {
				send(req.ID, nil, -32602, "Unknown tool")
				continue
			}
			if err := validateToolInput(p.Name, p.Arguments); err != nil {
				b, _ := json.Marshal(toolFailure(nil, err))
				payload := map[string]any{"content": []any{map[string]any{"type": "text", "text": string(b)}}, "isError": true}
				if oneOf(version, "2025-06-18", "2025-11-25") {
					payload["structuredContent"] = json.RawMessage(b)
				}
				send(req.ID, payload, 0, "")
				continue
			}
			if g == nil && setup != nil && !isPrivacyName(p.Name) && p.Name != "seconded_products" {
				engine, install, err := setup()
				result := map[string]any{"status": "funding_required", "address": install.Address, "message": WalletCreatedMessage(install.Address), "funding_instructions": FundingInstructions, "next": "fund_wallet", "default_network": DefaultPaymentNetwork}
				if err != nil {
					result = map[string]any{"status": "failed", "reason": safeSetupError(err), "next": "run_setup", "message": SetupRequiredMessage, "charged": "no", "receipt": "none"}
					if errors.Is(err, ErrStorage) || errors.Is(err, ErrKeystoreData) {
						result["message"] = StorageErrorMessage(err)
					}
				} else {
					result["message"] = WalletFundingMessage(install, engine.Files.Dir)
					g = engine
					firstTool = false
					// First return the funding result; later ticks reconcile normally.
					startRefresh(engine, false)
				}
				b, _ := json.Marshal(result)
				payload := map[string]any{"content": []any{map[string]any{"type": "text", "text": string(b)}}, "isError": err != nil}
				if oneOf(version, "2025-06-18", "2025-11-25") {
					payload["structuredContent"] = json.RawMessage(b)
				}
				send(req.ID, payload, 0, "")
				continue
			}
			select {
			case slots <- struct{}{}:
			default:
				send(req.ID, nil, -32000, "Too many requests")
				continue
			}
			callCtx, cancel := context.WithTimeout(ctx, toolTimeout(p.Name))
			key := string(req.ID)
			cancelMu.Lock()
			if _, exists := cancels[key]; exists {
				cancelMu.Unlock()
				cancel()
				<-slots
				send(req.ID, nil, -32600, "Duplicate request ID")
				continue
			}
			cancels[key] = cancel
			cancelMu.Unlock()
			wg.Add(1)
			notice := ""
			if firstTool && g != nil && !isPrivacyName(p.Name) {
				if raw, err := g.Files.Read("install.json"); err == nil {
					var install Installation
					if DecodeStrict(raw, &install, ResponseLimit) == nil && addressPattern.MatchString(install.Address) {
						notice = WalletExistingMessage(install, g.Files.Dir)
					}
				}
				firstTool = false
			}
			go func(g *Engine, id json.RawMessage, name string, args json.RawMessage, protocol, notice string) {
				defer wg.Done()
				defer func() { cancel(); cancelMu.Lock(); delete(cancels, key); cancelMu.Unlock(); <-slots }()
				result, e := g.Call(callCtx, name, args)
				isError := e != nil
				if e != nil {
					result = toolFailure(result, e)
				}
				b, e := json.Marshal(result)
				if e != nil {
					b, _ = json.Marshal(failedResult(Result{}, errors.New("operation_failed")))
					isError = true
				}
				if notice != "" {
					// Discovery or a switch may have changed the selected wallet.
					if install, err := readInstallation(g.Files); err == nil {
						notice = WalletExistingMessage(install, g.Files.Dir)
					}
					var fields map[string]json.RawMessage
					if json.Unmarshal(b, &fields) == nil && fields != nil {
						fields["wallet_notice"], _ = json.Marshal(notice)
						b, _ = json.Marshal(fields)
					}
				}
				payload := map[string]any{"content": []any{map[string]any{"type": "text", "text": string(b)}}, "isError": isError}
				if oneOf(protocol, "2025-06-18", "2025-11-25") {
					payload["structuredContent"] = json.RawMessage(b)
				}
				send(id, payload, 0, "")
			}(g, req.ID, p.Name, p.Arguments, version, notice)
		default:
			send(req.ID, nil, -32601, "Method not found")
		}
	}
	cancelAll()
	wg.Wait()
	refreshWG.Wait()
	return scan.Err()
}

// Setup errors are public fixed strings; never return arbitrary store errors.
func safeSetupError(err error) string {
	s := err.Error()
	if oneOf(s, "release_digest_mismatch", "release_signature_required", "release_signature_invalid", "release_signature_unavailable", "release_signer_not_configured", "keystore_required_use_allow_file_key", "wallet_already_exists", "wallet_recovery_required", "fingerprint_mismatch") {
		return s
	}
	return safeError(err)
}
