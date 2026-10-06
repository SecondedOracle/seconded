# One tool call, start to finish

[`trade-check-request.json`](trade-check-request.json) is the MCP `tools/call` request an
agent sends before signing a USDC transfer on Base. The `input` is the catalog's own
`example_input` for Trade Check: a 5 USDC `transfer` to another address, sent as the
exact unsigned transaction, not as prose. The optional fields are:

| Field | Meaning |
| --- | --- |
| `max_price_usd` | Refuse the check if it would cost more. Optional unless the value gate is switched on. |
| `network` | The chain you **pay** on: `base` (default), `arc`, `robinhood`, or a testnet name. Not the chain the transaction is on; that comes from `chainId` inside the input. |
| `wait_seconds` | Schema range 0 to 25; the MCP client clamps requested waiting to eight seconds and has a separate 45-second default soft deadline. If the check is still running when the host deadline passes, the result carries a `check_id` and the agent calls `seconded_receipt` later. |

[`trade-check-catalog-entry.json`](trade-check-catalog-entry.json) is the matching
catalog entry. The `answer_to_action` map is what the agent is told to do with each
answer label:

| Label the receipt carries | Action |
| --- | --- |
| `proceed` | Sign exactly the input that was checked. Any change needs a new check. |
| `do_not_proceed` | STOP and show the owner. |
| `cannot_verify` | Pause or ask a human. |
| `NOT VERIFIED` (a `no_agreement` receipt, no label) | Pause or ask a human. Read signed billing; pending does not certify nonpayment. |

The service policy settles after both models agree and the answer is saved; a pending,
failed or NOT VERIFIED result means pause. The receipt in the tool result is the proof,
and [`../verify-receipt`](../verify-receipt/) shows how anyone checks it later.
