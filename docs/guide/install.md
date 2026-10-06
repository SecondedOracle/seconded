# Install and run

## 1. Build

Go 1.26 is required (`client/go.mod` names the toolchain). Dependencies are vendored, so
no network access is needed:

```sh
cd client
go build -o seconded-mcp ./cmd/seconded-mcp
./seconded-mcp --version      # seconded-mcp 0.4.1
```

Until a signed release exists this is the only way to get the client. See
[Release verification](release-verification.md) for what changes once one does.

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
A different profile directory can be chosen with `--profile /absolute/path`; use the same
`--profile` in the host command.

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

Recovery loads no wallet key and never buys the check again.

## Commands

`seconded-mcp --help` prints: `setup, host-snippet, serve, self-check, prove, recover,
limits, wallet, rollback, export-key, link coinbase, help, version`. `prove` checks that a
stored receipt is bound to a given input file. `export-key` prints the wallet key to the
controlling terminal after confirmation and is the only way the key leaves the machine.
`link coinbase` is a stub that reports it is not implemented.

## Keep the clock right

Receipt verification is strict about time: `observed_at <= outcome_at <= valid_until` and
`outcome_at <= now`, with at most 30 seconds of tolerance on issuance. A machine with a
lagging clock will reject fresh receipts until it catches up; recover the existing
purchase afterwards rather than buying again.
