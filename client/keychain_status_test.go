//go:build darwin

package client

import (
	"errors"
	"os"
	"testing"

	"github.com/zalando/go-keyring"
)

func TestKeychainCommandStatusMapping(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		want   error
	}{
		{"success", 0, nil},
		{"item absent", 44, keyring.ErrNotFound},
		{"interaction required", 36, ErrKeystoreUnavailable},
		{"authentication failed", 51, ErrKeystoreUnavailable},
		{"cancelled", 128, ErrKeystoreUnavailable},
		{"missing keychain", 50, ErrKeystoreUnavailable},
		{"no default keychain", 45, ErrKeystoreUnavailable},
		{"unavailable", 53, ErrKeystoreUnavailable},
		{"unknown", 1, ErrKeystoreUnavailable},
		{"timeout or launch failure", -1, ErrKeystoreUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := keychainCommandStatus(tc.status, true); !errors.Is(err, tc.want) {
				t.Fatal(err)
			}
			api := noPromptAPI(t)
			api.command = func([]string, string) ([]byte, int) { return []byte("fixture"), tc.status }
			_, err := readOSSecret(func(s, a string) (string, error) { return api.get("", s, a) }, "vault")
			want := tc.want
			if want == keyring.ErrNotFound {
				want = ErrNotFound
			}
			if !errors.Is(err, want) {
				t.Fatal("wallet classification", err)
			}
			want = nil
			if tc.status != 0 {
				want = ErrKeystoreUnavailable
			}
			if err := api.set("", "seconded", "vault", "fixture"); !errors.Is(err, want) {
				t.Fatal("write classification", err)
			}
		})
	}
}

func TestKeychainMetadataFailuresDoNotMeanAbsentWallet(t *testing.T) {
	for _, stage := range []string{"open", "default", "path", "empty ref", "bad path"} {
		t.Run(stage, func(t *testing.T) {
			api := noPromptAPI(t)
			path := ""
			switch stage {
			case "open":
				path = "/throwaway.keychain-db"
				api.open = func(string, *uintptr) int32 { return -25300 }
			case "default":
				api.copyDefault = func(*uintptr) int32 { return -25300 }
			case "path":
				api.path = func(uintptr, *uint32, *byte) int32 { return -25300 }
			case "empty ref":
				api.copyDefault = func(*uintptr) int32 { return 0 }
			case "bad path":
				api.path = func(_ uintptr, n *uint32, _ *byte) int32 { *n = 4096; return 0 }
			}
			api.command = func([]string, string) ([]byte, int) { t.Fatal("CLI after metadata failure"); return nil, 0 }
			if _, err := api.get(path, "seconded", "vault"); !errors.Is(err, ErrKeystoreUnavailable) {
				t.Fatal(err)
			}
			if err := api.set(path, "seconded", "vault", "fixture"); !errors.Is(err, ErrKeystoreUnavailable) {
				t.Fatal(err)
			}
		})
	}
}

// Set with -X in the native harness to produce distinct ad-hoc-signed executables.
var keychainFixtureBuild = "unit"

func nativeUpgradeMode(t *testing.T) string {
	t.Helper()
	mode := os.Getenv("SECONDED_TEST_KEYCHAIN_UPGRADE")
	if mode == "" {
		t.Skip("opt-in throwaway keychain required")
	}
	if (mode != "create" && mode != "read") || os.Getenv("SECONDED_KEYCHAIN_PATH") == "" {
		t.Fatal("explicit keychain and valid upgrade mode required")
	}
	return mode
}

func TestNativeKeychainUpgradeCompatibility(t *testing.T) {
	mode := nativeUpgradeMode(t)
	store := OSStore{}
	const account = "cross-build-acceptance-fixture"
	const value = "public upgrade test value"
	if mode == "create" {
		if keychainFixtureBuild != "build-A" {
			t.Fatal("wrong writer build")
		}
		if err := store.Set(account, value); err != nil {
			t.Fatal(err)
		}
	} else if keychainFixtureBuild != "build-B" {
		t.Fatal("wrong reader build")
	}
	if got, err := store.Get(account); err != nil || got != value {
		t.Fatal("cross-build read", err)
	}
	if got, err := store.Get("legacy-acceptance-fixture"); err != nil || got != "public legacy test value" {
		t.Fatal("legacy security-created read", err)
	}
	if _, err := store.Get("absent-acceptance-fixture"); !errors.Is(err, ErrNotFound) {
		t.Fatal("absent positive control", err)
	}
}
