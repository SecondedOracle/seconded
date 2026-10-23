package client

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

// Exercise the shipped CLI, including its real running-image fingerprint reader.
// The enrollment has the 0.4.4 on-disk shape; no historical binary or real wallet
// is needed. Only an ephemeral TEST release signer is compiled into this binary.
func TestSigned044To045ProcessUpgrade(t *testing.T) {
	exe, sign := signedSetupFixture(t)
	exe = filepath.Join(filepath.Dir(exe), "seconded-test")
	build := exec.Command("go", "build", "-mod=vendor", "-trimpath", "-buildvcs=false", "-ldflags",
		"-X main.version=0.4.5 -X github.com/SecondedOracle/seconded/client.Version=0.4.5 -X github.com/SecondedOracle/seconded/client.releaseSigner="+releaseSigner,
		"-o", exe, "./cmd/seconded-mcp")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build test-signed CLI: %v %s", err, output)
	}
	asset := "seconded-mcp_" + runtime.GOOS + "_" + runtime.GOARCH
	if runtime.GOOS == "windows" {
		asset += ".exe"
	}
	manifest := releaseVersionPrefix + "0.4.5\n" + selfDigest(t, exe) + "  " + asset + "\n"
	sign(manifest, "seconded-release")
	oldVersion := Version
	Version = "0.4.4"
	t.Cleanup(func() { Version = oldVersion })
	oldExe := filepath.Join(t.TempDir(), "old-image")
	if err := os.WriteFile(oldExe, []byte("0.4.4-shaped enrollment fixture"), 0700); err != nil {
		t.Fatal(err)
	}
	files, err := OpenFiles(filepath.Join(t.TempDir(), "profile"))
	if err != nil {
		t.Fatal(err)
	}
	store := &memoryStore{err: ErrKeystoreUnavailable}
	i, err := SetupWallet(files, store, false, QuickSettings(), oldExe, selfDigest(t, oldExe))
	if err != nil {
		t.Fatal(err)
	}
	// Prevent discovery of any real host credential store in the child process.
	i.EarlierCheckPending = false
	if err := saveInstallation(files, i); err != nil {
		t.Fatal(err)
	}
	before, err := LoadVault(FileStore{files})
	if err != nil {
		t.Fatal(err)
	}
	ledger, _ := files.Read("ledger.json")
	install, _ := files.Read("install.json")
	run := func(wantSuccess bool) {
		t.Helper()
		cmd := exec.Command(exe, "--profile", files.Dir, "self-check")
		output, err := cmd.CombinedOutput()
		if bytes.Contains(output, []byte(before.Key)) {
			t.Fatal("CLI disclosed wallet key")
		}
		if (err == nil) != wantSuccess {
			t.Fatalf("CLI success=%t: %v %s", wantSuccess, err, output)
		}
		if wantSuccess && !strings.Contains(string(output), `"fingerprint_matches":true`) {
			t.Fatal("CLI did not open migrated wallet")
		}
	}
	// Authentic but wrong-version manifest must fail without changing enrollment.
	sign(strings.Replace(manifest, "0.4.5", "0.4.4", 1), "seconded-release")
	run(false)
	after, _ := files.Read("install.json")
	if !bytes.Equal(after, install) {
		t.Fatal("wrong manifest mutated enrollment")
	}
	sign(manifest, "seconded-release")
	run(true)
	run(true)
	got, err := LoadVault(activeWalletStore{files, store})
	if err != nil || got.Key != before.Key || got.Address != before.Address || !reflect.DeepEqual(got.Settings, before.Settings) || got.PreviousFingerprint != before.Fingerprint || got.Fingerprint != selfDigest(t, exe) || got.Version != "0.4.5" {
		t.Fatal("upgrade did not preserve wallet and bind 0.4.5", err)
	}
	afterLedger, _ := files.Read("ledger.json")
	if !bytes.Equal(ledger, afterLedger) {
		t.Fatal("upgrade changed payment records")
	}
}
