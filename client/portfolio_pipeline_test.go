package client

import (
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

// The Python payment test supplies a receipt recovered from a real disposable DB.
func TestPortfolioPipelineReceipt(t *testing.T) {
	path := os.Getenv("SECONDED_PORTFOLIO_PIPELINE_FIXTURE")
	if path == "" {
		t.Skip("run tests/payments/test_portfolio_pipeline.py for the server-to-client bridge")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var f struct {
		Receipt   SignedReceipt `json:"receipt"`
		Request   Request       `json:"request"`
		Entry     Entry         `json:"entry"`
		Now       int64         `json:"now"`
		PublicKey string        `json:"public_key"`
	}
	if err := json.Unmarshal(data, &f); err != nil {
		t.Fatal(err)
	}
	if ClientVersion != "0.4.2" {
		t.Fatal("stable client required")
	}
	scope, err := portfolioRequestScope(f.Request)
	if err != nil || scope != f.Entry.PortfolioScopeSHA256 || f.Request.Digest() != f.Entry.InputDigest {
		t.Fatal("request binding", err)
	}
	key, err := hex.DecodeString(f.PublicKey)
	if err != nil || len(key) != ed25519.PublicKeySize {
		t.Fatal("test public key")
	}
	v := ReceiptVerifier{map[string]ed25519.PublicKey{f.Receipt.KeyID: key}, currentReceiptLabels()}
	if err := v.Verify(f.Receipt, f.Entry, time.Unix(f.Now, 0)); err != nil {
		t.Fatal("actual recovered Python receipt", err)
	}
	report := f.Receipt.Envelope.Answer.PortfolioReport
	if len(pa(report.fields["positions"])) != 25 || len(pa(po(report.fields["scope"])["networks"])) != 3 {
		t.Fatal("full recorded wallet missing")
	}
	if err := v.VerifyStored(f.Receipt, f.Entry, time.Unix(f.Now+86400, 0)); err != nil {
		t.Fatal("historical recovery", err)
	}
	_, expired := portfolioText(report, f.Now+86400)
	if !expired {
		t.Fatal("history must display expiry")
	}
	bad := f.Entry
	bad.PortfolioScopeSHA256 = strings.Repeat("0", 64)
	if v.Verify(f.Receipt, bad, time.Unix(f.Now, 0)) == nil {
		t.Fatal("wrong scope accepted")
	}
	tampered := f.Receipt
	tampered.Envelope.Billing.Amount = "1"
	if v.Verify(tampered, f.Entry, time.Unix(f.Now, 0)) == nil {
		t.Fatal("unsigned billing tamper accepted")
	}
}
