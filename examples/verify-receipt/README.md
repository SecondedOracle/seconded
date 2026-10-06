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
      state       refused: request refused before admission; consult signed billing and payment evidence
      product     null   check_id null   issued_at 2026-10-06T03:57:35.018936Z
      checked_by  none named
      billing     charged=no settlement=none network=eip155:8453 amount_atomic=null tx=null
```

Add `-json` for one JSON object per receipt, which is easier to feed into scripts.

## Verify your own receipt

Delivered check results carry receipts; `seconded_receipt` can recover them subject
to service availability and retention. Keep a local copy. Save one to a file and run the same
command. An agreed receipt prints the product, the answer label, both labs under
`checked_by`, and the settlement transaction under `billing.tx` when present. An agreed answer
may arrive before settlement; a signature is not independent chain-finality evidence.

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

Change a signed envelope value and the signature fails. Reformatting whitespace or
changing unsigned wrapper metadata preserves the signature:

```sh
sed 's/"charged": "no"/"charged": "yes"/' testdata/refused-base-mainnet-2026-10-06.json > /tmp/tampered.json
go run ./cmd/seconded-verify /tmp/tampered.json
```

```text
FAIL  /tmp/tampered.json
      signature does not verify
```

The exit status is 1 whenever any receipt on the command line fails.

## Public paid and disagreement examples

Both were already published in the [official documentation](https://secondedoracle.xyz/docs#example-receipts):

```sh
cd verifier
go run ./cmd/seconded-verify testdata/trade-paid-base-mainnet-2026-09-29.json
go run ./cmd/seconded-verify -json testdata/token-not-verified-base-sepolia-2026-09-29.json
```

The Trade receipt signs `charged=yes`, amount `250000` atomic units ($0.25),
`settlement=included` and a transaction hash. The disagreement signs `charged=pending`
and no transaction. The verifier authenticates these issuer statements; it does not
query a chain to certify payment or nonpayment.

The parser rejects trailing JSON and duplicate keys, including in unsigned wrapper
metadata. It accepts one direct receipt or one check-reply wrapper containing a direct
receipt, and never authenticates the wrapper's other fields.
