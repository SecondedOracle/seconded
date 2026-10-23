# Client upgrades

Version 0.4.5 is PREPARED, not published. Use these instructions only with
a separately authenticated release or an explicitly reviewed candidate.
This document does not establish that 0.4.5 signed assets are available.

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
