# How a check works

SECONDED answers one question for an AI agent: *may I act on this?* The agent sends a
bounded, typed input (an unsigned transaction, a token address, a message, an x402 offer),
pays a small fixed price over x402, and gets back a signed receipt. Standard checks require
agreement between models from rival labs; otherwise they return NOT VERIFIED. Degen Check
also delivers signed disagreements, which can be charged and do not authorize an action.
The [billing policy](#billing-policy) distinguishes these outcomes from payment finality.

```mermaid
sequenceDiagram
    participant A as Agent (MCP host)
    participant C as seconded-mcp (local)
    participant S as api.secondedoracle.xyz
    participant M as Two models (OpenAI, Anthropic)
    participant L as Chain (Base, Arc or Robinhood)
    A->>C: tools/call a check {input}
    C->>C: canonicalize input, check limits, choose payment network
    C->>S: POST /v1/x402/checks (no payment yet)
    S-->>C: 402 challenge: price, payTo, nonce, validity
    C->>C: compare payTo with the catalog pin, sign EIP-3009 authorization (local key)
    C->>S: POST /v1/x402/checks + payment authorization
    S->>M: the same evidence to both models, independently
    M-->>S: two answers
    alt both agree
        S->>L: settle the authorization
        S-->>C: signed agreed receipt, answer label; settlement may be pending
    else Degen returns a usable signed disagreement
        S->>L: settle the authorization
        S-->>C: signed models_disagree answer; settlement may be pending
    else standard disagreement or no usable answer
        S-->>C: signed receipt: no_agreement, NOT VERIFIED, billing pending
    end
    C->>C: verify signature under the compiled key pin, bind to the purchase
    C-->>A: answer label and the action it maps to, plus the receipt
```

## Step by step

1. **The agent calls a tool.** Each paid check is one MCP tool, for example
   `seconded_trade_check`. The input is a typed JSON object; message checks include prose inside its text field:
   the exact unsigned
   transaction, the token address and network, the message text. Inputs are bounded and
   priced by size tier (small up to 8,192 bytes, medium up to 65,536, large up to 131,072;
   `client/products.json`, `tiers`).
2. **The client asks for a price.** It posts the input to the standard x402 door,
   `POST /v1/x402/checks`, and receives an HTTP 402 challenge naming the price, the asset,
   the `payTo` address and a validity window. Before signing anything it compares `payTo`
   with the address pinned in the shipped catalog for that network (`client/products.json`,
   `networks[].pay_to`).
3. **The client signs a payment authorization.** Payments use x402 v2 with the `exact`
   scheme: an EIP-3009 `transferWithAuthorization` signed by the dedicated check wallet.
   The private key never leaves the client. Spending limits (per check, per hour, per day,
   outstanding) are enforced before signing. Base mainnet in USDC is the default; Arc (USDC)
   and Robinhood Chain (USDG) are used only when the check names them.
4. **Two models check the same evidence.** The service gathers evidence (for on-chain
   facts, two independent readers must agree at one pinned block) and gives it to two
   models from different labs. Receipts record which labs under `checked_by`; the values the
   client accepts are `OpenAI` and `Anthropic` (`client/receipt.go`).
5. **Read outcome and billing separately.** Standard checks settle after an agreed answer
   is saved; Degen can also settle after a signed disagreement is saved. Either answer
   can be released while settlement remains pending. A standard disagreement returns
   NOT VERIFIED with no answer. Degen disagreement carries facts and both opinions but
   still tells the agent to pause. Read the signed billing fields and the policy below:
   `pending` is not a certificate of nonpayment.
6. **The client verifies the receipt.** Every receipt is Ed25519-signed under a key the
   client compiles in; see [Receipts and verification](receipts.md). The client also binds
   the receipt to the purchase it made (check id, request commitment, payer, amount) and
   maps the answer label to an action through the catalog's `answer_to_action`.

## Billing policy

| Result | Service settlement policy | What the agent should do |
| --- | --- | --- |
| Standard check, usable agreed answer saved | Charge the quoted price. | Follow that label's action for the exact checked input. |
| Standard disagreement (`no_agreement`) or no usable answer | No charge under service policy; unresolved signed billing may remain `pending`. | Pause or ask a human. |
| Degen signed `models_disagree` answer saved | Costs $0.10 for the saved signed facts and both opinions. | Pause or ask a human; disagreement does not authorize an action. |

`released`, `included` and `final` describe delivery and reported settlement, not
agreement by themselves. Read `outcome`, the answer and signed `billing` together.
The paying client resolves payment status using independent chain evidence. A receipt
signature authenticates the service's assertion; it does not independently prove settlement.

The 0.4.4 initialization instructions and bundled catalog share the same billing text.
Standard checks charge only after agreement and a saved answer; standard disagreement
is signed and free. Degen costs $0.10 for a saved signed answer, including disagreement.

The live catalog's generic usage text also mentions `POST /v1/checks`. The released
client's payment flow uses `POST /v1/x402/checks`, as shown above (`client/standard.go`).
The alternate endpoint has not been exercised in this documentation check; catalog
prose alone does not establish its current purchase behavior.

## What the agent is told to do

The catalog, not the model, decides what each label means. For Trade Check:

| Label | Action |
| --- | --- |
| `proceed` | Sign exactly the input that was checked. |
| `do_not_proceed` | STOP and show the owner. |
| `cannot_verify` | Pause or ask a human. |
| NOT VERIFIED | Pause or ask a human. |

For Trade Check, only an agreed signed receipt authorizes the listed next step, and only for the exact
input supplied. Pending, failed or NOT VERIFIED results mean pause
(`client/products.json`, `usage`).
Degen disagreement and stale historical results also mean pause, even when charged.

## Timeouts and recovery

`wait_seconds` accepts 0 through 25, but the MCP client clamps its requested wait to
eight seconds. The tool has a separate 45-second default soft deadline; an admitted
check may continue after either wait ends. If an answer is not ready, the result
carries a `check_id` for `seconded_receipt`. The CLI equivalent is
`seconded-mcp recover --check-id ID`.

Retained standard-door recovery replays the same stored request and payment
authorization without loading a signing key or buying a new check. Original-door or
archived ownership recovery may require a wallet signing key; the keyless CLI can
report `wallet_recovery_required` for those cases. Recovery can contact the API and
chain readers. Fetching remains subject to service availability and retention; the
[published policy](https://secondedoracle.xyz/privacy) removes finished-check records
after 90 days from last activity. Keep receipts you need to verify later.

## Freshness

Newer answers can include signed dataset dates and freshness status; historical
receipts may omit them. Fetching an old answer does not add new evidence. The client
preserves its historical dates and may mark formerly current data `not_verified`
when it expires.

## What this is not

- Not financial advice, not an endorsement, not a guarantee. The catalog says so in the
  `disclaimer` the receipt carries for financial products.
- Not a trading, bridging or signing service. No check executes anything.
- Not a judgement about the owner's own rules or words; no launch check covers them.
