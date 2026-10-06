# Release verification

Client 0.4.1 was published on 2026-10-06 as the npm package
[`@seconded/mcp`](https://www.npmjs.com/package/@seconded/mcp) and the GitHub release
[`SecondedOracle/seconded-mcp-releases` tag `v0.4.1`](https://github.com/SecondedOracle/seconded-mcp-releases/releases/tag/v0.4.1).
The release contains five native binaries, an MCPB bundle, build information,
`SHA-256SUMS` and its OpenSSH signature. Verify the signed manifest and the selected
artifact before running it. This repository's client source is a separate development
snapshot; see the source-provenance note in the [README](../../README.md).

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

## Distribution status

npm and GitHub: 0.4.1 published. Official MCP Registry: active
`xyz.secondedoracle/seconded-mcp` entry at 0.3.3 and older
`io.github.SecondedOracle/seconded-mcp` entry at 0.3.0, observed 2026-10-06.
Homebrew: the expected public tap was not found in the review. Smithery: publication
status not verified.

The npm launcher requires Node 20 or newer. A version-pinned invocation is
`npx --yes @seconded/mcp@0.4.1 serve --host generic`. The documented Homebrew formula
and Smithery templates do not by themselves establish a public installation channel.

Public metadata checked on 2026-10-06:

- [npm package metadata](https://registry.npmjs.org/@seconded%2Fmcp).
- [GitHub release](https://github.com/SecondedOracle/seconded-mcp-releases/releases/tag/v0.4.1).
- [Official MCP Registry search](https://registry.modelcontextprotocol.io/v0.1/servers?search=secondedoracle).

The reviewed npm metadata contains a local archive-path disclosure. This checkout does
not package or republish that release. Distribution metadata cleanup requires a
separate maintainer action; no signing-key disclosure was established.
