# Client 0.4.3

Publication, distribution links and current counts are recorded in
[README Status](../../README.md#status). These notes describe the released client.

- `seconded_degen_check` accepts a Pons V2 token address on Robinhood Chain.
  The frozen bundle and live availability differ; see the
  [catalog explanation](pricing-and-coverage.md#catalog-versions-and-availability)
  and the price table there. A server quote is required before any purchase.
- Degen receipts bind the token, network and evidence and carry signed facts,
  both opinions and freshness. A signed `models_disagree` answer may be charged.
  Disagreement and stale historical results do not authorize an action.
- Discovery includes Degen alongside the existing checks and utilities. Token Check's
  description identifies official-token identity and controls as its scope.
- Existing product prices and the six utility tools are unchanged.

## Source provenance

The release's signed `build-info.json` identifies source commit
`a8d6360be09f2f95314aac22110a49e1fbe95956` and client tree
`fa40ebcd1191bbc4f35b70a662e06657f4e7fec3`. The public client at
`cc8b49a1f7a95b364685d5647e0221431c2b36b0` preserves that release's default runtime
after the module rename from `seconded.local/client` to
`github.com/SecondedOracle/seconded/client`. Its tests are a selected public subset;
optional build-tag implementations and private integration tests are not exported.
The standalone verifier and examples are maintained separately in this repository.

The public module name and source packaging change build digests, so a source build
does not reproduce the signed release binary hashes. Verify downloaded binaries
using the [release verification guide](release-verification.md).
The documentation and standalone verifier update leaves the released client runtime
and frozen catalogs intact. Live catalog observations do not test paid execution;
see [measurement boundaries](../CLAIMS.md#current-measurements).
