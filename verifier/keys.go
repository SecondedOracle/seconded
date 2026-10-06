package verifier

import (
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
)

// ReleaseKeysHex pins the Ed25519 receipt public keys the shipped client trusts.
// It must stay identical to releaseReceiptKeys and mainnetReceiptKeys in
// client/receipt.go. A key rotation is a client release that carries both the
// old and the new pin; nothing fetched over the network can add to this map.
var ReleaseKeysHex = map[string]string{
	"rk-2026-09-a": "438d9301c477c27fecc3f56da0a7d5a6b3894cef69815bae63b5349dc2cddf0b",
}

// ReleaseKeys returns the compiled pins as usable public keys.
func ReleaseKeys() map[string]ed25519.PublicKey {
	keys, err := KeysFromHex(ReleaseKeysHex)
	if err != nil {
		panic("compiled receipt key pin is malformed: " + err.Error())
	}
	return keys
}

var keyID = regexp.MustCompile(`^[a-z][a-z0-9-]{0,63}$`)
var hex64 = regexp.MustCompile(`^[0-9a-f]{64}$`)

// KeysFromHex turns key_id -> lowercase hex public key into a key set.
func KeysFromHex(hexKeys map[string]string) (map[string]ed25519.PublicKey, error) {
	keys := make(map[string]ed25519.PublicKey, len(hexKeys))
	for id, value := range hexKeys {
		if !keyID.MatchString(id) {
			return nil, fmt.Errorf("key id %q: invalid", id)
		}
		if !hex64.MatchString(value) {
			return nil, fmt.Errorf("key %q: public key must be 64 lowercase hex characters", id)
		}
		raw, err := hex.DecodeString(value)
		if err != nil || len(raw) != ed25519.PublicKeySize {
			return nil, fmt.Errorf("key %q: not a 32-byte Ed25519 public key", id)
		}
		keys[id] = ed25519.PublicKey(raw)
	}
	if len(keys) == 0 {
		return nil, errors.New("no keys")
	}
	return keys, nil
}

// KeyDocument is the shape of GET /v1/keys on the public API. Passing that
// document to the verifier replaces the compiled pins for that run; it does not
// extend them. The client itself never trusts this document for verification.
type KeyDocument struct {
	Network string `json:"network"`
	Keys    []struct {
		ID        string `json:"key_id"`
		Algorithm string `json:"algorithm"`
		PublicKey string `json:"public_key_hex"`
	} `json:"keys"`
}

// KeysFromDocument parses a /v1/keys document into a key set.
func KeysFromDocument(data []byte) (map[string]ed25519.PublicKey, error) {
	var document KeyDocument
	if err := json.Unmarshal(data, &document); err != nil {
		return nil, fmt.Errorf("key document: %w", err)
	}
	hexKeys := map[string]string{}
	for _, key := range document.Keys {
		if key.Algorithm != "Ed25519" {
			return nil, fmt.Errorf("key %q: algorithm %q is not Ed25519", key.ID, key.Algorithm)
		}
		if _, dup := hexKeys[key.ID]; dup {
			return nil, fmt.Errorf("key %q: listed twice", key.ID)
		}
		hexKeys[key.ID] = key.PublicKey
	}
	return KeysFromHex(hexKeys)
}
