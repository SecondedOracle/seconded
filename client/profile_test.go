package client

import (
	"encoding/json"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

type keyedTestStore map[string]string

func (s keyedTestStore) Get(k string) (string, error) {
	v, ok := s[k]
	if !ok {
		return "", ErrNotFound
	}
	return v, nil
}
func (s keyedTestStore) Set(k, v string) error { s[k] = v; return nil }

func TestProfileStoreIsolation(t *testing.T) {
	base := keyedTestStore{"vault": "existing default must not be read"}
	a := ProfileStore{Store: base, Profile: filepath.Join(t.TempDir(), "a")}
	b := ProfileStore{Store: base, Profile: filepath.Join(t.TempDir(), "b")}
	if _, err := a.Get("vault"); err != ErrNotFound {
		t.Fatal("profile opened default vault", err)
	}
	if err := a.Set("vault", "profile a"); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Get("vault"); err != ErrNotFound {
		t.Fatal("profiles share vault", err)
	}
	if got, err := a.Get("vault"); err != nil || got != "profile a" || base["vault"] != "existing default must not be read" {
		t.Fatal("isolation positive control", err)
	}
}

func TestProfileHostSnippets(t *testing.T) {
	exe := filepath.Join(t.TempDir(), "seconded-mcp")
	profile := filepath.Join(t.TempDir(), "fresh customer")
	for _, host := range []string{"generic", "cursor", "claude-code", "claude-desktop", "codex"} {
		snippet, err := HostSnippetProfile(host, exe, profile)
		if err != nil {
			t.Fatal(err)
		}
		if host == "codex" {
			encoded, _ := json.Marshal([]string{"--profile", profile, "serve", "--host", host})
			if !strings.Contains(snippet, "args = "+string(encoded)) {
				t.Fatal("profile missing from TOML")
			}
		} else if host == "claude-code" {
			if !strings.Contains(snippet, "claude mcp add --scope user seconded --") || !strings.Contains(snippet, profile) || !strings.Contains(snippet, exe) {
				t.Fatal("profile missing from Claude CLI command", snippet)
			}
		} else {
			var decoded struct {
				Servers map[string]struct {
					Args []string `json:"args"`
				} `json:"mcpServers"`
			}
			if json.Unmarshal([]byte(snippet), &decoded) != nil || decoded.Servers["seconded"].Args[1] != profile {
				t.Fatal("profile missing from JSON")
			}
		}
	}
	if _, err := HostSnippetProfile("generic", exe, "relative"); err == nil {
		t.Fatal("relative profile accepted")
	}
}

func TestClaudeCommandPreservesShellArguments(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX shell execution control; Windows output requires PowerShell")
	}
	exe := filepath.Join(t.TempDir(), "owner's binary")
	profile := filepath.Join(t.TempDir(), "owner's profile")
	snippet, err := HostSnippetProfile("claude-code", exe, profile)
	if err != nil {
		t.Fatal(err)
	}
	// A shell function records argv without invoking any installed MCP host.
	cmd := exec.Command("/bin/sh", "-c", "claude() { printf '%s\\n' \"$@\"; }; "+snippet)
	output, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"mcp", "add", "--scope", "user", "seconded", "--", exe, "--profile", profile, "serve", "--host", "claude-code"}
	if got := strings.Split(strings.TrimSuffix(string(output), "\n"), "\n"); !reflect.DeepEqual(got, want) {
		t.Fatal(got, want)
	}
}
