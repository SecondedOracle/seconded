package client

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

func TestOwnerPresenceRouting(t *testing.T) {
	public, _, _ := ed25519.GenerateKey(rand.Reader)
	for _, tc := range []struct {
		name      string
		result    error
		keyCalled bool
		approved  bool
	}{
		{"biometric approval", nil, false, true},
		{"biometric rejection cannot downgrade", ErrTerminalPolicyRequired, false, false},
		{"unavailable default denies", errPresenceUnavailable, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			called := false
			err := confirmOwnerPresence("exact action", strings.NewReader("public-address\n"), io.Discard, func(string) error { return tc.result }, func() (ed25519.PublicKey, error) { called = true; return public, errPresenceUnavailable })
			if (err == nil) != tc.approved || called != tc.keyCalled {
				t.Fatal(err, called)
			}
		})
	}
}

func TestOfflineOwnerApprovalBindingAndExpiry(t *testing.T) {
	public, private, _ := ed25519.GenerateKey(rand.Reader)
	_, other, _ := ed25519.GenerateKey(rand.Reader)
	var previous []byte
	for _, mode := range []string{"valid", "replay", "different action", "different signer", "public suffix", "expired", "unterminated"} {
		t.Run(mode, func(t *testing.T) {
			in, write := io.Pipe()
			defer in.Close()
			var output bytes.Buffer
			now := time.Now()
			calls := 0
			reader := &approvalReader{Reader: in, before: func() {
				text := output.String()
				parts := strings.Split(text, "Challenge: ")
				if len(parts) != 2 {
					t.Error("challenge missing")
					write.Close()
					return
				}
				raw, err := base64.RawStdEncoding.DecodeString(strings.Split(parts[1], "\n")[0])
				if err != nil {
					t.Error(err)
				}
				var fields map[string]any
				if json.Unmarshal(raw, &fields) != nil || fields["domain"] != "seconded-owner-approval-v1" || fields["reason"] != "Raise exact policy for wallet A" {
					t.Error("missing action binding")
				}
				payload := raw
				signer := private
				switch mode {
				case "replay":
					payload = previous
				case "different action":
					payload = bytes.ReplaceAll(raw, []byte("wallet A"), []byte("wallet B"))
				case "different signer":
					signer = other
				}
				answer := base64.RawStdEncoding.EncodeToString(ed25519.Sign(signer, payload)) + "\n"
				if mode == "public suffix" {
					answer = "abcdef\n"
				}
				if mode == "unterminated" {
					answer = strings.TrimSuffix(answer, "\n")
				}
				if mode == "valid" {
					previous = append([]byte(nil), raw...)
				}
				go func() { io.WriteString(write, answer); write.Close() }()
			}}
			err := signedOwnerApproval("Raise exact policy for wallet A", reader, &output, public, func() time.Time {
				calls++
				if mode == "expired" && calls > 1 {
					return now.Add(3 * time.Minute)
				}
				return now
			})
			if (err == nil) != (mode == "valid") {
				t.Fatal(mode, err)
			}
		})
	}
}

type approvalReader struct {
	io.Reader
	before func()
}

func (r *approvalReader) Read(p []byte) (int, error) {
	if r.before != nil {
		f := r.before
		r.before = nil
		f()
	}
	return r.Reader.Read(p)
}

func TestPresenceErrorIsPolicyRefusal(t *testing.T) {
	if !errors.Is(confirmOwnerPresence("test", strings.NewReader(""), io.Discard, func(string) error { return errors.New("cancelled") }, func() (ed25519.PublicKey, error) { t.Fatal("downgraded"); return nil, nil }), ErrTerminalPolicyRequired) {
		t.Fatal("wrong error")
	}
}
