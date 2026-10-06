// Package verifier checks SECONDED receipts offline.
//
// A receipt is {"envelope": {...}, "sig": "<base64url, no padding>", "key_id": "rk-..."}.
// The signature is Ed25519 over a version prefix ("SECONDED-RECEIPT/v1\x00", v2, v3)
// followed by the RFC 8785 canonical form of the envelope. This package reproduces
// exactly that check, plus the structural facts a reader needs to interpret the
// receipt: which state it is in, what was charged, which labs checked it, and which
// key signed it.
//
// It deliberately does less than the client. The client additionally binds a
// receipt to the purchase it made (check id, request commitment, payer, amount),
// validates product-specific answers against the catalog, and tracks receipt
// sequence across recovery. Those checks need the private purchase ledger; a
// third party cannot run them, so this verifier does not pretend to.
package verifier

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"seconded.local/verifier/internal/jsoncanonicalizer"
)

// MaxReceiptBytes bounds the input; the client uses the same response limit.
const MaxReceiptBytes = 1 << 20

// Billing is the signed billing block.
type Billing struct {
	Mode       string  `json:"mode"`
	Charged    string  `json:"charged"`
	Network    string  `json:"network"`
	Asset      string  `json:"asset"`
	Amount     *string `json:"amount_atomic"`
	Payer      *string `json:"payer"`
	PayTo      string  `json:"pay_to"`
	Tx         *string `json:"tx"`
	Settlement string  `json:"settlement"`
	Refund     *string `json:"refund"`
}

// Envelope holds the signed fields this verifier reads. Unknown fields are
// allowed: the signature covers the whole envelope regardless of what this
// struct knows about, and a newer server may add fields.
type Envelope struct {
	Version     int      `json:"v"`
	Kind        string   `json:"kind"`
	KeyID       string   `json:"key_id"`
	CheckID     *string  `json:"check_id"`
	Seq         int64    `json:"seq"`
	IssuedAt    string   `json:"issued_at"`
	OutcomeAt   *string  `json:"outcome_at"`
	Product     *string  `json:"product"`
	Tier        *string  `json:"tier"`
	ModelPairID *string  `json:"model_pair_id"`
	CheckedBy   []string `json:"checked_by"`
	State       string   `json:"state"`
	Outcome     *string  `json:"outcome"`
	Status      string   `json:"status"`
	Message     string   `json:"message"`
	Reason      string   `json:"reason"`
	Billing     Billing  `json:"billing"`
	Answer      *struct {
		Option  int    `json:"option"`
		LabelID string `json:"label_id"`
	} `json:"answer"`
	Verification *struct {
		Schema         string `json:"schema"`
		EvidenceSHA256 string `json:"evidence_sha256"`
	} `json:"verification"`
}

// Report is what Verify returns for a receipt whose signature checked out.
type Report struct {
	KeyID     string
	Version   int
	Envelope  Envelope
	Canonical []byte // the exact bytes the signature covers, without the prefix
}

// States lists every receipt state the client knows how to interpret.
var States = map[string]string{
	"released":         "agreed answer; settlement pending or certified nonpayment",
	"included":         "agreed answer; payment included in a block",
	"final":            "agreed answer; payment final",
	"no_agreement":     "NOT VERIFIED: no usable agreement; nothing charged",
	"trial_delivered":  "agreed answer on a free trial; nothing charged",
	"refund_owed":      "a refund is owed",
	"refunded":         "refunded",
	"content_refused":  "the models refused the content; nothing charged",
	"service_failed":   "the service failed; nothing charged",
	"closed_no_charge": "authorization cancelled or expired; certified nonpayment",
	"frozen_unsettled": "frozen before settlement; nothing charged",
	"refused":          "the request was refused before admission; nothing charged",
	"running":          "in progress",
	"settling":         "settling",
	"delayed":          "delayed",
	"unavailable":      "temporarily unavailable",
}

// Labs lists the model providers a receipt may name in checked_by.
var Labs = map[string]bool{"OpenAI": true, "Anthropic": true}

var (
	ErrInput     = errors.New("input is not a receipt")
	ErrKey       = errors.New("receipt key is not pinned")
	ErrSignature = errors.New("signature does not verify")
	ErrEnvelope  = errors.New("envelope is malformed")
)

// Verify checks the receipt in data against keys. It returns a Report when the
// Ed25519 signature verifies under the pinned key the receipt names and the
// envelope passes the structural checks described in the package comment.
func Verify(data []byte, keys map[string]ed25519.PublicKey) (*Report, error) {
	if len(data) > MaxReceiptBytes {
		return nil, fmt.Errorf("%w: larger than %d bytes", ErrInput, MaxReceiptBytes)
	}
	var wire struct {
		Envelope  json.RawMessage `json:"envelope"`
		Signature string          `json:"sig"`
		KeyID     string          `json:"key_id"`
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&wire); err != nil {
		// Also accept a check reply, which carries the receipt under "receipt".
		var reply struct {
			Receipt json.RawMessage `json:"receipt"`
		}
		if json.Unmarshal(data, &reply) != nil || len(reply.Receipt) == 0 {
			return nil, fmt.Errorf("%w: expected {envelope, sig, key_id}", ErrInput)
		}
		return Verify(reply.Receipt, keys)
	}
	if len(bytes.TrimSpace(wire.Envelope)) == 0 || bytes.TrimSpace(wire.Envelope)[0] != '{' {
		return nil, fmt.Errorf("%w: envelope must be a JSON object", ErrInput)
	}
	key, ok := keys[wire.KeyID]
	if !ok || len(key) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("%w: key_id %q", ErrKey, wire.KeyID)
	}
	signature, err := base64.RawURLEncoding.DecodeString(wire.Signature)
	if err != nil || len(signature) != ed25519.SignatureSize {
		return nil, fmt.Errorf("%w: sig is not a base64url Ed25519 signature", ErrSignature)
	}
	var envelope Envelope
	if err := json.Unmarshal(wire.Envelope, &envelope); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrEnvelope, err)
	}
	if envelope.Version < 1 || envelope.Version > 3 {
		return nil, fmt.Errorf("%w: unsupported receipt version %d", ErrEnvelope, envelope.Version)
	}
	if envelope.Kind != "seconded-receipt" {
		return nil, fmt.Errorf("%w: kind %q", ErrEnvelope, envelope.Kind)
	}
	if envelope.KeyID != wire.KeyID {
		return nil, fmt.Errorf("%w: envelope key_id %q differs from outer key_id %q", ErrEnvelope, envelope.KeyID, wire.KeyID)
	}
	canonical, err := Canonical(wire.Envelope)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrEnvelope, err)
	}
	message := append([]byte(fmt.Sprintf("SECONDED-RECEIPT/v%d\x00", envelope.Version)), canonical...)
	if !ed25519.Verify(key, message, signature) {
		return nil, ErrSignature
	}
	// Structural facts, checked only after the signature so an attacker cannot
	// learn anything from error ordering that the signature would not reveal.
	if _, known := States[envelope.State]; !known {
		return nil, fmt.Errorf("%w: unknown state %q", ErrEnvelope, envelope.State)
	}
	issued, err := time.Parse(time.RFC3339Nano, envelope.IssuedAt)
	if err != nil {
		return nil, fmt.Errorf("%w: issued_at %q", ErrEnvelope, envelope.IssuedAt)
	}
	if envelope.OutcomeAt != nil {
		outcome, err := time.Parse(time.RFC3339Nano, *envelope.OutcomeAt)
		if err != nil || outcome.After(issued) {
			return nil, fmt.Errorf("%w: outcome_at must parse and not follow issued_at", ErrEnvelope)
		}
	}
	if (envelope.Version == 3) != (envelope.Verification != nil) {
		return nil, fmt.Errorf("%w: a verification block is present exactly on v3 receipts", ErrEnvelope)
	}
	seen := map[string]bool{}
	for _, lab := range envelope.CheckedBy {
		if !Labs[lab] || seen[lab] {
			return nil, fmt.Errorf("%w: checked_by names %q", ErrEnvelope, lab)
		}
		seen[lab] = true
	}
	return &Report{KeyID: wire.KeyID, Version: envelope.Version, Envelope: envelope, Canonical: canonical}, nil
}

// Canonical returns the RFC 8785 canonical form of one JSON value. It wraps the
// value in an array before transforming and unwraps it afterwards, exactly as the
// client does, so a top-level scalar canonicalizes the same way in both.
func Canonical(data []byte) ([]byte, error) {
	wrapped := make([]byte, 0, len(data)+2)
	wrapped = append(wrapped, '[')
	wrapped = append(wrapped, data...)
	wrapped = append(wrapped, ']')
	out, err := jsoncanonicalizer.Transform(wrapped)
	if err != nil {
		return nil, err
	}
	if len(out) < 2 || out[0] != '[' || out[len(out)-1] != ']' {
		return nil, errors.New("canonicalizer returned an unexpected shape")
	}
	return out[1 : len(out)-1], nil
}
