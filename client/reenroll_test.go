package client

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func reenrollFixture(t *testing.T, backend string) (*Files, SecretStore, string, string, Vault) {
	t.Helper()
	allowReenrollmentForTest(t)
	files, store, oldExe, exe, before := upgradeFixture(t, backend)
	Version = "0.4.5"
	i, err := readInstallation(files)
	if err != nil {
		t.Fatal(err)
	}
	i.Version, before.Version, i.EarlierCheckPending = Version, Version, false
	selected, err := selectedEnrollmentStore(files, store, i)
	if err != nil {
		t.Fatal(err)
	}
	if err := SaveVault(selected, before); err != nil {
		t.Fatal(err)
	}
	if err := saveInstallation(files, i); err != nil {
		t.Fatal(err)
	}
	return files, store, oldExe, exe, before
}

func TestReenrollSameVersionBothBackends(t *testing.T) {
	for _, backend := range []string{"file", "os_keystore"} {
		t.Run(backend, func(t *testing.T) {
			files, store, _, exe, before := reenrollFixture(t, backend)
			if err := CheckFingerprint(files, store, exe); !errors.Is(err, errFingerprintMismatch) {
				t.Fatal("same-version negative control", err)
			}
			ledger, _ := files.Read("ledger.json")
			// Capture the only migration log as well as both standard streams.
			stdout, stderr := os.Stdout, os.Stderr
			r, w, err := os.Pipe()
			if err != nil {
				t.Fatal(err)
			}
			os.Stdout, os.Stderr = w, w
			i, err := ReenrollWallet(files.Dir, store, exe, selfDigest(t, exe))
			os.Stdout, os.Stderr = stdout, stderr
			w.Close()
			output, _ := io.ReadAll(r)
			r.Close()
			if bytes.Contains(output, []byte(before.Key)) || !bytes.Contains(append([]byte(before.Key), output...), []byte(before.Key)) {
				t.Fatal("key disclosure detector failed")
			}
			if err != nil || i.Backend != backend || i.PreviousFingerprint != before.Fingerprint || i.Fingerprint != selfDigest(t, exe) || i.EnrollmentSlot == "" {
				t.Fatal("re-enroll failed", err)
			}
			v, err := LoadVault(activeWalletStore{files, store})
			if err != nil || v.Key != before.Key || v.Address != before.Address || !reflect.DeepEqual(v.Settings, before.Settings) || v.PreviousFingerprint != before.Fingerprint || v.Version != "0.4.5" {
				t.Fatal("wallet, policy or fingerprint changed incorrectly", err)
			}
			after, _ := files.Read("ledger.json")
			if !bytes.Equal(ledger, after) {
				t.Fatal("payment records changed")
			}
			if err := CheckFingerprint(files, store, exe); err != nil {
				t.Fatal("reopen", err)
			}
			again, err := ReenrollWallet(files.Dir, store, exe, selfDigest(t, exe))
			if err != nil || !reflect.DeepEqual(i, again) {
				t.Fatal("idempotent reenroll lost rollback metadata", err)
			}
		})
	}
}

func TestReenrollRefusalsPreserveWallet(t *testing.T) {
	for _, backend := range []string{"file", "os_keystore"} {
		for _, mode := range []string{"wrong-hash", "missing-hash", "downgrade", "invalid-version", "address", "fingerprint", "version", "missing-wallet", "unavailable-store", "wrong-running-image"} {
			t.Run(backend+"/"+mode, func(t *testing.T) {
				files, store, _, exe, before := reenrollFixture(t, backend)
				i, _ := readInstallation(files)
				digest := selfDigest(t, exe)
				want := "local_storage_unavailable"
				switch mode {
				case "wrong-hash":
					digest, want = strings.Repeat("0", 64), "release_digest_mismatch"
				case "missing-hash":
					digest, want = "", "release_digest_required"
				case "downgrade":
					Version, want = "0.4.4", "reenroll_downgrade_refused"
				case "invalid-version":
					Version, want = "unknown", "reenroll_downgrade_refused"
				case "address":
					i.Address = "0x" + strings.Repeat("1", 40)
				case "fingerprint":
					i.Fingerprint = strings.Repeat("1", 64)
				case "version":
					i.Version = "0.4.4"
				case "missing-wallet":
					// A valid but absent slot must never fall back to retained keys.
					i.EnrollmentSlot, want = strings.Repeat("1", 64), "reenroll_wallet_missing"
				case "unavailable-store":
					if backend == "file" {
						return // OS availability cannot affect a selected file backend.
					}
					store, want = &memoryStore{err: ErrKeystoreUnavailable}, ErrKeystoreUnavailable.Error()
				case "wrong-running-image":
					previous := enrollmentFingerprint
					enrollmentFingerprint = func(string) (string, error) { return strings.Repeat("2", 64), nil }
					t.Cleanup(func() { enrollmentFingerprint = previous })
					want = "release_digest_mismatch"
				}
				if err := saveInstallation(files, i); err != nil {
					t.Fatal(err)
				}
				install, _ := files.Read("install.json")
				_, err := ReenrollWallet(files.Dir, store, exe, digest)
				if err == nil || (want == ErrStorage.Error() && !errors.Is(err, ErrStorage)) || (want != ErrStorage.Error() && err.Error() != want) {
					t.Fatal("expected refusal", want, err)
				}
				after, _ := files.Read("install.json")
				if !bytes.Equal(install, after) {
					t.Fatal("refusal changed installation")
				}
				// Validate the retained wallet, without emitting its contents.
				if backend == "file" {
					v, err := LoadVault(FileStore{files})
					if err != nil || !reflect.DeepEqual(before, v) {
						t.Fatal("refusal changed original wallet", err)
					}
				}
			})
		}
	}
	profile := filepath.Join(t.TempDir(), "missing")
	if _, err := ReenrollWallet(profile, publicReenrollStore{}, "unused", strings.Repeat("0", 64)); err == nil || err.Error() != "reenroll_wallet_missing" {
		t.Fatal("missing profile was accepted", err)
	}
	if _, err := os.Stat(profile); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("re-enrollment created a profile")
	}
}

func TestRollbackAfterReenroll(t *testing.T) {
	allowReenrollmentForTest(t)
	for _, backend := range []string{"file", "os_keystore"} {
		t.Run(backend, func(t *testing.T) {
			files, store, _, signedOlder, before := upgradeFixture(t, backend)
			if err := CheckFingerprint(files, store, signedOlder); err != nil {
				t.Fatal(err)
			}
			Version = "0.4.5"
			rebuilt := filepath.Join(t.TempDir(), "rebuilt")
			if err := os.WriteFile(rebuilt, []byte("independently trusted rebuild"), 0700); err != nil {
				t.Fatal(err)
			}
			if _, err := ReenrollWallet(files.Dir, store, rebuilt, selfDigest(t, rebuilt)); err != nil {
				t.Fatal(err)
			}
			Version = "0.3.1-test"
			if _, err := ReenrollWallet(files.Dir, store, signedOlder, selfDigest(t, signedOlder)); err == nil || err.Error() != "reenroll_downgrade_refused" {
				t.Fatal("manual hash bypassed rollback authorization", err)
			}
			unlock, err := files.Lock()
			if err != nil {
				t.Fatal(err)
			}
			defer unlock()
			// The existing terminal presence gate is covered by the PTY suite.
			if err := checkEnrollmentLocked(files, store, signedOlder, true); err != nil {
				t.Fatal("authorized signed rollback after reenroll", err)
			}
			v, err := LoadVault(activeWalletStore{files, store})
			if err != nil || v.Key != before.Key || v.Fingerprint != selfDigest(t, signedOlder) || v.PreviousFingerprint != selfDigest(t, rebuilt) {
				t.Fatal("rollback lost wallet", err)
			}
		})
	}
}

func TestFingerprintMismatchMCPGuidance(t *testing.T) {
	files, store, _, exe, v := reenrollFixture(t, "file")
	err := CheckFingerprint(files, store, exe)
	command := "setup --reenroll --release-sha256 <published hash>"
	if !errors.Is(err, errFingerprintMismatch) || !strings.Contains(FingerprintMismatchMessage(err), command) {
		t.Fatal("missing human recovery guidance", err)
	}
	if got := toolFailure(nil, err); got["reason"] != "fingerprint_mismatch" || !strings.Contains(got["message"].(string), command) {
		t.Fatal("MCP failure lost guidance")
	}
	input := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"test","version":"1"}}}` + "\n" + `{"jsonrpc":"2.0","method":"notifications/initialized"}` + "\n" + `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"seconded_wallet","arguments":{}}}` + "\n"
	var output bytes.Buffer
	if e := ServeWithSetup(context.Background(), strings.NewReader(input), &output, nil, func() (*Engine, Installation, error) { return nil, Installation{}, err }); e != nil {
		t.Fatal(e)
	}
	if !strings.Contains(output.String(), "fingerprint_mismatch") || !strings.Contains(output.String(), "human_verification_required") || strings.Contains(output.String(), selfDigest(t, exe)) || strings.Contains(output.String(), files.Dir) || strings.Contains(output.String(), v.Key) {
		t.Fatal("MCP setup failed to explain mismatch safely")
	}
}

func TestReenrollCrashBeforeCommit(t *testing.T) {
	files, original, oldExe, exe, before := reenrollFixture(t, "os_keystore")
	store := &interruptedEnrollmentStore{publicReenrollStore: original.(publicReenrollStore)}
	if _, err := ReenrollWallet(files.Dir, store, exe, selfDigest(t, exe)); err == nil || !store.staged {
		t.Fatal("staged-write fault not exercised", err)
	}
	if err := CheckFingerprint(files, original, oldExe); err != nil {
		t.Fatal("old enrollment stranded by failed commit", err)
	}
	v, err := LoadVault(activeWalletStore{files, original})
	if err != nil || !reflect.DeepEqual(v, before) {
		t.Fatal("failed commit changed active wallet", err)
	}
	if _, err := ReenrollWallet(files.Dir, original, exe, selfDigest(t, exe)); err != nil {
		t.Fatal("retry failed", err)
	}
}

func TestReenrollHoldsProfileLockBeforeHash(t *testing.T) {
	files, store, _, exe, _ := reenrollFixture(t, "file")
	digest := selfDigest(t, exe)
	unlock, err := files.Lock()
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if unlock != nil {
			unlock()
		}
	}()
	previous := enrollmentFingerprint
	reached := make(chan struct{}, 1)
	enrollmentFingerprint = func(path string) (string, error) {
		reached <- struct{}{}
		return previous(path)
	}
	t.Cleanup(func() { enrollmentFingerprint = previous })
	done := make(chan error, 1)
	go func() {
		_, err := ReenrollWallet(files.Dir, store, exe, digest)
		done <- err
	}()
	select {
	case <-reached:
		t.Fatal("digest checked outside profile lock")
	case err := <-done:
		t.Fatal("re-enrollment did not wait for profile lock", err)
	case <-time.After(100 * time.Millisecond):
	}
	unlock()
	unlock = nil
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("re-enrollment failed to resume after unlock")
	}
	select {
	case <-reached:
	default:
		t.Fatal("positive control did not check running image")
	}
}

// Transaction tests replace only owner interaction; PTY and detached subprocess
// tests below exercise the production gate, including independent presence.
func allowReenrollmentForTest(t *testing.T) {
	t.Helper()
	previous := confirmReenrollment
	confirmReenrollment = func(Installation, string, string, string) error { return nil }
	t.Cleanup(func() { confirmReenrollment = previous })
}
