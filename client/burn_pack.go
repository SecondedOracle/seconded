package client

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math/big"
	"time"
)

type BurnPackSpec struct {
	RecoveryAction string `json:"recovery_action,omitempty"`
	PackID         string `json:"pack_id"`
	AgentID        string `json:"agent_id"`
	FaceMicros     int64  `json:"face_usd_micros"`
	InputMicros    int64  `json:"input_usdg_micros"`
	SlippageBPS    int64  `json:"slippage_bps"`
	GasLimit       uint64 `json:"gas_limit"`
	MaxFeeWei      string `json:"max_fee_per_gas_wei"`
	MaxPriorityWei string `json:"max_priority_fee_per_gas_wei"`
	MaxGasWei      string `json:"max_total_gas_wei"`
}

func (s BurnPackSpec) validate() error {
	return s.validateMinimum(MinBurnPackFace)
}

func (s BurnPackSpec) validateMinimum(minimum int64) error {
	fee, f := burnInteger(s.MaxFeeWei)
	tip, t := burnInteger(s.MaxPriorityWei)
	total, g := burnInteger(s.MaxGasWei)
	if !hexID.MatchString(s.PackID) || !validCreditAgent(s.AgentID) || s.FaceMicros < minimum || s.FaceMicros > MaxBurnPackFace || s.InputMicros < s.FaceMicros*8/10 || s.InputMicros > MaxBurnPackInput || s.SlippageBPS < 0 || s.SlippageBPS > 1000 || s.GasLimit < 21000 || s.GasLimit > 2000000 || !f || !t || !g || tip.Cmp(fee) > 0 {
		return ErrInvalid
	}
	worst := new(big.Int).Mul(new(big.Int).SetUint64(s.GasLimit*4), fee)
	if worst.Cmp(total) > 0 {
		return errors.New("burn_gas_cap_exceeded")
	}
	return nil
}

type burnPackRecord struct {
	TransactionCount int             `json:"transaction_count,omitempty"`
	NextNonce        *uint64         `json:"next_nonce,omitempty"`
	Abandoned        bool            `json:"abandoned,omitempty"`
	PreviousQuotes   []BurnQuote     `json:"previous_quotes,omitempty"`
	RedeemAttempted  bool            `json:"redeem_attempted"`
	Spec             BurnPackSpec    `json:"spec"`
	Payer            string          `json:"payer"`
	Origin           string          `json:"origin"`
	Quote            BurnQuote       `json:"quote"`
	MinimumOut       string          `json:"minimum_out"`
	Swapped          string          `json:"swapped,omitempty"`
	Stage            int             `json:"stage"`
	Nonce            uint64          `json:"nonce"`
	Raw              string          `json:"raw,omitempty"`
	Hash             string          `json:"hash,omitempty"`
	BurnHash         string          `json:"burn_hash,omitempty"`
	Started          int64           `json:"started"`
	Failed           bool            `json:"failed"`
	Grant            *BurnRedemption `json:"grant,omitempty"`
}
type BurnPackResult struct {
	Charged  string          `json:"charged"`
	PackID   string          `json:"pack_id"`
	Status   string          `json:"status"`
	Stage    int             `json:"stage"`
	TxHash   string          `json:"tx_hash,omitempty"`
	BurnHash string          `json:"burn_tx_hash,omitempty"`
	Quote    BurnQuote       `json:"quote"`
	Credit   *BurnRedemption `json:"redemption,omitempty"`
	Next     string          `json:"next,omitempty"`
}

func (p burnPackRecord) result() BurnPackResult {
	r := BurnPackResult{Charged: "not_yet_known", PackID: p.Spec.PackID, Status: "pending", Stage: p.Stage, TxHash: p.Hash, BurnHash: p.BurnHash, Quote: p.Quote, Credit: p.Grant, Next: "Repeat seconded_credits_buy with the same pack_id and arguments to continue this pack."}
	if p.Stage >= 3 {
		r.Charged = "yes_final"
	}
	if p.Grant != nil {
		r.Status = "redeemed"
		r.Next = ""
	} else if p.Abandoned {
		r.Status = "abandoned"
		r.Next = "This pack is closed. Any completed swap remains in the wallet and counts toward spending limits."
		if p.Stage < 3 {
			r.Charged = "no"
		}
	} else if p.Failed {
		r.Status = "failed"
		r.Next = "Inspect the transaction and retained quote before starting another pack."
	}
	return r
}
func readBurnPacks(files *Files) ([]burnPackRecord, error) {
	raw, err := files.Read("burn-packs.json")
	if errors.Is(err, ErrNotFound) {
		return []burnPackRecord{}, nil
	}
	if err != nil {
		return nil, err
	}
	var packs []burnPackRecord
	if DecodeStrict(raw, &packs, 32<<20) != nil || len(packs) > 10000 {
		return nil, ErrStorage
	}
	seen := map[string]bool{}
	for i := range packs {
		p := &packs[i]
		// Historical $1 packs must remain readable for budget accounting.
		if p.Spec.validateMinimum(1000000) != nil || !addressPattern.MatchString(p.Payer) || p.Origin == "" || p.Stage < 0 || p.Stage > 4 || p.Started <= 0 || seen[p.Spec.PackID] || p.Quote.Payer != p.Payer || p.Quote.AgentID != p.Spec.AgentID || p.Quote.FaceMicros != p.Spec.FaceMicros {
			return nil, ErrStorage
		}
		if p.Spec.RecoveryAction != "" || (p.Abandoned && (p.Raw != "" || p.Hash != "" || p.BurnHash != "" || p.Stage >= 4 || p.Grant != nil)) {
			return nil, ErrStorage
		}
		if p.TransactionCount == 0 {
			p.TransactionCount = p.Stage
			if p.Raw != "" {
				p.TransactionCount++
			}
		}
		if p.TransactionCount < p.Stage || p.TransactionCount > 10000 || (p.Raw != "" && p.TransactionCount <= p.Stage) {
			return nil, ErrStorage
		}
		seen[p.Spec.PackID] = true
		min, ok := burnInteger(p.MinimumOut)
		floor, valid := burnInteger(p.Quote.MinimumBurn)
		if !ok || !valid || min.BitLen() > 128 || min.Cmp(floor) < 0 || p.Quote.validateMinimum(p.Payer, p.Quote.IssuedAt, 1000000) != nil || (p.Raw == "") != (p.Hash == "") {
			return nil, ErrStorage
		}
		if p.Stage >= 3 {
			amount, ok := burnInteger(p.Swapped)
			if !ok || amount.Cmp(min) < 0 {
				return nil, ErrStorage
			}
		}
		if p.Raw != "" {
			raw, err := hex.DecodeString(trimHex(p.Raw))
			if err != nil || len(raw) == 0 || "0x"+hex.EncodeToString(keccak(raw)) != p.Hash {
				return nil, ErrStorage
			}
		}
	}
	return packs, nil
}
func saveBurnPacks(files *Files, packs []burnPackRecord) error {
	raw, err := json.Marshal(packs)
	if err != nil {
		return err
	}
	return files.Write("burn-packs.json", raw)
}
func addBurnPackBudgets(files *Files, ledger *Ledger) error {
	packs, err := readBurnPacks(files)
	if err != nil {
		return err
	}
	for _, p := range packs {
		state := "OUTSTANDING"
		if p.Stage >= 3 {
			state = "SPENT"
		} else if p.Failed || p.Abandoned {
			continue
		}
		ledger.Entries = append(ledger.Entries, Entry{InputDigest: "burn-pack:" + p.Spec.PackID, Network: "eip155:4663", Amount: p.Spec.InputMicros, State: state, SignedAt: p.Started, SpentAt: p.Started})
	}
	return nil
}

// BuyCreditPack advances only this durable pack. Signed raw transactions are
// checkpointed before broadcast; retries use the identical nonce and bytes.
func (g *Engine) BuyCreditPack(ctx context.Context, spec BurnPackSpec, chain BurnPackChain) (BurnPackResult, error) {
	action := spec.RecoveryAction
	spec.RecoveryAction = ""
	if !oneOf(action, "", "requote", "abandon") || spec.validate() != nil || chain == nil || g.API == nil {
		return BurnPackResult{}, ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	unlock, err := g.Files.Lock()
	if err != nil {
		return BurnPackResult{}, err
	}
	defer unlock()
	v, err := LoadVault(g.Store)
	if err != nil {
		return BurnPackResult{}, err
	}
	packs, err := readBurnPacks(g.Files)
	if err != nil {
		return BurnPackResult{}, err
	}
	index := -1
	for i, p := range packs {
		if p.Spec.PackID == spec.PackID {
			index = i
			break
		}
	}
	if index >= 0 && (packs[index].Spec != spec || packs[index].Payer != v.Address || packs[index].Origin != g.API.url) {
		return BurnPackResult{}, ErrInvalid
	}
	if index >= 0 && (packs[index].Grant != nil || packs[index].Failed || packs[index].Abandoned) {
		return packs[index].result(), nil
	}
	if v.Settings.Frozen {
		return BurnPackResult{}, errors.New("frozen_local")
	}
	if err = chain.Validate(ctx, v.Address); err != nil {
		return BurnPackResult{}, err
	}
	if index < 0 {
		if action != "" {
			return BurnPackResult{}, ErrInvalid
		}
		for _, p := range packs {
			if p.Grant == nil && !p.Failed && !p.Abandoned {
				return p.result(), errors.New("burn_pack_pending")
			}
		}
		ledger, err := ReadLedger(g.Files)
		if err != nil {
			return BurnPackResult{}, err
		}
		policy, err := g.policyLedger(ledger)
		if err != nil {
			return BurnPackResult{}, err
		}
		if err = policy.requireRecovery(); err != nil {
			return BurnPackResult{}, err
		}
		if err = policy.burnPackBudget(v.Settings, spec.InputMicros, time.Now().Unix()); err != nil {
			return BurnPackResult{}, err
		}
		quote, err := g.API.BurnQuote(ctx, v.Address, spec.AgentID, spec.FaceMicros)
		if err != nil {
			return BurnPackResult{}, err
		}
		out, err := chain.Quote(ctx, spec.InputMicros)
		if err != nil {
			return BurnPackResult{}, err
		}
		if out == nil || out.Sign() <= 0 || out.BitLen() > 128 {
			return BurnPackResult{}, ErrInvalid
		}
		minimum := new(big.Int).Div(new(big.Int).Mul(out, big.NewInt(10000-spec.SlippageBPS)), big.NewInt(10000))
		floor, _ := burnInteger(quote.Quote.MinimumBurn)
		if minimum.Cmp(floor) < 0 {
			return BurnPackResult{}, errors.New("burn_slippage_exceeds_bound")
		}
		nonce, err := chain.Nonce(ctx, v.Address)
		if err != nil {
			return BurnPackResult{}, err
		}
		packs = append(packs, burnPackRecord{NextNonce: &nonce, Spec: spec, Payer: v.Address, Origin: g.API.url, Quote: quote.Quote, MinimumOut: minimum.String(), Started: time.Now().Unix()})
		index = len(packs) - 1
		if err = saveBurnPacks(g.Files, packs); err != nil {
			return BurnPackResult{}, err
		}
	}
	p := &packs[index]
	signer, err := signerFrom(v)
	if err != nil {
		return p.result(), err
	}
	defer signer.Close()
	for p.Stage < 4 {
		if action != "" && p.Hash == "" {
			if err = p.verifyUnbroadcast(ctx, chain); err != nil {
				return p.result(), err
			}
			if action == "abandon" {
				p.Abandoned = true
				if err = saveBurnPacks(g.Files, packs); err != nil {
					return p.result(), err
				}
				return p.result(), nil
			}
			if err = g.requotePack(ctx, p, chain); err != nil {
				return p.result(), err
			}
			if err = saveBurnPacks(g.Files, packs); err != nil {
				return p.result(), err
			}
			action = ""
		}
		if p.Hash != "" {
			amount, _ := burnInteger(p.MinimumOut)
			if p.Stage == 3 {
				amount, _ = burnInteger(p.Swapped)
			}
			to, data := burnPackTransaction(p.Stage, spec, p.Quote, amount)
			raw, hash, err := signedBurnTransaction(signer, p.Nonce, to, data, spec)
			if err != nil || raw != p.Raw || hash != p.Hash {
				return p.result(), ErrStorage
			}
			receipt, readErr := chain.Receipt(ctx, p.Hash, p.Payer)
			if readErr == nil && receipt != nil && receipt.Final {
				if !receipt.Success {
					p.Failed = true
					if err = saveBurnPacks(g.Files, packs); err != nil {
						return p.result(), err
					}
					return p.result(), errors.New("burn_transaction_reverted")
				}
				if p.Stage == 2 {
					received, ok := burnInteger(receipt.Received)
					minimum, _ := burnInteger(p.MinimumOut)
					if !ok || received.Cmp(minimum) < 0 {
						return p.result(), errors.New("burn_slippage_exceeds_bound")
					}
					p.Swapped = received.String()
				}
				if p.Stage == 3 {
					p.BurnHash = p.Hash
				}
				if p.Nonce == ^uint64(0) {
					return p.result(), ErrStorage
				}
				nextNonce := p.Nonce + 1
				p.NextNonce = &nextNonce
				p.Stage++
				p.Raw = ""
				p.Hash = ""
				if err = saveBurnPacks(g.Files, packs); err != nil {
					return p.result(), err
				}
				continue
			}
			if readErr != nil || receipt != nil {
				return p.result(), nil
			}
		}
		if action != "" {
			return p.result(), errors.New("burn_pack_pending")
		}
		// An uncertain transaction retains its original quote and exact bytes,
		// even after expiry. Only a stage with no signed transaction can requote.
		if p.Raw == "" && p.Quote.ExpiresAt <= time.Now().Unix() {
			if err = p.verifyUnbroadcast(ctx, chain); err != nil {
				return p.result(), err
			}
			if err = g.requotePack(ctx, p, chain); err != nil {
				return p.result(), err
			}
			if err = saveBurnPacks(g.Files, packs); err != nil {
				return p.result(), err
			}
		}
		if p.Raw == "" {
			if err = p.remainingGasBudget(p.Stage); err != nil {
				return p.result(), err
			}
			active, err := ReadLedger(g.Files)
			if err != nil {
				return p.result(), err
			}
			policy, err := g.policyLedger(active)
			if err != nil {
				return p.result(), err
			}
			filtered := policy.Entries[:0]
			for _, entry := range policy.Entries {
				if entry.InputDigest != "burn-pack:"+spec.PackID {
					filtered = append(filtered, entry)
				}
			}
			policy.Entries = filtered
			if err = policy.burnPackBudget(v.Settings, spec.InputMicros, time.Now().Unix()); err != nil {
				return p.result(), err
			}
			amount, _ := burnInteger(p.MinimumOut)
			if p.Stage == 3 {
				amount, _ = burnInteger(p.Swapped)
			}
			if p.Stage == 2 {
				current, err := chain.Quote(ctx, spec.InputMicros)
				if err != nil {
					return p.result(), err
				}
				if current == nil || current.Cmp(amount) < 0 {
					return p.result(), errors.New("burn_slippage_exceeds_bound")
				}
			}
			if err = p.verifyUnbroadcast(ctx, chain); err != nil {
				return p.result(), err
			}
			p.Nonce = *p.NextNonce
			to, data := burnPackTransaction(p.Stage, spec, p.Quote, amount)
			p.Raw, p.Hash, err = signedBurnTransaction(signer, p.Nonce, to, data, spec)
			if err != nil {
				return p.result(), err
			}
			p.NextNonce = nil
			p.TransactionCount++
			if err = saveBurnPacks(g.Files, packs); err != nil {
				return p.result(), err
			}
		}
		amount, _ := burnInteger(p.MinimumOut)
		if p.Stage == 3 {
			amount, _ = burnInteger(p.Swapped)
		}
		to, data := burnPackTransaction(p.Stage, spec, p.Quote, amount)
		expected, hash, err := signedBurnTransaction(signer, p.Nonce, to, data, spec)
		if err != nil || expected != p.Raw || hash != p.Hash {
			return p.result(), ErrStorage
		}
		// Never create a replacement authorization after an uncertain broadcast.
		if err = chain.Send(ctx, p.Raw, p.Hash); err != nil {
			return p.result(), nil
		}
		return p.result(), nil
	}
	if action != "" {
		return p.result(), errors.New("burn_pack_pending")
	}
	replay := p.RedeemAttempted
	if !replay && p.Quote.validate(p.Payer, time.Now().Unix()) != nil {
		return p.result(), errors.New("burn_quote_expired")
	}
	p.RedeemAttempted = true
	if err = saveBurnPacks(g.Files, packs); err != nil {
		return p.result(), err
	}
	redemption, err := g.API.redeemBurn(ctx, p.Quote, p.BurnHash, signer, replay)
	if err != nil {
		return p.result(), err
	}
	p.Grant = &redemption
	if err = saveBurnPacks(g.Files, packs); err != nil {
		return p.result(), err
	}
	return p.result(), nil
}

// A burn that outlives its quote can be redeemed explicitly with a fresh quote.
// The server's successful claim is sufficient to close the matching pack.
func recordPackRedemption(files *Files, result BurnRedemption) error {
	packs, err := readBurnPacks(files)
	if err != nil {
		return err
	}
	changed := false
	for i := range packs {
		p := &packs[i]
		c := result.Credit
		matches := p.BurnHash == c.TxHash || (p.Stage == 3 && p.Hash == c.TxHash)
		if matches && p.Payer == c.Payer && p.Spec.AgentID == c.AgentID && c.FaceMicros > 0 && c.FaceMicros <= p.Spec.FaceMicros {
			p.Grant = &result
			p.Stage = 4
			p.TransactionCount = max(p.TransactionCount, 4)
			p.BurnHash = c.TxHash
			p.Raw = ""
			p.Hash = ""
			changed = true
		}
	}
	if changed {
		return saveBurnPacks(files, packs)
	}
	return nil
}

// No raw bytes/hash means this stage has never crossed the broadcast boundary.
// Legacy journals retain the last finalized nonce, so its successor is pinned.
func (p *burnPackRecord) verifyUnbroadcast(ctx context.Context, chain BurnPackChain) error {
	if p.Raw != "" || p.Hash != "" || p.BurnHash != "" || p.Stage >= 4 {
		return errors.New("burn_pack_pending")
	}
	nonce, err := chain.Nonce(ctx, p.Payer)
	if err != nil {
		return err
	}
	expected := p.NextNonce
	if expected == nil {
		next := p.Nonce
		if p.Stage > 0 {
			if next == ^uint64(0) {
				return ErrStorage
			}
			next++
		} else {
			// Old stage-zero records predate the initial nonce checkpoint and
			// contain no signed transaction. Establish and cross-check it now.
			next = nonce
			nonce, err = chain.Nonce(ctx, p.Payer)
			if err != nil {
				return err
			}
		}
		expected = &next
	}
	if nonce != *expected {
		return errors.New("burn_pack_nonce_changed")
	}
	p.NextNonce = expected
	return nil
}

func (g *Engine) requotePack(ctx context.Context, p *burnPackRecord, chain BurnPackChain) error {
	stage := p.Stage
	// Permit2's approval is deadline-bound too. Renew it before the swap;
	// a completed swap goes straight to burning and is never repeated.
	if stage == 2 {
		stage = 1
	}
	if err := p.remainingGasBudget(stage); err != nil {
		return err
	}
	quote, err := g.API.BurnQuote(ctx, p.Payer, p.Spec.AgentID, p.Spec.FaceMicros)
	if err != nil {
		return err
	}
	floor, _ := burnInteger(quote.Quote.MinimumBurn)
	var minimum *big.Int
	if p.Stage == 3 {
		minimum, _ = burnInteger(p.Swapped)
		if minimum == nil || minimum.Cmp(floor) < 0 {
			return &BurnCreditError{Code: "burn_below_quote_minimum"}
		}
	} else {
		out, err := chain.Quote(ctx, p.Spec.InputMicros)
		if err != nil {
			return err
		}
		if out == nil || out.Sign() <= 0 || out.BitLen() > 128 {
			return ErrInvalid
		}
		minimum = new(big.Int).Div(new(big.Int).Mul(out, big.NewInt(10000-p.Spec.SlippageBPS)), big.NewInt(10000))
		if minimum.Cmp(floor) < 0 {
			return errors.New("burn_slippage_exceeds_bound")
		}
	}
	p.PreviousQuotes = append(p.PreviousQuotes, p.Quote)
	p.Quote, p.MinimumOut, p.Stage = quote.Quote, minimum.String(), stage
	return nil
}

func (p burnPackRecord) remainingGasBudget(stage int) error {
	fee, _ := burnInteger(p.Spec.MaxFeeWei)
	cap, _ := burnInteger(p.Spec.MaxGasWei)
	transactions := int64(p.TransactionCount + 4 - stage)
	worst := new(big.Int).Mul(new(big.Int).SetUint64(p.Spec.GasLimit), fee)
	worst.Mul(worst, big.NewInt(transactions))
	if worst.Cmp(cap) > 0 {
		return errors.New("burn_gas_cap_exceeded")
	}
	return nil
}
