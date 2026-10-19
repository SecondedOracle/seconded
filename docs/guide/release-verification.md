# Release verification

Use the published release linked in [README Status](../../README.md#status).
Release tags and assets are hosted in `SecondedOracle/seconded-mcp-releases`;
this repository contains the public source and independent receipt verifier.
The release contains five native binaries, an MCPB bundle, build information,
`SHA-256SUMS` and its OpenSSH signature. Verify the signed manifest and the selected
artifact before running it. Release binaries are built from private release source
and verified by the signed manifest. This public tree uses a renamed module path
and a selected test subset. See the exact [source provenance](client-0.4.3.md#source-provenance);
building the public source does not reproduce release binary hashes.

## What a release contains

| Asset | Purpose |
| --- | --- |
| `seconded-mcp_darwin_arm64`, `seconded-mcp_darwin_amd64`, `seconded-mcp_linux_amd64`, `seconded-mcp_linux_arm64`, `seconded-mcp_windows_amd64.exe` | The client, one native binary per platform; published build information declares `CGO_ENABLED=0`, `-trimpath`, `-buildvcs=false` and a stripped build id (`client/Makefile`). |
| `seconded-mcp_VERSION.mcpb` | The same binary packaged as an MCP bundle for hosts that install bundles. |
| `build-info.json` | Source commit, toolchain and flags. |
| `SHA-256SUMS` | First line `# seconded-release-version: VERSION`, then one `<sha256>  <asset>` line per asset. |
| `SHA-256SUMS.sig` | An OpenSSH signature over `SHA-256SUMS`, namespace `seconded-release`, made with the publisher's SSH Ed25519 key. |

The release documents no Apple notarization or Windows Authenticode signature. Its
SSH-signed checksum manifest is a separate publisher-verification mechanism; it does
not bypass operating-system security prompts. Follow the platform's review flow if
it blocks execution.

## Verify before running

```sh
# 1. Put the announced publisher key into an allowed_signers file.
printf 'release@seconded namespaces="seconded-release" ssh-ed25519 %s\n' 'ANNOUNCED_PUBLIC_KEY' > allowed_signers

# 2. Verify the manifest signature.
ssh-keygen -Y verify -f allowed_signers -I release@seconded -n seconded-release -s SHA-256SUMS.sig < SHA-256SUMS

# 3. Verify the binary against the manifest.
shasum -a 256 -c SHA-256SUMS
```

Authenticate `ANNOUNCED_PUBLIC_KEY` through a channel other than the download itself. A
key published only beside its own signature proves nothing.

## What the client checks on its own

`seconded-mcp setup` repeats steps 2 and 3 before creating a wallet: it requires
`SHA-256SUMS` and `SHA-256SUMS.sig` beside the executable, verifies the signature with the
system `ssh-keygen` against the publisher key compiled into that release, then compares
the executable's SHA-256 with the manifest entry for its platform. A mismatch stops setup
with `release_digest_mismatch`. The client first proves that `ssh-keygen -Y verify` works on
the machine using a public capability fixture (`client/release_capability.go`), so a broken
helper is reported as `release_signature_unavailable` rather than passing silently.

A source build has no publisher key compiled in. To set it up anyway, compute the
binary's digest yourself and pass it explicitly:

```sh
cd client && go build -o seconded-mcp ./cmd/seconded-mcp
./seconded-mcp setup --manual-fingerprint --release-sha256 "$(shasum -a 256 seconded-mcp | cut -d' ' -f1)"
```

## Distribution boundaries

Publication and latest-version facts live in [README Status](../../README.md#status).
The npm launcher requires Node 20 or newer. For this release, a version-pinned
invocation is `npx --yes @seconded/mcp@0.4.3 serve --host generic`.
Use top-level `--help` for command discovery and the
[standalone verifier](../../examples/verify-receipt/) for receipt files.

The source in this repository is Apache-2.0. The separately packaged npm distribution
still declares `license: UNLICENSED` in registry metadata measured on 2026-10-19.
That discrepancy needs clarification by the package publisher; this repository does
not establish a licence grant for a separate artifact or silently replace its metadata.

The npm browser page can return HTTP 403 in automated checks. The
[registry document](https://registry.npmjs.org/@seconded%2Fmcp) provides package versions
and publication timestamps; an HTML access failure is not evidence that a release is absent.

### Historical distribution observations (2026-10-06)

The [claims register](../CLAIMS.md#historical-distribution-observations-2026-10-06)
preserves older npm, GitHub and MCP Registry observations. Homebrew and Smithery
availability has not been re-established here; templates alone do not prove publication.
The Registry search timed out again on 2026-10-19, so its listing status must
be checked independently before recommending that installation channel.

Historical npm metadata contains publisher-identity and local archive-path disclosures.
The published package README also retains template text. These are external publication
issues; this checkout does not package or republish npm artifacts. Cleanup requires
package-publisher action. No private signing-key disclosure was established by the audit.
