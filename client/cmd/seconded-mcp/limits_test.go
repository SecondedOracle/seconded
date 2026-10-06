package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	client "github.com/SecondedOracle/seconded/client"
)

func TestCLILimitsPatchContract(t *testing.T) {
	g, key := exportFixture(t)
	for _, patch := range []string{`{"daily_usd":25}`, `{"daily_usd":20,"frozen":true}`, `{"hourly_usd":"1.25","dedupe":true}`} {
		out, stderr, err := captureCLIOutput(t, func() error {
			return run([]string{"--profile", g.Files.Dir, "limits", "--patch", patch})
		})
		var result struct {
			Limits map[string]any `json:"limits"`
		}
		if err != nil || stderr != "" || strings.Contains(out, key) || json.Unmarshal([]byte(out), &result) != nil || result.Limits["chat_daily_ceiling_usd"] != "25.00" {
			t.Fatal("bounded patch positive control", patch, err)
		}
	}
	for _, patch := range []string{`{"daily_usd":100.000001}`, `{"daily_usd":null}`, `{"dedupe":false}`, `{"loop_brake":false}`, `{"frozen":false}`, `{"daily_usd":10,"dedupe":false}`} {
		before, err := g.Store.Get("vault")
		if err != nil {
			t.Fatal(err)
		}
		out, stderr, err := captureCLIOutput(t, func() error {
			return run([]string{"--profile", g.Files.Dir, "limits", "--patch", patch})
		})
		after, readErr := g.Store.Get("vault")
		if !errors.Is(err, client.ErrTerminalPolicyRequired) || readErr != nil || before != after || out != "" || stderr != "" {
			t.Fatal("forbidden patch", patch, err)
		}
		for _, want := range []string{g.Files.Dir, "limits --human --patch '" + patch + "'", "last 6"} {
			if !strings.Contains(err.Error(), want) || strings.Contains(err.Error(), key) {
				t.Fatal("missing exact owner command or leaked key", patch)
			}
		}
	}
	for _, args := range [][]string{{}, {"--show"}} {
		out, stderr, err := captureCLIOutput(t, func() error {
			return run(append([]string{"--profile", g.Files.Dir, "limits"}, args...))
		})
		var got map[string]any
		if err != nil || stderr != "" || json.Unmarshal([]byte(out), &got) != nil || got["daily_usd"] != "20.00" || got["frozen"] != true {
			t.Fatal("stored policy show contract", err)
		}
	}
}

func TestCLILimitsRejectsInvalidArgumentsWithoutMutation(t *testing.T) {
	g, _ := exportFixture(t)
	before, err := g.Store.Get("vault")
	if err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"--set", ""}, {"--patch", ""}, {"--human"}, {"--show", "--human"},
		{"--show", "--set", "day=10"}, {"--show", "--patch", `{"daily_usd":10}`},
		{"--set", "day=10", "--patch", `{"daily_usd":20}`},
		{"--set", "day=10=20"}, {"--set", "dedupe=false"}, {"--set", "unknown=on"},
		{"--patch", `{"daily_usd":-1}`}, {"--patch", `{"unknown":1}`},
		{"--patch", `{"daily_usd":10,"daily_usd":20}`},
		{"--human", "--patch", `{"unknown":1}`}, {"--patch", "{"},
		{"--human", "--yes", "--patch", `{"daily_usd":200}`},
		{"--set", "day=10", "extra"},
	} {
		out, stderr, err := captureCLIOutput(t, func() error {
			return run(append([]string{"--profile", g.Files.Dir, "limits"}, args...))
		})
		after, readErr := g.Store.Get("vault")
		if err == nil || out != "" || stderr != "" || readErr != nil || before != after {
			t.Fatal("invalid argument accepted or changed state", args, err)
		}
	}
}

func TestCLIPolicyRefusalPreservesGuidanceAtProcessBoundary(t *testing.T) {
	profile := filepath.Join(t.TempDir(), "owner's profile")
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("SECONDED_EXPORT_FIXTURE_PROFILE", profile)
	t.Setenv("SECONDED_EXPORT_FIXTURE_BINARY", exe)
	TestExportPTYFixture(t)
	before, err := os.ReadFile(filepath.Join(profile, "wallet.key"))
	if err != nil {
		t.Fatal(err)
	}
	out, stderr, err := runCLI(t, "irrelevant piped confirmation\n", "--profile", profile, "limits", "--patch", `{"daily_usd":null}`)
	if err == nil || out != "" || !strings.Contains(stderr, "Run this in your terminal:") || !strings.Contains(stderr, `limits --human --patch '{"daily_usd":null}'`) || !strings.Contains(stderr, `'"'"'`) || strings.Contains(stderr, "operation_failed") {
		t.Fatal("owner guidance lost at main", err, stderr)
	}
	after, err := os.ReadFile(filepath.Join(profile, "wallet.key"))
	if err != nil || string(before) != string(after) {
		t.Fatal("process refusal mutated vault", err)
	}
}
