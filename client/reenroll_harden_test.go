package client

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestReenrollRejectsNonTTYInput(t *testing.T) {
	if os.Getenv("SECONDED_TEST_REENROLL_NO_TTY") == "1" {
		files, store, _, exe, before := reenrollFixture(t, "file")
		confirmReenrollment = confirmReenrollmentTerminal
		raw, _ := files.Read("install.json")
		_, err := ReenrollWallet(files.Dir, store, exe, selfDigest(t, exe))
		if !errors.Is(err, ErrTerminalPolicyRequired) {
			t.Fatal("non-terminal must be refused", err)
		}
		after, _ := files.Read("install.json")
		if !bytes.Equal(raw, after) {
			t.Fatal("refusal changed selection")
		}
		got, err := LoadVault(FileStore{files})
		if err != nil || !reflect.DeepEqual(before, got) {
			t.Fatal("refusal changed wallet", err)
		}
		return
	}
	if runtime.GOOS == "windows" {
		t.Skip("Unix detached session")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	child := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestReenrollRejectsNonTTYInput$")
	child.Env = append(os.Environ(), "SECONDED_TEST_REENROLL_NO_TTY=1")
	child.Stdin = strings.NewReader("reenroll 000000\n")
	child.SysProcAttr = &syscall.SysProcAttr{}
	reflect.ValueOf(child.SysProcAttr).Elem().FieldByName("Setsid").SetBool(true)
	if output, err := child.CombinedOutput(); err != nil {
		t.Fatalf("detached gate: %v %s", err, output)
	}
}

func TestPinnedImageSurvivesPathReplacement(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "image")
	if err := os.WriteFile(path, []byte("original running image"), 0700); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	want := selfDigest(t, path)
	replacement := filepath.Join(dir, "replacement")
	if err := os.WriteFile(replacement, []byte("untrusted replacement"), 0700); err != nil {
		t.Fatal(err)
	}
	// Preserve both fixture files; only the pathname now identifies another inode.
	if err := os.Rename(path, filepath.Join(dir, "retained-original")); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(replacement, path); err != nil {
		t.Fatal(err)
	}
	if selfDigest(t, path) == want {
		t.Fatal("replacement positive control ineffective")
	}
	for n := 0; n < 2; n++ {
		got, err := fingerprintImage(f)
		if err != nil || got != want {
			t.Fatal("descriptor followed replacement path", err)
		}
	}
	// Check the production reader independently of the inert-image fixture override.
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	got, err := runningImageFingerprint(path)
	if err != nil || got != selfDigest(t, exe) {
		t.Fatal("production reader consulted caller pathname", err)
	}
}

func TestExistingWalletPendingNotice(t *testing.T) {
	i := Installation{Address: "0x" + strings.Repeat("1", 40), Backend: "file", EarlierCheckPending: true}
	got := WalletExistingMessage(i, t.TempDir())
	if !strings.Contains(got, "Keep using this enrolled address") || !strings.Contains(got, "OS store could not be checked") || strings.Contains(got, "don't fund") {
		t.Fatal("existing wallet notice incorrect")
	}
	if !strings.Contains(WalletFundingMessage(i, t.TempDir()), "don't fund") {
		t.Fatal("new-wallet warning control lost")
	}
	i.EarlierCheckPending = false
	if strings.Contains(WalletExistingMessage(i, t.TempDir()), EarlierEnrolledWalletNotice) {
		t.Fatal("notice without pending discovery")
	}
}

func TestUnselectedEnrollmentCanBeCommittedHistory(t *testing.T) {
	files, store, _, exe, _ := reenrollFixture(t, "file")
	first, err := ReenrollWallet(files.Dir, store, exe, selfDigest(t, exe))
	if err != nil {
		t.Fatal(err)
	}
	selected := walletFilename(first)
	retained, err := files.Read(selected)
	if err != nil {
		t.Fatal(err)
	}
	next := filepath.Join(t.TempDir(), "next")
	if err := os.WriteFile(next, []byte("another trusted rebuild"), 0700); err != nil {
		t.Fatal(err)
	}
	second, err := ReenrollWallet(files.Dir, store, next, selfDigest(t, next))
	if err != nil || second.EnrollmentSlot == first.EnrollmentSlot {
		t.Fatal("distinct committed history control", err)
	}
	got, err := files.Read(selected)
	if err != nil || !bytes.Equal(got, retained) {
		t.Fatal("committed recovery record lost", err)
	}
	if err := CheckFingerprint(files, store, next); err != nil {
		t.Fatal("current selection damaged", err)
	}
}

func TestReenrollPromptAndOwnerPresence(t *testing.T) {
	originalPresence, originalKey := authenticateOwnerPresence, loadOwnerPresenceApprovalKey
	t.Cleanup(func() { authenticateOwnerPresence, loadOwnerPresenceApprovalKey = originalPresence, originalKey })
	loadOwnerPresenceApprovalKey = func() (ed25519.PublicKey, error) { return nil, os.ErrNotExist }
	i := Installation{Address: "0x" + strings.Repeat("a", 40), Version: "0.4.4", Fingerprint: strings.Repeat("1", 64)}
	hash := strings.Repeat("2", 64)
	for _, mode := range []string{"approve", "deny", "fallback", "wrong-phrase", "eof"} {
		t.Run(mode, func(t *testing.T) {
			calls := 0
			authenticateOwnerPresence = func(reason string) error {
				calls++
				for _, want := range []string{i.Address, i.Version, Version, i.Fingerprint, hash, "/profile", "/executable"} {
					if !strings.Contains(reason, want) {
						t.Fatal("owner challenge missing binding", want)
					}
				}
				if mode == "approve" {
					return nil
				}
				if mode == "fallback" {
					return errPresenceUnavailable
				}
				return ErrTerminalPolicyRequired
			}
			answer := "reenroll aaaaaa\n"
			if mode == "wrong-phrase" {
				answer = "reenroll bbbbbb\n"
			}
			if mode == "eof" {
				answer = ""
			}
			var output bytes.Buffer
			err := confirmReenrollmentPrompt(i, "/executable", "/profile", hash, strings.NewReader(answer), &output)
			success := mode == "approve" || mode == "fallback"
			if (err == nil) != success || (!success && !errors.Is(err, ErrTerminalPolicyRequired)) {
				t.Fatal("owner decision ignored", err)
			}
			wantCalls := 1
			if mode == "wrong-phrase" || mode == "eof" {
				wantCalls = 0
			}
			if calls != wantCalls {
				t.Fatal("presence gate order incorrect", calls)
			}
			for _, want := range []string{i.Address, i.Version, Version, i.Fingerprint, hash, "Type reenroll aaaaaa to confirm:"} {
				if !strings.Contains(output.String(), want) {
					t.Fatal("prompt missing information", want)
				}
			}
		})
	}
}
