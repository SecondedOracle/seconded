# Claims register

## Current measurements

[README Status](../README.md#status) is the current release/count summary. This register
records the supporting measurements and preserves older observations under historical labels.
[Source provenance](guide/client-0.4.3.md#source-provenance) maps the public client to
the released source; documentation and standalone verifier fixes do not alter that client.

On 2026-10-19, unauthenticated GETs confirmed the published distribution and the live
catalog selected by `SECONDED-CLIENT-VERSION: 0.4.3`. The client built, passed its public
tests and vet offline on Darwin arm64 with Go 1.26.7. The verifier suite passed,
including signed agreed/disagreement reporting through both CLI formats and tamper controls.
No wallet was created, no paid check was purchased, and no chain settlement was measured.

| Measurement | Observed value | Evidence |
| --- | --- | --- |
| npm distribution | Latest 0.4.3, published 2026-10-18T15:37:41.840Z | [Registry metadata](https://registry.npmjs.org/@seconded%2Fmcp) |
| Signed GitHub release | v0.4.3, published 2026-10-18T13:10:32Z, non-draft/non-prerelease | [Release](https://github.com/SecondedOracle/seconded-mcp-releases/releases/tag/v0.4.3); signed manifest and build-info checked |
| Versioned live catalog | All bundled product IDs advertised as available; testing products remain not purchasable | [Products API](https://api.secondedoracle.xyz/v1/products) with the client-version header above; counts below |
| Unversioned live catalog | Eight legacy checks, including Lending at $0.50 | Same endpoint without the header; not the current client's advertised catalog |
| Bundled versus live contracts | Prices and payment-network entries match; descriptions, some validation constraints and Degen status differ | [Catalog explanation](guide/pricing-and-coverage.md#catalog-versions-and-availability) |
| Separate npm licence metadata | `UNLICENSED`, despite this source repository's Apache-2.0 licence | [Distribution boundaries](guide/release-verification.md#distribution-boundaries); no licence grant for separate artifacts inferred |

Exact response-body SHA-256 values from the current measurement (mutable endpoints):

| Response | SHA-256 on 2026-10-19 |
| --- | --- |
| Products with client-version header | `874043f53b0d5fc1ae4f9e86447c49afd94dc83450a94099f61aca7cd212acd9` |
| Products without header | `81b1db47208297ca913c79cfb20c806800ac084ca25ed2da897e81e110d5e264` |
| npm registry metadata | `8e275c358700d911328361ce5699b53301337b8d2fcc3e4bb90f0fcb8a38bb78` |
| GitHub release metadata | `6eccebd51c687c14a348930c4ef55e232263dc368bdcaff8ce640ad204897fd9` |
| Signed build information | `dc5b6100458063412cdcc52ada06d7bddf5434286505a544d6e227d3883951be` |
| Public receipt key document | `ae2885103599cb01ff02ebff4b45ed04e98dfcfe89d4e2b70fc90362315ac613` |

This register records principal claims, their sources and how they were checked.
Vendor or maintainer statements are identified separately. This checkout includes a
signed refusal for an offline smoke test. The official documentation also publishes
an agreed paid Trade Check and a Token Check disagreement, whose signatures verify
with the same pinned key. No purchase was made for these checks and their chain
settlement was not independently verified. Dates describe point-in-time evidence,
not a guarantee of future availability or a universal security verdict.

## The pitch

| Claim | Evidence | Verified |
| --- | --- | --- |
| An agent pays per check over x402 | `client/standard.go` posts to `/v1/x402/checks` and accepts only the x402 `exact` scheme against the pinned asset and `payTo`; `client/api.go` sends `SECONDED-CLIENT-VERSION`. | 2026-10-06, by reading the source; the client builds. |
| Standard checks require rival-lab agreement; Degen can return signed disagreement | `client/receipt.go`: an agreed receipt must name labs in `checked_by` drawn from `OpenAI` and `Anthropic` without repeats; a `no_agreement` receipt must carry the fixed NOT VERIFIED message and no answer. Degen permits `outcome=models_disagree` with `answer.label_id=models_disagree`, `option=0` (`client/degen.go`). The generic catalog agreement-only text omits this exception; see [billing policy](guide/how-it-works.md#billing-policy). | 2026-10-19. The client-side contract is verified by reading the source; the server's behaviour is a vendor statement, not observable from this repository. |
| Every answer carries an Ed25519-signed receipt | `client/receipt.go` `verify()`: `ed25519.Verify` over the version prefix plus the RFC 8785 canonical envelope. `verifier/` reproduces it. | 2026-10-06: `cd verifier && go run ./cmd/seconded-verify testdata/refused-base-mainnet-2026-10-06.json` printed `OK ... Ed25519 by rk-2026-09-a over receipt v1 (807 canonical bytes)`, exit 0. |
| Standard agreement and Degen signed disagreement can be charged; pending billing is unresolved | The product-specific [billing policy](guide/how-it-works.md#billing-policy) qualifies generic catalog `usage`. `client/receipt.go` requires pending billing for `no_agreement`; chain evidence resolves nonpayment. The public disagreement signs `charged=pending`, `settlement=none`, `tx=null`. | 2026-10-19: both public v3 fixtures verify offline; the Degen outcome contract was inspected in source. A receipt signature authenticates the billing assertion, not independent nonpayment/finality. |
| Three mainnet payment networks advertised | `client/chains.go`, `client/products.json` and the current versioned GET list 8453, 5042 and 4663. | 2026-10-19: live and bundled network entries match. No three-network paid canary or settlement test. |

## Keys, addresses, versions

| Claim | Evidence | Verified |
| --- | --- | --- |
| Receipt key `rk-2026-09-a` is `438d9301c477c27fecc3f56da0a7d5a6b3894cef69815bae63b5349dc2cddf0b` | `client/receipt.go` `releaseReceiptKeys`; `verifier/keys.go`. | 2026-10-06: the production-signed fixture verifies under exactly this key and fails under any other (`verifier/receipt_test.go`). |
| Pay-to address `0x010ab46d566cde25cca0ee55eb105e781c7bcf3a` on all three mainnets | `client/products.json` `networks[].pay_to`; `client/policy.go` `PayTo`; the 2026-09-30 capture; the fixture's `billing.pay_to`. | 2026-10-06, four sources agree; on-chain activity not inspected from here. |
| Payment assets: USDC `0x8335…2913` on Base, USDC `0x3600…0000` on Arc, USDG `0x5fc5…d168` on Robinhood Chain | `client/chains.go` `chainPins`; `client/products.json` `networks[].asset`. | 2026-10-06, both files agree. |
| Current client source and release provenance | `client/api.go` reports the version in [README Status](../README.md#status); [source mapping](guide/client-0.4.3.md#source-provenance) identifies the release commit and intentional public export differences. | 2026-10-19: source-built executable reports 0.4.3. Module renaming/source packaging changes binary digests; no release-byte equivalence claim. |
| At least two configured RPC provider labels per chain | `client/chains.go` pins Base, Arc and Robinhood endpoint sets; quorum guards require agreeing readers. | 2026-10-06, source inspection. Actual provider independence and endpoint availability were not independently measured. |

## Historical source snapshot (2026-10-06)

| Claim at that date | Evidence at that date | Verification at that date |
| --- | --- | --- |
| Historical source snapshot (2026-10-06) reported client version 0.4.2 | `client/api.go` `ClientVersion`; [published build-info](https://github.com/SecondedOracle/seconded-mcp-releases/releases/download/v0.4.1/build-info.json) names release source `b8cfd7d1375d4f3a65087410a4a600e081e715bc`. Baseline development snapshot was updated to the final 0.4.2 client from commit `36c762f4`. Release binaries are built from private release source and verified by the signed manifest; that historical public tree had earlier snapshot differences as well as renamed module paths. | 2026-10-06: version constant 0.4.2 verified; offline build and tests pass. After the public module rename, local go.mod SHA-256 is `c3e75a610b5b4466560bfeee1199016e1d44ce24d5b774170ca1c77b2c511204`, unlike the published `937453641c403bb42fd3a45c050db92d365a20b55bfdb687a18f907ff00e1a8a`. Module paths change build digests; no released-binary equivalence claim. |

## Catalog facts

| Claim | Evidence | Verified |
| --- | --- | --- |
| 13 check interfaces, 6 utility tools, 19 MCP tools | `client/products.json` has 13 entries in `products`; `client/tools-list.json` has 19 tools. Twelve bundled checks are available; Degen is bundled as `unavailable` but advertised live as `available`. | 2026-10-19, parsed both local JSON files and the header-aware live response; checked unique IDs and matching product sets. |
| Prices per tier and the $0.15 Robinhood override | `client/products.json` and the current versioned GET, `price_usd` and `price_usd_by_network`; [price table](guide/pricing-and-coverage.md#paid-checks). | 2026-10-19: all thirteen IDs, tier prices and network overrides agree. Degen has no Robinhood surcharge. Advertisement only; no purchase. |
| Historical: the 2026-09-30 live catalog listed 7 checks at $0.25 small for the lookup checks | `client/products-live-v1.json`. | 2026-10-06 with `jq`: 7 products; `stock_token_check`, `token_check`, `counterparty_check`, `cross_chain_compare`, `scam_check` at small `0.25`. |
| Historical (2026-10-06): the 0.4.1 header selected lower prices than the unversioned response | Dated [0.4.1](guide/evidence/products-client-0.4.1-2026-10-06.json) and [unversioned](guide/evidence/products-unversioned-2026-10-06.json) GET captures. The >=0.4.0 threshold is attributed to release notes in `CHANGELOG.md`. | 2026-10-06: twelve versus eight advertised checks; Lending $0.25 versus $0.50. Five legacy small-tier lookups $0.10 versus $0.25 (Robinhood $0.15 override). |
| Historical (2026-10-06): all twelve checks were advertised to client 0.4.1 | [Versioned GET capture](guide/evidence/products-client-0.4.1-2026-10-06.json); historical September missing rows and unsigned refusal reason do not establish current unavailability. | 2026-10-06: product IDs and prices compared with the compiled catalog. All six unavailable products still marked not purchasable. |
| Six products are in testing and not purchasable | `client/products.json` `unavailable_products`, each `purchasable: false`. | 2026-10-06 with `jq`. |
| Seven free local privacy tools, in testing | `client/privacy-tools.json`: 7 entries, `status: in_testing`, `price_usd: 0.00`, `execution: agent_local` (one `agent_local_or_hosted_neutral`). | 2026-10-06 with `jq`. |
| Size tiers 8,192 / 65,536 / 131,072 bytes | `client/products.json` `tiers[].max_billable_bytes`. | 2026-10-06. |
| Subject chains per check | Each product's `input_schema` network or `chainId` enum in `client/products.json`. | 2026-10-06 with `jq`. |
| `wait_seconds` schema is 0–25; effective MCP requested wait is at most eight seconds | `client/tools-list.json` defines the range and six network aliases; `client/mcp.go` clamps requested waits to eight seconds. | 2026-10-06, source and schema inspection. A separate soft deadline applies. |
| Default limits $2.50 per check and $25 per day; chat can only tighten | `client/cmd/seconded-mcp/main.go` setup flag defaults (`daily-limit` 25, `per-check-limit` 2.50, hourly and outstanding `none`); `limits --human` and `wallet --human` are the only raising paths. | 2026-10-06, by reading the source. |
| 45-second default soft deadline | `client/deadline.go` defines `defaultToolSoftDeadline = 45 * time.Second`; `toolSoftDeadline` uses it for absent or invalid overrides. | 2026-10-06, directly verified in source. |

## Receipts and verification

| Claim | Evidence | Verified |
| --- | --- | --- |
| Signature prefix `SECONDED-RECEIPT/v{1,2,3}\x00` plus RFC 8785 canonical envelope | `client/receipt.go` `verify()`; `client/strict.go` `Canonical` wraps `jsoncanonicalizer.Transform`. | 2026-10-06: the standalone verifier, using the same library, verifies the production fixture; signing under the wrong prefix fails (`TestSyntheticNegativePaths`). |
| Clock tolerance of 30 seconds on `issued_at` | `client/receipt.go`: `issued.After(now.Add(30*time.Second))` rejects. | 2026-10-06, by reading the source. |
| Current and constrained legacy NOT VERIFIED messages | `client/receipt.go` `CurrentNotVerifiedMessage`, `NotVerifiedMessage` and state-specific guards. | 2026-10-06, source inspection; public disagreement fixture has no answer and pending billing. |
| The standalone verifier omits purchase binding and product semantics | `verifier/receipt.go` checks signature and structural subset. `client/receipt.go`, `client/standard.go` bind original commitments or standard-door purchase association to local history. | 2026-10-06, source inspection. Many schema/arithmetic checks are independently implementable; purchase-history binding requires local records. |
| `/v1/keys` is advisory and cannot change the pin | `client/identity.go` `validateReceiptKeyDocument`: a document key that differs from a pin is a mismatch error; unknown ids are ignored, never adopted. | 2026-10-06, by reading the source. |

## Repository and build

| Claim | Evidence | Verified |
| --- | --- | --- |
| The client builds offline with an installed/cached toolchain | `client/go.mod` requires Go 1.26.6 and selects 1.26.7; dependencies are vendored. | 2026-10-19: Go 1.26.7 darwin/arm64, GOPROXY=off; build/vet/test pass. 126 passing test/subtest events and six skips. Skips need native keychain, private pipeline, PTY or signed-install fixtures; no live acceptance claim. |
| 22 client test files ship in the two documented glob locations | `client/*_test.go` and `client/cmd/seconded-mcp/*_test.go` inventory. Public subset omits excluded fixtures/helpers and private integration context. | 2026-10-06: 22 files. The review counted 119 in those private globs at its initial snapshot (121 total), then 120 (122 total) after an independent update. This checkout does not claim current private-suite parity. |
| The verifier builds and its tests pass | `verifier/receipt_test.go` includes signed outcome descriptions in text/JSON, signature tampering, canonicalization, public paid/pending and strict-parser controls. | 2026-10-19: offline `go test ./...` passes, 45 passing test/subtest events and no skips. Three committed public receipts verify; changed signed billing fails. |
| The verifier has no third-party module dependencies | `verifier/go.mod` has no `require`; the canonicalizer is an unmodified copy under `verifier/internal/jsoncanonicalizer` (Apache-2.0). | 2026-10-06. |
| No findings from configured directory/history scans; coverage is bounded | `.github/gitleaks.toml` allows 44 exact reviewed public identifiers: three token contracts, receipt public key, Morpho market id and 39 challenge tickets. No blanket 32-byte-hex, vendor or testdata exemption. | 2026-10-19: configured publication-tree and reachable-history scans exit 0. Separate synthetic GitHub-token, bare-key and 0x-prefixed-key controls yield three detections. Detector controls establish tested coverage, not universal secret absence. |
| Requested publication-marker scan reports zero hits outside vendored source | Tracked publication tree marker scan; synthetic marker positive control. Internal round and approved-host comments have been replaced. Useful relative server references identify private code; vendor attribution remains. | 2026-10-06: all requested marker patterns zero outside `client/vendor`. Separately, public npm metadata contains a local archive-path disclosure; external distribution metadata is outside this checkout. |
| CI is configured to build/test on ubuntu-latest | `.github/workflows/ci.yml` defines client build/vet/test and verifier build/test/fixture checks. | 2026-10-19, workflow inspection and local macOS checks; no remote Ubuntu run measured for this update. |

## Statements that are the maintainers' word

- Receipts are signed by a key whose private half stays on the service host; the public
  value was derived there. (Client documentation; unobservable from outside.)
- The service gives both models the same evidence and follows the [billing policy](guide/how-it-works.md#billing-policy), including chargeable Degen disagreement. (The
  client enforces the receipt shape; the server is private.)
- Full development history and server source are shared privately with the Colosseum
  judges. (Maintainers' statement.)

## Historical distribution observations (2026-10-06)

The version and listing claims in this section describe that date only. See
[README Status](../README.md#status) for current distribution. Receipt signatures
remain useful historical evidence; payment finality was not independently queried.

| Claim | Evidence | Verified |
| --- | --- | --- |
| npm and GitHub 0.4.1 were published at this observation | [npm metadata](https://registry.npmjs.org/@seconded%2Fmcp) latest 0.4.1 published 2026-10-06T15:55:29.162Z; [GitHub v0.4.1](https://github.com/SecondedOracle/seconded-mcp-releases/releases/tag/v0.4.1) published 15:50:51Z, non-draft/non-prerelease, nine assets. | 2026-10-06 unauthenticated GETs; no launcher or released native binary executed. |
| Two observed Registry versions lagged 0.4.1 at this observation | [Official search](https://registry.modelcontextprotocol.io/v0.1/servers?search=secondedoracle) reports active domain namespace `xyz.secondedoracle/seconded-mcp` 0.3.3 and older GitHub namespace `io.github.SecondedOracle/seconded-mcp` 0.3.0. | 2026-10-06 unauthenticated GET; neither observed entry advertises 0.4.1. |
| Homebrew and Smithery status is bounded | Prior review's unauthenticated GitHub organization GET found only the release repo; expected tap endpoint 404 with release-repo 200 positive control. Smithery unmeasured. | Prior review on 2026-10-06; not a universal absence claim. |
| Public PAID Trade receipt signs $0.25 and inclusion | [Official examples](https://secondedoracle.xyz/docs#example-receipts); [committed receipt](../verifier/testdata/trade-paid-base-mainnet-2026-09-29.json): v3, `included`, `charged=yes`, amount 250000, tx `0x9472572e3cfe100973002a46b1d943a97ca3839be0c834863654d4a4e4fa1b13`. | 2026-10-06: CLI verifies under compiled pin; signed-value tampering fails. Signed billing, not independently queried chain evidence. |
| Public NOT VERIFIED Token receipt has pending billing | [Official examples](https://secondedoracle.xyz/docs#example-receipts); [committed receipt](../verifier/testdata/token-not-verified-base-sepolia-2026-09-29.json): v3, `no_agreement`, `charged=pending`, `tx=null`. Website describes a Base Sepolia subject; signed billing network is Base mainnet. | 2026-10-06: CLI verifies under compiled pin; signed-value tampering fails. Pending does not certify nonpayment. |
| Public code is licensed under Apache-2.0 | Root [LICENSE](../LICENSE) and [NOTICE](../NOTICE); [THIRD_PARTY.md](../THIRD_PARTY.md) inventories all nine vendored client modules and the verifier canonicalizer. The name and logo remain trademarks. | Maintainer licence decision applied to this tree; dependency LICENSE files inspected. Previously published npm metadata is unchanged. |
| Platform prompts still apply | [Published release](https://github.com/SecondedOracle/seconded-mcp-releases/releases/tag/v0.4.1) describes no Apple Developer ID/notarization; guide does not claim SSH manifests bypass OS checks. | Release declaration; native platform protection flows not exercised. |

## Guide and interface boundaries

| Claim | Evidence | Verified |
| --- | --- | --- |
| Signatures cover canonical envelopes, not arbitrary response bytes | `client/receipt.go`, `verifier/receipt.go`; unsigned metadata/challenges are distinct from receipts. Whitespace/key order and unsigned wrapper changes verify; signed billing changes fail. | 2026-10-06 offline signature tests and CLI controls; no model-correctness certification. |
| Parser rejects duplicates and trailing input | `verifier/receipt.go` checks one bounded JSON value, all object keys, exact direct receipt shape or one wrapper. | `TestReceiptParserRejectsAmbiguousInput`: valid/whitespace/wrapper positives; trailing object/scalar/garbage, escaped/nested duplicates, mixed/nested wrapper and malformed inputs rejected. |
| NOT VERIFIED and state labels do not independently certify money movement | `verifier/receipt.go` state descriptions identify signer assertions and direct pending states to signed billing/payment evidence; CLI displays billing verbatim. | Public pending receipt regression and CLI verification; no chain query. |
| Inputs and credentials can persist locally | `client/standard.go` `newRecovery` retains body/canonical input/authorization; `client/engine.go` saves ledger before sending; `client/ledger.go` serializes it. `archiveRecovery` clears eligible records on successful save. | Source trace, 2026-10-06. Owner-private ledger is unencrypted; pending records/backups/history can retain data. No real profile opened. |
| Standard-door purchase binding differs from original-door commitments | `client/standard.go` builds canonical request/purchase association and a local recovery ID; `client/receipt.go` adopts server ID/commitment only after matching authenticated association. | Source inspection; no prepayment salt/server-ID claim for standard door. |
| Archived standard-door input proofs have a known limitation | `client/standard.go` clears `CanonicalRequest` during archival; `client/proof.go` `ProveInput` still compares supplied canonical bytes to it. | Source-proven rejection after archival; documented as follow-up, not fixed by retaining raw inputs. No real user-profile reproduction. |
| Recovery is conditionally keyless and retention-bounded | `client/mcp.go` guidance states retention/availability limits; `client/standard.go` `replay` reuses retained authorization; `recoverySigner` can require a key for original/archived records. [Privacy policy](https://secondedoracle.xyz/privacy) states 90-day deletion of finished records after last activity. | Source/policy inspection. Deployed retention and perpetual service availability unmeasured. |
| Wallet export is a supported interface, not an OS secrecy guarantee | `client/cmd/seconded-mcp/export.go` and platform helpers; same-user hot-wallet access documented. | Source and shipped export/owner tests; no actual key exported. |
| Timing rules are product-specific beyond issuance | `client/receipt.go` bounds future issuance by 30s, outcomes by issuance and purchase time; product validators add observation/expiry bounds. Historical freshness may be absent or downgraded on expiry. | Source inspection, 2026-10-06; no universal observation-time promise. |
| Model fields are signed issuer attribution | `client/receipt.go` checks labs/fingerprints; neither verifier re-runs remote models. | Public receipt verification; actual model execution/configuration unmeasured. |
| Paid tools share four fields; x402 adds `signing` | `client/tools-list.json`, `client/local_input.go`, `client/x402_context.go`; typed inputs can contain message prose. | Schema/source inspection and catalog-generator equality; utility descriptions are complete concise summaries. |
| Coverage depends on product | `client/products.json`, `client/registry_observations.go`, `client/cross_chain_comparison.go`: message checks can be chain-independent, list-only screening zero RPC, comparison two chains. Route enums restrict 8453/5042/4663 to 1–4 hops. | Catalog/source inspection; no claim every product has coverage text or that a no-match proves safety. |

## Historical capture integrity (2026-10-06)

The public catalog captures store exact HTTP response bodies. The intended header and
SHA-256 are recorded in [capture provenance](guide/evidence/README.md). External
metadata below was read without authentication and is not copied into the publication
tree; hashes identify the observed response bodies, not immutable endpoint contents.

| Response | SHA-256 on 2026-10-06 |
| --- | --- |
| npm metadata | `ca67503c5c0f3dda9ed5dad26b22ef66629c2d0d49e859ecdbd0816d6bf15ad8` |
| GitHub v0.4.1 release metadata | `0281b4a56e12902e54ee5cc021cdcb0ffe3730a7b2ff96d9cffc477e1be1c2c5` |
| Official Registry search | `693efdf3113b458bbf6b108553d4dc370a1cb49b6616fb098a4e3a7c6222f8c1` |
| Published build information | `984e5dbdb321486d2c859944491fcdbc3f89e7dc1fbba4384ae1f76445021638` |
| Official documentation with receipt examples | `93ae0e1b21709c9417b2319d100789d239a62026ea69ec6d003c875dce2518d5` |
| Privacy policy | `ef627c87d6b2136ff16937ead35671fe4065d4155e6fad818fd2a59e73b2082f` |
