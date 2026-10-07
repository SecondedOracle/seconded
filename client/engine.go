package client

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Engine struct {
	Files *Files
	Store SecretStore
	API   *API
	Chain ChainReader
}
type Result struct {
	TradeFacts              []string          `json:"trade_facts,omitempty"`
	CheckedSubject          *CheckedSubject   `json:"checked_subject"`
	SubjectInstruction      string            `json:"subject_instruction"`
	ObservationFacts        []string          `json:"observation_facts,omitempty"`
	PortfolioFacts          []string          `json:"portfolio_facts,omitempty"`
	PortfolioExpired        *bool             `json:"portfolio_expired,omitempty"`
	LendingFacts            []string          `json:"lending_facts,omitempty"`
	LendingExpired          *bool             `json:"lending_expired,omitempty"`
	ComparisonFacts         []string          `json:"comparison_facts,omitempty"`
	ComparisonExpired       *bool             `json:"comparison_expired,omitempty"`
	StockFacts              []string          `json:"stock_facts,omitempty"`
	CheckedBy               []string          `json:"checked_by"`
	ModelConfigFingerprint  string            `json:"model_config_fingerprint,omitempty"`
	ModelConfigFingerprints map[string]string `json:"model_config_fingerprints,omitempty"`
	Status                  string            `json:"status"`
	CheckID                 string            `json:"check_id,omitempty"`
	Product                 string            `json:"product,omitempty"`
	Tier                    string            `json:"tier,omitempty"`
	Price                   string            `json:"price_usd,omitempty"`
	Verdict                 *Answer           `json:"verdict,omitempty"`
	Verification            *Verification     `json:"verification,omitempty"`
	Charged                 string            `json:"charged"`
	Receipt                 string            `json:"receipt"`
	Reason                  string            `json:"reason,omitempty"`
	Message                 string            `json:"message,omitempty"`
	Next                    string            `json:"next,omitempty"`
	After                   int               `json:"after_seconds,omitempty"`
	Notices                 []string          `json:"notices"`
	NoticeCodes             []string          `json:"notice_codes,omitempty"`
	Disclaimer              string            `json:"disclaimer,omitempty"`
}

func pending(e Entry) Result {
	after := 2
	if e.Recovery != nil {
		after = max(after, int(e.retryAt()-time.Now().Unix()))
	}
	return Result{CheckedSubject: e.CheckedSubject, SubjectInstruction: subjectInstruction, CheckedBy: []string{}, Status: "pending", CheckID: e.CheckID, Product: e.Product, Tier: e.Tier, Price: Dollars(e.Amount), Charged: "not_yet_known", Receipt: "none", Next: "call_receipt_after", After: after, Notices: []string{}, Disclaimer: disclaimer(e.Product), Message: fmt.Sprintf("Pending — call seconded_receipt with check_id %s in ~%d s. An admitted check continues server-side; receipt recovery does not buy another check.", e.CheckID, after)}
}
func resultFor(e Entry) Result {
	r := pending(e)
	if e.RecoveryRequired {
		r.Reason = "presentation_indeterminate"
		r.Message += " Payment may have been sent; do not submit another check."
	}
	if e.State == "SPENT" {
		r.Charged = "yes_final"
	} else if e.State == "RELEASED" || e.State == "RESERVED" {
		r.Charged = "no"
	}
	if e.Receipt == nil {
		return r
	}
	s := e.Receipt.Envelope
	if !oneOf(s.State, "running", "settling", "delayed", "unavailable") {
		r.Message = ""
	}
	r.Receipt = "verified"
	// Receipt v3's signed findings and coverage; v1 and v2 receipts carry none and the field is omitted.
	r.Verification = s.Verification
	r.CheckedBy = append([]string{}, s.CheckedBy...)
	r.ModelConfigFingerprint = s.ModelConfigFingerprint
	r.ModelConfigFingerprints = s.ModelConfigFingerprints
	switch s.State {
	case "included", "final", "released":
		r.Status = "agreed"
		r.Verdict = s.Answer
		r.Next = ""
		r.After = 0
		if e.State != "SPENT" && s.State != "released" {
			r.addNotices("receipt_ledger_mismatch")
			r.Next, r.After = "call_receipt_after", 2
		}
	case "trial_delivered":
		r.Status = "agreed"
		r.Verdict = s.Answer
		r.Charged = "no"
		r.Next = ""
		r.After = 0
	case "refund_owed", "refunded":
		r.Status = s.State
		r.addNotices("server_" + s.State)
		r.Next = ""
		r.After = 0
	case "no_agreement":
		r.Status = "not_verified"
		r.Message = s.Message
		if explanation := publicErrorMessages[s.Reason]; explanation != "" {
			r.Message += " " + explanation
		}
		r.Reason = s.Reason
		r.Next = "pause_or_ask_human"
		r.After = 0
	case "closed_no_charge":
		r.Status = "no_agreement"
		r.Reason = "closed_no_charge"
		r.Message = publicErrorMessages[r.Reason]
		r.Charged = "no"
		r.Next = "retry_check"
		r.After = 0
		if e.State == "SPENT" {
			// Conflicting local payment evidence still requires accounting recovery.
			r.Charged = "yes_final"
			r.Reason = "receipt_ledger_mismatch"
			r.Message = "Payment records disagree. Recover the receipt before retrying."
			r.addNotices("receipt_ledger_mismatch")
			r.Next, r.After = "call_receipt_after", 2
		}
	case "refused", "content_refused", "frozen_unsettled":
		r.Status = "refused"
		r.Next = ""
		r.After = 0
	case "service_failed":
		r.Status = "failed"
		r.Next = ""
		r.After = 0
	}
	if oneOf(s.State, "service_failed", "refused", "content_refused", "frozen_unsettled") {
		r.Reason = s.State
		if s.Reason != "" {
			r.Reason = safeError(errors.New(s.Reason))
		}
		r.Message = publicErrorMessages[r.Reason]
		r.Next = "call_receipt_after"
	}
	if e.Recovery != nil && s.State == "refused" && e.State != "SPENT" && s.Billing.Charged == "no" {
		r.Charged = "no"
		// Keep the live authorization until validBefore; display does not release its reservation.
		r.After = 2
	}
	if e.Recovery != nil && s.State == "service_failed" {
		r.Charged, r.Next, r.After = "no", "pause_or_ask_human", 0
		if e.Recovery.NewPurchaseAllowed {
			r.Next = "retry_check"
		}
	}
	r.TradeFacts = tradeFactsText(r.Verdict)
	r.StockFacts = stockFactsText(r.Verdict)
	r.ObservationFacts = observationFactsText(r.Verdict)
	if r.Verdict != nil && r.Verdict.PortfolioReport != nil {
		facts, expired := portfolioText(r.Verdict.PortfolioReport, time.Now().Unix())
		r.PortfolioFacts, r.PortfolioExpired = facts, &expired
		r.Next = "review_portfolio"
	}
	if r.Verdict != nil && r.Verdict.LendingObservations != nil {
		facts, expired := lendingText(r.Verdict.LendingObservations, time.Now().Unix())
		r.LendingFacts, r.LendingExpired = facts, &expired
		r.Next = "review_lending_position"
	}
	if r.Verdict != nil && r.Verdict.Comparison != nil {
		facts, expired := comparisonText(r.Verdict, time.Now().Unix())
		r.ComparisonFacts, r.ComparisonExpired = facts, &expired
		r.Next = "inspect_quotes"
	}
	return r
}
func signerFrom(v Vault) (*LocalSigner, error) {
	b, e := hex.DecodeString(v.Key)
	if e != nil {
		return nil, ErrStorage
	}
	defer clear(b)
	return NewLocalSigner(b)
}
func (g *Engine) Check(ctx context.Context, req Request, maxPrice string, wait int) (result Result, err error) {
	return g.check(ctx, req, maxPrice, wait, nil)
}

func (g *Engine) check(ctx context.Context, req Request, maxPrice string, wait int, signing *X402SigningRequest) (result Result, err error) {
	ctx, cancel := checkContext(ctx)
	defer cancel()
	var recoveryEntry *Entry
	defer func() {
		result.SubjectInstruction = subjectInstruction
		if err != nil {
			if recoveryEntry != nil {
				result = resultFor(*recoveryEntry)
			} else {
				result.Charged = "no"
			}
			result = failedResult(result, err)
		}
	}()
	if !supportedNetwork(req.Options.Network) || req.Options.Network == "" || !deploymentPaymentNetwork(req.Options.Network) {
		return Result{}, inputError("network", "Choose a supported payment network.")
	}
	if wait < 0 || wait > 25 {
		return Result{}, inputError("wait_seconds", "Use an integer from 0 to 25.")
	}
	if err = validateAddressCharacters(req.Input); err != nil {
		if errors.Is(err, ErrInvalid) {
			return Result{}, inputError("input", "Supply the required input object using valid JSON.")
		}
		return Result{}, err
	}
	if err = validateProductInput(req.Product, req.Input); err != nil {
		return Result{}, err
	}
	if req.Product == "x402_payment_check" {
		req.Input, err = bindX402SigningContext(req.Input, signing)
		if err != nil {
			return Result{}, err
		}
	}
	req, err = normalizePortfolioRequest(req)
	if err != nil {
		return Result{}, err
	}
	size, e := req.Size()
	if e != nil {
		return Result{}, inputError("input", "Use valid input matching the product schema from seconded_products.")
	}
	tier, amount, e := ProductPriceOnNetwork(req.Product, size, req.Options.Network)
	if e != nil {
		return Result{}, e
	}
	if maxPrice != "" {
		max, e := USD(maxPrice)
		if e != nil || amount > max {
			return Result{}, errors.New("price_above_max")
		}
	}
	unlock, e := g.Files.Lock()
	if e != nil {
		return Result{}, e
	}
	defer unlock()
	vault, e := LoadVault(g.Store)
	if e != nil {
		return Result{}, e
	}
	l, e := ReadLedger(g.Files)
	if e != nil {
		return Result{}, e
	}
	if duplicate := l.Duplicate(req.Product, req.Digest(), time.Now().Unix(), vault.Settings.Dedupe); duplicate != nil {
		// A failed re-verification must retain the existing check's uncertainty,
		// without exposing the now-unverified receipt or verdict.
		recovery := *duplicate
		recovery.Receipt = nil
		recoveryEntry = &recovery
		if duplicate.Receipt != nil && g.API != nil {
			if g.API.verifier.VerifyStored(*duplicate.Receipt, *duplicate, time.Now()) != nil {
				return Result{}, ErrInvalid
			}
		}
		return resultFor(*duplicate), nil
	}
	policyLedger, e := g.policyLedger(l)
	if e != nil {
		return Result{}, e
	}
	if e = policyLedger.requireRecovery(); e != nil {
		return Result{}, e
	}
	if vault.Settings.ValueGate && maxPrice == "" {
		return Result{}, errors.New("value_gate_requires_max_price")
	}
	preflight := vault.Settings
	// Window headroom may become available only after refreshing chain evidence.
	preflight.Limits.Hour, preflight.Limits.Day, preflight.Limits.Outstanding = nil, nil, nil
	if e = policyLedger.Budget(preflight, amount, time.Now().Unix()); e != nil {
		return Result{}, e
	}
	if g.API == nil {
		return Result{}, errors.New("release_identity_not_configured")
	}
	if g.Chain == nil {
		return Result{}, errors.New("budget_blocked_on_chain_evidence")
	}
	chain, e := g.chainFor(req.Options.Network)
	if e != nil {
		return Result{}, e
	}
	snap, e := chain.Snapshot(ctx, vault.Address)
	if e != nil {
		return Result{}, &budgetEvidenceError{stage: "snapshot", cause: e}
	}
	if normalizedNetwork(snap.Network) != req.Options.Network {
		return Result{}, ErrInvalid
	}
	if e = l.PreparePayment(ctx, chain, snap); e != nil {
		return Result{}, e
	}
	if e = l.Save(g.Files); e != nil {
		return Result{}, e
	}
	reserved := l.ReservedForNetwork(req.Options.Network)
	if snap.Balance < amount || snap.Balance-amount < reserved {
		return Result{}, errors.New("wallet_unfunded")
	}
	req.Options.Payer = vault.Address
	q, e := g.API.Challenge(ctx, req)
	if e != nil {
		return Result{}, e
	}
	policyLedger, e = g.policyLedger(l)
	if e != nil {
		return Result{}, e
	}
	// Re-read protected settings immediately before reserving and signing.
	vault, e = LoadVault(g.Store)
	if e != nil {
		return Result{}, e
	}
	if e = policyLedger.Budget(vault.Settings, amount, budgetTime(snap)); e != nil {
		return Result{}, e
	}
	if vault.Settings.ValueGate && maxPrice == "" {
		return Result{}, errors.New("value_gate_requires_max_price")
	}
	now := time.Now().Unix()
	if ctx.Err() != nil {
		return Result{}, errors.New("api_unreachable")
	}
	if e = q.Validate(req, now); e != nil {
		return Result{}, e
	}
	for _, prior := range l.Entries {
		if prior.Nonce == q.Accepts[0].Extra.Nonce || prior.CheckID == q.Seconded.CheckID {
			return Result{}, errors.New("duplicate_authorization")
		}
	}
	after := now - 5
	before := after + ValiditySeconds
	if q.standard != nil {
		// T=380 and a 31-second backdate tolerate the published 30-second
		// clock skew without making validAfter equal the settlement clock.
		after, before = now-31, now+q.standard.Accepts[0].Timeout
	}
	if before > q.Seconded.MaxBefore {
		before = q.Seconded.MaxBefore
	}
	if before < now+q.Seconded.MinRemaining {
		return Result{}, ErrInvalid
	}
	if checker, ok := chain.(interface {
		CheckEOA(context.Context, string) error
	}); ok {
		if err := checker.CheckEOA(ctx, vault.Address); err != nil {
			return Result{}, err
		}
	} else {
		return Result{}, errors.New("payer_code_check_unavailable")
	}
	registryBinding, err := registryRequestHash(req)
	if err != nil {
		return Result{}, err
	}
	comparisonBinding, err := comparisonRequestHash(req)
	if err != nil {
		return Result{}, err
	}
	portfolioBinding, err := portfolioRequestScope(req)
	if err != nil {
		return Result{}, err
	}
	entry := Entry{CheckedSubject: checkedSubject(req), PortfolioScopeSHA256: portfolioBinding, ComparisonRequestSHA256: comparisonBinding, RegistryRequestSHA256: registryBinding, Network: req.Options.Network, CheckID: q.Seconded.CheckID, Product: req.Product, Tier: tier, InputDigest: req.Digest(), Commitment: q.Seconded.Commitment, CommitmentSalt: q.Seconded.Salt, Nonce: q.Accepts[0].Extra.Nonce, Payer: vault.Address, Amount: amount, State: "RESERVED", SignedAt: now, ValidBefore: before, StartBlock: snap.Number}
	if q.standard != nil {
		entry.PaymentDoor = "standard"
	}
	l.Entries = append(l.Entries, entry)
	if e = l.Save(g.Files); e != nil {
		return Result{}, e
	}
	signer, e := signerFrom(vault)
	if e != nil {
		return Result{}, e
	}
	defer signer.Close()
	auth := Authorization{vault.Address, PayTo, strconv.FormatInt(amount, 10), strconv.FormatInt(after, 10), strconv.FormatInt(before, 10), entry.Nonce}
	digest, e := auth.DigestFor(req.Options.Network)
	if q.standard != nil {
		digest, e = auth.standardDigestFor(req.Options.Network)
	}
	if e != nil {
		return Result{}, e
	}
	sig, e := signChecked(signer, digest)
	if e != nil {
		return Result{}, e
	}
	payload := PaymentPayload{Version: 2, Accepted: q.Accepts[0]}
	payload.Payload.Signature = sig
	payload.Payload.Authorization = auth
	b, e := json.Marshal(payload)
	if e != nil {
		return Result{}, ErrInvalid
	}
	entry.Payload = base64.StdEncoding.EncodeToString(b)
	if q.standard != nil {
		entry.Recovery, e = newRecovery(g.API.url, req, q.standard.Accepts[0], auth, sig)
		if e != nil {
			return Result{}, e
		}
		entry.Payload = entry.Recovery.PaymentSignature
	}
	entry.State = "OUTSTANDING"
	entry.RecoveryRequired = true
	l.Entries[len(l.Entries)-1] = entry
	if e = l.Save(g.Files); e != nil {
		return Result{}, e
	}
	// Persist uncertainty before the sole send: a crash may lose its response.
	// Standard recovery replays only this durable credential; rollback uses ownership GET.
	recoveryEntry = &entry
	var reply CheckReply
	if entry.Recovery != nil {
		reply, e = g.API.replay(ctx, entry, wait)
	} else {
		reply, e = g.API.Present(ctx, req, entry.Payload, wait)
	}
	if e == nil {
		// The signed comparison outcome must be fresh when committed. Delivery
		// after its original expiry is still the same paid historical answer;
		// resultFor marks it expired without withholding it for recovery.
		e = g.API.verifier.Verify(reply.Receipt, entry, time.Now())
		if e == nil {
			e = validateReplyState(reply)
		}
	}
	if e != nil {
		if saveErr := l.Save(g.Files); saveErr != nil {
			return Result{}, saveErr
		}
		r := resultFor(entry)
		if identityFailure(e) {
			r.Reason = safeError(e)
			r.Message = publicErrorMessages[r.Reason] + " Call seconded_receipt; do not submit another check."
		}
		r.addNotices(l.Alerts(vault.Settings, budgetTime(snap))...)
		return r, nil
	}
	// Only a verified, internally consistent reply resolves send uncertainty.
	// If its durable write fails, return the prior pending state without an
	// actionable answer from memory that was never committed to the ledger.
	unsaved := entry
	recoveryEntry = &unsaved
	entry.acceptReply(reply)
	l.Entries[len(l.Entries)-1] = entry
	if e = l.Save(g.Files); e != nil {
		return Result{}, e
	}
	r := resultFor(entry)
	r.addNotices(l.Alerts(vault.Settings, budgetTime(snap))...)
	return r, nil
}

// Refresh uses only wallet-owned check IDs already in the durable ledger.
// Failures preserve reservations and are never interpreted as nonpayment.
func (g *Engine) Refresh(ctx context.Context) error {
	if g.API == nil || g.Chain == nil {
		return nil
	}
	unlock, e := g.Files.Lock()
	if e != nil {
		return e
	}
	defer unlock()
	v, e := LoadVault(g.Store)
	if e != nil {
		return e
	}
	l, e := ReadLedger(g.Files)
	if e != nil {
		return e
	}
	work := false
	for _, entry := range l.Entries {
		owed := entry.Receipt != nil && entry.Receipt.Envelope.State == "refund_owed" &&
			entry.State != "RESERVED" && !entry.RecoveryRequired
		// Finalized payments still need collection until their owed refund completes.
		if entry.State == "OUTSTANDING" || owed {
			work = true
			break
		}
	}
	if !work {
		return nil
	}
	g.reconcileNetworks(ctx, &l, v.Address)
	s, e := signerFrom(v)
	if e != nil {
		return e
	}
	defer s.Close()
	for i := range l.Entries {
		entry := &l.Entries[i]
		if ctx.Err() != nil {
			break
		}
		// An owed refund may since have been paid. Re-signed unchanged, it fails Verify below and the stored copy stands.
		if entry.Receipt != nil && !oneOf(entry.Receipt.Envelope.State, "running", "settling", "delayed", "unavailable", "refund_owed") {
			continue
		}
		if entry.State == "RESERVED" || entry.RecoveryRequired || entry.Recovery != nil {
			continue
		}
		reply, err := g.API.Collect(ctx, *entry, s)
		if err == nil && g.API.verifier.Verify(reply.Receipt, *entry, time.Now()) == nil && validateReplyState(reply) == nil {
			entry.acceptReply(reply)
		}
	}
	return l.Save(g.Files)
}
func (g *Engine) Quote(ctx context.Context, req Request) (Quote, error) {
	if !supportedNetwork(req.Options.Network) || req.Options.Network == "" || !deploymentPaymentNetwork(req.Options.Network) {
		return Quote{}, inputError("network", "Choose a supported payment network.")
	}
	if err := validateProductInput(req.Product, req.Input); err != nil {
		return Quote{}, err
	}
	if g.API == nil {
		return Quote{}, errors.New("release_identity_not_configured")
	}
	v, e := LoadVault(g.Store)
	if e != nil {
		return Quote{}, e
	}
	req.Options.Payer = v.Address
	return g.API.Quote(ctx, req)
}

// A no-argument receipt listing returns the newest recentReceipts checks and older checks still awaiting a
// receipt, newest first, at most maxListedReceipts in all.
const (
	recentReceipts    = 20
	maxListedReceipts = 100
)

func (g *Engine) Receipts(ctx context.Context, id string) ([]Result, error) {
	ctx, cancel := checkContext(ctx)
	defer cancel()
	if id != "" && !hexID.MatchString(id) {
		return nil, inputError("check_id", "Use 32 lowercase hexadecimal characters, or omit check_id to list recent checks.")
	}
	unlock, e := g.Files.Lock()
	if e != nil {
		return nil, e
	}
	defer unlock()
	l, e := ReadLedger(g.Files)
	if e != nil {
		return nil, e
	}
	if id != "" && l.Find(id) == nil {
		return nil, errors.New("unknown_check")
	}
	if g.API == nil {
		return nil, errors.New("release_identity_not_configured")
	}
	var s *LocalSigner
	defer func() {
		if s != nil {
			s.Close()
		}
	}()
	if g.Chain != nil && len(l.Entries) > 0 {
		g.reconcileNetworks(ctx, &l, l.Entries[0].Payer)
	}
	out := []Result{}
	// The ledger is append-only and oldest first. Walk it newest first: without an ID, list the most recent
	// checks plus any older check still awaiting its receipt, since only this listing clears RecoveryRequired.
	for i := len(l.Entries) - 1; i >= 0; i-- {
		entry := &l.Entries[i]
		if id != "" && entry.CheckID != id {
			continue
		}
		awaiting := entry.RecoveryRequired || receiptNeedsCollection(entry.Receipt)
		if id == "" && i < len(l.Entries)-recentReceipts && !awaiting {
			continue
		}
		if entry.Receipt != nil {
			if g.API.verifier.VerifyStored(*entry.Receipt, *entry, time.Now()) != nil {
				return nil, errors.New("api_identity_mismatch")
			}
		}
		if entry.Recovery != nil && entry.Recovery.State == "unresolved" && entry.Receipt != nil && time.Now().Unix() > entry.ValidBefore && oneOf(entry.Receipt.Envelope.State, "refused", "content_refused", "frozen_unsettled") {
			entry.acceptReply(CheckReply{Receipt: *entry.Receipt, Hints: Hints{NewQuoteAllowed: entry.Recovery.NewPurchaseAllowed}})
		}
		// An owed refund is collected until the operator records it as refunded (docs/runbooks/refunds.md).
		if receiptNeedsCollection(entry.Receipt) && time.Now().Unix() >= entry.retryAt() {
			if s == nil {
				s, e = g.recoverySigner(*entry)
				if e != nil {
					return nil, e
				}
			}
			reply, err := g.API.Collect(ctx, *entry, s)
			if identityFailure(err) {
				return nil, err
			}
			if err == nil && !g.unchangedPending(reply, *entry) {
				if g.API.verifier.Verify(reply.Receipt, *entry, time.Now()) != nil || validateReplyState(reply) != nil {
					return nil, errors.New("api_identity_mismatch")
				}
				entry.acceptReply(reply)
				// A newly collected FINAL receipt can provide the locator that
				// was unavailable during the pre-collection chain check.
				g.reconcileFinalReceipt(ctx, &l, entry)
			}
		}
		// Also minimize records resolved by earlier client versions, after re-verification.
		entry.archiveRecovery()
		out = append(out, resultFor(*entry))
		if len(out) >= maxListedReceipts {
			break
		}
	}
	if e = l.Save(g.Files); e != nil {
		return nil, e
	}
	return out, nil
}

// unchangedPending authenticates a reissued pending receipt; the stored copy stands.
func (g *Engine) unchangedPending(reply CheckReply, e Entry) bool {
	return e.Receipt != nil && receiptNeedsCollection(e.Receipt) && validateReplyState(reply) == nil &&
		g.API.verifier.sameRevision(reply.Receipt, e, time.Now())
}
func (g *Engine) Wallet(ctx context.Context, action string) (any, error) {
	unlock, e := g.Files.Lock()
	if e != nil {
		return nil, e
	}
	defer unlock()
	v, e := LoadVault(g.Store)
	if e != nil {
		return nil, e
	}
	if action == "freeze" {
		next := v.Settings
		next.Frozen = true
		if e = g.savePolicyLocked(v, next, "chat"); e != nil {
			return nil, e
		}
		v.Settings = next
	} else if action == "unfreeze" {
		off := false
		return nil, g.terminalPolicyError(limitUpdate{Frozen: &off})
	} else if action != "status" && action != "" {
		return nil, ErrInvalid
	}
	l, e := ReadLedger(g.Files)
	if e != nil {
		return nil, e
	}
	return g.walletStatus(ctx, v.Address, v.Settings, l)
}

// Wallet holds the profile lock; status reads need no signing credential.
func (g *Engine) walletStatus(ctx context.Context, address string, settings Settings, l Ledger) (any, error) {
	now := time.Now().Unix()
	var balance *string
	evidence := "unavailable"
	balances, snapshot := g.walletBalances(ctx, address)
	if snapshot != nil {
		if chain, err := walletReader(g.Chain, DefaultPaymentNetwork); err == nil {
			now = budgetTime(*snapshot)
			if l.Reconcile(ctx, chain, *snapshot) == nil {
				b := Dollars(snapshot.Balance)
				balance = &b
				evidence = "finalized"
				if e := l.Save(g.Files); e != nil {
					return nil, e
				}
			}
		}
	}
	funding, messages := g.walletFunding(ctx, address, balances)
	status := "wallet_status"
	for _, f := range funding {
		if f.Status == "pending" {
			status = "pending"
			if f.Network == DefaultPaymentNetwork {
				balance, evidence = nil, "pending"
			}
		}
	}
	h, d, o := l.Totals(now)
	return map[string]any{"status": status, "message": strings.Join(messages, "; "), "funding": funding, "address": address, "network": DefaultPaymentNetwork, "asset": pins(DefaultPaymentNetwork).Asset, "balance_usd": balance, "chain_evidence": evidence, "settings": publicLimits(settings), "hour_used_usd": Dollars(h), "day_used_usd": Dollars(d), "outstanding_usd": Dollars(o), "notices": noticeMessages(l.Alerts(settings, now)), "notice_codes": l.Alerts(settings, now), "testnet_only": false, "default_network_is_testnet": testnet(DefaultPaymentNetwork), "balances": balances, "funding_instructions": FundingInstructions}, nil
}

// A configured reader supplies an explicit per-network view or only the Base view.
func (g *Engine) chainFor(network string) (ChainReader, error) {
	if router, ok := g.Chain.(interface {
		ForNetwork(string) (ChainReader, error)
	}); ok {
		return router.ForNetwork(normalizedNetwork(network))
	}
	if normalizedNetwork(network) == Network && g.Chain != nil {
		return g.Chain, nil
	}
	return ReleaseEvidenceFor(network)
}

// Recheck only this Robinhood entry after verified FINAL delivery. A failed
// independent certificate preserves the reservation; the signed receipt alone
// never marks it spent. The caller's final ledger save commits both together.
func (g *Engine) reconcileFinalReceipt(ctx context.Context, ledger *Ledger, entry *Entry) {
	if g.Chain == nil || entry.State != "OUTSTANDING" || !robinhoodNetwork(normalizedNetwork(entry.Network)) || entry.Receipt == nil || entry.Receipt.Envelope.State != "final" {
		return
	}
	chain, err := g.chainFor(entry.Network)
	if err != nil {
		return
	}
	snap, err := chain.Snapshot(ctx, entry.Payer)
	if err != nil || normalizedNetwork(snap.Network) != normalizedNetwork(entry.Network) {
		return
	}
	candidate := Ledger{Version: ledger.Version, Entries: []Entry{*entry}, Anchor: ledger.Anchor, Anchors: maps.Clone(ledger.Anchors)}
	if candidate.Reconcile(ctx, chain, snap) != nil {
		return
	}
	*entry = candidate.Entries[0]
	ledger.Anchor, ledger.Anchors = candidate.Anchor, candidate.Anchors
}

func (g *Engine) reconcileNetworks(ctx context.Context, ledger *Ledger, payer string) {
	networks := map[string]bool{}
	for _, entry := range ledger.Entries {
		networks[normalizedNetwork(entry.Network)] = true
	}
	for network := range networks {
		chain, err := g.chainFor(network)
		if err != nil {
			continue
		}
		snap, err := chain.Snapshot(ctx, payer)
		if err == nil && normalizedNetwork(snap.Network) == network {
			if network == "eip155:5042" || network == "eip155:5042002" {
				_ = ledger.reconcileEntries(ctx, chain, snap, g.Files)
			} else {
				_ = ledger.Reconcile(ctx, chain, snap)
			}
		}
	}
}

// Funding observations are transient display data; only finalized snapshots
// enter the durable ledger. A positive latest-minus-finalized delta is arriving.
type FundingStatus struct {
	Network   string  `json:"network"`
	Status    string  `json:"status"`
	Finalized *string `json:"finalized_balance_usd"`
	Latest    *string `json:"latest_balance_usd"`
	Pending   *string `json:"pending_usd"`
	Message   string  `json:"message"`
}

func (g *Engine) walletFunding(ctx context.Context, address string, balances []WalletBalance) ([]FundingStatus, []string) {
	out := make([]FundingStatus, len(balances))
	var wg sync.WaitGroup
	for i, balance := range balances {
		out[i] = FundingStatus{Network: balance.Network, Status: "unavailable", Finalized: balance.Balance, Message: "Funding observation unavailable; check again later."}
		if balance.Evidence == "recent_agreement" {
			out[i].Finalized = nil
			out[i].Latest = balance.Balance
			out[i].Status = "recent_agreement"
			out[i].Message = "Both readers confirm your recent balance. Final payment confirmation is separate; start a check when ready and the balance will be checked again."
			continue
		}
		if balance.Balance == nil {
			continue
		}
		chain, err := walletReader(g.Chain, balance.Network)
		if err != nil {
			continue
		}
		reader, ok := chain.(interface {
			LatestSnapshot(context.Context, string) (ChainSnapshot, error)
		})
		if !ok {
			continue
		}
		wg.Add(1)
		go func(i int, balance WalletBalance) {
			defer wg.Done()
			readCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
			defer cancel()
			latest, err := reader.LatestSnapshot(readCtx, address)
			if err != nil || normalizedNetwork(latest.Network) != balance.Network || latest.Balance < 0 {
				return
			}
			finalized, err := USD(*balance.Balance)
			if err != nil {
				return
			}
			value := Dollars(latest.Balance)
			out[i].Latest = &value
			pending := int64(0)
			if latest.Balance > finalized {
				pending = latest.Balance - finalized
			}
			amount := Dollars(pending)
			out[i].Pending = &amount
			out[i].Status = "none"
			out[i].Message = "Your displayed balance is confirmed and available for checks. Start a check when ready, or fund this network if you need more."
			if pending > 0 {
				out[i].Status = "pending"
				out[i].Message = fmt.Sprintf("pending (%s %s arriving, not yet final)", amount, balance.Token)
				// Do not present a finalized zero as the total visible wallet balance.
				// The numeric finalized amount remains explicit in FundingStatus.
				balances[i].Balance, balances[i].Evidence = nil, "pending"
			}
		}(i, balance)
	}
	wg.Wait()
	messages := []string{}
	for i, funding := range out {
		if funding.Status == "pending" {
			messages = append(messages, balances[i].Name+": "+funding.Message)
		}
	}
	if len(messages) == 0 {
		messages = append(messages, "Balances identify whether confirmation is final or recent. Choose the network for your check and fund it if needed.")
	}
	return out, messages
}
