# MCP v1 client qualification

Version 0.4.5 is PREPARED, not published. Its catalog has 21 tools; see
[the client changelog](CHANGELOG.md) for the current candidate. The older
qualification observations below are historical, including the earlier
18-tool catalog; this preparation makes no live availability measurement.

The default payment adapter uses `POST /v1/x402/checks` with x402 v2 exact
EIP-3009. The original adapter is retained only for an explicit launch-time
rollback: `seconded-mcp serve --original-door-rollback`. It is never selected
by an MCP argument or an automatic retry. Keep rollback until the separately
authorized Base, Arc and Robinhood mainnet canaries pass.

Each purchase has a local check/recovery ID before sending. The private ledger
atomically stores the exact canonical HTTP body, URL, payment header, entire
accepted requirement, quote, canonical purchase request, association digest and
unresolved state before any paid request. Receipt verification authenticates the
server check ID through the signed purchase association; the local ID remains
stable for `seconded_receipt` after a lost response. No new purchase is permitted
while a retained record is unresolved. A receipt call performs at most one
same-credential POST per entry, respects persisted retry timing, and requires
no wallet key for standard entries, even in a mixed ledger. A signer is loaded
only when a selected original-door entry actually needs collection.
`seconded-mcp recover` also opens only the private ledger and pinned API when
the wallet cannot be unlocked. No signing or payment wrapper participates
in this replay. Verified terminal service failures are archived even on HTTP 503;
a new purchase then requires `new_quote_allowed`. Affirmative answers require
HTTP 200. Unverified responses cannot
release uncertainty; generic pre-admission refusals without a signed purchase
association also remain unresolved. Refusals retain the credential until
authorization expiry.

The recovery record contains sensitive input and a usable payment credential.
It is unencrypted inside the owner-private ledger (private directory, file modes
or Windows ACLs, atomic replacement and sync). It never enters MCP output or logs.
Verified terminal receipts are stored atomically with removal of the raw payment
header, duplicate payload, exact body and canonical request. The archival record
retains hashes and purchase association metadata for receipt verification. A
certified refusal retains replay material until validBefore has passed; unresolved
records retain it for recovery. Old resolved records are minimized when retrieved
and re-verified. Protect backups too: atomic replacement does not erase old backup
copies. Before admission, capture of the standard payment header can allow
someone else to use the authorization.
Use direct TLS, never proxies or shared logs. The client does not opt into SIWX.

New setups use $2.50 per check and $25/day. The agent can only tighten limits or
freeze payments. Chat can set a daily limit only at or below $25, including
when replacing an absent or higher owner-approved daily limit. All increases,
removals, safety-switch weakening, unfreezing and
wallet changes require a controlling-terminal human confirmation. Use
`seconded-mcp wallet --human --switch back` or `--switch newer`, including the
same `--profile` prefix used by the host. Legacy profiles with no daily cap or
above $25/day freeze once pending owner review; terminal-approved policy survives
subsequent starts. Retained-wallet charges and unresolved credentials continue to
count against profile policy after an owner switches wallets.

Setup prefers the OS credential store. Automatic fallback stores an **unencrypted**
private wallet file, clearly identifies the backend before funding, and recommends
only a few dollars in the dedicated wallet. Same-user processes can read that file.

`host-snippet --host gemini` emits the `mcpServers` entry for
`~/.gemini/settings.json`; merge it at user scope and restart Gemini CLI. Existing
Claude Desktop, Claude Code, Cursor, Codex and generic snippets remain supported.
This task verifies rendered snippets, not native host import.

`products-live-v1.json` is the read-only capture of
https://api.secondedoracle.xyz/v1/products on 2026-09-30. Schema, status, tiers,
prices, networks and payment pins are compared structurally with `products.json`.
The test's explicit annotation allowlist permits local concise descriptions,
examples, coverage and expanded label-to-action guidance; it cannot hide input
schema or price drift.
The client supports 12 products and 18 MCP tools. Products comprise
`trade_check`, `stock_token_check`, `token_check`, `agent_registry_check`, `counterparty_check`, `cross_chain_compare`, `scam_check`, `lending_check`, `job_escrow_check`, `x402_payment_check`, `shielded_route_check`, `route_check`.
Tools comprise the 12 corresponding check tools (
`seconded_trade_check`, `seconded_stock_token_check`, `seconded_token_check`, `seconded_agent_registry_check`, `seconded_counterparty_check`, `seconded_cross_chain_compare`, `seconded_scam_check`, `seconded_lending_check`, `seconded_job_escrow_check`, `seconded_x402_payment_check`, `seconded_shielded_route_check`, `seconded_route_check`) plus six account and control
tools (`seconded_get_limits`, `seconded_set_limits`, `seconded_products`,
`seconded_quote`, `seconded_receipt`, `seconded_wallet`). Small-tier lookup checks
are $0.10 ($0.15 on Robinhood); Trade, Agent Registry, Lending, Agent Work Payout,
and Shielded Route are $0.25. Medium is
$1.50 and large $2.50 where sold.
Vault, Private Receive, Portfolio, Hidden Prompt and Owner Instruction remain in testing.
The four newly available checks require stable client version 0.4.0 or newer;
older, absent-version and prerelease clients do not discover or purchase them.

Cross-Chain Compare (`seconded_cross_chain_compare`) is $0.10 ($0.15 on Robinhood) (Small only). One
purchase covers both chains on the standard x402 door (`POST /v1/x402/checks`),
requiring stable client version 0.3.1 or higher. Supported comparisons are fixed
Circle USDC to EURC across Base (8453, Uniswap V3, 500 ppm fee) and Arc (5042,
Uniswap V4, 180 ppm fee) for trade sizes of 1 to 10,000 USDC inclusive (`exact_in`).
Quotes are pool simulations after pool fees; do not subtract fees again. The tool
does not perform arbitrary routing, trade execution, bridging, transaction signing,
or non-Circle asset swaps. No ranking or savings guarantee is provided. Liquidity
depth, slippage, and price impact are not checked; execution gas and total costs
are unknown; bridge costs are excluded.

Keep the device clock synchronized. Client receipt verification strictly enforces
exact timestamps: `observed_at <= outcome_at <= valid_until` and `outcome_at <= client_now`.
Future timestamps are rejected without clock tolerance or grace windows. If device
clock lag temporarily rejects a comparison receipt, retry collection for the
existing purchase after the clock catches up, including later historical recovery.
Use `seconded_receipt` with the retained `check_id`, or the same profile used for
the purchase:

```sh
seconded-mcp --profile /absolute/profile/directory recover --check-id LOCAL_CHECK_ID
```

Omit `--check-id` to collect recent and unresolved checks. Never repurchase merely
to collect the original result.
Signed quote validity (`valid_until`) and the MCP presentation flag
(`comparison_expired: true` on expired historical quotes) remain distinct.

Run `go test ./...` from `client/`. Install the optional offline oracles with:

```
uv venv interop/.venv
uv pip install --python interop/.venv/bin/python --require-hashes -r interop/requirements.lock
npm ci --ignore-scripts --prefix interop
go test -v -run TestStandardPinnedInteropOracles .
```

Python x402 2.24.0 and TypeScript @x402 2.27.0 decode the same local 402/payment
fixtures. Independent EIP-712 implementations recover the Go signer for Base,
Arc and Robinhood, and reject a changed amount. Python also verifies the signed
202 receipt and recomputed purchase association. No oracle sends HTTP or loads
real keys; only transient never-funded fixture wallets are used. This is offline
codec/cryptography conformance, not live stock-SDK transport or settlement proof.

Release identity is GitHub/Smithery `SecondedOracle`, support
`support@secondedoracle.xyz`; announce the publisher key on @SecondedOracle and
secondedoracle.xyz. Ship platform-unsigned under the operator decision: no Apple
notarization or Windows Authenticode is claimed. Use published checksums and the
signed release manifest, with terminal/package install paths rather than browser
quarantine flows. Release packaging, publication, and the $0.75 total mainnet
canaries are separate operator-authorized work. No live payment is part of these
tests or this implementation task.

### Optional SECONDED burn credits

Paid check tools accept `payment_preference: "seconded_discount"`,
`network: "robinhood"`, and a stable `agent_id`. They spend prepaid credits first
and use the existing x402 payment path on an explicit insufficient-credit challenge.
Owner spending limits still apply. After a timeout, use `seconded_receipt`.

Use `seconded_credits_quote` with `agent_id` and `face_usd_micros` (minimum
`5000000`) to obtain a 300-second quote. Use `seconded_credits_redeem` with
`tx_hash` and the entire unchanged `quote` object to redeem a finalized burn from
the check wallet. Redemption signs a deployment-bound proof and returns the lot;
it does not send a chain transaction. Quotes use spot valuation and exclude swap
fees, slippage, and gas. These tools require the server burn-credit feature.

`seconded_credits_buy` runs the pinned USDG → native ETH → SECONDED route,
burns exactly the received SECONDED, waits for ten successors agreed by two
readers, and redeems the burn. It requires all of:

- `pack_id`: a new 32-character lowercase hex identifier. Repeat the **same ID
  and arguments** while the result is pending; retries reuse signed transactions.
- `agent_id`, `face_usd_micros` (`5000000`–`10000000`, or $5–$10).
- `input_usdg_micros`: the exact USDG input, including the swap-fee/slippage
  allowance you choose, at least 80% of face and at most `10000000` ($10).
  Packs have a separate $10 per-pack cap; the $2.50 per-check cap still applies
  to ordinary check payments. Hour, day, outstanding limits and freezes still apply.
- `slippage_bps`: 0–1000. The output minimum must also cover the burn quote.
- `gas_limit`: 21000–2000000 per transaction, `max_fee_per_gas_wei`,
  `max_priority_fee_per_gas_wei`, and `max_total_gas_wei` as decimal strings.
  The total must cover four times gas limit times maximum fee.

Each call advances the durable pack and may return pending. The four possible
transactions are exact USDG approval to Permit2, expiring exact Permit2 approval
to the pinned router, swap, and burn. Token acquisition and later credit spending
both count conservatively toward owner USD limits; gas has its separate explicit
wei cap. No unlimited approval or nonce replacement is used. No new transaction
is signed or broadcast under an expired quote. A previously broadcast transaction
may still confirm later. If a burn outlives its quote, obtain a fresh quote and use
`seconded_credits_redeem` with that burn hash; a successful redemption closes the
matching local pack. If only the swap completed, the tokens remain in the wallet.

The client retains its private pack journal for retry and accounting. Raw signed
transactions are cleared after confirmed stages. Quotes, public transaction
hashes, and credit lots remain. Deployment code pins and ABI are tested locally;
live transaction execution requires separate operational validation.

## Export a signed receipt

`seconded-mcp [--profile /absolute/directory] export-receipt CHECK_ID > RECEIPT.json`
exports the locally saved purchase receipt after signature and purchase-binding
verification. It requires no wallet key or network access. The output is the
standalone `envelope`, `sig`, `key_id` document accepted by `seconded-verify`.

For MCP, call `seconded_receipt` with `{"check_id":"CHECK_ID","export":true}`.
This reads the saved receipt without recovering a newer revision. Omit `export`
for the existing recovery/listing behavior. Export requires a local check ID and
fails if no signed receipt is saved or verification fails. The signed server ID
inside the envelope can differ from that local handle.

The export includes signed billing addresses and findings, but excludes local
request bytes, payment credentials and commitment salts. Signed billing states
what the service attests; independent chain evidence is still required to confirm
payment finality. Save or share the exported file only with its intended recipient.

## Client 0.4.5 candidate

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

### Settlement diagnostics

Set `SECONDED_DEBUG_SETTLEMENT=1` when running `recover` to send chain-reader
hostnames, RPC method names, fixed error classes and reconciliation decisions to
stderr. The diagnostic omits URL paths, credentials, signatures, addresses and
request/response bodies. It is disabled by default and leaves JSON stdout intact.
Finalized Robinhood receipt recovery allows up to eight seconds for the snapshot
and independent certificate together, subject to an earlier caller deadline.

Developers can run a read-only, profile-free diagnostic against the compiled
Robinhood mainnet reader pool with an exported signed Degen receipt:

```sh
SECONDED_LIVE_SETTLEMENT_RECEIPT=/path/to/exported-receipt.json \
SECONDED_LIVE_SETTLEMENT_CONTROLS=1 \
go test -mod=vendor -run '^TestLiveSettlementCertificate$' -count=1 -v
```

This opt-in test reconstructs public purchase fields and reads the authorization
nonce from the chain. It calls the production recovery function with its normal
reader construction and timeout, then checks that a wrong nonce retains the
reservation. It does not open a profile or send a payment. It cannot reproduce
profile-specific anchors or establish why an earlier recovery failed.

## Wallet upgrades and re-enrollment

Newer signed releases automatically move an existing wallet to the running
binary when its platform digest and version match the authenticated
`SHA-256SUMS` and `SHA-256SUMS.sig` beside it. Same-version rebuilds, unsigned
builds and mismatching release metadata require an explicit trust decision.

If you see `fingerprint_mismatch`, independently verify the replacement build
against the published SHA-256SUMS, then run this template in your own
terminal after replacing VERIFIED_SHA256 with the published digest:

```sh
seconded-mcp --profile /absolute/profile/directory setup --reenroll --release-sha256 VERIFIED_SHA256
```

Use the same executable and profile as your MCP host. Omit `--profile` only if
you use the default profile. `VERIFIED_SHA256` is the 64-character digest from
your independently trusted source; a displayed or locally computed hash alone
does not authenticate a build. Re-enrollment supports file and OS keystore
wallets, requires the existing wallet, and never prints or exports its key.
It keeps the address, backend, limits, payment records and previous fingerprint.
Legacy policy review still applies: profiles without a daily cap or above
$25/day freeze pending owner review. The new credential is staged before the
installation selection changes; previous records remain available for recovery.

A wrong hash, missing wallet or inconsistent enrollment is refused. Do not
create or fund a replacement wallet to work around this error. Preserve and
restore the existing profile if records are missing or inconsistent. Unlock an
unavailable OS credential store and retry; re-enrollment never switches backends.

Re-enrollment refuses versions below the enrolled version, including prereleases
below a stable version. To downgrade, run the older **signed** executable with
`--profile /absolute/profile/directory rollback` and complete the existing human
terminal confirmation. Same-version builds can be selected again by explicitly
re-enrolling their independently verified hash. Afterward, restart your MCP host.
No download, payment, key export or publication occurs during re-enrollment.

Re-enrollment requires a controlling terminal and the exact displayed phrase
`reenroll <last six address characters>`. Review the wallet address, old and new
fingerprints, and versions before confirming. The independent owner-presence
check used for rollback also applies; without a terminal the operation returns
`terminal_policy_required`. MCP returns `next: human_verification_required` and
never supplies the running binary's digest or a completed recovery command.
A software-controlled PTY remains a residual risk when independent owner
presence is unavailable; the terminal fallback has the same limits as rollback.

Enrollment hashes a descriptor opened at process startup (`/proc/self/exe` on
Linux), so later replacement of the executable pathname cannot redirect the
check or commit. This does not attest bytes before the initial open on desktop
platforms or protect against in-place modification of that same inode.

Unselected enrollment slots are retained. Existing records do not distinguish
an interrupted stage from previously committed recovery history, and the OS
store interface has neither enumeration nor deletion. Deleting every unselected
slot is therefore unsafe. A future format can journal staging intent before
writing a credential and clean only that identified, unselected stage.
