package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	client "github.com/SecondedOracle/seconded/client"
)

func TestCLIReenrollAndMismatchGuidance(t *testing.T) {
	g, key := exportFixture(t)
	v, err := client.LoadVault(g.Store)
	if err != nil {
		t.Fatal(err)
	}
	exe, _ := os.Executable()
	exe, _ = filepath.EvalSymlinks(exe)
	digest, err := client.Fingerprint(exe)
	if err != nil {
		t.Fatal(err)
	}
	v.Fingerprint = strings.Repeat("1", 64)
	if err := client.SaveVault(client.FileStore{Files: g.Files}, v); err != nil {
		t.Fatal(err)
	}
	i := client.Installation{Version: v.Version, Address: v.Address, Backend: "file", Fingerprint: v.Fingerprint, Executable: exe}
	raw, _ := json.Marshal(i)
	if err := g.Files.Write("install.json", raw); err != nil {
		t.Fatal(err)
	}
	for _, command := range []string{"self-check", "setup", "limits"} {
		out, stderr, err := runCLI(t, "", "--profile", g.Files.Dir, command)
		if err == nil || out != "" || !strings.Contains(stderr, "fingerprint_mismatch") || !strings.Contains(stderr, "setup --reenroll --release-sha256 <published hash>") || strings.Contains(stderr, digest) || strings.Contains(stderr, g.Files.Dir) || strings.Contains(stderr, key) {
			t.Fatal("CLI mismatch guidance failed", command)
		}
	}
	for _, flags := range [][]string{{}, {"--release-sha256", strings.Repeat("0", 64)}, {"--release-sha256", digest, "--advanced"}, {"--release-sha256", digest, "--daily-limit", "1"}} {
		args := append([]string{"--profile", g.Files.Dir, "setup", "--reenroll"}, flags...)
		out, stderr, err := runCLI(t, "", args...)
		if err == nil || out != "" || strings.Contains(stderr, key) {
			t.Fatal("invalid re-enroll accepted or disclosed key")
		}
		after, _ := g.Files.Read("install.json")
		if string(after) != string(raw) {
			t.Fatal("refusal mutated enrollment")
		}
	}
	out, stderr, err := runCLI(t, "", "--profile", g.Files.Dir, "setup", "--reenroll", "--release-sha256", digest)
	if err == nil || out != "" || !strings.Contains(stderr, "terminal_policy_required") || strings.Contains(stderr, key) {
		t.Fatal("CLI must refuse re-enrollment without controlling terminal", err)
	}
	after, _ := g.Files.Read("install.json")
	if string(after) != string(raw) {
		t.Fatal("no-terminal refusal changed enrollment")
	}
	// Positive controls prove both the scanners and the original wallet still work.
	if !strings.Contains(stderr+key, key) || !strings.Contains(stderr+digest, digest) {
		t.Fatal("disclosure scanner ineffective")
	}
	i.Fingerprint, v.Fingerprint = digest, digest
	if err := client.SaveVault(client.FileStore{Files: g.Files}, v); err != nil {
		t.Fatal(err)
	}
	restored, _ := json.Marshal(i)
	if err := g.Files.Write("install.json", restored); err != nil {
		t.Fatal(err)
	}
	out, stderr, err = runCLI(t, "", "--profile", g.Files.Dir, "self-check")
	if err != nil || !strings.Contains(out, `"fingerprint_matches":true`) || strings.Contains(out+stderr, key) {
		t.Fatal("CLI positive control cannot reopen enrolled wallet", err)
	}
}

func TestCLIReenrollHelp(t *testing.T) {
	out, stderr, err := runCLI(t, "", "setup", "--help")
	if err != nil || stderr != "" || !strings.Contains(out, "--reenroll --release-sha256 SHA256") || !strings.Contains(out, "rollback") {
		t.Fatal("re-enrollment missing from setup help")
	}
}
