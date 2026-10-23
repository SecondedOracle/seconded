package client

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Self-contained public test fixtures: only ephemeral keys and inert images.
func init() { enrollmentFingerprint = Fingerprint }

type memoryStore struct {
	value string
	err   error
}

func (s *memoryStore) Get(string) (string, error) {
	if s.err != nil {
		return "", s.err
	}
	if s.value == "" {
		return "", ErrNotFound
	}
	return s.value, nil
}
func (s *memoryStore) Set(_ string, v string) error {
	if s.err != nil {
		return s.err
	}
	s.value = v
	return nil
}

type publicReenrollStore map[string]string

func (s publicReenrollStore) Get(k string) (string, error) {
	v, ok := s[k]
	if !ok {
		return "", ErrNotFound
	}
	return v, nil
}
func (s publicReenrollStore) Set(k, v string) error { s[k] = v; return nil }
func selfDigest(t *testing.T, exe string) string {
	t.Helper()
	b, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	return fmt.Sprintf("%x", sha256.Sum256(b))
}

type interruptedEnrollmentStore struct {
	publicReenrollStore
	staged bool
}

func (s *interruptedEnrollmentStore) Set(k, v string) error {
	s.publicReenrollStore[k] = v
	if strings.HasPrefix(k, "vault-enrollment-") {
		s.staged = true
		return errors.New("simulated crash after credential write")
	}
	return nil
}
func signedSetupFixture(t *testing.T) (string, func(string, string)) {
	t.Helper()
	dir := t.TempDir()
	key := filepath.Join(dir, "fixture-key")
	command := exec.Command(systemSSHKeygen(), "-q", "-t", "ed25519", "-N", "", "-f", key)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("fixture signing key: %v %s", err, output)
	}
	public, err := os.ReadFile(key + ".pub")
	if err != nil {
		t.Fatal(err)
	}
	previous := releaseSigner
	releaseSigner = strings.Fields(string(public))[1]
	t.Cleanup(func() { releaseSigner = previous })
	exe := filepath.Join(dir, "renamed-seconded")
	if err := os.WriteFile(exe, []byte("fixture executable bytes"), 0700); err != nil {
		t.Fatal(err)
	}
	asset := "seconded-mcp_" + runtime.GOOS + "_" + runtime.GOARCH
	if runtime.GOOS == "windows" {
		asset += ".exe"
	}
	sign := func(content, namespace string) {
		t.Helper()
		// Sign stdin so repeated negative controls never overwrite a signing input.
		cmd := exec.Command(systemSSHKeygen(), "-Y", "sign", "-f", key, "-n", namespace)
		cmd.Stdin = strings.NewReader(content)
		signature, err := cmd.Output()
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(dir, "SHA-256SUMS"), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(dir, "SHA-256SUMS.sig"), signature, 0600); err != nil {
			t.Fatal(err)
		}
	}
	sign(selfDigest(t, exe)+"  "+asset+"\n", "seconded-release")
	return exe, sign
}
func upgradeFixture(t *testing.T, backend string) (*Files, SecretStore, string, string, Vault) {
	t.Helper()
	oldVersion := Version
	Version = "0.3.0-candidate"
	t.Cleanup(func() { Version = oldVersion })
	exe, sign := signedSetupFixture(t)
	manifest, err := os.ReadFile(filepath.Join(filepath.Dir(exe), "SHA-256SUMS"))
	if err != nil {
		t.Fatal(err)
	}
	sign("# seconded-release-version: 0.3.1-test\n"+string(manifest), "seconded-release")
	oldExe := filepath.Join(t.TempDir(), "previous")
	if err := os.WriteFile(oldExe, []byte("previous enrolled bytes"), 0700); err != nil {
		t.Fatal(err)
	}
	files, err := OpenFiles(filepath.Join(t.TempDir(), "profile"))
	if err != nil {
		t.Fatal(err)
	}
	var store SecretStore = publicReenrollStore{}
	if backend == "file" {
		store = &memoryStore{err: ErrKeystoreUnavailable}
	}
	if _, err = SetupWallet(files, store, false, QuickSettings(), oldExe, selfDigest(t, oldExe)); err != nil {
		t.Fatal(err)
	}
	v, err := LoadVault(activeWalletStore{files, store})
	if err != nil {
		t.Fatal(err)
	}
	Version = "0.3.1-test"
	return files, store, oldExe, exe, v
}
