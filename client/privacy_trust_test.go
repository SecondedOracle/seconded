//go:build !windows

package client

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestPrivacyNativeTrustDocumentIntegrity(t *testing.T) {
	root := t.TempDir()
	t.Setenv("SECONDED_PRIVACY_TRUST", root)
	path := filepath.Join(root, "issuer-public-keys.json")
	if err := os.WriteFile(path, []byte(`{"network":"eip155:8453","keys":[]}`), 0600); err != nil {
		t.Fatal(err)
	}
	if privacyTrustedDocument("issuer-public-keys.json") == nil {
		t.Fatal("trusted document positive control failed")
	}
	if privacyTrustedDocument("../issuer-public-keys.json") != nil {
		t.Fatal("path traversal")
	}
	if err := os.Chmod(path, 0666); err != nil {
		t.Fatal(err)
	}
	if privacyTrustedDocument("issuer-public-keys.json") != nil {
		t.Fatal("writable trust document")
	}
	if err := os.Chmod(path, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(path, filepath.Join(root, "invoice-alias.json")); err != nil {
		t.Fatal(err)
	}
	if privacyTrustedDocument("invoice-alias.json") != nil {
		t.Fatal("followed trust symlink")
	}
	if err := os.WriteFile(path, []byte(`{"network":"one","network":"two"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if privacyTrustedDocument("issuer-public-keys.json") != nil {
		t.Fatal("accepted duplicate trust fields")
	}
	fifo := filepath.Join(root, "invoice-fifo.json")
	if err := syscall.Mkfifo(fifo, 0600); err != nil {
		t.Fatal(err)
	}
	if privacyTrustedDocument("invoice-fifo.json") != nil {
		t.Fatal("accepted FIFO")
	}
}
