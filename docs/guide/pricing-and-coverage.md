# Pricing and coverage

Source of truth: `client/products.json` (the catalog compiled into client 0.4.1,
`price_table_version` `pt-2026-09-29-small-025`) and `client/products-live-v1.json`
(a read-only capture of `GET https://api.secondedoracle.xyz/v1/products` taken on
2026-09-30). Prices are USD. "—" means the tier is not sold for that product.

## Paid checks in the 0.4.1 catalog

| Product | MCP tool | Small | Medium | Large | Subject chains | Observed live |
| --- | --- | ---: | ---: | ---: | --- | --- |
| Trade Check | `seconded_trade_check` | 0.25 | 1.50 | — | Base 8453, Arc 5042, Robinhood 4663 | ✓ 2026-09-30 |
| Stock Token Check | `seconded_stock_token_check` | 0.10 ¹ | — | — | Robinhood 4663, Base 8453 | ✓ 2026-09-30 ² |
| Token Check | `seconded_token_check` | 0.10 ¹ | — | — | Base 8453, Arc 5042, Robinhood 4663, Base Sepolia 84532 | ✓ 2026-09-30 ² |
| Agent Registry Check | `seconded_agent_registry_check` | 0.25 | — | — | Base 8453, Arc 5042, Robinhood 4663 | ✓ 2026-09-30 |
| Address Screening Check | `seconded_counterparty_check` | 0.10 ¹ | — | — | Base 8453, Arc 5042, Robinhood 4663 | ✓ 2026-09-30 ² |
| Cross-Chain Compare | `seconded_cross_chain_compare` | 0.10 ¹ | — | — | Base (Uniswap V3) and Arc (Uniswap V4), fixed USDC to EURC | ✓ 2026-09-30 ² |
| Scam Check | `seconded_scam_check` | 0.10 ¹ | 1.50 | 2.50 | chain-independent | ✓ 2026-09-30 ² |
| Lending Check | `seconded_lending_check` | 0.25 | — | — | Base 8453, Arc 5042 (Morpho Blue) | not in the capture ³ |
| Agent Work Payout Check | `seconded_job_escrow_check` | 0.25 | — | — | Base 8453 | not in the capture ³ |
| x402 Payment Check | `seconded_x402_payment_check` | 0.10 ¹ | — | — | Base 8453, Arc 5042, Robinhood 4663 | not in the capture ³ |
| Shielded Route Check | `seconded_shielded_route_check` | 0.25 | — | — | Base 8453, Arc 5042, Robinhood 4663 | refused as `unknown_product` on 2026-10-06 ³ |
| Bridge Route Check | `seconded_route_check` | 0.10 ¹ | — | — | typed EVM routes, 1 to 4 hops | not in the capture ³ |

¹ **$0.15 when paying on Robinhood Chain** (`eip155:4663`, testnet `eip155:46630`);
`price_usd_by_network` in the catalog. Scam Check's medium and large tiers are the same
on every network.

² The 2026-09-30 capture lists these at **$0.25** small. The 0.4.0 release notes state that
the lower price is selected for clients sending `SECONDED-CLIENT-VERSION` 0.4.0 or newer;
no 0.4.x client has been released, so an older client sees the captured price.

³ These five products are marked `available` in the 0.4.1 catalog but were not in the
2026-09-30 live capture, and on 2026-10-06T03:57:35Z the production API refused a quote
for `shielded_route_check` with `unknown_product`. The signed refusal is
`verifier/testdata/refused-base-mainnet-2026-10-06.json`. Treat them as **not live** until
`GET /v1/products` lists them.

Subject chains are the `network` or `chainId` enums in each product's `input_schema`.
The catalog also carries `coverage` text per product describing what is and is not
checked; read it before relying on a verdict. For example Address Screening Check matches
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
pay-to address are identical in the 2026-09-30 live capture. Billing mode is
`settle-on-agreement`.

## Products in testing (not purchasable)

| Product | Status in the 0.4.1 catalog |
| --- | --- |
| Vault Check | in_testing |
| Private Receive Scan | in_testing |
| Portfolio Check | in_testing |
| Code Review | in_testing |
| Owner Instruction Check | in_testing |
| Hidden Prompt Check | in_testing |

They appear under `unavailable_products` with `purchasable: false`; the client refuses to
quote or buy them. The live capture lists Code Review, Owner Instruction Check and Hidden
Prompt Check the same way.

## Free, local privacy tools (in testing)

Seven tools in `client/privacy-tools.json` run inside the client binary at `price_usd`
`0.00` with `execution: agent_local` (one may optionally consult a hosted neutral source).
Their status is `in_testing` and they are not in the published tools list:
`seconded_privacy_shielded_route`, `seconded_privacy_route_check`,
`seconded_privacy_pool_check`, `seconded_private_receive_prepare`,
`seconded_private_purchase_prepare`, `seconded_private_swap_quote`,
`seconded_private_fact_prove`. Nothing in them signs or broadcasts a transaction.
