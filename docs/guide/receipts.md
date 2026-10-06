# Receipts and verification

Every response from the service, including refusals and failures, is a signed receipt:

```json
{
  "envelope": { "v": 1, "kind": "seconded-receipt", "key_id": "rk-2026-09-a", "...": "..." },
  "sig": "<Ed25519 signature, base64url without padding>",
  "key_id": "rk-2026-09-a"
}
```

## What the signature covers

The signed message is a version prefix followed by the envelope in RFC 8785 canonical
form (JSON Canonicalization Scheme: sorted keys, no whitespace, ES6 number formatting):

```text
"SECONDED-RECEIPT/v1\x00" + JCS(envelope)      receipt version 1
"SECONDED-RECEIPT/v2\x00" + JCS(envelope)      receipt version 2 (adds the released state)
"SECONDED-RECEIPT/v3\x00" + JCS(envelope)      receipt version 3 (adds the verification block)
```

Each version signs under its own prefix, so no envelope verifies as another version.
The client and the verifier use the same canonicalization library
(`github.com/cyberphone/json-canonicalization`), vendored in both.

## The key pin

Receipts are verified under an Ed25519 public key compiled into the client, not under a
key fetched from the network:

| Key id | Public key (hex) | Networks |
| --- | --- | --- |
| `rk-2026-09-a` | `438d9301c477c27fecc3f56da0a7d5a6b3894cef69815bae63b5349dc2cddf0b` | Base, Arc and Robinhood Chain mainnets and testnets |

The pin lives in `client/receipt.go` (`releaseReceiptKeys`, `mainnetReceiptKeys`) and in
`verifier/keys.go` (`ReleaseKeysHex`). `GET /v1/keys` on the API publishes the same key as
advisory metadata; the client checks that document against the pin and refuses to run if
they disagree, but the document can never add or replace a pin. A key rotation is a client
release that carries both the old and the new key.

## Fields worth knowing

| Field | Meaning |
| --- | --- |
| `state` | Where the check is. Terminal agreed states: `final`, `included`, `released`, `trial_delivered`. No charge: `no_agreement`, `content_refused`, `service_failed`, `closed_no_charge`, `frozen_unsettled`, `refused`. Refunds: `refund_owed`, `refunded`. In progress: `running`, `settling`, `delayed`, `unavailable`. |
| `outcome` | `agreed`, `no_agreement`, or the failure state repeated. |
| `answer.label_id`, `answer.option` | The agreed label and its one-based index in the product's label list. Map it through the catalog's `answer_to_action`. |
| `checked_by` | The labs whose models checked the evidence: `OpenAI` and `Anthropic`. |
| `model_config_fingerprint` | A 64-hex fingerprint of the model configuration used; two receipts with the same fingerprint were produced under the same configuration, without revealing it. |
| `billing` | `mode` (`paid` or `free-trial`), `charged` (`yes`, `no`, `pending`, `refund_owed`, `refunded`), `network`, `asset`, `amount_atomic`, `payer`, `pay_to`, `tx` (the settlement transaction when there is one), `settlement`, `refund`. |
| `request_commitment` | A 64-hex commitment to the exact input and a salt; the client recomputes it before paying and refuses a receipt that names a different one. |
| `issued_at`, `outcome_at` | When the receipt was signed and when the outcome was reached. The client tolerates at most 30 seconds of clock skew on `issued_at` and rejects an `outcome_at` later than `issued_at`. |
| `verification` (v3 only) | Schema `seconded-verification/v1`: the measured findings, per-fact coverage (`checked`, `partial`, `not_checked`, `unavailable` with a reason), the pinned block, and the SHA-256 of the canonical fact sheet the models saw. Findings are fixed once published; a later receipt for the same check may not drop or change them. |
| `data_as_of`, `data_freshness` | Inside the answer: dataset dates and whether they were current when the answer was produced. |

## Two verifiers, two jobs

**`verifier/`** is for anyone. It checks that the signature verifies under the pinned key
the receipt names, that the version prefix matches, and that the envelope is structurally
a receipt (known state, parseable timestamps, `checked_by` naming known labs, a
verification block exactly on v3). It needs no account and no network.

**The client** does everything the verifier does and then binds the receipt to its own
purchase: the check id and request commitment it computed before paying, the payer, the
amount, the network's asset and `payTo`, the signed purchase association used for
recovery, and the receipt sequence (a later receipt may not roll back an earlier outcome).
It also validates product-specific answers against the catalog: label tables, lending
arithmetic, cross-chain comparison bounds, stock parity facts. Those checks need the
private ledger of the wallet that paid, so a third party cannot run them, and the verifier
does not claim to.

## Verify one now

```sh
cd verifier
go run ./cmd/seconded-verify testdata/refused-base-mainnet-2026-10-06.json
```

The fixture is the reply the production service returned on 2026-10-06 when refusing a
free quote request; `verifier/testdata/README.md` explains why that is the one published.
Your own agreed receipts verify the same way. [`examples/verify-receipt`](../../examples/verify-receipt/)
shows the output, the tamper case, and how to cross-check the pin against `/v1/keys`.
