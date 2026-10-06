# Verify a receipt offline

The verifier needs Go, the receipt, and nothing else. It makes no network calls.

```sh
cd verifier
go run ./cmd/seconded-verify testdata/refused-base-mainnet-2026-10-06.json
```

Recorded output (2026-10-06, Go 1.26.7):

```text
OK    testdata/refused-base-mainnet-2026-10-06.json
      signature   Ed25519 by rk-2026-09-a over receipt v1 (807 canonical bytes)
      state       refused: the request was refused before admission; nothing charged
      product     null   check_id null   issued_at 2026-10-06T03:57:35.018936Z
      checked_by  none named
      billing     charged=no settlement=none network=eip155:8453 amount_atomic=null tx=null
```

Add `-json` for one JSON object per receipt, which is easier to feed into scripts.

## Verify your own receipt

Every check the client buys returns a receipt in the tool result, and
`seconded_receipt` returns them again later. Save one to a file and run the same
command. An agreed receipt prints the product, the answer label, both labs under
`checked_by`, and the settlement transaction under `billing.tx`.

## Compare the compiled pin with the key the service publishes

```sh
curl -s https://api.secondedoracle.xyz/v1/keys -o keys.json
cd verifier && go run ./cmd/seconded-verify -keys ../keys.json testdata/refused-base-mainnet-2026-10-06.json
```

`-keys` replaces the compiled pin for that run with the keys in the document. If the
document ever listed a different key under `rk-2026-09-a`, the fixture would fail to
verify, which is the point: the client never trusts that document, and now you can see
whether it agrees with the pin.

## What a failure looks like

Change any byte of the envelope and the signature no longer covers it:

```sh
sed 's/"charged": "no"/"charged": "yes"/' testdata/refused-base-mainnet-2026-10-06.json > /tmp/tampered.json
go run ./cmd/seconded-verify /tmp/tampered.json
```

```text
FAIL  /tmp/tampered.json
      signature does not verify
```

The exit status is 1 whenever any receipt on the command line fails.
