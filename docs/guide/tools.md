# Tools reference

The 0.4.1 client exposes **18 MCP tools**: 12 paid checks and 6 utility tools. The exact
`tools/list` response, including every input schema, is recorded in
`client/tools-list.json`; `go run ./internal/cataloggen` from `client/` prints the same
schemas from the compiled catalog without creating a profile.

## Utility tools (free, local or public API)

| Tool | What it does |
| --- | --- |
| `seconded_get_limits` | Use when the owner asks about limits or affordability needs missing budget information. Shows current limits and safety switches; null means no user limit. Not  |
| `seconded_set_limits` | Change spending limits at the user's request, without a password. For 'set my daily limit to $20', pass {"daily_usd":20}. Chat can only tighten limits or freeze |
| `seconded_products` | Use when tool descriptions do not identify the needed check or input shape. With complete input for a matching check, call it directly. Lists prices, examples a |
| `seconded_quote` | Use when a price preview is requested or needed to decide affordability. Quotes price only; does not assess the action. A fully specified bounded check can be c |
| `seconded_receipt` | Recover a check or recent checks by fetching status only. With no check_id, lists the 20 newest checks and any older check still awaiting its receipt, newest fi |
| `seconded_wallet` | Show the dedicated check wallet address, balances per network and how to fund it, or freeze new authorizations. Wallet switching is terminal-only: seconded-mcp  |

## Paid checks

Every check tool takes the same four arguments:

| Argument | Required | Meaning |
| --- | --- | --- |
| `input` | yes | The product-specific structured input; its schema is in the catalog entry. |
| `max_price_usd` | no ¹ | Refuse the check if it would cost more. Cannot raise the owner's per-check limit. |
| `network` | no | The chain you pay on: `base` (default), `arc`, `robinhood`, `base_sepolia`, `arc_testnet`, `robinhood_testnet`. |
| `wait_seconds` | no | 0 to 25. Longer checks return a `check_id` to collect with `seconded_receipt`. |

¹ Required when the owner has switched the value gate on.

| Tool | Use when |
| --- | --- |
| `seconded_trade_check` | Before signing a trade, approval, transfer or permit. |
| `seconded_stock_token_check` | Before trading a Robinhood or Coinbase Base stock token. |
| `seconded_token_check` | Before interacting with a token. |
| `seconded_agent_registry_check` | Before paying or trusting another agent that claims an ERC-8004 identity. |
| `seconded_counterparty_check` | Before relying on an address. |
| `seconded_cross_chain_compare` | Before selecting a chain for a USDC -> EURC swap. |
| `seconded_scam_check` | Before acting on an incoming message. |
| `seconded_lending_check` | Reviewing one Morpho account and market at a block. |
| `seconded_job_escrow_check` | Before funding or taking an agent job. |
| `seconded_x402_payment_check` | Before authorizing an x402 offer. Send: one input object; never input.input. Copy offer fields exactly; do not rename amount. Supply actual pending ty |
| `seconded_shielded_route_check` | Compare bounded shielded route quotes. |
| `seconded_route_check` | Check a proposed public route for bounded linkability evidence. |

Each tool's full description in `client/tools-list.json` follows one shape: *Use when*,
*Checks*, *Send*, *Answer → do* (the label-to-action map), *Cost*, *Not for*. Agents are
expected to read that description, not this page.

## Results

A check result carries the answer label, the action the catalog maps it to, and the
signed receipt. A `no_agreement` receipt produces `status: "not_verified"`, the fixed
`NOT VERIFIED` message, a closed `reason` enum and `next: "pause_or_ask_human"`; there is no
verdict and nothing was charged. The client rejects a result whose top-level fields
contradict its signed envelope.

## Privacy tools (in testing)

Seven free tools in `client/privacy-tools.json` are compiled into the binary but have
status `in_testing` and are not in the published list above. See
[Pricing and coverage](pricing-and-coverage.md#free-local-privacy-tools-in-testing).
