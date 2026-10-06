package client

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"
)

// ProveInput verifies the stored signature and opens its input binding locally.
// Reconstruct terms from the signed receipt so future price changes cannot break proofs.
func (v ReceiptVerifier) ProveInput(e Entry, req Request, now time.Time) error {
	if e.Receipt == nil || v.VerifyStored(*e.Receipt, e, now) != nil {
		return ErrInvalid
	}
	s := e.Receipt.Envelope
	if req.Product != s.Product || normalizedNetwork(req.Options.Network) != s.Billing.Network {
		return ErrInvalid
	}
	if _, err := Canonical(req.Input); err != nil {
		return ErrInvalid
	}
	if e.Recovery != nil {
		canonical, err := standardCanonical(req, e.Recovery.Accepted)
		if err != nil || string(canonical) != e.Recovery.CanonicalRequest {
			return ErrInvalid
		}
		return nil
	}
	if s.Digest != "" {
		if req.Digest() != s.Digest {
			return ErrInvalid
		}
		return nil
	}
	if !hexDigest.MatchString(e.CommitmentSalt) {
		return errors.New("receipt_salt_unavailable")
	}
	salt, _ := hex.DecodeString(e.CommitmentSalt)
	canonical, err := canonicalValue(map[string]any{
		"v": 1, "input": req.Input, "product": s.Product,
		"product_schema_version": s.SchemaVersion, "predicate_version": s.PredicateVersion,
		"tier": s.Tier, "price_atomic": s.Billing.Amount, "asset": s.Billing.Asset,
		"network": s.Billing.Network, "scheme": "exact", "pay_to": s.Billing.PayTo,
		"billing_mode": s.Billing.Mode,
	})
	if err != nil {
		return ErrInvalid
	}
	h := sha256.New()
	h.Write([]byte("seconded-request/v1"))
	h.Write(salt)
	h.Write(canonical)
	if hex.EncodeToString(h.Sum(nil)) != s.Commitment {
		return ErrInvalid
	}
	return nil
}

// ProveLocalReceipt needs neither a wallet key nor a server/RPC connection.
// Its output never contains the saved salt or input.
func ProveLocalReceipt(files *Files, checkID string, req Request) error {
	unlock, err := files.Lock()
	if err != nil {
		return err
	}
	defer unlock()
	ledger, err := ReadLedger(files)
	if err != nil {
		return err
	}
	entry := ledger.Find(checkID)
	if entry == nil {
		return ErrNotFound
	}
	return (ReceiptVerifier{labels: releaseLabels}).ProveInput(*entry, req, time.Now())
}
