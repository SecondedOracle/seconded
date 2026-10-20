<div align="center">

<img src="assets/repo-banner-1600x500.png" alt="SECONDED. The oracle for agents." width="800">

**The oracle for agents: before an AI agent acts, two AI models from rival labs check the input, and every answer comes with a signed receipt; no answer, no charge.**

![client](https://img.shields.io/badge/client-0.4.4-blue)
![go](https://img.shields.io/badge/go-1.26.7-00ADD8)
![receipts](https://img.shields.io/badge/receipts-Ed25519%20%C2%B7%20RFC%208785-success)
![chains](https://img.shields.io/badge/pays%20on-Base%20%C2%B7%20Arc%20%C2%B7%20Robinhood%20Chain-informational)
[![license](https://img.shields.io/badge/license-Apache--2.0-blue)](LICENSE)
[![ci](https://github.com/SecondedOracle/seconded/actions/workflows/ci.yml/badge.svg)](https://github.com/SecondedOracle/seconded/actions/workflows/ci.yml)

</div>

> See [Status](#status) for the published release, tool counts and measured live availability.
> Standard checks require agreement; Degen can also return a chargeable signed
> `models_disagree` answer. See [the release notes](docs/guide/client-0.4.4.md)
> and [billing policy](docs/guide/how-it-works.md#billing-policy).

**Verify a receipt**<br>
Use the [Go verifier](examples/verify-receipt/) with Go 1.26.6+ from the repository root.<br>
`(cd verifier && go run ./cmd/seconded-verify testdata/refused-base-mainnet-2026-10-06.json)`



---


### Why it matters

- **Autonomous agents make high-stakes mistakes:** Before an AI agent signs a transaction, executes a trade, or trusts external input, it adds an independent check before the agent acts.
- **Rival-lab consensus reduces single-vendor blind spots:** Every check queries models from competing labs; standard checks require agreement before an answer is accepted. Degen Check can also deliver a chargeable signed disagreement.
- **Signed receipts anyone can verify:** Every response carries an Ed25519-signed receipt over canonical JSON; standard checks that disagree or cannot verify the action return NOT VERIFIED; inspect signed billing for the payment state. Degen disagreement is a separate chargeable answer.

---

## Try it

### 1. Install the MCP client

Client 0.4.4 is published on npm and GitHub. Install the pinned package or matching signed binaries; verify `SHA-256SUMS.sig` with `allowed_signers`, then check the artifact hashes as described in the [release verification guide](docs/guide/release-verification.md):

- **npm:**
  ```sh
  npx @seconded/mcp@0.4.4 --version
  ```
- **Releases repository:** Precompiled binaries and signed release manifests are available at [SecondedOracle/seconded-mcp-releases](https://github.com/SecondedOracle/seconded-mcp-releases).

To connect the client to Claude Code, Claude Desktop, Cursor, or Gemini, consult the [Installation guide](docs/guide/install.md).

### 2. Verify a receipt offline

You can verify signed receipts independently without an account, API key, or network access using the standalone verifier:

```sh
(cd verifier && go run ./cmd/seconded-verify testdata/refused-base-mainnet-2026-10-06.json)
```

The verifier checks the Ed25519 signature against the pinned key and prints the canonical envelope details:

```text
OK    testdata/refused-base-mainnet-2026-10-06.json
      signature   Ed25519 by rk-2026-09-a over receipt v1 (807 canonical bytes)
      state       refused: request refused before admission; consult signed billing and payment evidence
      product     null   check_id null   issued_at 2026-10-06T03:57:35.018936Z
      checked_by  none named
      billing     charged=no settlement=none network=eip155:8453 amount_atomic=null tx=null
```

You can also test with the committed paid trade check and token disagreement receipts:

```sh
(cd verifier && go run ./cmd/seconded-verify testdata/trade-paid-base-mainnet-2026-09-29.json testdata/token-not-verified-base-sepolia-2026-09-29.json)
```

See [docs/guide/receipts.md](docs/guide/receipts.md) for details on receipt structure and verification.

---

## For developers

This repository contains the buildable public surface: the Go MCP client, the standalone receipt verifier, tool and price catalogs, and documentation. For comprehensive technical details, see the guides below:

- **[How a check works](docs/guide/how-it-works.md):** The x402 payment flow, rival-model evaluation, and settlement policy.
- **[Receipts and verification](docs/guide/receipts.md):** Ed25519 signatures, RFC 8785 canonicalization, and verifying receipts offline.
- **[Tools reference](docs/guide/tools.md):** MCP checks and utilities, their arguments and result formats.
- **[Pricing and coverage](docs/guide/pricing-and-coverage.md):** Product prices, subject chains and payment networks (Base, Arc, Robinhood Chain).
- **[Install and run](docs/guide/install.md):** Building from source, setting up the dedicated wallet, and configuring spending limits.
- **[Security model](docs/guide/security-model.md):** Trust boundaries, hot wallet protections, and verification limits.
- **[Release verification](docs/guide/release-verification.md):** Verifying published release manifests and binary provenance.
- **[Claims register](docs/CLAIMS.md):** Principal claims, point-in-time evidence, and documented boundaries.

---

## Status

| Component | State on 2026-10-19 | Evidence |
| --- | --- | --- |
| Public API (`api.secondedoracle.xyz`) | Products and keys endpoints reachable; 3 mainnet payment networks advertised | Header-aware GETs in [CLAIMS](docs/CLAIMS.md#current-measurements) |
| Client source | Released 0.4.4 runtime with the public module rename and selected tests | `client/api.go`, [source provenance](docs/guide/client-0.4.4.md#source-provenance) |
| Receipt verifier | Builds; verifies public fixtures; parser and tamper controls | `verifier/receipt_test.go` |
| Catalog and tools | 18 MCP tools: 12 check interfaces and 6 utilities for 13 products. Asset Check selects token or stock_token. All 13 are available in the labelled offline snapshot; current availability requires a quote. Six other products remain in testing. | [Measured counts](docs/CLAIMS.md#catalog-facts), [bundled versus live catalog](docs/guide/pricing-and-coverage.md#catalog-versions-and-availability) |
| Distribution | npm `@seconded/mcp@0.4.4` is published and latest; GitHub v0.4.4 assets are signed | [npm metadata](https://registry.npmjs.org/@seconded%2Fmcp/0.4.4), [GitHub release](https://github.com/SecondedOracle/seconded-mcp-releases/releases/tag/v0.4.4), [release verification](docs/guide/release-verification.md) |
| Licence | Apache-2.0 | [LICENSE](LICENSE), [NOTICE](NOTICE) |

Release tags and downloads live in [seconded-mcp-releases](https://github.com/SecondedOracle/seconded-mcp-releases), separately from this source repository.
Availability is an advertisement, not evidence of a successful paid check on every network.
The separately packaged npm distribution has a [licensing metadata discrepancy](docs/guide/release-verification.md#distribution-boundaries).

---

## $SECONDED token

The only official $SECONDED contract, on Robinhood Chain:

```
0x833a7fA750628b391c35bf384d7706652c7e8202
```

Always check the address against [secondedoracle.xyz/token](https://secondedoracle.xyz/token) before interacting. Any other address is not ours.

---

## Colosseum

This repository is SECONDED's public showcase and trust surface. The full development history and the server source are shared privately with the Colosseum judges through a separate private repository, which serves as the official submission link.

---

## Contributing, security, licence

- **Contributing:** See [CONTRIBUTING.md](CONTRIBUTING.md) for build instructions and submission guidelines.
- **Security:** Report security vulnerabilities to <support@secondedoracle.xyz> with `SECURITY` in the subject. See [SECURITY.md](SECURITY.md).
- **Licence:** Code in this repository is licensed under [Apache-2.0](LICENSE); see [NOTICE](NOTICE). Vendored dependencies retain their own licences, listed in [THIRD_PARTY.md](THIRD_PARTY.md).
- **Trademarks:** SECONDED and the glass-S mark are trademarks of the maintainers; the licence covers the code, not the name or logo.
- **Changelog:** [CHANGELOG.md](CHANGELOG.md) records changes across client releases.
