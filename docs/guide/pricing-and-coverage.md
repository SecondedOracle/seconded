# Pricing and coverage

The bundled catalog, `client/products.json`, defines this checkout's product interface.
The live [products endpoint](https://api.secondedoracle.xyz/v1/products) advertises
availability for the requested client version. See [README Status](../../README.md#status)
for the release and measured counts. Prices below are USD; "—" means the tier is not sold.

## Paid checks

| Product | MCP tool | Small | Medium | Large | Subject chains | Live availability on 2026-10-19 ² |
| --- | --- | ---: | ---: | ---: | --- | --- |
| Trade Check | `seconded_trade_check` | 0.25 | 1.50 | — | Base 8453, Arc 5042, Robinhood 4663 | yes |
| Stock Token Check | `seconded_stock_token_check` | 0.10 ¹ | — | — | Robinhood 4663, Base 8453 | yes |
| Token Check | `seconded_token_check` | 0.10 ¹ | — | — | Base 8453, Arc 5042, Robinhood 4663, Base Sepolia 84532 | yes |
| Agent Registry Check | `seconded_agent_registry_check` | 0.25 | — | — | Base 8453, Arc 5042, Robinhood 4663 | yes |
| Address Screening Check | `seconded_counterparty_check` | 0.10 ¹ | — | — | Base 8453, Arc 5042, Robinhood 4663 | yes |
| Cross-Chain Compare | `seconded_cross_chain_compare` | 0.10 ¹ | — | — | Base (Uniswap V3) and Arc (Uniswap V4), fixed USDC to EURC | yes |
| Scam Check | `seconded_scam_check` | 0.10 ¹ | 1.50 | 2.50 | chain-independent | yes |
| Lending Check | `seconded_lending_check` | 0.25 | — | — | Base 8453, Arc 5042 (Morpho Blue) | yes |
| Agent Work Payout Check | `seconded_job_escrow_check` | 0.25 | — | — | Base 8453 | yes |
| Degen Check | `seconded_degen_check` | 0.10 | — | — | Robinhood 4663 (Pons V2 tokens) | yes; bundled `unavailable` |
| x402 Payment Check | `seconded_x402_payment_check` | 0.10 ¹ | — | — | Base 8453, Arc 5042, Robinhood 4663 | yes |
| Shielded Route Check | `seconded_shielded_route_check` | 0.25 | — | — | Base 8453, Arc 5042, Robinhood 4663 | yes |
| Bridge Route Check | `seconded_route_check` | 0.10 ¹ | — | — | Base 8453, Arc 5042 and Robinhood 4663; typed connected routes of 1–4 hops | yes |

¹ **$0.15 when paying on Robinhood Chain** (`eip155:4663`, testnet `eip155:46630`);
`price_usd_by_network` in the catalog. Scam Check's medium and large tiers are the same
on every network.

² Live availability was measured with `SECONDED-CLIENT-VERSION: 0.4.3`.
Degen stays $0.10 on every payment network; it has no Robinhood surcharge.
The table records advertisement, not successful paid execution on every network.

## Catalog versions and availability

`price_table_version` identifies a catalog snapshot, not the client version or a
price inferred from its date/name. The frozen bundle uses `pt-2026-09-29-small-025`;
the live versioned response measured on 2026-10-19 uses `pt-2026-10-05-v040-network`.
Their product price maps and network overrides match despite those different labels.
Degen is `unavailable` in the bundle and `available` in that live response. The server
must offer a quote before purchase; a bundled tool alone does not guarantee a sale.

Live text can also change separately from the bundle. The measured descriptions differ
for Stock Token, Token, Agent Registry, Shielded Route and Degen checks. Live schemas add
validation constraints for Trade, Agent Work Payout, x402 Payment and Bridge Route.
Use the bundled tool schema to call this client, and the versioned live response for
direct API integration. The frozen catalog remains part of the released source.

An unversioned GET on the same date still advertises eight checks at legacy prices,
including Lending at $0.50. Always send the intended client version when inspecting
availability. [CLAIMS](../CLAIMS.md#current-measurements) records response hashes.

### Historical captures

The [2026-10-06 captures](evidence/README.md) show twelve checks with a client 0.4.1
header and eight without it. `client/products-live-v1.json` is the unchanged 2026-09-30
capture. Neither older capture establishes current availability. The old quote-refusal
fixture records one request; its outer `unknown_product` reason is unsigned.

Subject chains are the `network` or `chainId` enums in each product's `input_schema`.
The catalog also carries coverage text for products that provide it, alongside the
product schemas and descriptions; read these before relying on a verdict. For example Address Screening Check matches
the OFAC SDN digital-currency extract and configured deployments; it does not check scam,
phishing or drainer lists, and a no-match is not proof of safety.

## Size tiers

| Tier | Maximum billable bytes of canonical input |
| --- | ---: |
| small | 8,192 |
| medium | 65,536 |
| large | 131,072 |

## Payment networks

| Network | Chain id | Asset | Pay-to address |
| --- | --- | --- | --- |
| Base (default) | `eip155:8453` | USDC `0x833589fcd6edb6e08f4c7c32d4f71b54bda02913` | `0x010ab46d566cde25cca0ee55eb105e781c7bcf3a` |
| Arc | `eip155:5042` | USDC `0x3600000000000000000000000000000000000000` | `0x010ab46d566cde25cca0ee55eb105e781c7bcf3a` |
| Robinhood Chain | `eip155:4663` | USDG `0x5fc5360d0400a0fd4f2af552add042d716f1d168` | `0x010ab46d566cde25cca0ee55eb105e781c7bcf3a` |

Testnets (Base Sepolia `eip155:84532`, Arc testnet `eip155:5042002`, Robinhood testnet
`eip155:46630`) are used only when a check names them. The three mainnet entries and the
pay-to address are identical in the historical 2026-09-30 capture.
Standard checks use agreement-only billing; Degen signed disagreement may also be
chargeable. See the shared [billing policy](how-it-works.md#billing-policy) before
interpreting the catalog's generic `settle-on-agreement` label.

## Products in testing (not purchasable)

| Product | Bundled and measured live status |
| --- | --- |
| Vault Check | in_testing |
| Private Receive Scan | in_testing |
| Portfolio Check | in_testing |
| Code Review | in_testing |
| Owner Instruction Check | in_testing |
| Hidden Prompt Check | in_testing |

They appear under `unavailable_products` with `purchasable: false`; the client refuses to
quote or buy them. The dated header-aware GET lists all six as unavailable; the historical capture lists
Code Review, Owner Instruction Check and Hidden Prompt Check the same way.

## Free, local privacy tools (in testing)

Seven tools in `client/privacy-tools.json` run inside the client binary at `price_usd`
`0.00` with `execution: agent_local` (one may optionally consult a hosted neutral source).
Their status is `in_testing` and they are not in the published tools list:
`seconded_privacy_shielded_route`, `seconded_privacy_route_check`,
`seconded_privacy_pool_check`, `seconded_private_receive_prepare`,
`seconded_private_purchase_prepare`, `seconded_private_swap_quote`,
`seconded_private_fact_prove`. Nothing in them signs or broadcasts a transaction.
