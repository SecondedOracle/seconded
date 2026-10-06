package verifier

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
)

const fixture = "testdata/refused-base-mainnet-2026-10-06.json"

func TestProductionFixtureVerifiesUnderCompiledPin(t *testing.T) {
	data, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatal(err)
	}
	report, err := Verify(data, ReleaseKeys())
	if err != nil {
		t.Fatalf("production-signed fixture rejected: %v", err)
	}
	e := report.Envelope
	if report.KeyID != "rk-2026-09-a" || report.Version != 1 || e.State != "refused" || e.Billing.Charged != "no" || e.Billing.Network != "eip155:8453" {
		t.Fatalf("unexpected report: %+v", e)
	}
	if e.Billing.PayTo != "0x010ab46d566cde25cca0ee55eb105e781c7bcf3a" {
		t.Fatalf("pay_to %q", e.Billing.PayTo)
	}
	if len(report.Canonical) == 0 || report.Canonical[0] != '{' {
		t.Fatal("canonical bytes missing")
	}
}

func TestProductionFixtureFailsUnderOtherKeys(t *testing.T) {
	data, _ := os.ReadFile(fixture)
	other, _, _ := ed25519.GenerateKey(rand.Reader)
	sameID := map[string]ed25519.PublicKey{"rk-2026-09-a": other}
	if _, err := Verify(data, sameID); !errors.Is(err, ErrSignature) {
		t.Fatalf("want ErrSignature under a different key with the same id, got %v", err)
	}
	unknownID := map[string]ed25519.PublicKey{"rk-other": ReleaseKeys()["rk-2026-09-a"]}
	if _, err := Verify(data, unknownID); !errors.Is(err, ErrKey) {
		t.Fatalf("want ErrKey when the receipt's key id is not pinned, got %v", err)
	}
}

func TestProductionFixtureTamperIsDetected(t *testing.T) {
	data, _ := os.ReadFile(fixture)
	text := string(data)
	if !strings.Contains(text, `"charged": "no"`) {
		t.Fatal("fixture shape changed; update the tamper test")
	}
	tampered := strings.Replace(text, `"charged": "no"`, `"charged": "yes"`, 1)
	if _, err := Verify([]byte(tampered), ReleaseKeys()); !errors.Is(err, ErrSignature) {
		t.Fatalf("tampered billing accepted: %v", err)
	}
	// Reformatting without changing content must still verify: the signature is over the canonical form.
	var loose map[string]any
	if err := json.Unmarshal(data, &loose); err != nil {
		t.Fatal(err)
	}
	reformatted, _ := json.MarshalIndent(loose, "", "      ")
	if _, err := Verify(reformatted, ReleaseKeys()); err != nil {
		t.Fatalf("re-serialized fixture rejected: %v", err)
	}
}

func testKeys(t *testing.T) (ed25519.PrivateKey, map[string]ed25519.PublicKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return priv, map[string]ed25519.PublicKey{"rk-test": pub}
}

func envelope(version int) map[string]any {
	e := map[string]any{
		"v": version, "kind": "seconded-receipt", "key_id": "rk-test", "check_id": "0123abcd", "seq": 1,
		"issued_at": "2026-10-06T00:00:10Z", "outcome_at": "2026-10-06T00:00:05Z",
		"product": "scam_check", "tier": "small", "model_pair_id": "pair-a",
		"checked_by": []string{"OpenAI", "Anthropic"}, "state": "final", "outcome": "agreed",
		"answer": map[string]any{"option": 1, "label_id": "benign"},
		"billing": map[string]any{"mode": "paid", "charged": "yes", "network": "eip155:8453",
			"asset": "eip155:8453/erc20:0x833589fcd6edb6e08f4c7c32d4f71b54bda02913", "amount_atomic": "100000",
			"payer": "0x1111111111111111111111111111111111111111", "pay_to": "0x010ab46d566cde25cca0ee55eb105e781c7bcf3a",
			"tx": "0x" + strings.Repeat("ab", 32), "settlement": "final", "refund": nil},
	}
	if version == 3 {
		e["verification"] = map[string]any{"schema": "seconded-verification/v1", "evidence_sha256": strings.Repeat("0", 64), "block": nil, "findings": []any{}, "coverage": []any{map[string]any{"fact": "message", "status": "checked", "reason": nil}}}
	}
	return e
}

func sign(t *testing.T, priv ed25519.PrivateKey, signAs int, e map[string]any, keyID string) []byte {
	t.Helper()
	raw, err := json.Marshal(e)
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := Canonical(raw)
	if err != nil {
		t.Fatal(err)
	}
	sig := ed25519.Sign(priv, append([]byte(fmt.Sprintf("SECONDED-RECEIPT/v%d\x00", signAs)), canonical...))
	wire, _ := json.Marshal(map[string]any{"envelope": json.RawMessage(raw), "sig": base64.RawURLEncoding.EncodeToString(sig), "key_id": keyID})
	return wire
}

func TestSyntheticReceiptsRoundTrip(t *testing.T) {
	priv, keys := testKeys(t)
	for _, version := range []int{1, 2, 3} {
		report, err := Verify(sign(t, priv, version, envelope(version), "rk-test"), keys)
		if err != nil {
			t.Fatalf("v%d: %v", version, err)
		}
		if report.Version != version || report.Envelope.Answer == nil || report.Envelope.Answer.LabelID != "benign" {
			t.Fatalf("v%d: unexpected report", version)
		}
	}
}

func TestSyntheticNegativePaths(t *testing.T) {
	priv, keys := testKeys(t)
	cases := []struct {
		name string
		wire []byte
		want error
	}{
		{"signed under the wrong version prefix", sign(t, priv, 2, envelope(1), "rk-test"), ErrSignature},
		{"outer key id differs from envelope key id", sign(t, priv, 1, envelope(1), "rk-other"), ErrKey},
		{"unknown state", sign(t, priv, 1, with(envelope(1), "state", "paid_in_full"), "rk-test"), ErrEnvelope},
		{"duplicate lab", sign(t, priv, 1, with(envelope(1), "checked_by", []string{"OpenAI", "OpenAI"}), "rk-test"), ErrEnvelope},
		{"unknown lab", sign(t, priv, 1, with(envelope(1), "checked_by", []string{"OpenAI", "Example"}), "rk-test"), ErrEnvelope},
		{"outcome after issuance", sign(t, priv, 1, with(envelope(1), "outcome_at", "2026-10-06T00:00:11Z"), "rk-test"), ErrEnvelope},
		{"verification block on v1", sign(t, priv, 1, with(envelope(1), "verification", envelope(3)["verification"]), "rk-test"), ErrEnvelope},
		{"v3 without verification block", sign(t, priv, 3, with(envelope(3), "verification", nil), "rk-test"), ErrEnvelope},
		{"wrong kind", sign(t, priv, 1, with(envelope(1), "kind", "seconded-quote"), "rk-test"), ErrEnvelope},
		{"version 4", sign(t, priv, 4, with(envelope(1), "v", 4), "rk-test"), ErrEnvelope},
		{"not a receipt", []byte(`{"hello":"world"}`), ErrInput},
		{"envelope not an object", []byte(`{"envelope":[1],"sig":"AA","key_id":"rk-test"}`), ErrInput},
	}
	for _, c := range cases {
		if _, err := Verify(c.wire, keys); !errors.Is(err, c.want) {
			t.Errorf("%s: want %v, got %v", c.name, c.want, err)
		}
	}
	// Amount tampering after signing.
	wire := sign(t, priv, 1, envelope(1), "rk-test")
	tampered := strings.Replace(string(wire), `"amount_atomic":"100000"`, `"amount_atomic":"100001"`, 1)
	if tampered == string(wire) {
		t.Fatal("tamper replacement did not apply")
	}
	if _, err := Verify([]byte(tampered), keys); !errors.Is(err, ErrSignature) {
		t.Fatalf("tampered amount accepted: %v", err)
	}
}

func with(e map[string]any, key string, value any) map[string]any {
	e[key] = value
	return e
}

func TestReplyWrapperIsAccepted(t *testing.T) {
	priv, keys := testKeys(t)
	receipt := sign(t, priv, 1, envelope(1), "rk-test")
	reply := []byte(`{"status":"","hints":{"poll_after_s":null},"receipt":` + string(receipt) + `}`)
	if _, err := Verify(reply, keys); err != nil {
		t.Fatal(err)
	}
}

func TestReceiptParserRejectsAmbiguousInput(t *testing.T) {
	priv, keys := testKeys(t)
	direct := string(sign(t, priv, 1, envelope(1), "rk-test"))
	reply := `{"status":"pending","receipt":` + direct + `}`
	for _, valid := range []string{direct, " \n" + direct + "\t", reply} {
		if _, err := Verify([]byte(valid), keys); err != nil {
			t.Fatalf("valid control rejected: %v", err)
		}
	}
	cases := map[string]string{
		"trailing object":             direct + ` {"unsigned":true}`,
		"trailing scalar":             direct + ` null`,
		"trailing garbage":            direct + ` broken`,
		"duplicate direct key":        `{"key_id":"rk-test",` + direct[1:],
		"escaped duplicate key":       `{"key_\u0069d":"rk-test",` + direct[1:],
		"duplicate signed key":        strings.Replace(direct, `"state":"final"`, `"state":"final","state":"final"`, 1),
		"duplicate wrapper receipt":   `{"receipt":` + direct + `,"receipt":` + direct + `}`,
		"duplicate unsigned metadata": `{"hints":{"x":1,"x":2},"receipt":` + direct + `}`,
		"trailing wrapper value":      reply + ` []`,
		"mixed shapes":                `{"envelope":{},"receipt":` + direct + `}`,
		"nested wrapper":              `{"receipt":` + reply + `}`,
		"null receipt":                `{"receipt":null}`,
		"unknown direct field":        `{"extra":true,` + direct[1:],
		"incorrect field case":        strings.Replace(direct, `"key_id":"rk-test","sig"`, `"KEY_ID":"rk-test","sig"`, 1),
		"malformed wrapper":           `{"receipt":` + direct + `,}`,
	}
	for name, wire := range cases {
		t.Run(name, func(t *testing.T) {
			if wire == direct || wire == reply {
				t.Fatal("negative control did not modify the input")
			}
			if _, err := Verify([]byte(wire), keys); !errors.Is(err, ErrInput) {
				t.Fatalf("want ErrInput, got %v", err)
			}
		})
	}
}

func TestPublicPaidAndPendingReceipts(t *testing.T) {
	cases := []struct {
		file, state, charged string
	}{
		{"testdata/trade-paid-base-mainnet-2026-09-29.json", "included", "yes"},
		{"testdata/token-not-verified-base-sepolia-2026-09-29.json", "no_agreement", "pending"},
	}
	for _, c := range cases {
		data, err := os.ReadFile(c.file)
		if err != nil {
			t.Fatal(err)
		}
		r, err := Verify(data, ReleaseKeys())
		if err != nil {
			t.Fatal(err)
		}
		if r.Envelope.State != c.state || r.Envelope.Billing.Charged != c.charged || r.Version != 3 {
			t.Fatalf("unexpected public receipt: %+v", r.Envelope)
		}
		if c.charged == "pending" && (strings.Contains(States[c.state], "nothing charged") || r.Envelope.Billing.Tx != nil) {
			t.Fatal("pending payment misreported as nonpayment")
		}
		// Authentication is still required for both public examples.
		tampered := strings.Replace(string(data), `"amount_atomic": "250000"`, `"amount_atomic": "250001"`, 1)
		if tampered == string(data) {
			t.Fatal("tamper control did not modify the input")
		}
		if _, err := Verify([]byte(tampered), ReleaseKeys()); !errors.Is(err, ErrSignature) {
			t.Fatalf("tampered public receipt accepted: %v", err)
		}
	}
}

func TestKeysFromDocument(t *testing.T) {
	doc := []byte(`{"network":"eip155:8453","keys":[{"key_id":"rk-2026-09-a","algorithm":"Ed25519","public_key_hex":"` + ReleaseKeysHex["rk-2026-09-a"] + `"}]}`)
	keys, err := KeysFromDocument(doc)
	if err != nil {
		t.Fatal(err)
	}
	if hex.EncodeToString(keys["rk-2026-09-a"]) != ReleaseKeysHex["rk-2026-09-a"] {
		t.Fatal("document key differs from pin")
	}
	data, _ := os.ReadFile(fixture)
	if _, err := Verify(data, keys); err != nil {
		t.Fatalf("fixture rejected under the document keys: %v", err)
	}
	bad := []byte(`{"network":"eip155:8453","keys":[{"key_id":"rk-x","algorithm":"ECDSA","public_key_hex":"` + strings.Repeat("0", 64) + `"}]}`)
	if _, err := KeysFromDocument(bad); err == nil {
		t.Fatal("non-Ed25519 key accepted")
	}
}

func TestCanonicalMatchesRFC8785(t *testing.T) {
	got, err := Canonical([]byte(` {"b" : 1, "a": [1E21, 0.1, "xé", true, null], "€": {"z":2,"y":3}} `))
	if err != nil {
		t.Fatal(err)
	}
	want := `{"a":[1e+21,0.1,"xé",true,null],"b":1,"€":{"y":3,"z":2}}`
	if string(got) != want {
		t.Fatalf("canonical form\n got %s\nwant %s", got, want)
	}
	if _, err := Canonical([]byte(`{"a":1,}`)); err == nil {
		t.Fatal("invalid JSON accepted")
	}
}
