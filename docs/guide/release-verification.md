# Release verification

**Status on 2026-10-06: no release has been published.** The client source here is version
`0.4.1` (`client/api.go`, `ClientVersion`), an unsigned build candidate. The procedure
below is fixed by the shipped client's own setup code (`client/release_digest.go`), so it
is documented now; the publisher key and the download location will be announced at
https://secondedoracle.xyz and on @SecondedOracle when the first release is cut.

## What a release contains

| Asset | Purpose |
| --- | --- |
| `seconded-mcp_darwin_arm64`, `seconded-mcp_darwin_amd64`, `seconded-mcp_linux_amd64`, `seconded-mcp_linux_arm64`, `seconded-mcp_windows_amd64.exe` | The client, one static binary per platform, built with `CGO_ENABLED=0`, `-trimpath`, `-buildvcs=false` and a stripped build id (`client/Makefile`). |
| `seconded-mcp_VERSION.mcpb` | The same binary packaged as an MCP bundle for hosts that install bundles. |
| `build-info.json` | Source commit, toolchain and flags. |
| `SHA-256SUMS` | First line `# seconded-release-version: VERSION`, then one `<sha256>  <asset>` line per asset. |
| `SHA-256SUMS.sig` | An OpenSSH signature over `SHA-256SUMS`, namespace `seconded-release`, made with the publisher's SSH Ed25519 key. |

Releases are deliberately **not** Apple-notarized or Windows-Authenticode-signed; the SSH
signature over the manifest is the publisher authentication. Install through a terminal
or a package manager rather than a browser download so no quarantine flow is involved.

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

## Planned distribution channels

These names come from the packaging templates in the private repository and are listed so
nobody is surprised later. None exists yet.

- GitHub release repository: `SecondedOracle/seconded-mcp-releases`, with an
  `install.sh` that performs the verification above before installing.
- npm launcher: `@seconded/mcp` (`npx --yes @seconded/mcp@VERSION serve --host generic`, Node 20+).
- Homebrew: `brew install SecondedOracle/tap/seconded-mcp`, with the formula pinning every
  asset checksum and verifying the SSH signature.
- Official MCP Registry name `io.github.SecondedOracle/seconded-mcp`, and Smithery.
