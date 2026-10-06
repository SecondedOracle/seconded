package main

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/zalando/go-keyring"
	client "seconded.local/client"
)

func TestCLILimitsDollarView(t *testing.T) {
	for _, unlimited := range []bool{false, true} {
		t.Run(map[bool]string{false: "amounts", true: "no_limits"}[unlimited], func(t *testing.T) {
			g, key := exportFixture(t)
			vault, err := client.LoadVault(g.Store)
			if err != nil {
				t.Fatal(err)
			}
			perCheck, hourly, daily, outstanding := int64(2500000), int64(1234567), int64(20000000), int64(0)
			vault.Settings = client.Settings{
				Limits:    client.Limits{PerCheck: &perCheck, Hour: &hourly, Day: &daily, Outstanding: &outstanding},
				ValueGate: true, Dedupe: false, LoopBrake: true, Alerts: false, Frozen: true,
			}
			want := map[string]any{
				"per_check_usd": "2.50", "hourly_usd": "1.234567", "daily_usd": "20.00", "outstanding_usd": "0.00",
				"value_gate": true, "dedupe": false, "loop_brake": true, "alerts": false, "frozen": true,
				"max_authorization_usd": "2.50", "chat_daily_ceiling_usd": "25.00",
			}
			if unlimited {
				vault.Settings.Limits = client.Limits{}
				for _, name := range []string{"per_check_usd", "hourly_usd", "daily_usd", "outstanding_usd"} {
					want[name] = nil
				}
			}
			if err := client.SaveVault(g.Store, vault); err != nil {
				t.Fatal(err)
			}
			before, err := g.Store.Get("vault")
			if err != nil {
				t.Fatal(err)
			}
			mcpView, err := g.Limits(json.RawMessage(`{}`), false)
			if err != nil || !reflect.DeepEqual(mcpView, want) {
				t.Fatal("MCP limits view", mcpView, err)
			}
			for _, flags := range [][]string{{}, {"--show"}} {
				out, stderr, err := captureCLIOutput(t, func() error {
					return run(append([]string{"--profile", g.Files.Dir, "limits"}, flags...))
				})
				if err != nil || stderr != "" || strings.Contains(out, key) {
					t.Fatal("limits display failed or disclosed key", flags, err)
				}
				var got map[string]any
				if err := json.Unmarshal([]byte(out), &got); err != nil || !reflect.DeepEqual(got, want) {
					t.Fatal("CLI limits must match the dollar view", flags, got, err)
				}
				after, err := g.Store.Get("vault")
				if err != nil || before != after {
					t.Fatal("limits display mutated stored policy", err)
				}
			}
		})
	}
}

func TestCLIProcess(t *testing.T) {
	if os.Getenv("SECONDED_CLI_TEST_PROCESS") != "1" {
		return
	}
	// Test-only store; production binaries have no environment-based backend.
	if os.Getenv("SECONDED_CLI_MOCK_KEYSTORE") == "1" {
		keyring.MockInit()
	}
	for i, arg := range os.Args {
		if arg == "--" {
			os.Args = append([]string{os.Args[0]}, os.Args[i+1:]...)
			main()
			os.Exit(0)
		}
	}
	os.Exit(2)
}

func runCLI(t *testing.T, input string, args ...string) (string, string, error) {
	t.Helper()
	cmd := exec.Command(os.Args[0], append([]string{"-test.run=^TestCLIProcess$", "--"}, args...)...)
	cmd.Env = append(os.Environ(), "SECONDED_CLI_TEST_PROCESS=1")
	cmd.Stdin = strings.NewReader(input)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

func TestFreshHostInitializesBeforeSetup(t *testing.T) {
	t.Setenv("HOME", "")
	t.Setenv("USERPROFILE", "")
	profile := filepath.Join(t.TempDir(), "profile")
	input := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"fresh-host","version":"1"}}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"seconded_wallet"}}`,
		`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"seconded_hidden_prompt_check"}}`,
		`{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"seconded_products"}}`,
	}, "\n") + "\n"
	stdout, stderr, err := runCLI(t, input, "--profile", profile, "serve")
	if err != nil || stderr != "" {
		t.Fatal("fresh launch failed", err, stderr)
	}
	replies := map[int]map[string]json.RawMessage{}
	for _, line := range strings.Split(strings.TrimSpace(stdout), "\n") {
		var reply map[string]json.RawMessage
		if err := json.Unmarshal([]byte(line), &reply); err != nil {
			t.Fatal(err, line)
		}
		var id int
		json.Unmarshal(reply["id"], &id)
		replies[id] = reply
	}
	if len(replies) != 5 || !strings.Contains(string(replies[1]["result"]), "protocolVersion") {
		t.Fatal("missing initialization", stdout)
	}
	tools := string(replies[2]["result"])
	if !strings.Contains(tools, "seconded_trade_check") || strings.Contains(tools, "seconded_hidden_prompt_check") {
		t.Fatal("wrong tools", tools)
	}
	var failed struct {
		IsError    bool `json:"isError"`
		Structured struct {
			Reason  string `json:"reason"`
			Message string `json:"message"`
			Next    string `json:"next"`
		} `json:"structuredContent"`
	}
	if json.Unmarshal(replies[3]["result"], &failed) != nil || !failed.IsError || failed.Structured.Reason != "release_signature_required" || failed.Structured.Next != "run_setup" || !strings.Contains(failed.Structured.Message, "seconded-mcp setup") {
		t.Fatal("missing setup guidance", stdout)
	}
	if !strings.Contains(string(replies[4]["error"]), "Unknown tool") || !strings.Contains(string(replies[5]["result"]), `"products"`) {
		t.Fatal("unavailable tool or public catalogue", stdout)
	}
	entries, err := os.ReadDir(profile)
	if err != nil || len(entries) != 0 {
		t.Fatal("discovery created wallet state", entries, err)
	}
}

func TestUnattendedSetupReachesSignedReleaseVerification(t *testing.T) {
	profile := filepath.Join(t.TempDir(), "profile")
	stdout, stderr, err := runCLI(t, "yes\n", "--profile", profile, "setup")
	if err == nil || stdout != "" || !strings.Contains(stderr, "release_signature_required:") || strings.Contains(stderr, "owner_terminal_required:") {
		t.Fatal("terminal boundary", stdout, stderr, err)
	}
	if _, err := os.Stat(profile); !os.IsNotExist(err) {
		t.Fatal("unattended setup created state", err)
	}
}

func TestCorruptInstallationStillFailsBeforeServe(t *testing.T) {
	profile := t.TempDir()
	if err := os.WriteFile(filepath.Join(profile, "install.json"), []byte(`{}`), 0600); err != nil {
		t.Fatal(err)
	}
	stdout, stderr, err := runCLI(t, "", "--profile", profile, "serve")
	if err == nil || stdout != "" || !strings.Contains(stderr, "local_storage_unavailable") {
		t.Fatal("invalid install accepted", stdout, stderr, err)
	}
}

func TestLaunchStrings(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout := os.Stdout
	os.Stdout = w
	runErr := run([]string{"--help"})
	os.Stdout = stdout
	w.Close()
	banner, _ := io.ReadAll(r)
	if runErr != nil || !strings.Contains(string(banner), networkSummary) {
		t.Fatal("banner", string(banner), runErr)
	}
	for _, text := range []string{string(banner), fundingNotice} {
		for _, bad := range []string{"Testnet only", "Never send mainnet funds", "private test release", "testnet by default", "Base Sepolia testnet unless"} {
			if strings.Contains(text, bad) {
				t.Fatal("pre-launch string", bad)
			}
		}
	}
	for _, want := range []string{"Base mainnet unless", "only when a check names network=arc, robinhood, base_sepolia, arc_testnet or robinhood_testnet", "USDG on Robinhood"} {
		if !strings.Contains(fundingNotice, want) {
			t.Fatal("funding notice omits", want)
		}
	}
}

func TestSetupModesRequireReleaseButNeverOwnerTerminal(t *testing.T) {
	for _, mode := range [][]string{{}, {"--advanced"}, {"--manual-fingerprint"}, {"--advanced", "--manual-fingerprint"}, {"--host", "claude-desktop"}, {"--host", "cursor"}, {"--host", "claude-code"}} {
		profile := filepath.Join(t.TempDir(), "profile")
		args := append([]string{"--profile", profile, "setup"}, mode...)
		stdout, stderr, err := runCLI(t, strings.Repeat("yes\n", 10), args...)
		want := "release_signature_required:"
		for _, option := range mode {
			if option == "--manual-fingerprint" {
				want = "release_digest_required"
			}
		}
		if err == nil || stdout != "" || !strings.Contains(stderr, want) {
			t.Fatal(args, err, stdout, stderr)
		}
		if _, err := os.Stat(profile); !os.IsNotExist(err) {
			t.Fatal("agent created state", err)
		}
	}
}

func TestQuickCustomPolicyRequiresAdvanced(t *testing.T) {
	for _, option := range []string{"--daily-limit=20", "--allow-file-key", "--value-gate=false", "--dedupe=false", "--loop-brake=false", "--alerts=false"} {
		stdout, stderr, err := runCLI(t, "yes\n", "--profile", filepath.Join(t.TempDir(), "profile"), "setup", option)
		if err == nil || stdout != "" || !strings.Contains(stderr, "setup_advanced_required") {
			t.Fatal(option, stdout, stderr, err)
		}
	}
}

func TestHostSnippetWithoutOwnerOrWallet(t *testing.T) {
	t.Setenv("HOME", "")
	t.Setenv("USERPROFILE", "")
	for _, host := range []string{"claude-code", "claude-desktop", "cursor", "codex", "generic"} {
		profile := filepath.Join(t.TempDir(), "no wallet")
		stdout, stderr, err := runCLI(t, "", "--profile", profile, "host-snippet", "--host", host)
		if err != nil || !strings.Contains(stdout, profile) || !strings.Contains(stdout, host) || strings.Contains(stderr, "owner_terminal_required") {
			t.Fatal(stdout, stderr, err)
		}
		if host == "claude-code" && !strings.Contains(stdout, "claude mcp add --scope user seconded --") {
			t.Fatal(stdout)
		}
		if _, err := os.Stat(profile); !os.IsNotExist(err) {
			t.Fatal("snippet touched profile", err)
		}
	}
	stdout, _, err := runCLI(t, "", "host-snippet", "--host", "unknown")
	if err == nil || stdout != "" {
		t.Fatal("unknown host accepted")
	}
	stdout, _, err = runCLI(t, "", "host-snippet", "--host", "claude-code")
	if err != nil || !strings.Contains(stdout, "claude mcp add --scope user") {
		t.Fatal("snippet requires no home directory", stdout, err)
	}
}

// The recipient harness signs this test executable with an ephemeral fixture key
// compiled through the real release pin. Regular tests do not assume a signer.
func TestCLIZeroTouchWithSignedBinary(t *testing.T) {
	if os.Getenv("SECONDED_CLI_SIGNED_FIXTURE") != "1" {
		t.Skip("recipient harness supplies signed fixture binary")
	}
	t.Setenv("SECONDED_CLI_MOCK_KEYSTORE", "1")
	profile := filepath.Join(t.TempDir(), "profile")
	stdout, stderr, err := runCLI(t, "", "--profile", profile, "setup", "--host", "generic")
	if err != nil || !strings.Contains(stdout, "mcpServers") || !strings.Contains(stderr, "SECONDED created a check wallet: 0x") || !strings.Contains(stderr, "daily limit: $25.00") || strings.Contains(stderr, "Type yes") {
		t.Fatal(stdout, stderr, err)
	}
	raw, err := os.ReadFile(filepath.Join(profile, "install.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"backend":"os_keystore"`) {
		t.Fatal(string(raw))
	}
	if _, err := os.Stat(filepath.Join(profile, "wallet.key")); !os.IsNotExist(err) {
		t.Fatal("file fallback", err)
	}
	input := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"pipe","version":"1"}}}` + "\n" + `{"jsonrpc":"2.0","method":"notifications/initialized"}` + "\n" + `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"seconded_scam_check","arguments":{"input":{"message":"fixture"}}}}` + "\n"
	stdout, stderr, err = runCLI(t, input, "--profile", filepath.Join(t.TempDir(), "profile"), "serve")
	if err != nil || stderr != "" || !strings.Contains(stdout, "funding_required") || !strings.Contains(stdout, "SECONDED created a check wallet: 0x") {
		t.Fatal(stdout, stderr, err)
	}
	for _, flag := range []string{"--daily-limit=11", "--hourly-limit=6", "--outstanding-limit=6", "--per-check-limit=none", "--value-gate=false", "--dedupe=false", "--loop-brake=false", "--alerts=false", "--allow-file-key"} {
		args := []string{"--profile", filepath.Join(t.TempDir(), "profile"), "setup", "--advanced", "--daily-limit=10", "--hourly-limit=5", "--outstanding-limit=5", "--per-check-limit=2.50", flag}
		stdout, stderr, err = runCLI(t, strings.Repeat("yes\n", 12), args...)
		if flag == "--dedupe=false" || flag == "--loop-brake=false" {
			if err == nil || stdout != "" || !strings.Contains(stderr, "terminal_policy_required") {
				t.Fatal("advanced setup bypassed chat policy", flag, stderr, err)
			}
			continue
		}
		if err != nil || !strings.Contains(stdout, "mcpServers") || strings.Contains(stderr, "Type yes") || strings.Contains(stderr, "owner_confirmation_required") {
			t.Fatal(flag, stdout, stderr, err)
		}
	}
}

func TestNoKeyExportOrWalletDeletionCommand(t *testing.T) {
	for _, command := range []string{"export", "reveal", "delete"} {
		stdout, _, err := runCLI(t, "yes\n", "--profile", filepath.Join(t.TempDir(), "profile"), command)
		if err == nil || stdout != "" {
			t.Fatal(command, stdout, err)
		}
	}
}

func TestCLIHelpAndMissingCommand(t *testing.T) {
	profile := filepath.Join(t.TempDir(), "unopened-profile")
	for _, prefix := range [][]string{nil, {"--profile", profile}} {
		for _, command := range []string{"--help", "-h", "help"} {
			out, stderr, err := runCLI(t, "", append(append([]string{}, prefix...), command)...)
			if err != nil || stderr != "" || !strings.Contains(out, "Usage:") || !strings.Contains(out, commandList) {
				t.Fatalf("help %v %s: %q %q %v", prefix, command, out, stderr, err)
			}
		}
		out, stderr, err := runCLI(t, "", prefix...)
		if err == nil || out != "" || !strings.Contains(stderr, "missing_command") || !strings.Contains(stderr, commandList) || strings.Contains(stderr, "profile_requires_absolute_path") || strings.Contains(stderr, "operation_failed") {
			t.Fatalf("missing command %v: %q %q %v", prefix, out, stderr, err)
		}
	}
	for _, args := range [][]string{{"--profile"}, {"--profile", "relative"}, {"--profile", "relative", "help"}} {
		_, stderr, err := runCLI(t, "", args...)
		if err == nil || !strings.Contains(stderr, "profile_requires_absolute_path") {
			t.Fatalf("invalid profile %v: %q %v", args, stderr, err)
		}
	}
	if _, err := os.Stat(profile); !os.IsNotExist(err) {
		t.Fatal("help or missing command accessed profile", err)
	}
}

func TestCLIVersionAliasesDoNotOpenProfile(t *testing.T) {
	t.Setenv("HOME", "")
	t.Setenv("USERPROFILE", "")
	profile := filepath.Join(t.TempDir(), "unopened-profile")
	for _, prefix := range [][]string{nil, {"--profile", profile}} {
		for _, command := range []string{"version", "--version"} {
			stdout, stderr, err := runCLI(t, "", append(append([]string{}, prefix...), command)...)
			if err != nil || stderr != "" || stdout != "seconded-mcp "+version+"\n" {
				t.Fatalf("%v %s: %q %q %v", prefix, command, stdout, stderr, err)
			}
		}
	}
	if _, err := os.Stat(profile); !os.IsNotExist(err) {
		t.Fatal("version command accessed profile", err)
	}
}

func TestSetupProfilePermissionGuidance(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix directory permissions")
	}
	profile := filepath.Join(t.TempDir(), "profile's $directory")
	if err := os.Mkdir(profile, 0700); err != nil {
		t.Fatal(err)
	}
	// Existing metadata reaches the permission boundary without touching a key
	// store or requiring a signed release; after chmod it remains corrupt.
	if err := os.WriteFile(filepath.Join(profile, "install.json"), []byte(`{}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(profile, 0755); err != nil {
		t.Fatal(err)
	}
	stdout, stderr, err := runCLI(t, "", "--profile", profile, "setup")
	command := client.ProfilePermissionCommand(profile, client.ErrStorage)
	if err == nil || stdout != "" || command == "" || !strings.Contains(stderr, command) || !strings.Contains(stderr, "local_storage_unavailable") {
		t.Fatal("missing actionable permission error", stdout, stderr, err)
	}
	if out, err := exec.Command("sh", "-c", command).CombinedOutput(); err != nil {
		t.Fatalf("suggested command is not executable: %v %s", err, out)
	}
	info, err := os.Stat(profile)
	if err != nil || info.Mode().Perm() != 0700 {
		t.Fatal("command did not repair the exact directory", err)
	}
	_, stderr, err = runCLI(t, "", "--profile", profile, "setup")
	if err == nil || strings.Contains(stderr, "chmod") || !strings.Contains(stderr, "local_storage_unavailable") {
		t.Fatal("corrupt metadata incorrectly blamed on permissions", stderr, err)
	}
	if client.ProfilePermissionCommand(profile, client.ErrKeystoreData) != "" {
		t.Fatal("credential corruption incorrectly recommends chmod")
	}
}
