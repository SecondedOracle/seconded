//go:build darwin

package client

import (
	"encoding/base64"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/zalando/go-keyring"
)

func noPromptAPI(t *testing.T) *keychainAPI {
	t.Helper()
	allowed := true
	check := func() {
		t.Helper()
		if allowed {
			t.Fatal("credential API called with interaction enabled")
		}
	}
	checked := false
	return &keychainAPI{
		interaction: func(v bool) int32 {
			if v {
				t.Fatal("interaction re-enabled")
			}
			allowed = v
			return 0
		},
		open:        func(_ string, ref *uintptr) int32 { check(); *ref = 42; return 0 },
		copyDefault: func(ref *uintptr) int32 { check(); *ref = 42; return 0 },
		path: func(ref uintptr, length *uint32, buffer *byte) int32 {
			check()
			if ref != 42 {
				t.Fatal("wrong default keychain")
			}
			path := "/default.keychain-db"
			copy(unsafe.Slice(buffer, int(*length)), path)
			*length = uint32(len(path))
			return 0
		},
		status: func(ref uintptr, status *uint32) int32 {
			check()
			if ref != 42 {
				t.Fatal("wrong status target")
			}
			checked = true
			*status = 7
			return 0
		},
		release: func(ref uintptr) {
			check()
			if ref != 42 {
				t.Fatal("wrong release")
			}
		},
		command: func(args []string, input string) ([]byte, int) {
			check()
			if !checked {
				t.Fatal("CLI called without lock check")
			}
			if args[0] == "find-generic-password" {
				return nil, 44
			}
			return nil, 0
		},
	}
}

func TestKeychainNoPromptBeforeAnyOperation(t *testing.T) {
	for _, path := range []string{"", "/throwaway.keychain-db"} {
		api := noPromptAPI(t)
		if _, err := api.get(path, "seconded", "vault"); !errors.Is(err, keyring.ErrNotFound) {
			t.Fatal(err)
		}
		if err := api.set(path, "seconded", "vault", "fixture"); err != nil {
			t.Fatal(err)
		}
		api = noPromptAPI(t)
		api.interaction = func(bool) int32 { return -50 }
		api.command = func([]string, string) ([]byte, int) { t.Fatal("CLI after policy failure"); return nil, 0 }
		if _, err := api.get(path, "seconded", "vault"); !errors.Is(err, ErrKeystoreUnavailable) {
			t.Fatal(err)
		}
		if err := api.set(path, "seconded", "vault", "fixture"); !errors.Is(err, ErrKeystoreUnavailable) {
			t.Fatal(err)
		}
	}
}

func TestKeychainInteractionFailureIsUnavailable(t *testing.T) {
	for _, status := range []int32{-25308, -25293, -128, -25291, -25300, -25294, -50} {
		for _, path := range []string{"", "/throwaway.keychain-db"} {
			api := noPromptAPI(t)
			api.status = func(uintptr, *uint32) int32 { return status }
			api.command = func([]string, string) ([]byte, int) { t.Fatal("CLI after metadata failure"); return nil, 0 }
			if _, err := readOSSecret(func(s, a string) (string, error) { return api.get(path, s, a) }, "vault"); !errors.Is(err, ErrKeystoreUnavailable) {
				t.Fatal(status, err)
			}
			if err := api.set(path, "seconded", "vault", "fixture"); !errors.Is(err, ErrKeystoreUnavailable) {
				t.Fatal(status, err)
			}
		}
	}
}

func TestKeychainLockedNeverLaunchesSecurity(t *testing.T) {
	for _, path := range []string{"", "/throwaway.keychain-db"} {
		for _, bits := range []uint32{0, 2, 4, 6} {
			api := noPromptAPI(t)
			api.status = func(_ uintptr, out *uint32) int32 { *out = bits; return 0 }
			api.command = func([]string, string) ([]byte, int) { t.Fatal("locked keychain launched CLI"); return nil, 0 }
			if _, err := api.get(path, "seconded", "vault"); !errors.Is(err, ErrKeystoreUnavailable) {
				t.Fatal(bits, err)
			}
			if err := api.set(path, "seconded", "vault", "fixture"); !errors.Is(err, ErrKeystoreUnavailable) {
				t.Fatal(bits, err)
			}
		}
	}
}

func TestKeychainReadCompatibilityAndRelease(t *testing.T) {
	for _, path := range []string{"", "/throwaway.keychain-db"} {
		for _, encoded := range []string{"fixture", "go-keyring-encoded:66697874757265", "go-keyring-base64:" + base64.StdEncoding.EncodeToString([]byte("fixture"))} {
			api := noPromptAPI(t)
			released := false
			api.command = func(args []string, input string) ([]byte, int) {
				target := path
				if target == "" {
					target = "/default.keychain-db"
				}
				if !reflect.DeepEqual(args, []string{"find-generic-password", "-s", "seconded", "-wa", "vault", target}) || input != "" {
					t.Fatal("read target or arguments changed", args)
				}
				return []byte(encoded + "\n"), 0
			}
			api.release = func(uintptr) { released = true }
			value, err := api.get(path, "seconded", "vault")
			if err != nil || value != "fixture" || !released {
				t.Fatal(value, err, released)
			}
		}
	}
}

func TestKeychainUpdateAndReadback(t *testing.T) {
	for _, path := range []string{"", "/throwaway.keychain-db"} {
		api := noPromptAPI(t)
		var saved []byte
		checks, calls := 0, 0
		api.status = func(_ uintptr, status *uint32) int32 { checks++; *status = 1; return 0 }
		api.command = func(args []string, input string) ([]byte, int) {
			calls++
			if checks != calls {
				t.Fatal("missing per-operation lock check")
			}
			if args[0] == "find-generic-password" {
				return saved, 0
			}
			target := path
			if target == "" {
				target = "/default.keychain-db"
			}
			encoded := "go-keyring-base64:" + base64.StdEncoding.EncodeToString([]byte("fixture"))
			want := "add-generic-password -U -s seconded -a vault -w " + encoded + " " + target + "\n"
			if !reflect.DeepEqual(args, []string{"-i"}) || input != want {
				t.Fatal("write not confined to stdin and target")
			}
			saved = []byte(encoded)
			return nil, 0
		}
		err := writeOSSecret(func(s, a string) (string, error) { return api.get(path, s, a) }, func(s, a, v string) error { return api.set(path, s, a, v) }, "vault", "fixture")
		if err != nil || checks != 2 {
			t.Fatal(err, checks)
		}
	}
}

func TestNativeKeychainInteractionDisabled(t *testing.T) {
	api, err := nativeKeychain()
	if err != nil {
		t.Fatal("Security.framework bridge unavailable", err)
	}
	keychainMu.Lock()
	defer keychainMu.Unlock()
	if api.interaction(false) != 0 {
		t.Fatal("could not disable interaction")
	}
	err = func() error {
		handle, err := purego.Dlopen("/System/Library/Frameworks/Security.framework/Security", purego.RTLD_NOW|purego.RTLD_LOCAL)
		if err != nil {
			return err
		}
		defer purego.Dlclose(handle)
		var getInteraction func(*bool) int32
		purego.RegisterLibFunc(&getInteraction, handle, "SecKeychainGetUserInteractionAllowed")
		allowed := true
		if status := getInteraction(&allowed); status != 0 || allowed {
			t.Fatalf("no-UI policy not active: status=%d allowed=%v", status, allowed)
		}
		return nil
	}()
	if err != nil {
		t.Fatal(err)
	}
}

// The native harness runs this same compiled test executable before and after
// locking its throwaway keychain. Ordinary unit tests never access real secrets.
func TestNativeKeychainLockedReadWrite(t *testing.T) {
	mode := os.Getenv("SECONDED_TEST_KEYCHAIN_MODE")
	if mode == "" {
		t.Skip("opt-in throwaway keychain required")
	}
	if mode != "unlocked" && mode != "locked" {
		t.Fatal("invalid fixture mode")
	}
	if os.Getenv("SECONDED_KEYCHAIN_PATH") == "" {
		t.Fatal("explicit fixture path required")
	}
	store := OSStore{}
	const account = "noprompt-acceptance-fixture"
	const value = "public test value"
	if mode == "unlocked" {
		if err := store.Set(account, value); err != nil {
			t.Fatal("unlocked write positive control", err)
		}
		if got, err := store.Get(account); err != nil || got != value {
			t.Fatal("unlocked read positive control", err)
		}
		return
	}
	for _, operation := range []string{"read", "write"} {
		start := time.Now()
		var err error
		if operation == "read" {
			_, err = store.Get(account)
		} else {
			err = store.Set(account, value)
		}
		if !errors.Is(err, ErrKeystoreUnavailable) {
			t.Fatal(operation, "locked store not unavailable", err)
		}
		if elapsed := time.Since(start); elapsed >= time.Second {
			t.Fatal(operation, "did not fail promptly", elapsed)
		}
	}
}

func TestKeychainOversizeAndMalformedData(t *testing.T) {
	api := noPromptAPI(t)
	if err := api.set("", "seconded", "vault", strings.Repeat("x", 4096)); !errors.Is(err, keyring.ErrSetDataTooBig) {
		t.Fatal(err)
	}
	_, err := readOSSecret(func(string, string) (string, error) { return "", ErrKeystoreData }, "vault")
	if !errors.Is(err, ErrKeystoreData) {
		t.Fatal("data failure became fallback", err)
	}
}
