# Tools reference

The 0.4.2 client exposes **18 MCP tools**: 12 paid checks and 6 utility tools. The exact
`tools/list` response, including every input schema, is recorded in
`client/tools-list.json`; `go run ./internal/cataloggen` from `client/` prints the same
schemas from the compiled catalog without creating a profile.

## Utility tools (free, local or public API)

| Tool | What it does |
| --- | --- |
| `seconded_get_limits` | Show current spending limits and safety switches; null means no owner limit for that window. |
| `seconded_set_limits` | Tighten limits, enable safety switches or freeze payments in chat. Loosening requires the owner-terminal path. |
| `seconded_products` | List the compiled product catalog, prices, coverage and input examples. |
| `seconded_quote` | Preview a check's price without purchasing it. |
| `seconded_receipt` | Collect a known check, or list the twenty newest and older unresolved checks, without creating a new payment authorization. |
| `seconded_wallet` | Show the dedicated wallet's address, balances and funding guidance, or freeze new authorizations. Switching wallets is terminal-only. |

## Paid checks

Paid checks share the four arguments below. `seconded_x402_payment_check` also
accepts the top-level `signing` object defined in its input schema.

| Argument | Required | Meaning |
| --- | --- | --- |
| `input` | yes | The product-specific structured input; its schema is in the catalog entry. |
| `max_price_usd` | no ¹ | Refuse the check if it would cost more. Cannot raise the owner's per-check limit. |
| `network` | no | The chain you pay on: `base` (default), `arc`, `robinhood`, `base_sepolia`, `arc_testnet`, `robinhood_testnet`. |
| `wait_seconds` | no | 0 through 25 in the schema; the MCP client clamps its requested wait to eight seconds. A separate 45-second default soft deadline bounds the tool call; admitted checks can continue afterwards. |

¹ Required when the owner has switched the value gate on.

For x402 checks, supply the actual pending signing context through `signing` when
available; missing authorization context limits the conclusions. Preserve offer
fields exactly.

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
| `seconded_x402_payment_check` | Before authorizing an x402 offer; send the exact offer and, when available, its pending signing context. |
| `seconded_shielded_route_check` | Compare bounded shielded route quotes. |
| `seconded_route_check` | Check a proposed public route for bounded linkability evidence. |

Each tool's full description in `client/tools-list.json` follows one shape: *Use when*,
*Checks*, *Send*, *Answer → do* (the label-to-action map), *Cost*, *Not for*. Agents are
expected to read that description, not this page.

## Results

An agreed result carries the answer label, its mapped action and the signed receipt.
A `no_agreement` receipt has no answer and returns NOT VERIFIED guidance with a
closed reason enum and `pause_or_ask_human`. The client accepts the current message
and a constrained legacy message. Its signed billing status is pending until payment
evidence resolves it. The client rejects contradictory duplicated reply fields.

## Privacy tools (in testing)

Seven free tools in `client/privacy-tools.json` are compiled into the binary but have
status `in_testing` and are not in the published list above. See
[Pricing and coverage](pricing-and-coverage.md#free-local-privacy-tools-in-testing).
