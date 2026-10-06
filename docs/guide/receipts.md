# Receipts and verification

Check receipts, including signed refusal receipts, authenticate their canonical
envelope. Public metadata, payment challenges, and some transport or validation
errors are not signed receipts. The offline verifier checks the receipt signature;
it does not establish the truth of the models' work or unsigned wrapper fields.

A receipt has this form:

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
| `state` | Answer-bearing states include `released`, `included`, `final` and free-trial `trial_delivered`. Included payments and released answers with unknown settlement still require collection. `no_agreement` withholds an answer and can retain `charged: pending`. Refusal, failure, certified-nonpayment and refund states have distinct billing fields; do not infer final payment status from the state name alone. |
| `outcome` | Records agreement, disagreement or the applicable failure outcome; nonterminal or other states can carry null or a pending/none value as defined by the client contract. |
| `answer.label_id`, `answer.option` | The agreed label and its one-based index in the product's label list. Map it through the catalog's `answer_to_action`. |
| `checked_by` | The service's signed attribution to known labs, OpenAI and Anthropic; it does not independently prove which remote model ran. |
| `model_config_fingerprint` | The service's signed digest of the configuration it reports using. Equal digests identify the same reported configuration under the hash assumption; they do not independently prove which remote model ran. |
| `billing` | `mode` (`paid` or `free-trial`), `charged` (`yes`, `no`, `pending`, `refund_owed`, `refunded`), `network`, `asset`, `amount_atomic`, `payer`, `pay_to`, `tx` (the settlement transaction when there is one), `settlement`, `refund`. |
| `request_commitment` | The signed commitment to input and terms. The original payment door recomputes it from a challenge salt. The standard x402 door instead validates a locally computed purchase association, then adopts the server check ID and commitment from the first matching signed receipt; its public challenge carries no such salt or server ID. |
| `issued_at`, `outcome_at` | When the receipt was signed and the outcome reached. The client rejects issuance more than 30 seconds in the future and outcomes later than issuance, with additional purchase-time checks. Observation and expiry bounds depend on the product. |
| `verification` (v3 only) | Schema `seconded-verification/v1`: the measured findings, per-fact coverage (`checked`, `partial`, `not_checked`, `unavailable` with a reason), the pinned block, and the SHA-256 of the canonical fact sheet the models saw. Findings are fixed once published; a later receipt for the same check may not drop or change them. |
| `data_as_of`, `data_freshness` | Inside the answer: dataset dates and whether they were current when the answer was produced. |

## Two verifiers, two jobs

**`verifier/`** is for anyone. It checks that the signature verifies under the pinned key
the receipt names, that the version prefix matches, and that the envelope is structurally
a receipt (known state, parseable timestamps, `checked_by` naming known labs, a
verification block exactly on v3). It needs no account and no network.

By default the verifier uses its compiled receipt key. `-keys` explicitly replaces
those defaults for that run. It reports the signed envelope, not the surrounding HTTP
reply. The parser requires one JSON object, rejects duplicate keys at every depth and
trailing data, and accepts either an exact direct receipt or one check-reply wrapper
containing it. Wrapper metadata remains unsigned; mixed direct/wrapper and nested
wrapper shapes are rejected.

A successful result authenticates the canonical signed envelope and the listed
structural checks. It does not certify payment finality, nonpayment, model independence,
purchase ownership, complete product semantics, or truth of the reported findings.

**The client** binds a receipt to its local purchase using the applicable
request/commitment or standard-door purchase association, payer, amount, network asset,
payee and receipt history. It also checks product-specific labels, arithmetic and
answer structure. Purchase-history binding needs local records; many schema and
arithmetic checks can be implemented independently by a third party. The standalone
verifier intentionally omits these semantic checks.

## Verify one now

```sh
cd verifier
go run ./cmd/seconded-verify testdata/refused-base-mainnet-2026-10-06.json
```

The fixture is the reply the production service returned on 2026-10-06 when refusing a
free quote request. Two already-public paid/disagreement examples from the
[official documentation](https://secondedoracle.xyz/docs#example-receipts) are also
committed under `verifier/testdata/`; its [README](../../verifier/testdata/README.md)
records provenance and billing boundaries.
Your own agreed receipts verify the same way. [`examples/verify-receipt`](../../examples/verify-receipt/)
shows the output, the tamper case, and how to cross-check the pin against `/v1/keys`.
