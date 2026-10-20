# Changelog

Entries use `[Added]`, `[Changed]`, `[Fixed]`, `[Removed]`, `[Security]`. Client versions
are the `ClientVersion` constant in `client/api.go`. Earlier history lives in the private
repository.

Current release and availability: [README Status](README.md#status).
Entries below are historical release records; counts describe each release at that time.

## Client 0.4.4 (published 2026-10-20 UTC)

Published on npm as `@seconded/mcp@0.4.4` and on GitHub as [v0.4.4](https://github.com/SecondedOracle/seconded-mcp-releases/releases/tag/v0.4.4). Release assets are signed; verify `SHA-256SUMS.sig` with `allowed_signers` and check the artifact hashes using the [release verification guide](docs/guide/release-verification.md).

- [Changed] Label Degen availability as a bundled offline snapshot and state its $0.10 signed-disagreement billing exception alongside free standard disagreement.
- [Fixed] Reject malformed utility input before wallet access, surface unsigned 503 diagnostics, and unblock new purchases after independently certified nonpayment.
- [Fixed] Add proof/recovery help, specific error reasons, a profile-level missing-ledger message and an exact billing-parity regression.
- [Changed] Preserve Asset Check's token/stock-token selector and explicit quote payment networks: 13 products through 12 check tools and six utilities, with unchanged prices.

## Public repository documentation and verifier update (2026-10-19)

- [Fixed] The standalone verifier describes signed Degen disagreements without calling them agreed answers, with signed outcome and tamper regression controls.
- [Fixed] Installation, billing, pricing, contribution terms and release provenance reflect the published client. The frozen client runtime and catalogs are unchanged.
- [Changed] Current release and availability facts have one status section; older measurements are labeled as history.

## Client 0.4.3 (published 2026-10-18)

- [Added] `seconded_degen_check` for Pons V2 tokens on Robinhood Chain. The bundled catalog marks Degen Check unavailable; purchases depend on the server's availability gate. Its small-tier price is $0.10 when offered.
- [Added] Verification and recovery of signed Degen facts and model opinions, including chargeable `models_disagree` answers and stale historical results. Disagreement and stale results do not authorize an action.
- [Changed] The client lists 19 MCP tools and clarifies Token Check's identity-and-controls scope. The twelve existing available checks keep their prices; the six products in testing remain in testing.
- [Fixed] Release catalogs and listings match the client tool surface while distinguishing Degen client support from enabled sales.
- [Added] Separate experimental AgentKit integrations: an action provider for available checks and a receipt guard for transaction/permit signing. These are not included in the MCPB or `@seconded/mcp` package, and this release entry does not claim a separate distribution is available.

## Historical client 0.4.2 publication (2026-10-06)

Hardening release; no new features. Signed release and npm package published from the neutral publisher account.

## Historical public repository assembly (2026-10-06)

- [Fixed] Release and Registry status, development-source provenance, version-aware
  availability, retention, purchase binding, recovery and tool-interface documentation.
- [Fixed] Offline verifier rejects duplicate JSON keys, trailing input and ambiguous
  direct/wrapper shapes; payment descriptions identify signed assertions.
- [Security] Secret scanning exempts exact reviewed public identifiers and detects bare
  and 0x-prefixed synthetic private keys.
- [Added] Two already-public signed v3 receipts: a paid Trade Check and a pending
  NOT VERIFIED Token Check, with offline regression controls and provenance.

- [Added] Public repository assembled from a development snapshot reporting client 0.4.1: the Go MCP client with
  vendored dependencies, the shipped catalogs (`products.json`, `products-live-v1.json`,
  `privacy-tools.json`, `tools-list.json`), and 22 of the client's test files.
- [Added] `verifier/`: a standalone offline receipt verifier with a production-signed
  fixture, a CLI (`seconded-verify`), and tests covering tampering, wrong keys, wrong
  version prefixes and the RFC 8785 canonical form.
- [Added] Documentation under `docs/guide/`, a claims register in `docs/CLAIMS.md`,
  examples, issue templates, and a CI workflow that builds the client and runs both test
  suites on `ubuntu-latest`.

## Historical client 0.4.2 hardening changes

- [Fixed] Bounded wallet recovery: clamps stuck reservations and heals legacy ledger entries without releasing outstanding payment obligations early.
- [Changed] Owner presence: falls back to human terminal confirmation when biometrics are unavailable and no administrator approval key is installed; rejected approvals and invalid installed keys still fail closed.
- [Fixed] Installation no longer waits indefinitely on the reviewed lockup path; client crypto dependencies updated.
- [Added] Expanded reason vocabulary: decodes 23 additional server-side refusal and abstention reasons and four Token Check STOP warning labels.
- [Changed] Address Screening description: documents bounded list coverage.

## Historical client 0.4.1 (published 2026-10-06)

- [Fixed] Wallet payments no longer stall when Base finality lags. Admission accepts fresh,
  depth-confirmed balances agreed by two public readers while retaining all pending
  reservations and the existing budget anchor; conflicting hashes, balances, stale recent
  state or insufficient funds still block payment.
- [Changed] Settlement and reservation release continue to require fresh finalized
  evidence; outstanding payments count against all configured limits until certified.
- [Added] Additional pinned RPC readers: Tenderly and dRPC for Base, Blockdaemon and dRPC
  for Arc, dRPC for Robinhood Chain. The original public pairs remain first.
- [Changed] The 12 paid checks, 6 utility tools, prices, labels, discovery and the x402
  payment contract are unchanged from 0.4.0.

## Client 0.4.0

- [Added] Agent Work Payout Check (`seconded_job_escrow_check`), x402 Payment Check
  (`seconded_x402_payment_check`), Shielded Route Check (`seconded_shielded_route_check`)
  and Bridge Route Check (`seconded_route_check`) in the catalog; Vault Check and Private
  Receive Scan staged as `in_testing`.
- [Added] Verified host profiles and `host-snippet` output for Claude Code, Codex CLI and
  Gemini CLI alongside Claude Desktop, Cursor and generic hosts.
- [Added] A 45-second default soft deadline with structured pending results (`check_id`)
  and receipt recovery through `seconded_receipt`.
- [Changed] Small-tier price of Token Check, Stock Token Check, Address Screening Check,
  Cross-Chain Compare and Scam Check reduced from $0.25 to $0.10 ($0.15 on Robinhood
  Chain) for clients sending `SECONDED-CLIENT-VERSION` 0.4.0 or newer; Lending Check from
  $0.50 to $0.25.
- [Changed] Trade Check adds caller-owned typed spending intent matching and approval and
  signature scope checks. Token Check adds the `issuer_controlled_expected` verdict so
  issuer-controlled tokens such as USDC are no longer flagged as owner-controlled.
- [Fixed] Replay-based receipt recovery over the standard x402 door preserves the original
  credential without re-authorizing or duplicating charges across restarts.
- [Removed] Portfolio Check, Hidden Prompt Check and Owner Instruction Check moved from
  available products to `unavailable_products` with status `in_testing`.
