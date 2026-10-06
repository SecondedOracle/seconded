# Receipt fixtures

| File | What it is | Signed by |
| --- | --- | --- |
| `refused-base-mainnet-2026-10-06.json` | A receipt the production service at `https://api.secondedoracle.xyz` returned on 2026-10-06T03:57:35Z to a free price-quote request for `shielded_route_check` on Base mainnet. The service refused the request (`state: refused`, `error: invalid_input`, `reason: unknown_product`) and signed the refusal. Nothing was charged (`billing.charged: no`). | The production receipt key `rk-2026-09-a` (the pin compiled into the client and into this verifier). |

Why a refusal and not an agreed answer? Agreed receipts are returned to the wallet that
paid for the check and contain the purchaser's address and the answer bought. The one
above is the only production-signed receipt the maintainers were willing to publish as
of 2026-10-06: it proves the signing path and the key pin against the live service
without publishing anyone's paid check. If you run a check yourself, your own agreed
receipt verifies the same way.

The tests in `../receipt_test.go` also sign synthetic receipts with throwaway keys
generated at test time. Those never leave the test process and prove nothing about the
service; they exercise the verifier's negative paths (tampering, wrong key, wrong
version prefix).
