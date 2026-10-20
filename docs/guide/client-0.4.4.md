# Client 0.4.4

Client 0.4.4 is published on npm as [`@seconded/mcp@0.4.4`](https://registry.npmjs.org/@seconded%2Fmcp/0.4.4) and on GitHub as [v0.4.4](https://github.com/SecondedOracle/seconded-mcp-releases/releases/tag/v0.4.4). Release assets are signed: verify `SHA-256SUMS.sig` with `allowed_signers`, then check the downloaded artifact hashes. Publication, distribution links and current counts are recorded in [README Status](../../README.md#status); publication alone does not establish live server availability.

- Degen is labelled available in the bundled offline snapshot. Standard disagreement is signed and free; Degen costs $0.10 for a saved signed answer, including `models_disagree`.
- Utility tools reject malformed arguments before wallet access. Unsigned 503 diagnostics remain diagnostics; independently certified nonpayment permits the next purchase.
- `prove --help` and `recover --help` print usage. Missing-ledger recovery describes the profile without referring to an unspecified check.
- Asset Check selects token or stock-token checks. Discovery has 18 tools for 13 products and six utilities; quotes accept explicit payment networks. Prices are unchanged.

## Source provenance

The release's signed `build-info.json` identifies source commit `7aeb47c829c87613b12c4f2af5484dfee48fc301` and client tree `1a6f67bb362ce095271b97b1744642b80d4a80e6`. This incremental source sync starts from public commit `917369618a0afa41eda3e88a470a34b8aa1510b6`, preserving the curated client subset, verifier, fixtures, licences and CI. The default client runtime matches those inputs after the established module rename from `seconded.local/client` to `github.com/SecondedOracle/seconded/client`; optional private build tags and integration tests remain outside this source tree.

The public module rename and selected source packaging change binary hashes. This source tree is not byte-identical to release executables; verify downloaded artifacts with the independently authenticated publisher key and signed manifest. The new billing-parity regression pins initialization text to the catalog's usage line.

The existing offline client and verifier checks apply. The published checksum signature verifies with the release repository's signer; native installation/upgrade/rollback, paid checks and settlement were not measured by this source sync. See [release verification](release-verification.md) and [billing policy](how-it-works.md#billing-policy).
