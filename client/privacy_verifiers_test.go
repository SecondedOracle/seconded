package client

import (
	"bytes"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"sync"
	"time"
)

func (n *privacyNative) validateReceive(plan privacyMap) bool {
	if n.closed {
		return false
	}
	now := n.now()
	p, ok := n.receivePlans[privacyString(plan["plan_hash"])]
	if !ok || now.Before(p.issued) || !now.Before(p.expires) || now.Unix() >= privacyInt(plan["expires_at"]) || privacyDeployment(now) == nil {
		return false
	}
	raw, e := privacyCanonical(plan)
	return e == nil && bytes.Equal(raw, p.raw)
}

func (n *privacyNative) validateSwap(plan, expected privacyMap) bool {
	if n.closed {
		return false
	}
	r := privacySwapRequest(expected)
	if r == nil {
		return false
	}
	p, ok := n.swapPlans[privacyString(plan["plan_hash"])]
	now := n.now()
	if !ok || now.Before(p.issued) || !now.Before(p.expires) {
		return false
	}
	raw, e := privacyCanonical(plan)
	req, _ := privacyCanonical(r)
	return e == nil && bytes.Equal(raw, p.raw) && bytes.Equal(req, p.request)
}

// A trusted local signer can retain this verifier; it is never an MCP argument.
// claim must atomically consume payment IDs and be durable across processes in
// a production signer. No signer or execution path is registered by these tools.
type privacyPurchaseVerifier struct {
	mu                      sync.Mutex
	plan, request, snapshot privacyMap
	issued                  time.Time
	now                     func() time.Time
	claim                   func(string) bool
}

func (v *privacyPurchaseVerifier) reverify(plan, tx privacyMap) (privacyMap, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.plan == nil || v.claim == nil {
		return nil, ErrInvalid
	}
	a, e := privacyCanonical(plan)
	b, _ := privacyCanonical(v.plan)
	c, e2 := privacyCanonical(tx)
	d, _ := privacyCanonical(v.plan["transaction"])
	if e != nil || e2 != nil || !bytes.Equal(a, b) || !bytes.Equal(c, d) || v.plan["status"] != "unsigned_plan" {
		return nil, ErrInvalid
	}
	now := v.now()
	if now.Before(v.issued) || privacyWallSeconds(now) < privacySeconds(v.plan["prepared_at"]) || privacyWallSeconds(now) >= privacySeconds(v.plan["valid_until"]) || privacyPurchaseScreen(v.request, v.snapshot, now)["status"] != "not_listed" || !v.claim(privacyString(v.plan["payment_id"])) {
		return nil, ErrInvalid
	}
	return privacyObject(privacyClone(v.plan["transaction"])), nil
}

// Recipient-owned context and atomic claim are mandatory. The callback may use
// a durable store and must bind the claim to any resulting business effect.
func privacyVerifyPresentation(presentation, keys privacyMap, audience, nonce string, expiry int64, now time.Time, expectedHolder string, claim func(string, string, int64) bool) bool {
	if claim == nil || !privacyFactPlain(presentation) || !privacyExact(presentation, "envelope", "sig") {
		return false
	}
	envelope := privacyObject(presentation["envelope"])
	if !privacyExact(envelope, "type", "attestation", "audience", "nonce", "expiry", "holder_public_key") {
		return false
	}
	fact := privacyVerifyFact(privacyObject(envelope["attestation"]), keys, now)
	if fact == nil || !privacyFactContext(audience, nonce, expiry, now, fact) || envelope["type"] != "seconded-fact-presentation/v1" || envelope["audience"] != audience || envelope["nonce"] != nonce || privacyInt(envelope["expiry"]) != expiry || !privacyMatch(`[0-9a-f]{64}`, envelope["holder_public_key"]) || (expectedHolder != "" && envelope["holder_public_key"] != expectedHolder) {
		return false
	}
	public, _ := hex.DecodeString(privacyString(envelope["holder_public_key"]))
	sig := privacyFactSignature(presentation["sig"])
	raw, e := canonicalValue(envelope)
	if e != nil || len(sig) != 64 || !ed25519.Verify(public, append([]byte(privacyPresentationPrefix), raw...), sig) {
		return false
	}
	return claim(audience, nonce, expiry)
}

func privacySeconds(value any) float64 {
	if n, ok := value.(json.Number); ok {
		f, _ := n.Float64()
		return f
	}
	return float64(privacyInt(value))
}
