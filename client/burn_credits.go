package client

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"
)

const creditPreference = "seconded_discount"

type CreditAuthorization struct {
	Nonce     string `json:"nonce"`
	ExpiresAt int64  `json:"expires_at"`
	Signature string `json:"signature"`
}

// The signed body remains private until a receipt identifies the admission.
// Its hash and nonce retain the receipt association after body minimization.
type CreditRecovery struct {
	BalanceBasis      string              `json:"balance_basis,omitempty"`
	ServiceError      string              `json:"-"`
	BalanceObservedAt int64               `json:"balance_observed_at,omitempty"`
	Body              json.RawMessage     `json:"body,omitempty"`
	RequestHash       string              `json:"request_hash"`
	Association       PurchaseAssociation `json:"association"`
	ServerCheckID     string              `json:"server_check_id,omitempty"`
	Origin            string              `json:"origin"`
	Balance           *int64              `json:"balance_usd_micros,omitempty"`
}

func creditDomain() []byte {
	return keccak(keccak([]byte("EIP712Domain(string name,string version,uint256 chainId)")), keccak([]byte("SECONDED credits")), keccak([]byte("1")), word(4663))
}
func creditRequestHash(req Request) ([]byte, error) {
	req.Options.CreditAuthorization = nil
	body, err := canonicalValue(req)
	if err != nil {
		return nil, err
	}
	return keccak(body), nil
}
func creditSpendDigest(req Request, origin, nonce string, expiry int64) ([]byte, error) {
	if origin == "" || !hexWord.MatchString(nonce) || expiry <= 0 {
		return nil, ErrInvalid
	}
	hash, err := creditRequestHash(req)
	if err != nil {
		return nil, err
	}
	rawNonce, _ := hex.DecodeString(nonce[2:])
	message := keccak(keccak([]byte("CreditSpend(string origin,bytes32 requestHash,bytes32 nonce,uint256 expiresAt)")), keccak([]byte(origin)), hash, rawNonce, word(expiry))
	return keccak([]byte{0x19, 0x01}, creditDomain(), message), nil
}
func creditAssociation(hash []byte, nonce string) PurchaseAssociation {
	rawNonce, _ := hex.DecodeString(nonce[2:])
	digest := sha256.Sum256(append(append([]byte("seconded-credit-association/v1\x00"), hash...), rawNonce...))
	return PurchaseAssociation{Version: 1, SHA256: hex.EncodeToString(digest[:])}
}
func signCreditRequest(req Request, origin, nonce string, signer Signer) (Request, error) {
	expiry := time.Now().Unix() + 300
	digest, err := creditSpendDigest(req, origin, nonce, expiry)
	if err != nil {
		return req, err
	}
	sig, err := signChecked(signer, digest)
	if err != nil {
		return req, err
	}
	req.Options.CreditAuthorization = &CreditAuthorization{nonce, expiry, sig}
	return req, nil
}
func (c CreditRecovery) validate(e Entry) error {
	hash, err := hex.DecodeString(c.RequestHash)
	if err != nil || len(hash) != 32 || c.Origin == "" || c.Association != creditAssociation(hash, e.Nonce) || e.Network != "eip155:4663" || e.Recovery != nil || e.Tx != "" {
		return ErrStorage
	}
	if c.ServerCheckID != "" && !hexID.MatchString(c.ServerCheckID) {
		return ErrStorage
	}
	if len(c.Body) == 0 {
		if e.State == "RELEASED" && !e.RecoveryRequired && c.ServerCheckID == "" && (e.Receipt == nil || e.Receipt.Envelope.NotAdmitted) {
			return nil
		}
		if c.ServerCheckID == "" || e.Receipt == nil {
			return ErrStorage
		}
		return nil
	}
	var req Request
	if DecodeStrict(c.Body, &req, MessageLimit) != nil || req.Options.CreditAuthorization == nil || req.Options.PaymentPreference != creditPreference || req.Options.AgentID == "" || req.Options.Payer != e.Payer || req.Product != e.Product || req.Digest() != e.InputDigest || req.Options.Network != e.Network {
		return ErrStorage
	}
	proof := req.Options.CreditAuthorization
	h, err := creditRequestHash(req)
	if err != nil || hex.EncodeToString(h) != c.RequestHash || proof.Nonce != e.Nonce || proof.ExpiresAt != e.ValidBefore {
		return ErrStorage
	}
	digest, err := creditSpendDigest(req, c.Origin, proof.Nonce, proof.ExpiresAt)
	sig, decodeErr := hex.DecodeString(trimHex(proof.Signature))
	if err != nil || decodeErr != nil || verifyPaymentSigner(digest, sig, e.Payer) != nil {
		return ErrStorage
	}
	return nil
}
func trimHex(s string) string {
	if len(s) >= 2 && s[:2] == "0x" {
		return s[2:]
	}
	return ""
}

type creditStatus struct {
	Balance *int64      `json:"balance_usd_micros"`
	Refusal *CheckReply `json:"-"`
	CheckID *string     `json:"check_id"`
}

func (a *API) creditStatus(ctx context.Context, req Request) (creditStatus, error) {
	var result creditStatus
	status, b, _, err := a.request(ctx, "POST", "/v1/credits/status", req, nil)
	if err != nil && err.Error() != "client_upgrade_required" {
		return result, err
	}
	if status != 200 || err != nil {
		reply, parseErr := parseReply(status, b)
		if parseErr == nil && reply.Receipt.Envelope.NotAdmitted {
			result.Refusal = &reply
			return result, nil
		}
		if err != nil {
			return result, err
		}
		return result, publicAPIError(status, b)
	}
	if DecodeStrict(b, &result, ResponseLimit) != nil || (result.Balance != nil && *result.Balance < 0) || (result.CheckID != nil && !hexID.MatchString(*result.CheckID)) {
		return result, ErrInvalid
	}
	return result, nil
}

func (g *Engine) tryCredit(ctx context.Context, req Request, vault Vault, ledger *Ledger, tier string, amount int64, wait int) (Result, bool, error) {
	signer, err := signerFrom(vault)
	if err != nil {
		return Result{}, true, err
	}
	defer signer.Close()
	nonce := make([]byte, 32)
	rand.Read(nonce)
	req.Options.Payer = vault.Address
	req.Options.MaxPrice = Dollars(amount)
	req, err = signCreditRequest(req, g.API.url, "0x"+hex.EncodeToString(nonce), signer)
	if err != nil {
		return Result{}, true, err
	}
	body, err := canonicalValue(req)
	if err != nil {
		return Result{}, true, err
	}
	hash, _ := creditRequestHash(req)
	registry, err := registryRequestHash(req)
	if err != nil {
		return Result{}, true, err
	}
	comparison, err := comparisonRequestHash(req)
	if err != nil {
		return Result{}, true, err
	}
	portfolio, err := portfolioRequestScope(req)
	if err != nil {
		return Result{}, true, err
	}
	entry := Entry{Credit: &CreditRecovery{Body: body, RequestHash: hex.EncodeToString(hash), Association: creditAssociation(hash, req.Options.CreditAuthorization.Nonce), Origin: g.API.url}, CheckedSubject: checkedSubject(req), RegistryRequestSHA256: registry, ComparisonRequestSHA256: comparison, PortfolioScopeSHA256: portfolio, Network: req.Options.Network, CheckID: hex.EncodeToString(nonce[:16]), Product: req.Product, Tier: tier, InputDigest: req.Digest(), Nonce: req.Options.CreditAuthorization.Nonce, Payer: vault.Address, Amount: amount, State: "OUTSTANDING", SignedAt: time.Now().Unix(), ValidBefore: req.Options.CreditAuthorization.ExpiresAt, RecoveryRequired: true}
	ledger.Entries = append(ledger.Entries, entry)
	if err = ledger.Save(g.Files); err != nil {
		return Result{}, true, err
	}
	path := standardDoor
	if g.API.originalDoor {
		path = "/v1/checks"
	}
	status, b, _, requestErr := g.API.request(ctx, "POST", path, json.RawMessage(body), map[string]string{"Prefer": "wait=" + strconv.Itoa(wait)})
	fallback := false
	if requestErr == nil || requestErr.Error() == "client_upgrade_required" {
		reply, parseErr := parseReply(status, b)
		if parseErr == nil && (requestErr == nil || reply.Receipt.Envelope.NotAdmitted) && g.API.verifier.Verify(reply.Receipt, entry, time.Now()) == nil && validateReplyState(reply) == nil {
			entry.acceptReply(reply)
			fallback = requestErr == nil && status == 402 && reply.Receipt.Envelope.NotAdmitted && reply.Receipt.Envelope.Reason == "insufficient_credit"
		} else {
			entry.Credit.setDiagnostic(publicAPIError(status, b))
		}
	}
	if entry.Credit.Balance == nil && (entry.RecoveryRequired || entry.State == "SPENT") {
		if observed, readErr := g.API.creditStatus(ctx, req); readErr == nil {
			entry.Credit.observe(observed)
			if reply := observed.Refusal; reply != nil && g.API.verifier.Verify(reply.Receipt, entry, time.Now()) == nil && validateReplyState(*reply) == nil {
				entry.acceptReply(*reply)
			}
		}
	}
	ledger.Entries[len(ledger.Entries)-1] = entry
	if err = ledger.Save(g.Files); err != nil {
		return pending(entry), true, nil
	}
	if fallback {
		return Result{}, false, nil
	}
	return resultFor(entry), true, nil
}
func (a *API) collectCredit(ctx context.Context, e Entry, signer Signer) (CheckReply, error) {
	c := e.Credit
	if c == nil || c.validate(e) != nil || c.Origin != a.url || signer == nil || signer.Address() != e.Payer {
		return CheckReply{}, ErrStorage
	}
	if c.ServerCheckID == "" {
		var req Request
		if DecodeStrict(c.Body, &req, MessageLimit) != nil {
			return CheckReply{}, ErrStorage
		}
		fresh, err := signCreditRequest(req, a.url, e.Nonce, signer)
		if err != nil {
			return CheckReply{}, err
		}
		state, err := a.creditStatus(ctx, fresh)
		if err != nil {
			c.setDiagnostic(err)
			return CheckReply{}, err
		}
		if state.Refusal != nil {
			return *state.Refusal, nil
		}
		c.observe(state)
		if state.CheckID == nil {
			if time.Now().Unix() >= e.ValidBefore {
				return CheckReply{}, errors.New("presentation_indeterminate")
			}
			// A transport retry reuses the original body and signature byte for byte.
			path := standardDoor
			if a.originalDoor {
				path = "/v1/checks"
			}
			status, raw, _, err := a.request(ctx, "POST", path, json.RawMessage(c.Body), map[string]string{"Prefer": "wait=0"})
			if err != nil {
				return CheckReply{}, err
			}
			return parseReply(status, raw)
		}
		// The ID is only a locator until the signed receipt association verifies.
		e.CheckID = *state.CheckID
	} else {
		e.CheckID = c.ServerCheckID
	}
	e.Credit = nil
	return a.Collect(ctx, e, signer)
}

func validCreditAgent(agent string) bool {
	return agent != "" && len([]rune(agent)) <= 256 && strings.TrimSpace(agent) == agent && !strings.ContainsRune(agent, 0)
}

// This runs only after the common pinned-key and signing-domain verification.
// A refusal has no admission metadata; the original request/nonce association
// is its binding, including after the private signed body has been erased.
func verifyCreditRefusal(r SignedReceipt, e Entry, now time.Time) error {
	s, b, c := r.Envelope, r.Envelope.Billing, e.Credit
	if c == nil || c.validate(e) != nil || c.ServerCheckID != "" || e.State == "SPENT" || (e.Receipt != nil && !e.Receipt.Envelope.NotAdmitted) ||
		s.Version != 1 || s.Kind != "seconded-receipt" || !s.NotAdmitted || s.State != "refused" || s.CheckID != "" || s.Seq != 0 ||
		s.Product != e.Product || s.PurchaseAssociation == nil || *s.PurchaseAssociation != c.Association || !findingName.MatchString(s.Reason) ||
		s.Answer != nil || s.Verification != nil || s.Outcome != "" || s.OutcomeAt != nil || s.Commitment != "" || s.Digest != "" ||
		b.Mode != creditPreference || b.Charged != "no" || b.Network != "eip155:4663" || b.Asset != "usd_credit" || b.Amount != "0" || b.Payer != e.Payer || b.PayTo != "" || b.Tx != nil || b.Refund != nil || b.Settlement != "none" {
		return ErrInvalid
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(r.rawEnvelope, &fields) != nil || string(fields["check_id"]) != "null" {
		return ErrInvalid
	}
	issued, err := time.Parse(time.RFC3339, s.IssuedAt)
	if err != nil || issued.After(now.Add(30*time.Second)) || issued.Unix() < e.SignedAt-300 {
		return ErrInvalid
	}
	return nil
}

func (c *CreditRecovery) observe(status creditStatus) {
	c.Balance = status.Balance
	c.BalanceBasis = ""
	c.BalanceObservedAt = 0
	if c.Balance != nil {
		c.BalanceObservedAt = time.Now().Unix()
	}
}

func (c *CreditRecovery) setDiagnostic(err error) {
	c.ServiceError = ""
	if err != nil && safeError(err) != "operation_failed" {
		c.ServiceError = safeError(err)
	}
}

// Missing snapshots are valid for older servers. Partial or malformed snapshots
// cannot be presented as an observed balance after a check.
func (r CheckReply) validCreditBalance() bool {
	if r.CreditBalanceMicros == nil {
		return r.CreditBalanceBasis == "" && r.CreditBalanceObservedAt == 0
	}
	return *r.CreditBalanceMicros >= 0 && r.CreditBalanceBasis == "after_reservation" && r.CreditBalanceObservedAt > 0
}
