package client

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"time"
)

const privacyFactPrefix = "SECONDED-FACT-ATTESTATION/v1\x00"
const privacyPresentationPrefix = "SECONDED-FACT-PRESENTATION/v1\x00"

func privacyExact(m privacyMap, keys ...string) bool {
	if m == nil || len(m) != len(keys) {
		return false
	}
	for _, k := range keys {
		if _, ok := m[k]; !ok {
			return false
		}
	}
	return true
}
func privacyFactPlain(v any) bool {
	nodes := 0
	var valid func(any, int) bool
	valid = func(v any, depth int) bool {
		nodes++
		if nodes > 512 || depth > 8 {
			return false
		}
		switch x := v.(type) {
		case map[string]any:
			if len(x) > 32 {
				return false
			}
			for k, v := range x {
				if len(k) > 64 || !valid(v, depth+1) {
					return false
				}
			}
		case []any:
			if len(x) > 32 {
				return false
			}
			for _, v := range x {
				if !valid(v, depth+1) {
					return false
				}
			}
		case string:
			return len(x) <= 256
		default:
			return privacySchema(privacyMap{"type": "integer", "minimum": 0, "maximum": int64(9007199254740991)}, v, 0)
		}
		return true
	}
	return valid(v, 0)
}
func privacyFactSignature(v any) []byte {
	if !privacyMatch(`[A-Za-z0-9_-]{86}`, v) {
		return nil
	}
	b, e := base64.RawURLEncoding.Strict().DecodeString(privacyString(v))
	if e != nil || len(b) != 64 || base64.RawURLEncoding.EncodeToString(b) != v {
		return nil
	}
	return b
}
func privacyVerifyFact(attestation, keys privacyMap, now time.Time) privacyMap {
	if !privacyFactPlain(attestation) || !privacyFactPlain(keys) || !privacyExact(attestation, "envelope", "sig", "key_id") || !privacyExact(keys, "network", "keys") || !privacyMatch(`eip155:[1-9][0-9]{0,15}`, keys["network"]) || len(privacyArray(keys["keys"])) == 0 {
		return nil
	}
	envelope := privacyObject(attestation["envelope"])
	guide, _ := privacyTool("seconded_private_fact_prove")
	schema := privacyObject(privacyArray(privacyObject(guide.InputSchema)["oneOf"])[0])
	attSchema := privacyObject(schema["properties"])["attestation"]
	if !privacySchema(attSchema, attestation, 0) || envelope["key_id"] != attestation["key_id"] {
		return nil
	}
	issued, e := time.Parse("2006-01-02", privacyString(envelope["issued_day"]))
	expiry, e2 := time.Parse("2006-01-02", privacyString(envelope["expiry"]))
	if e != nil || e2 != nil || expiry.Sub(issued) != 24*time.Hour || now.Before(issued) || !now.Before(expiry) {
		return nil
	}
	seen := map[string]bool{}
	var public []byte
	for _, v := range privacyArray(keys["keys"]) {
		k := privacyObject(v)
		id := privacyString(k["key_id"])
		if !privacyExact(k, "key_id", "algorithm", "public_key_hex") || !privacyMatch(`[a-z][a-z0-9-]{0,63}`, id) || seen[id] || k["algorithm"] != "Ed25519" || !privacyMatch(`[0-9a-f]{64}`, k["public_key_hex"]) {
			return nil
		}
		seen[id] = true
		if id == attestation["key_id"] {
			public, _ = hex.DecodeString(privacyString(k["public_key_hex"]))
		}
	}
	signature := privacyFactSignature(attestation["sig"])
	raw, err := canonicalValue(envelope)
	if len(public) != 32 || len(signature) != 64 || err != nil || !ed25519.Verify(public, append([]byte(privacyFactPrefix), raw...), signature) {
		return nil
	}
	return envelope
}
func privacyFactContext(audience, nonce any, expiry int64, now time.Time, fact privacyMap) bool {
	end, e := time.Parse("2006-01-02", privacyString(fact["expiry"]))
	return e == nil && privacyMatch(`https://[a-z0-9](?:[a-z0-9.-]{0,190}[a-z0-9])?(?::[0-9]{1,5})?`, audience) && privacyMatch(`[0-9a-f]{32,64}`, nonce) && now.Unix() < expiry && expiry <= now.Unix()+600 && expiry <= end.Unix()
}
func (n *privacyNative) fact(input, keys privacyMap, holder ed25519.PrivateKey) privacyMap {
	rejected := privacyMap{"status": "rejected", "reason": "invalid_fact_presentation"}
	if !privacyFactPlain(input) {
		return rejected
	}
	if len(input) == 1 && oneOf(privacyString(input["mode"]), "solvency", "hidden_balance", "balance_gte") {
		return privacyMap{"status": "deferred", "reason": "hidden_balance_and_solvency_not_supported"}
	}
	if keys == nil {
		keys = privacyTrustedDocument("issuer-public-keys.json")
	}
	now := n.now()
	fact := privacyVerifyFact(privacyObject(input["attestation"]), keys, now)
	if fact == nil || !privacyFactContext(input["audience"], input["nonce"], privacyInt(input["expiry"]), now, fact) {
		return rejected
	}
	if holder == nil {
		_, key, e := ed25519.GenerateKey(n.random)
		if e != nil {
			return rejected
		}
		holder = key
		defer clear(key)
	}
	if len(holder) != ed25519.PrivateKeySize {
		return rejected
	}
	envelope := privacyMap{"type": "seconded-fact-presentation/v1", "attestation": privacyClone(input["attestation"]), "audience": input["audience"], "nonce": input["nonce"], "expiry": input["expiry"], "holder_public_key": hex.EncodeToString(holder.Public().(ed25519.PublicKey))}
	raw, e := canonicalValue(envelope)
	if e != nil {
		return rejected
	}
	sig := ed25519.Sign(holder, append([]byte(privacyPresentationPrefix), raw...))
	return privacyMap{"status": "ok", "privacy_notice": privacyDocument("text")["fact_notice"], "presentation": privacyMap{"envelope": envelope, "sig": base64.RawURLEncoding.EncodeToString(sig)}}
}
