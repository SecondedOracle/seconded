# Changelog

Entries use `[Added]`, `[Changed]`, `[Fixed]`, `[Removed]`, `[Security]`. Client versions
are the `ClientVersion` constant in `client/api.go`. Earlier history lives in the private
repository.

## Unreleased

- [Added] Public repository assembled from the client 0.4.1 source: the Go MCP client with
  vendored dependencies, the shipped catalogs (`products.json`, `products-live-v1.json`,
  `privacy-tools.json`, `tools-list.json`), and 22 of the client's test files.
- [Added] `verifier/`: a standalone offline receipt verifier with a production-signed
  fixture, a CLI (`seconded-verify`), and tests covering tampering, wrong keys, wrong
  version prefixes and the RFC 8785 canonical form.
- [Added] Documentation under `docs/guide/`, a claims register in `docs/CLAIMS.md`,
  examples, issue templates, and a CI workflow that builds the client and runs both test
  suites on `ubuntu-latest`.

## Client 0.4.1 (unreleased build candidate)

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
