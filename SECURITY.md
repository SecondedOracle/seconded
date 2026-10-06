# Security policy

## Reporting a vulnerability

Email **support@secondedoracle.xyz** with the subject line `SECURITY`. Please include:

- the component (`client/`, `verifier/`, the public API, or a release artifact);
- the version or commit;
- steps to reproduce, and what you observed versus what you expected;
- whether you believe funds, receipts, or wallet material are affected.

Please do not open a public GitHub issue for anything that could let someone spend a
wallet, forge or alter a receipt, or bypass a spending limit. Everything else can go
through the issue tracker.

We will acknowledge reports by email. There is no bug bounty programme at this time;
if that changes it will be announced at https://secondedoracle.xyz.

## What is in scope here

This repository contains the Go MCP client (`client/`) and the offline receipt verifier
(`verifier/`). The server, settler, worker, judge prompts and infrastructure are not
published here; reports about them are still welcome at the same address.

## Things worth knowing before you report

- The client wallet is a **hot wallet**. Software running as the same OS user can read
  its key file (file fallback) or use the OS credential store on its behalf. That is a
  documented limit, not a vulnerability. See
  [docs/guide/security-model.md](docs/guide/security-model.md).
- Spending limits are enforced by the client, not on chain. An attacker who controls the
  host can bypass them. Also documented.
- `NOT VERIFIED` is a deliberate outcome, not a failure: the models disagreed or the
  evidence was unusable, nothing was charged, and the agent is told to pause.
- Receipts are Ed25519-signed by the service. The compiled public key pin is in
  `client/receipt.go` and `verifier/keys.go`; `/v1/keys` is advisory and cannot change it.
  A receipt that verifies under a key that is not pinned is not a SECONDED receipt.

## Release verification

Release binaries are published with a `SHA-256SUMS` manifest and an SSH Ed25519
signature over it. The verification steps are in
[docs/guide/release-verification.md](docs/guide/release-verification.md). If a published
checksum or signature does not verify, stop and report it; do not run the binary.
