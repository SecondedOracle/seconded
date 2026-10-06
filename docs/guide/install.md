# Install and run

## 1. Build

The module requires Go 1.26.6 or newer and selects toolchain Go 1.26.7. For an
offline build, install or cache that toolchain first. Dependencies are vendored;
`GOPROXY=off` prevents dependency downloads:

```sh
cd client
GOPROXY=off go build -o seconded-mcp ./cmd/seconded-mcp
./seconded-mcp --version      # seconded-mcp 0.4.1
```

You can build this checkout or obtain client 0.4.1 from the published
[GitHub release](https://github.com/SecondedOracle/seconded-mcp-releases/releases/tag/v0.4.1)
or [npm package](https://www.npmjs.com/package/@seconded/mcp). These are different source
snapshots despite reporting the same version; choose the provenance you intend to review.
See [Release verification](release-verification.md).

Once the public repository is published, the Go install paths will be:

```sh
go install github.com/SecondedOracle/seconded/client/cmd/seconded-mcp@latest
go install github.com/SecondedOracle/seconded/verifier/cmd/seconded-verify@latest
```

These install commands require network access and place executables in `GOBIN`
(or `$(go env GOPATH)/bin` by default). They build the public modules; renamed
module paths change build digests. Published release binaries instead come from
private release source and are verified against its signed manifest. The source
build and setup examples here use the local `./seconded-mcp` executable.

## 2. Create the wallet and connect a host

```sh
./seconded-mcp setup --host claude-code \
  --manual-fingerprint --release-sha256 "$(shasum -a 256 seconded-mcp | cut -d' ' -f1)"
```

`setup` creates the dedicated check wallet, stores it in the OS credential store (or an
unencrypted private file if none is available, which it tells you before you fund
anything), applies the default limits, and prints the host configuration. Hosts:
`claude-code`, `claude-desktop`, `cursor`, `codex`, `gemini`, `generic`. The printed
snippets for each host are recorded in [`examples/host-config`](../../examples/host-config/).

`--manual-fingerprint` is for source builds only; a released binary verifies its signed
manifest automatically and refuses to run with a stale or altered one.

To print the host snippet again later: `./seconded-mcp host-snippet --host HOST`.
Use `seconded-mcp --profile /absolute/path COMMAND ...`; `--profile` precedes the
command. Use the same profile in the host command.

## 3. Fund it

Ask the agent for `seconded_wallet` (action `status`), or run `./seconded-mcp self-check`,
to see the address. Checks pay on **Base mainnet in USDC** unless a check names another
network: USDC on Arc, USDG on Robinhood Chain, or testnet funds on Base Sepolia, Arc
testnet or Robinhood testnet. Fund only what you intend to spend; the small-tier checks
cost $0.10 to $0.25.

## 4. Limits

| Default | Value |
| --- | --- |
| Per check | $2.50 |
| Per day | $25 |
| Per hour, outstanding | none |
| Value gate (require `max_price_usd` on every call) | off |
| Dedupe, loop brake, alerts | on |

The agent can tighten any of these through `seconded_set_limits` and can freeze new
authorizations. Raising, removing, unfreezing or switching wallets needs you at the
terminal:

```sh
./seconded-mcp limits --show
./seconded-mcp limits --set day=50 --human
./seconded-mcp wallet --human --switch newer
```

Custom limits at setup time need `setup --advanced`.

## 5. Recover a check

If a host deadline cut a call short, the agent calls `seconded_receipt`. From the shell:

```sh
./seconded-mcp recover                      # lists recent and unresolved checks
./seconded-mcp recover --check-id LOCAL_ID  # collects one
```

Retained standard-door recovery replays the same stored request and payment
authorization without loading a signing key or buying a new check. Original-door or
archived ownership recovery may require a wallet signing key; the keyless CLI can
report `wallet_recovery_required` for those cases. Recovery can contact the API and
chain readers. Fetching remains subject to service availability and retention; the
[published policy](https://secondedoracle.xyz/privacy) removes finished-check records
after 90 days from last activity. Keep receipts you need to verify later.

## Commands

`seconded-mcp --help` prints: `setup, host-snippet, serve, self-check, prove, recover,
limits, wallet, rollback, export-key, link coinbase, help, version`.
`prove CHECK_ID INPUT_FILE` checks a stored receipt against supplied input when the
required local binding is retained. In this snapshot it rejects standard-door records
after archival clears the canonical request; archived-hash support requires a follow-up
fix (`client/proof.go`, `client/standard.go`).

`export-key` is the supported client interface for displaying a wallet private key,
and it requires owner approval at the controlling terminal. This does not protect
against software running with the same user's access to the file or credential store.
`link coinbase` is a stub that reports it is not implemented.

## Keep the clock right

The client rejects receipts issued more than 30 seconds in the future and outcomes
later than issuance, with additional purchase-time checks. Cross-chain and portfolio
answers have product-specific observation/expiry rules. A lagging clock can reject a
fresh receipt; recover the same purchase after correcting the clock.

Setup and funding commands above are instructions, not evidence of a paid canary.
The offline checks described in [CLAIMS](../CLAIMS.md) did not create or fund a wallet.
