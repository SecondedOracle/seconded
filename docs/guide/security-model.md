# Security model (public level)

This page describes what the shipped client protects and what it does not. It is written
from `client/` source and the catalogs; it is not a security audit, and it does not list
findings.

## Trust anchors compiled into the client

| Anchor | Where | What it protects |
| --- | --- | --- |
| Receipt public key `rk-2026-09-a` | `client/receipt.go` | Every receipt. A server key document cannot replace it. |
| API origin `https://api.secondedoracle.xyz` | `client/api.go` | The client talks to this origin over web PKI (the local trust store) plus the receipt-key pin check at start. |
| Payment pins per network: chain id, asset contract, `payTo`, EIP-712 domain | `client/chains.go`, `client/products.json` | A 402 challenge that names a different asset, amount scheme or payee is refused before signing. |
| Two independent RPC readers per chain | `client/chains.go` | Balances, finality and chain facts the client relies on must agree between two operators at one block. |
| Publisher SSH key for releases | set by the release builder (`client/release_digest.go`) | Setup verifies the signed `SHA-256SUMS` beside the binary before any wallet exists. A source build has no key compiled in and must use `--manual-fingerprint`. |

## The wallet

- Setup creates a **dedicated check wallet**. Fund it with a few dollars; it is not for
  anything else. Checks pay on Base mainnet in USDC unless a check names Arc (USDC) or
  Robinhood Chain (USDG), or a testnet.
- The key is created from the OS CSPRNG and stored in the OS credential store when one is
  available. The automatic fallback stores a private key file **unencrypted**, protected
  by file permissions only, and says so before you fund it.
- The private key never appears in an MCP result, an environment variable, a log, or an
  HTTP request. `seconded-mcp export-key` is the only way out, and it requires typing on
  the controlling terminal.
- **This is a hot wallet.** Software running as the same OS user can read the key file or
  use the credential store on its behalf, and can alter or roll back local files. An EOA
  does not enforce these budgets on chain.

## Spending limits

- Defaults for a new setup: **$2.50 per check, $25 per day**, no hourly or outstanding
  limit, value gate off, dedupe and loop brake on (`cmd/seconded-mcp/main.go`,
  `client/policy.go`).
- Chat (the agent, via `seconded_set_limits`) can only **tighten** limits or freeze
  payments. Raising a limit, removing one, weakening a safety switch, unfreezing, or
  switching wallets requires a human at the controlling terminal
  (`seconded-mcp limits --human`, `seconded-mcp wallet --human`).
- Limits are reread immediately before each new authorization. All open authorizations
  count against every window until chain evidence certifies them.
- Admission requires the two-reader balance minus the new amount to cover every reserved
  and outstanding authorization on that network.

## Payments

- One durable ledger entry is written before any paid request. A receipt call never
  signs a second authorization for the same check; recovery replays the retained
  credential.
- The recovery record contains a usable payment credential until it is resolved. It is
  unencrypted inside the owner-private profile directory. Protect backups of that
  directory the same way.
- Settlement is certified from independent finalized chain evidence, not from the
  server's response. "Charged: pending" does not become "no" because a response was lost.

## Transport and identity

- API and RPC transport use TLS with the local trust store. The API additionally proves
  possession of the receipt key by signing; the identity document is only a cross-check.
- The client sends a `SECONDED-CLIENT-VERSION` header; the service can refuse a client
  below the minimum version for a product.

## What is out of scope

- A compromised host. The client runs with your permissions; it is a containment aid for
  an agent's spending, not a sandbox.
- Collusion of both RPC readers, or a lie every configured endpoint repeats.
- The models being wrong together. Two rival-lab models agreeing is evidence, not proof;
  that is why every answer maps to an action and a `NOT VERIFIED` path exists.
- Anything after the agent acts. SECONDED checks an input; it does not execute, bridge,
  sign, or watch what happens next.
- Server-side retention of inputs and answers, and RPC-provider retention of public
  metadata. The client does not persist raw inputs; the service's policy is at
  https://secondedoracle.xyz/privacy.

## Reporting

See [SECURITY.md](../../SECURITY.md). Email support@secondedoracle.xyz with `SECURITY`
in the subject.
