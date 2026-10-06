# Receipt fixtures

| File | What it is | Signed by |
| --- | --- | --- |
| `refused-base-mainnet-2026-10-06.json` | A receipt the production service at `https://api.secondedoracle.xyz` returned on 2026-10-06T03:57:35Z to a free price-quote request for `shielded_route_check` on Base mainnet. The service refused the request (`state: refused`, `error: invalid_input`, `reason: unknown_product`) and signed the refusal. The signer asserts nonpayment (`billing.charged: no`); wrapper diagnostics are unsigned. | The production receipt key `rk-2026-09-a` (the pin compiled into the client and into this verifier). |

The paid and disagreement fixtures below were extracted from the already-public
[official example receipts](https://secondedoracle.xyz/docs#example-receipts) on
2026-10-06. No customer directory, wallet, new purchase or private profile was used.
Only the receipt object was extracted; signatures still verify after formatting because
they cover the canonical envelope rather than wrapper bytes.

| File | What it signs | Boundary |
| --- | --- | --- |
| [trade-paid-base-mainnet-2026-09-29.json](trade-paid-base-mainnet-2026-09-29.json) | v3 Trade Check, `state=included`, `charged=yes`, amount 250000 atomic units ($0.25), `settlement=included`, both labs. | Transaction `0x9472572e3cfe100973002a46b1d943a97ca3839be0c834863654d4a4e4fa1b13` is a signed assertion; chain inclusion/finality was not independently queried. |
| [token-not-verified-base-sepolia-2026-09-29.json](token-not-verified-base-sepolia-2026-09-29.json) | v3 Token Check, `no_agreement`, no answer, `charged=pending`, `tx=null`, both labs. | The website describes a Base Sepolia subject; the signed billing network is Base mainnet. Pending billing does not certify nonpayment. |

All three fixtures verify under `rk-2026-09-a`. The refusal signs `state=refused` and
`charged=no`; its outer `error`, `reason` and `hints` are unsigned. It records one
historical request and cannot establish current product availability. The official
website's accompanying monetary narrative is not an independently verified chain fact.

Source-page response-body SHA-256 on 2026-10-06:
`93ae0e1b21709c9417b2319d100789d239a62026ea69ec6d003c875dce2518d5`.
The fixtures retain publicly disclosed payer/check identifiers as published; no new
purchaser data was obtained.

The tests in `../receipt_test.go` also sign synthetic receipts with throwaway keys
generated at test time. Those never leave the test process and prove nothing about the
service; they exercise the verifier's negative paths (tampering, wrong key, wrong
version prefix).
