# Client changelog

## Client 0.4.5 — PREPARED

Credit refusals now explain the specific cause and whether to wait and retry or
correct the request or service configuration, while preserving existing burns
and recovery steps. Unknown server codes still use the safe fallback.

Existing wallets can explicitly trust a verified same-version rebuild with
`setup --reenroll --release-sha256 SHA256`, using the same `--profile`. The
transition preserves the wallet, backend, policy, ledger and previous fingerprint.
Fingerprint errors now include a repair command in CLI and MCP output. Downgrades
still require the signed-release rollback and human confirmation. A process-level
test proves automatic signed 0.4.4-shaped enrollment → 0.4.5 migration.

This local candidate adds burn-credit buying, quoting and redemption to the MCP
catalog (21 tools). Repeat the same `pack_id` and arguments to recover a buy;
`seconded_receipt` recovers a check, and authenticated credit status is queried
when an older server omits a balance snapshot. There is no separate credit-status
MCP tool. Use `payment_preference: "seconded_discount"`, `network: "robinhood"`
and `agent_id` to opt in.

Credit-paid check results and CLI `recover` output show the dollar balance
**after this check**, its observation time, and `credit_balance_trust: "unsigned"`.
The spendable balance excludes reserved, expired and frozen credits. It is a
transport-authenticated account snapshot outside the signed receipt; it cannot
prove a balance later or change signed billing. Older replies without the new
fields remain supported; a separate status lookup is labelled accordingly.

Saved receipt export is available through `export-receipt CHECK_ID` and MCP
`seconded_receipt` with `export: true`. Export preserves only the original signed
receipt. The release carries forward 0.4.4 Asset Check, quote-network selection,
billing guidance, unsigned service diagnostics and certified-nonpayment recovery
fixes, plus the receipt-export and paid-run regression fixes.

Version 0.4.5 is prepared locally only. Publication, publisher signatures and paid
candidate end-to-end qualification remain separate steps.

Finalized Robinhood receipt recovery now budgets eight seconds for the combined
snapshot and certificate reads. `SECONDED_DEBUG_SETTLEMENT=1` enables sanitized
stderr diagnostics for reader methods, error classes and decisions. Independent
receipt, log, canonical-block and finality requirements are unchanged.

### 0.4.5 re-enrollment hardening

- Require rollback-equivalent controlling-terminal and owner-presence confirmation.
- Keep mismatch recovery human-only and omit computed hashes and finished commands.
- Pin the running executable descriptor before handling commands.
- Release the profile lock before credit-quote network work.
- Keep enrolled-wallet notices distinct from new-wallet funding warnings.
- Document why unselected slots cannot safely be classified as orphaned stages.
