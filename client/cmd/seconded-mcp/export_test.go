package main

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zalando/go-keyring"
	client "github.com/SecondedOracle/seconded/client"
)

func exportFixture(t *testing.T) (*client.Engine, string) {
	t.Helper()
	keyring.MockInit()
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	files, err := client.OpenFiles(dir)
	if err != nil {
		t.Fatal(err)
	}
	exe, _ := os.Executable()
	exe, _ = filepath.EvalSymlinks(exe)
	digest, err := client.Fingerprint(exe)
	if err != nil {
		t.Fatal(err)
	}
	store := client.ProfileOSStore(dir)
	if _, err := client.SetupWallet(files, store, false, client.QuickSettings(), exe, digest); err != nil {
		t.Fatal(err)
	}
	v, err := client.LoadVault(store)
	if err != nil {
		t.Fatal(err)
	}
	return &client.Engine{Files: files, Store: store}, v.Key
}

func captureCLIOutput(t *testing.T, fn func() error) (string, string, error) {
	t.Helper()
	or, ow, _ := os.Pipe()
	er, ew, _ := os.Pipe()
	oldOut, oldErr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = ow, ew
	defer func() { os.Stdout, os.Stderr = oldOut, oldErr }()
	err := fn()
	ow.Close()
	ew.Close()
	out, _ := io.ReadAll(or)
	stderr, _ := io.ReadAll(er)
	or.Close()
	er.Close()
	return string(out), string(stderr), err
}

func mockExportConsole(t *testing.T, confirmation string) (*exportConsole, func() string) {
	t.Helper()
	input, sender, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	output, dest, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(sender, confirmation); err != nil {
		t.Fatal(err)
	}
	sender.Close()
	terminal := &exportConsole{input: input, output: dest}
	t.Cleanup(func() { terminal.close(); output.Close() })
	return terminal, func() string {
		dest.Close()
		raw, err := io.ReadAll(output)
		if err != nil {
			t.Fatal(err)
		}
		return string(raw)
	}
}

func TestCLIExportRejectsOutputAndArguments(t *testing.T) {
	g, key := exportFixture(t)
	path := filepath.Join(t.TempDir(), "owner-export")
	old := openExportTerminal
	t.Cleanup(func() { openExportTerminal = old })
	openExportTerminal = func() (*exportConsole, error) {
		t.Fatal("invalid arguments reached terminal")
		return nil, errExportTerminal
	}
	for _, args := range [][]string{{"--output", path}, {"--output=" + path}, {path}, {"--yes"}, {"--"}} {
		command := append([]string{"--profile", g.Files.Dir, "export-key"}, args...)
		out, stderr, err := captureCLIOutput(t, func() error { return run(command) })
		if err == nil || out != "" || stderr != "" || strings.Contains(err.Error(), key) {
			t.Fatal("export accepted arguments or leaked output")
		}
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("export created a file", err)
	}
}

func TestCLIExportRequiresTerminalBeforeProfileAccess(t *testing.T) {
	old := openExportTerminal
	t.Cleanup(func() { openExportTerminal = old })
	openExportTerminal = func() (*exportConsole, error) { return nil, errors.New("no terminal") }
	profile := filepath.Join(t.TempDir(), "absent-profile")
	out, stderr, err := captureCLIOutput(t, func() error {
		return run([]string{"--profile", profile, "export-key"})
	})
	if !errors.Is(err, errExportTerminal) || out != "" || stderr != "" {
		t.Fatal("no-terminal export did not fail closed")
	}
	if _, err := os.Stat(profile); !os.IsNotExist(err) {
		t.Fatal("no-terminal export touched the profile", err)
	}
}

func TestCLIExportRequiresExactTerminalConfirmation(t *testing.T) {
	oldPresence := confirmExportPresence
	confirmExportPresence = func(string, io.Reader, io.Writer) error { return nil }
	t.Cleanup(func() { confirmExportPresence = oldPresence })
	g, key := exportFixture(t)
	vault, err := client.LoadVault(g.Store)
	if err != nil {
		t.Fatal(err)
	}
	suffix := vault.Address[len(vault.Address)-6:]
	for _, test := range []struct {
		name, input string
		accept      bool
	}{
		{"eof", "", false},
		{"empty", "\n", false},
		{"yes", "yes\n", false},
		{"wrong", "zzzzzz\n", false},
		{"unterminated", suffix, false},
		{"padded", " " + suffix + "\n", false},
		{"long", suffix + "extra\n", false},
		{"confirmed", suffix + "\n", true},
		{"windows-line", suffix + "\r\n", true},
		{"case-insensitive", strings.ToUpper(suffix) + "\n", true},
	} {
		t.Run(test.name, func(t *testing.T) {
			terminal, readOutput := mockExportConsole(t, test.input)
			out, stderr, err := captureCLIOutput(t, func() error { return exportToTerminal(g, terminal) })
			if out != "" || stderr != "" {
				t.Fatal("export reached captured process streams")
			}
			shown := readOutput()
			if !strings.Contains(shown, vault.Address) || !strings.Contains(shown, exportWarning) {
				t.Fatal("missing terminal prompt")
			}
			if test.accept {
				if err != nil || strings.Count(shown, key) != 1 {
					t.Fatal("confirmed export positive control failed")
				}
			} else if !errors.Is(err, errExportConfirmation) || strings.Contains(shown, key) {
				t.Fatal("unconfirmed export released key")
			}
		})
	}
}

func TestCLIExportWriteErrorsAreSanitized(t *testing.T) {
	g, key := exportFixture(t)
	terminal, _ := mockExportConsole(t, "unused\n")
	terminal.output.Close()
	out, stderr, err := captureCLIOutput(t, func() error { return exportToTerminal(g, terminal) })
	if !errors.Is(err, errExportWrite) || out != "" || stderr != "" || strings.Contains(err.Error(), key) {
		t.Fatal("terminal write error was not safe")
	}
}

func TestCLILimitsAndSwitchesEnforceChatPolicyThroughPipes(t *testing.T) {
	g, key := exportFixture(t)
	for _, value := range []string{"day=20", "day=none", "per_check=100", "hour=none", "outstanding=none", "value_gate=off", "dedupe=off", "loop_brake=off", "alerts=off", "frozen=on", "frozen=off"} {
		before, err := g.Store.Get("vault")
		if err != nil {
			t.Fatal(err)
		}
		out, stderr, err := captureCLIOutput(t, func() error { return run([]string{"--profile", g.Files.Dir, "limits", "--set", value}) })
		forbidden := value == "per_check=100" || value == "alerts=off" || value == "day=none" || value == "dedupe=off" || value == "loop_brake=off" || value == "frozen=off"
		if forbidden {
			if !errors.Is(err, client.ErrTerminalPolicyRequired) || out != "" || stderr != "" {
				t.Fatal("forbidden pipe mutation", value, err)
			}
			after, readErr := g.Store.Get("vault")
			if readErr != nil || before != after || strings.Contains(err.Error(), key) {
				t.Fatal("refused change mutated vault or disclosed key", value)
			}
			continue
		}
		if err != nil || stderr != "" || !strings.Contains(out, "limits") || strings.Contains(out, key) {
			t.Fatal(value, err)
		}
	}
}

func TestCLIManualSetupUsesExplicitDigestAndPartialSafeDefaults(t *testing.T) {
	t.Setenv("SECONDED_CLI_MOCK_KEYSTORE", "1")
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	digest, err := client.Fingerprint(exe)
	if err != nil {
		t.Fatal(err)
	}
	profile := filepath.Join(t.TempDir(), "profile")
	out, stderr, err := runCLI(t, "must never read this input", "--profile", profile, "setup", "--manual-fingerprint", "--release-sha256", digest, "--advanced", "--daily-limit", "20")
	if err != nil || !strings.Contains(out, "mcpServers") {
		t.Fatal(err, stderr)
	}
	for _, want := range []string{"per-check limit: $2.50", "hourly limit: none", "daily limit: $20.00", "outstanding limit: none", "SECONDED created a check wallet:"} {
		if !strings.Contains(stderr, want) {
			t.Fatal("missing setup output", want)
		}
	}
	for _, prompt := range []string{"Type yes", "Password:", "Paste the SHA", "limit in USD ("} {
		if strings.Contains(stderr, prompt) {
			t.Fatal("interactive prompt", prompt)
		}
	}
}

func TestCLIExportFromAutomaticFileFallback(t *testing.T) {
	oldPresence := confirmExportPresence
	confirmExportPresence = func(string, io.Reader, io.Writer) error { return nil }
	t.Cleanup(func() { confirmExportPresence = oldPresence })
	keyring.MockInitWithError(errors.New("credential service unavailable"))
	t.Cleanup(keyring.MockInit)
	dir := filepath.Join(t.TempDir(), "profile")
	files, err := client.OpenFiles(dir)
	if err != nil {
		t.Fatal(err)
	}
	exe, _ := os.Executable()
	exe, _ = filepath.EvalSymlinks(exe)
	digest, err := client.Fingerprint(exe)
	if err != nil {
		t.Fatal(err)
	}
	install, err := client.SetupWallet(files, client.ProfileOSStore(dir), false, client.QuickSettings(), exe, digest)
	if err != nil || install.Backend != "file" {
		t.Fatal(install, err)
	}
	vault, err := client.LoadVault(client.FileStore{Files: files})
	if err != nil {
		t.Fatal(err)
	}
	terminal, readOutput := mockExportConsole(t, vault.Address[len(vault.Address)-6:]+"\n")
	old := openExportTerminal
	t.Cleanup(func() { openExportTerminal = old })
	openExportTerminal = func() (*exportConsole, error) { return terminal, nil }
	out, stderr, err := captureCLIOutput(t, func() error { return run([]string{"--profile", dir, "export-key"}) })
	if err != nil || out != "" || stderr != "" {
		t.Fatal("fallback terminal export failed", err)
	}
	if shown := readOutput(); !strings.Contains(shown, vault.Key) || !strings.Contains(shown, exportWarning) {
		t.Fatal("fallback export positive control failed")
	}
}

// Only the PTY harness invokes this test to create a disposable, unfunded fixture.
// Production binaries have no test environment variables or backend overrides.
func TestExportPTYFixture(t *testing.T) {
	dir := os.Getenv("SECONDED_EXPORT_FIXTURE_PROFILE")
	if dir == "" {
		t.Skip("PTY harness supplies a disposable profile")
	}
	exe := os.Getenv("SECONDED_EXPORT_FIXTURE_BINARY")
	digest, err := client.Fingerprint(exe)
	if err != nil {
		t.Fatal(err)
	}
	keyring.MockInitWithError(errors.New("fixture credential store unavailable"))
	t.Cleanup(keyring.MockInit)
	files, err := client.OpenFiles(dir)
	if err != nil {
		t.Fatal(err)
	}
	install, err := client.SetupWallet(files, client.ProfileOSStore(dir), false, client.QuickSettings(), exe, digest)
	if err != nil || install.Backend != "file" {
		t.Fatal("automatic file fallback fixture failed", err)
	}
}

func TestExportPublicSuffixWithoutPresenceCannotReleaseKey(t *testing.T) {
	g, key := exportFixture(t)
	vault, err := client.LoadVault(g.Store)
	if err != nil {
		t.Fatal(err)
	}
	terminal, read := mockExportConsole(t, vault.Address[len(vault.Address)-6:]+"\n")
	old := confirmExportPresence
	t.Cleanup(func() { confirmExportPresence = old })
	called := false
	confirmExportPresence = func(reason string, _ io.Reader, _ io.Writer) error {
		called = true
		if !strings.Contains(reason, vault.Address) {
			t.Fatal("wallet unbound")
		}
		return client.ErrTerminalPolicyRequired
	}
	err = exportToTerminal(g, terminal)
	if !called || !errors.Is(err, errExportConfirmation) || strings.Contains(read(), key) {
		t.Fatal("suffix bypassed presence", err)
	}
}
