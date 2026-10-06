# Claims register

Every public claim made in this repository, what backs it, and how it was checked. Dates
are the day the check was run. "Vendor statement" marks a claim that rests on what the
service's own catalog or maintainers say rather than on something a third party has
measured. Nothing here was measured on a live paid check, because publishing one would
publish a customer's receipt; the two live data points are a catalog capture and a
signed refusal.

## The pitch

| Claim | Evidence | Verified |
| --- | --- | --- |
| An agent pays per check over x402 | `client/standard.go` posts to `/v1/x402/checks` and accepts only the x402 `exact` scheme against the pinned asset and `payTo`; `client/api.go` sends `SECONDED-CLIENT-VERSION`. | 2026-10-06, by reading the source; the client builds. |
| Two models from rival labs must agree | `client/receipt.go`: an agreed receipt must name labs in `checked_by` drawn from `OpenAI` and `Anthropic` without repeats; a `no_agreement` receipt must carry the fixed NOT VERIFIED message and no answer. The catalog's `usage` text: "You are charged only after both models agree and the answer is saved". | 2026-10-06. The client-side contract is verified by reading the source; the server's behaviour is a vendor statement, not observable from this repository. |
| Every answer carries an Ed25519-signed receipt | `client/receipt.go` `verify()`: `ed25519.Verify` over the version prefix plus the RFC 8785 canonical envelope. `verifier/` reproduces it. | 2026-10-06: `cd verifier && go run ./cmd/seconded-verify testdata/refused-base-mainnet-2026-10-06.json` printed `OK ... Ed25519 by rk-2026-09-a over receipt v1 (807 canonical bytes)`, exit 0. |
| No answer, no charge | Catalog `usage` text (vendor statement). Client rule in `client/receipt.go`: `no_agreement`, `content_refused`, `service_failed`, `refused`, `closed_no_charge` and `frozen_unsettled` receipts are rejected unless they carry no settlement transaction and `charged` is `no` or `pending`; the client certifies nonpayment from its own chain evidence (`closed_no_charge`). | 2026-10-06, by reading the source. The refusal fixture shows `charged: no` on a refused request. Not measured on a disagreement. |
| Live on Base, Arc and Robinhood Chain | `client/products-live-v1.json`, a capture of `GET https://api.secondedoracle.xyz/v1/products` from 2026-09-30, lists payment networks `eip155:8453`, `eip155:5042`, `eip155:4663` with one `pay_to`. The refusal fixture was signed by the production service on 2026-10-06 for `eip155:8453`. | Capture dated 2026-09-30; refusal dated 2026-10-06T03:57:35Z. The build environment for this repository had no network access, so the API was not re-probed on 2026-10-06; run `curl -s https://api.secondedoracle.xyz/v1/products` to repeat. |

## Keys, addresses, versions

| Claim | Evidence | Verified |
| --- | --- | --- |
| Receipt key `rk-2026-09-a` is `438d9301c477c27fecc3f56da0a7d5a6b3894cef69815bae63b5349dc2cddf0b` | `client/receipt.go` line 288; `verifier/keys.go`. | 2026-10-06: the production-signed fixture verifies under exactly this key and fails under any other (`verifier/receipt_test.go`). |
| Pay-to address `0x010ab46d566cde25cca0ee55eb105e781c7bcf3a` on all three mainnets | `client/products.json` `networks[].pay_to`; `client/policy.go` `PayTo`; the 2026-09-30 capture; the fixture's `billing.pay_to`. | 2026-10-06, four sources agree; on-chain activity not inspected from here. |
| Payment assets: USDC `0x8335…2913` on Base, USDC `0x3600…0000` on Arc, USDG `0x5fc5…d168` on Robinhood Chain | `client/chains.go` `chainPins`; `client/products.json` `networks[].asset`. | 2026-10-06, both files agree. |
| Client version 0.4.1 | `client/api.go` `ClientVersion = "0.4.1"`. | 2026-10-06: a build printed `seconded-mcp 0.4.1` for `--version`. |
| Two independent RPC readers per chain | `client/chains.go`: every mainnet pins at least two operators (Base: base, publicnode, tenderly, drpc; Arc: circle, quicknode, blockdaemon, drpc; Robinhood: robinhood, publicnode, drpc) and `supportedNetwork` requires at least two endpoints. | 2026-10-06, by reading the source. |

## Catalog facts

| Claim | Evidence | Verified |
| --- | --- | --- |
| 12 paid checks, 6 utility tools, 18 MCP tools | `client/products.json` has 12 entries in `products`; `client/tools-list.json` has 18 tools. | 2026-10-06 with `jq`. |
| Prices per tier and the $0.15 Robinhood price for the $0.10 checks | `client/products.json` `price_usd` and `price_usd_by_network`. | 2026-10-06 with `jq`; the table in `docs/guide/pricing-and-coverage.md` was generated from it. |
| The 2026-09-30 live catalog listed 7 checks at $0.25 small for the lookup checks | `client/products-live-v1.json`. | 2026-10-06 with `jq`: 7 products; `stock_token_check`, `token_check`, `counterparty_check`, `cross_chain_compare`, `scam_check` at small `0.25`. |
| The lower 0.4.x prices apply to clients 0.4.0 and newer | The 0.4.0 release notes (summarized in `CHANGELOG.md`). | Vendor statement; not observable without a 0.4.x client against the live service. |
| Five catalog checks were not live | Not present in the 2026-09-30 capture; the production API refused a `shielded_route_check` quote as `unknown_product` on 2026-10-06 (`verifier/testdata/refused-base-mainnet-2026-10-06.json`, fields `error` and `reason` beside the signed receipt). | 2026-10-06. |
| Six products are in testing and not purchasable | `client/products.json` `unavailable_products`, each `purchasable: false`. | 2026-10-06 with `jq`. |
| Seven free local privacy tools, in testing | `client/privacy-tools.json`: 7 entries, `status: in_testing`, `price_usd: 0.00`, `execution: agent_local` (one `agent_local_or_hosted_neutral`). | 2026-10-06 with `jq`. |
| Size tiers 8,192 / 65,536 / 131,072 bytes | `client/products.json` `tiers[].max_billable_bytes`. | 2026-10-06. |
| Subject chains per check | Each product's `input_schema` network or `chainId` enum in `client/products.json`. | 2026-10-06 with `jq`. |
| `wait_seconds` is 0 to 25; `network` names six values | `client/tools-list.json`, `seconded_trade_check.inputSchema`. | 2026-10-06 with `jq`. |
| Default limits $2.50 per check and $25 per day; chat can only tighten | `client/cmd/seconded-mcp/main.go` setup flag defaults (`daily-limit` 25, `per-check-limit` 2.50, hourly and outstanding `none`); `limits --human` and `wallet --human` are the only raising paths. | 2026-10-06, by reading the source. |
| 45-second default soft deadline | `client/deadline.go` reads `SECONDED_TOOL_SOFT_DEADLINE_SECONDS`; the default value is from the 0.4.0 release notes. | 2026-10-06: the variable exists in source; the default is a vendor statement. |

## Receipts and verification

| Claim | Evidence | Verified |
| --- | --- | --- |
| Signature prefix `SECONDED-RECEIPT/v{1,2,3}\x00` plus RFC 8785 canonical envelope | `client/receipt.go` `verify()`; `client/strict.go` `Canonical` wraps `jsoncanonicalizer.Transform`. | 2026-10-06: the standalone verifier, using the same library, verifies the production fixture; signing under the wrong prefix fails (`TestSyntheticNegativePaths`). |
| Clock tolerance of 30 seconds on `issued_at` | `client/receipt.go`: `issued.After(now.Add(30*time.Second))` rejects. | 2026-10-06, by reading the source. |
| The NOT VERIFIED message text | `client/receipt.go` `CurrentNotVerifiedMessage`. | 2026-10-06. |
| The verifier checks less than the client (no purchase binding) | `verifier/receipt.go` package comment lists what it checks; `client/receipt.go` `verify()` additionally binds check id, commitment, payer, amount, association and sequence. | 2026-10-06, by reading both. |
| `/v1/keys` is advisory and cannot change the pin | `client/identity.go` `validateReceiptKeyDocument`: a document key that differs from a pin is a mismatch error; unknown ids are ignored, never adopted. | 2026-10-06, by reading the source. |

## Repository and build

| Claim | Evidence | Verified |
| --- | --- | --- |
| The client builds offline from vendored source | `client/vendor/` with `modules.txt`; `client/go.mod` names Go 1.26.6 and toolchain 1.26.7. | 2026-10-06: `cd client && GOPROXY=off go build ./... && go vet ./... && go test ./...` → `ok seconded.local/client`, `ok seconded.local/client/cmd/seconded-mcp`. 32 tests passed, 4 skipped (opt-in live tests). |
| 22 of the client's test files ship | `ls client/*_test.go client/cmd/seconded-mcp/*_test.go`. | 2026-10-06: 22 files. The private tree has 116; the rest depend on private golden vectors or fixtures outside this repository. |
| The verifier builds and its tests pass | `verifier/receipt_test.go`: 8 tests. | 2026-10-06: `cd verifier && GOPROXY=off go build ./... && go test ./...` → `ok seconded.local/verifier`. |
| The verifier has no third-party module dependencies | `verifier/go.mod` has no `require`; the canonicalizer is an unmodified copy under `verifier/internal/jsoncanonicalizer` (Apache-2.0). | 2026-10-06. |
| No secrets in the tree | `gitleaks dir . --no-banner` (gitleaks 8.30.1). | 2026-10-06: the default ruleset reported 56 `generic-api-key` matches, every one a public hex value: 14 EVM addresses, 39 0x-prefixed 32-byte hashes in x402 challenge fixtures, 2 bare 64-hex values (the receipt public key, a lending market id), and 1 in a scratch file since deleted. With the narrow allowlist in `.github/gitleaks.toml`: 0 findings. A planted raw 64-hex key, AWS key, GitHub token and private-key block were all still reported with that config in place (positive control). |
| No internal hostnames, paths, task ids or personal identifiers | A grep for the maintainers' internal markers over every file to be published. | 2026-10-06: zero matches for all patterns, with one explanation: one marker, an underscore followed by the word "state", matches identifier fragments inside vendored Go (`golang.org/x/sys`) and wire field names such as the lending position state; the same marker followed by a slash, which is the internal directory form, has zero matches. |
| CI builds the client and runs both test suites on ubuntu-latest | `.github/workflows/ci.yml`. | Written 2026-10-06; it has not run yet because the repository has not been pushed. |

## Statements that are the maintainers' word

- Receipts are signed by a key whose private half stays on the service host; the public
  value was derived there. (Client documentation; unobservable from outside.)
- The service gives both models the same evidence and settles only on agreement. (The
  client enforces the receipt shape; the server is private.)
- No binary release, npm package, Homebrew tap, MCP Registry or Smithery listing has
  been published as of 2026-10-06. (From the maintainers' release runbook; not checked
  online from the build environment.)
- Full development history and server source are shared privately with the Colosseum
  judges. (Maintainers' statement.)
