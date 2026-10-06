# How a check works

SECONDED answers one question for an AI agent: *may I act on this?* The agent sends a
bounded, typed input (an unsigned transaction, a token address, a message, an x402 offer),
pays a small fixed price over x402, and gets back a signed receipt. The answer is accepted
only if two models from rival labs agree on it; otherwise the receipt says NOT VERIFIED.
Settlement is the service policy described below; pending billing is not certified nonpayment.

```mermaid
sequenceDiagram
    participant A as Agent (MCP host)
    participant C as seconded-mcp (local)
    participant S as api.secondedoracle.xyz
    participant M as Two models (OpenAI, Anthropic)
    participant L as Chain (Base, Arc or Robinhood)
    A->>C: tools/call seconded_trade_check {input}
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
    else no usable agreement
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
5. **Read agreement and billing separately.** The service's policy is to settle only after usable agreement is saved. An agreed
   answer can be released while settlement remains pending; included and final receipts
   carry the corresponding transaction state. Without usable agreement, the agent
   receives NOT VERIFIED guidance and must pause. Read the signed billing fields:
   `pending` is not a certificate of nonpayment. The paying client uses independent
   chain evidence to resolve payment status.
6. **The client verifies the receipt.** Every receipt is Ed25519-signed under a key the
   client compiles in; see [Receipts and verification](receipts.md). The client also binds
   the receipt to the purchase it made (check id, request commitment, payer, amount) and
   maps the answer label to an action through the catalog's `answer_to_action`.

## What the agent is told to do

The catalog, not the model, decides what each label means. For Trade Check:

| Label | Action |
| --- | --- |
| `proceed` | Sign exactly the input that was checked. |
| `do_not_proceed` | STOP and show the owner. |
| `cannot_verify` | Pause or ask a human. |
| NOT VERIFIED | Pause or ask a human. |

Only an agreed signed receipt authorizes the listed next step, and only for the exact
input supplied. Pending, failed or NOT VERIFIED results mean pause
(`client/products.json`, `usage`).

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
