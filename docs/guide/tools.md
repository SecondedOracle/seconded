# Tools reference

See [README Status](../../README.md#status) for the client version and measured tool counts. The exact
`tools/list` response, including every input schema, is recorded in
`client/tools-list.json`; `go run ./internal/cataloggen` from `client/` prints the same
schemas from the compiled catalog without creating a profile.

## Degen Check

`seconded_degen_check` accepts `input.network: eip155:4663` and a Pons V2 token
`input.address`. See [catalog availability and pricing](pricing-and-coverage.md#catalog-versions-and-availability)
for the frozen bundle versus live status; the server must offer a quote before purchase.
Signed `models_disagree` answers can be charged. Review the facts, both opinions
and freshness; disagreement and stale results do not authorize an action.
See the [release notes](client-0.4.4.md) for source provenance and verification boundaries.

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

| Product | Tool | Use when |
| --- | --- | --- |
| Trade Check | `seconded_trade_check` | Before signing a trade, approval, transfer or permit. |
| Stock Token Check | `seconded_asset_check` (`kind=stock_token`) | Before trading a Robinhood or Coinbase Base stock token. |
| Token Check | `seconded_asset_check` (`kind=token`) | Before interacting with a token. |
| Agent Registry Check | `seconded_agent_registry_check` | Before paying or trusting another agent that claims an ERC-8004 identity. |
| Address Screening Check | `seconded_counterparty_check` | Before relying on an address. |
| Cross-Chain Compare | `seconded_cross_chain_compare` | Before selecting a chain for a USDC -> EURC swap. |
| Scam Check | `seconded_scam_check` | Before acting on an incoming message. |
| Lending Check | `seconded_lending_check` | Reviewing one Morpho account and market at a block. |
| Agent Work Payout Check | `seconded_job_escrow_check` | Before funding or taking an agent job. |
| Degen Check | `seconded_degen_check` | Review Pons V2 token facts and both model opinions on Robinhood Chain. |
| x402 Payment Check | `seconded_x402_payment_check` | Before authorizing an x402 offer; send the exact offer and, when available, its pending signing context. |
| Shielded Route Check | `seconded_shielded_route_check` | Compare bounded shielded route quotes. |
| Bridge Route Check | `seconded_route_check` | Check a proposed public route for bounded linkability evidence. |

Check descriptions use sections such as *Use when*, *Checks*, *Send*, *Answer → do*
(the label-to-action map), *Cost* and *Not for*. Their formats vary; only Trade Check
contains all six headings. Utilities use concise descriptions of their own. Agents
should read each full description and schema in `client/tools-list.json` before calling it.

## Results

An agreed result carries the answer label, its mapped action and the signed receipt.
A `no_agreement` receipt has no answer and returns NOT VERIFIED guidance with a
closed reason enum and `pause_or_ask_human`. The client accepts the current message
and a constrained legacy message. Its signed billing status is pending until payment
evidence resolves it. The client rejects contradictory duplicated reply fields.
For Degen, `outcome=models_disagree` can instead carry a chargeable answer with
`label_id=models_disagree` and `option=0`. This is distinct from a standard
`no_agreement` receipt. See [billing policy](how-it-works.md#billing-policy).

## Privacy tools (in testing)

Seven free tools in `client/privacy-tools.json` are compiled into the binary but have
status `in_testing` and are not in the published list above. See
[Pricing and coverage](pricing-and-coverage.md#free-local-privacy-tools-in-testing).

## Asset Check and quote networks in 0.4.4

The 13 raw product contracts use 12 check tools plus six utilities. Asset Check combines token and stock-token inputs: select `input.kind=token` or `stock_token`. Existing direct-call aliases remain accepted, but discovery advertises `seconded_asset_check`. Quotes use `product=token_check` or `stock_token_check` and omit `kind` from `input`.

`seconded_quote.network` selects the payment rail: `base` (default), `arc`, `robinhood`, `base_sepolia`, `arc_testnet` or `robinhood_testnet`. The subject network stays in `input`. Bad utility arguments return `invalid_input` with `charged=no` before wallet access.

Standard disagreement is signed and free. Degen costs $0.10 for a saved signed answer, including `models_disagree`; no answer means no charge. Unsigned 503 diagnostics describe service availability, not authenticated payment state. After independently certified nonpayment, a new purchase can proceed; unresolved payment still requires receipt recovery.
