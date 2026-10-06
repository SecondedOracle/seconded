package client

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"errors"
)

var errReceiptKeyPinMismatch = errors.New("receipt_key_pin_mismatch")

func identityFailure(err error) bool {
	return errors.Is(err, errReceiptKeyPinMismatch) || errors.Is(err, errAPIIdentityMismatch)
}

type receiptKeyDocument struct {
	Network string `json:"network"`
	Keys    []struct {
		ID        string `json:"key_id"`
		Algorithm string `json:"algorithm"`
		PublicKey string `json:"public_key_hex"`
	} `json:"keys"`
}

func validateReceiptKeyDocument(data []byte, trusted map[string]ed25519.PublicKey) error {
	var document receiptKeyDocument
	if DecodeStrict(data, &document, 16384) != nil || !oneOf(document.Network, Network, "eip155:8453") || len(document.Keys) == 0 || len(document.Keys) > 32 || len(trusted) == 0 {
		return errReceiptKeyPinMismatch
	}
	seen := map[string]bool{}
	matched := false
	for _, key := range document.Keys {
		if !identifier.MatchString(key.ID) || seen[key.ID] || key.Algorithm != "Ed25519" || !hexDigest.MatchString(key.PublicKey) {
			return errReceiptKeyPinMismatch
		}
		seen[key.ID] = true
		if pin, known := trusted[key.ID]; known {
			if len(pin) != ed25519.PublicKeySize || hex.EncodeToString(pin) != key.PublicKey {
				return errReceiptKeyPinMismatch
			}
			matched = true
		}
	}
	if !matched {
		return errReceiptKeyPinMismatch
	}
	return nil
}

// VerifyIdentity checks the public key document against compiled receipt pins.
// It never imports advertised keys. The document currently describes Base
// Sepolia; the same key is independently compiled for Arc/Robinhood testnets.
// Web PKI protects this advisory document; only a signature proves possession.
func (a *API) VerifyIdentity(ctx context.Context) error {
	status, body, _, err := a.request(ctx, "GET", "/v1/keys", nil, nil)
	if err != nil {
		return err
	}
	if status != 200 {
		return errReceiptKeyPinMismatch
	}
	trusted := a.verifier.keys
	if trusted == nil {
		trusted = receiptKeysFor(Network)
	}
	return validateReceiptKeyDocument(body, trusted)
}
