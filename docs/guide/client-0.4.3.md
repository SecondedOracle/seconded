# Client source 0.4.3 candidate

This is prepared source, not a published 0.4.3 binary release. The public module
name and curated test subset are preserved. The private release builder uses a
separate source snapshot; building this mirror does not reproduce its binary hashes.

- `seconded_degen_check` accepts a Pons V2 token address on Robinhood Chain.
  Its bundled status is `unavailable`; the server decides whether to offer a quote.
  The price is $0.10 small when offered, without the other lookup checks' Robinhood surcharge.
- Degen receipts bind the token, network and evidence and carry signed facts,
  both opinions and freshness. A signed `models_disagree` answer may be charged.
  Disagreement and stale historical results do not authorize an action.
- Discovery has 19 tools: 13 check interfaces and six utilities. Twelve bundled
  products are available; six other products remain in testing. Token Check's
  description now identifies official-token identity and controls as its scope.
- Existing product prices and the six utility tools are unchanged.

The dated claims register describes the prior public snapshot. Production
availability, payment execution and signed-release installation are separate
operator gates; these notes do not claim those gates were run.
