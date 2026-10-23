package client

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"math/big"
	"strings"
	"time"
)

type BurnQuote struct {
	QuoteID     string `json:"quote_id"`
	ChainID     int64  `json:"chain_id"`
	Payer       string `json:"payer"`
	AgentID     string `json:"agent_id"`
	FaceMicros  int64  `json:"face_usd_micros"`
	DiscountBPS int64  `json:"discount_bps"`
	MinimumBurn string `json:"minimum_burn_atomic"`
	IssuedAt    int64  `json:"issued_at"`
	ExpiresAt   int64  `json:"expires_at"`
	Signature   string `json:"signature"`
	KeyID       string `json:"key_id"`
}
type BurnQuoteResponse struct {
	Quote   BurnQuote       `json:"quote"`
	Pricing json.RawMessage `json:"pricing"`
}
type BurnGrant struct {
	CreditID        string `json:"credit_id"`
	QuoteID         string `json:"quote_id"`
	ChainID         int64  `json:"chain_id"`
	TxHash          string `json:"burn_tx_hash"`
	LogIndex        int64  `json:"burn_log_index"`
	Payer           string `json:"payer"`
	AgentID         string `json:"agent_id"`
	BurnedAtomic    string `json:"burned_atomic"`
	FaceMicros      int64  `json:"face_usd_micros"`
	RemainingMicros int64  `json:"remaining_usd_micros"`
	DiscountBPS     int64  `json:"discount_bps"`
	IssuedAt        string `json:"issued_at"`
	ExpiresAt       string `json:"expires_at"`
}
type BurnRedemption struct {
	Credit BurnGrant `json:"credit"`
}

func burnInteger(s string) (*big.Int, bool) {
	if len(s) == 0 || len(s) > 78 || s[0] == '0' {
		return nil, false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return nil, false
		}
	}
	n, ok := new(big.Int).SetString(s, 10)
	return n, ok && n.Sign() > 0 && n.BitLen() <= 256
}
func (q BurnQuote) validate(payer string, now int64) error {
	return q.validateMinimum(payer, now, MinBurnPackFace)
}

func (q BurnQuote) validateMinimum(payer string, now, minimum int64) error {
	if q.ChainID != 4663 || q.Payer != payer || !addressPattern.MatchString(payer) || !validCreditAgent(q.AgentID) || q.FaceMicros < minimum || q.FaceMicros > MaxBurnPackFace || q.DiscountBPS != 2000 || q.QuoteID == "" || len(q.QuoteID) > 256 || q.KeyID == "" || len(q.KeyID) > 128 || !hexWord.MatchString(q.Signature) || q.IssuedAt <= 0 || q.IssuedAt > now+30 || q.ExpiresAt <= now || q.ExpiresAt-q.IssuedAt != 300 {
		return ErrInvalid
	}
	if _, ok := burnInteger(q.MinimumBurn); !ok {
		return ErrInvalid
	}
	return nil
}
func burnRedeemDigest(q BurnQuote, txHash, origin string) ([]byte, error) {
	if !hexWord.MatchString(txHash) || origin == "" {
		return nil, ErrInvalid
	}
	raw, err := canonicalValue(q)
	if err != nil {
		return nil, err
	}
	tx, _ := hex.DecodeString(txHash[2:])
	message := keccak(keccak([]byte("BurnRedeem(string origin,bytes32 quoteHash,bytes32 transactionHash)")), keccak([]byte(origin)), keccak(raw), tx)
	return keccak([]byte{0x19, 0x01}, creditDomain(), message), nil
}
func (a *API) BurnQuote(ctx context.Context, payer, agent string, face int64) (BurnQuoteResponse, error) {
	var result BurnQuoteResponse
	if !addressPattern.MatchString(payer) || !validCreditAgent(agent) || face < MinBurnPackFace || face > MaxBurnPackFace {
		return result, ErrInvalid
	}
	status, raw, _, err := a.request(ctx, "POST", "/v1/credits/quote", map[string]any{"payer": payer, "agent_id": agent, "face_usd_micros": face}, nil)
	if err != nil {
		return result, err
	}
	if status != 200 {
		return result, publicAPIError(status, raw)
	}
	if DecodeStrict(raw, &result, ResponseLimit) != nil || result.Quote.validate(payer, time.Now().Unix()) != nil || result.Quote.AgentID != agent || result.Quote.FaceMicros != face {
		return result, ErrInvalid
	}
	return result, nil
}
func (a *API) RedeemBurn(ctx context.Context, q BurnQuote, txHash string, signer Signer) (BurnRedemption, error) {
	return a.redeemBurn(ctx, q, txHash, signer, false)
}
func (a *API) redeemBurn(ctx context.Context, q BurnQuote, txHash string, signer Signer, replay bool) (BurnRedemption, error) {
	var result BurnRedemption
	now := time.Now().Unix()
	if replay {
		now = q.IssuedAt
	}
	txHash = strings.ToLower(txHash)
	if signer == nil || q.validate(signer.Address(), now) != nil {
		return result, ErrInvalid
	}
	digest, err := burnRedeemDigest(q, txHash, a.url)
	if err != nil {
		return result, err
	}
	sig, err := signChecked(signer, digest)
	if err != nil {
		return result, err
	}
	status, raw, _, err := a.request(ctx, "POST", "/v1/credits/redeem", map[string]any{"tx_hash": txHash, "quote": q, "signature": sig}, nil)
	if err != nil {
		return result, err
	}
	if status != 200 {
		return result, publicAPIError(status, raw)
	}
	if DecodeStrict(raw, &result, ResponseLimit) != nil {
		return result, ErrInvalid
	}
	c := result.Credit
	burned, ok := burnInteger(c.BurnedAtomic)
	minimum, _ := burnInteger(q.MinimumBurn)
	issued, e1 := time.Parse(time.RFC3339Nano, c.IssuedAt)
	expires, e2 := time.Parse(time.RFC3339Nano, c.ExpiresAt)
	if !ok || burned.Cmp(minimum) < 0 || c.CreditID == "" || c.QuoteID != q.QuoteID || c.ChainID != 4663 || c.TxHash != txHash || c.LogIndex < 0 || c.Payer != q.Payer || c.AgentID != q.AgentID || (c.FaceMicros <= 0 || c.FaceMicros > q.FaceMicros) || c.RemainingMicros < 0 || c.RemainingMicros > c.FaceMicros || c.DiscountBPS != q.DiscountBPS || e1 != nil || e2 != nil || expires.Sub(issued) != 30*24*time.Hour {
		return result, ErrInvalid
	}
	return result, nil
}
func (g *Engine) creditTool(ctx context.Context, name string, args json.RawMessage) (any, error) {
	if g.API == nil {
		return nil, ErrInvalid
	}
	var input struct {
		AgentID    string     `json:"agent_id,omitempty"`
		FaceMicros int64      `json:"face_usd_micros,omitempty"`
		Quote      *BurnQuote `json:"quote,omitempty"`
		TxHash     string     `json:"tx_hash,omitempty"`
	}
	if DecodeStrict(args, &input, MessageLimit) != nil {
		return nil, ErrInvalid
	}
	unlock, err := g.Files.Lock()
	if err != nil {
		return nil, err
	}
	v, err := LoadVault(g.Store)
	if err != nil {
		unlock()
		return nil, err
	}
	if name == "seconded_credits_quote" {
		address := v.Address
		unlock()
		if input.Quote != nil || input.TxHash != "" {
			return nil, ErrInvalid
		}
		return g.API.BurnQuote(ctx, address, input.AgentID, input.FaceMicros)
	}
	defer unlock()
	if input.Quote == nil || input.AgentID != "" || input.FaceMicros != 0 {
		return nil, ErrInvalid
	}
	signer, err := signerFrom(v)
	if err != nil {
		return nil, err
	}
	defer signer.Close()
	result, err := g.API.RedeemBurn(ctx, *input.Quote, input.TxHash, signer)
	if err != nil {
		return result, err
	}
	if err = recordPackRedemption(g.Files, result); err != nil {
		return result, err
	}
	return result, nil
}
func creditTools() []Tool {
	return []Tool{
		{"seconded_credits_buy", "Buy a burn-credit pack through the pinned Robinhood USDG to native ETH to SECONDED route, burn only its swap output, then redeem. Sends up to four initial chain transactions; safe requotes may renew approval within the original total gas cap. Face value $5–$10; USDG input capped at $10 per pack. Supply explicit USDG, slippage and gas caps. Repeat identical arguments and pack_id while pending; safe unbroadcast stages requote after expiry. Optional recovery_action=requote refreshes a safe stage; recovery_action=abandon closes it and retains any swapped tokens. Uncertain transactions cannot be abandoned; never choose a new ID to retry.", schema(map[string]any{"recovery_action": map[string]any{"type": "string", "enum": []string{"requote", "abandon"}}, "pack_id": map[string]any{"type": "string", "pattern": "^[0-9a-f]{32}$"}, "agent_id": map[string]any{"type": "string"}, "face_usd_micros": map[string]any{"type": "integer", "minimum": MinBurnPackFace, "maximum": MaxBurnPackFace}, "input_usdg_micros": map[string]any{"type": "integer", "minimum": 4000000, "maximum": MaxBurnPackInput}, "slippage_bps": map[string]any{"type": "integer", "minimum": 0, "maximum": 1000}, "gas_limit": map[string]any{"type": "integer", "minimum": 21000, "maximum": 2000000}, "max_fee_per_gas_wei": map[string]any{"type": "string"}, "max_priority_fee_per_gas_wei": map[string]any{"type": "string"}, "max_total_gas_wei": map[string]any{"type": "string"}}, "pack_id", "agent_id", "face_usd_micros", "input_usdg_micros", "slippage_bps", "gas_limit", "max_fee_per_gas_wei", "max_priority_fee_per_gas_wei", "max_total_gas_wei"), map[string]bool{"readOnlyHint": false, "destructiveHint": true, "idempotentHint": true, "openWorldHint": true}},
		{"seconded_credits_quote", "Get a fresh quote for a SECONDED burn-credit pack on Robinhood. Minimum face value $5; quotes expire in 300 seconds. Does not buy or burn tokens.", schema(map[string]any{"agent_id": map[string]any{"type": "string"}, "face_usd_micros": map[string]any{"type": "integer", "minimum": MinBurnPackFace, "maximum": MaxBurnPackFace}}, "agent_id", "face_usd_micros"), map[string]bool{"readOnlyHint": true, "openWorldHint": true}},
		{"seconded_credits_redeem", "Redeem an existing finalized burn transaction using its fresh quote. Signs a BurnRedeem proof with the check wallet; sends no chain transaction. Returns the credit lot and remaining face value.", schema(map[string]any{"tx_hash": map[string]any{"type": "string", "pattern": "^0x[0-9a-fA-F]{64}$"}, "quote": map[string]any{"type": "object", "description": "The entire unchanged quote object returned by seconded_credits_quote."}}, "tx_hash", "quote"), map[string]bool{"readOnlyHint": false, "idempotentHint": true, "openWorldHint": true}},
	}
}
