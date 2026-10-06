package client

import (
	"context"
	"encoding/base64"
	"errors"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"al.essio.dev/pkg/shellescape"
	"github.com/ebitengine/purego"
	"github.com/zalando/go-keyring"
)

// security is the stable trusted application on legacy and newly created items.
// Use the native API only for noninteractive metadata checks before invoking it.
// purego preserves the CGO_ENABLED=0 release contract.
type keychainAPI struct {
	interaction func(bool) int32
	open        func(string, *uintptr) int32
	copyDefault func(*uintptr) int32
	status      func(uintptr, *uint32) int32
	path        func(uintptr, *uint32, *byte) int32
	release     func(uintptr)
	command     func([]string, string) ([]byte, int)
}

var nativeKeychain = sync.OnceValues(loadKeychainAPI)
var keychainMu sync.Mutex

type nonInteractiveKeychain struct{}

func (nonInteractiveKeychain) Get(service, account string) (string, error) {
	return nativeKeychainGet("", service, account)
}
func (nonInteractiveKeychain) Set(service, account, value string) error {
	return nativeKeychainSet("", service, account, value)
}
func (nonInteractiveKeychain) Delete(string, string) error { return ErrKeystoreUnavailable }
func (nonInteractiveKeychain) DeleteAll(string) error      { return ErrKeystoreUnavailable }

func init() {
	// Register at startup so callers and the existing keyring test doubles use
	// one backend boundary. Wallet code never invokes credential deletion.
	keyring.SetProvider(nonInteractiveKeychain{})
}

func loadKeychainAPI() (api *keychainAPI, err error) {
	// Missing libraries/symbols make the store unavailable, never an interactive
	// fallback. Keep the framework loaded for the lifetime of its function pointers.
	defer func() {
		if recover() != nil {
			api, err = nil, ErrKeystoreUnavailable
		}
	}()
	handle, err := purego.Dlopen("/System/Library/Frameworks/Security.framework/Security", purego.RTLD_NOW|purego.RTLD_LOCAL)
	if err != nil {
		return nil, ErrKeystoreUnavailable
	}
	api = &keychainAPI{command: runKeychainCommand}
	purego.RegisterLibFunc(&api.interaction, handle, "SecKeychainSetUserInteractionAllowed")
	purego.RegisterLibFunc(&api.open, handle, "SecKeychainOpen")
	purego.RegisterLibFunc(&api.copyDefault, handle, "SecKeychainCopyDefault")
	purego.RegisterLibFunc(&api.status, handle, "SecKeychainGetStatus")
	purego.RegisterLibFunc(&api.path, handle, "SecKeychainGetPath")
	purego.RegisterLibFunc(&api.release, handle, "CFRelease")
	return api, nil
}

func keychainCommandStatus(status int, reading bool) error {
	switch status {
	case 0:
		return nil
	case 44: // errSecItemNotFound (-25300), truncated to a process exit status.
		if reading {
			return keyring.ErrNotFound
		}
	}
	return ErrKeystoreUnavailable
}

func runKeychainCommand(args []string, input string) ([]byte, int) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "/usr/bin/security", args...)
	cmd.Stdin = strings.NewReader(input)
	// Passwords travel through stdin/stdout, never argv or returned diagnostics.
	out, err := cmd.Output()
	if err == nil {
		return out, 0
	}
	var exit *exec.ExitError
	if ctx.Err() == nil && errors.As(err, &exit) {
		return nil, exit.ExitCode()
	}
	return nil, -1
}

// Callers serialize policy changes and operations. Never restore interaction:
// another goroutine must not gain permission to display a password dialog.
func (api *keychainAPI) withKeychain(path string, operation func(string) error) error {
	keychainMu.Lock()
	defer keychainMu.Unlock()
	if api.interaction(false) != 0 {
		return ErrKeystoreUnavailable
	}
	var ref uintptr
	if path != "" {
		if !filepath.IsAbs(path) || strings.IndexByte(path, 0) >= 0 || api.open(path, &ref) != 0 {
			return ErrKeystoreUnavailable
		}
	} else if api.copyDefault(&ref) != 0 {
		return ErrKeystoreUnavailable
	}
	if ref == 0 {
		return ErrKeystoreUnavailable
	}
	defer api.release(ref)
	if path == "" {
		// Pin the CLI to the same keychain we check, even if the default changes.
		var buffer [4096]byte
		length := uint32(len(buffer))
		if api.path(ref, &length, &buffer[0]) != 0 || length == 0 || length >= uint32(len(buffer)) {
			return ErrKeystoreUnavailable
		}
		path = string(buffer[:length])
		if !filepath.IsAbs(path) || strings.IndexByte(path, 0) >= 0 {
			return ErrKeystoreUnavailable
		}
	}
	var status uint32
	// Metadata errors (even item-not-found) say nothing about wallet absence.
	if api.status(ref, &status) != 0 || status&1 == 0 { // kSecUnlockStateStatus
		return ErrKeystoreUnavailable
	}
	// The subprocess has its own UI policy. A lock between this check and the
	// CLI operation can still prompt; the deadline bounds waiting, not dialog UI.
	return operation(path)
}

func (api *keychainAPI) get(path, service, account string) (value string, err error) {
	err = api.withKeychain(path, func(target string) error {
		out, status := api.command([]string{"find-generic-password", "-s", service, "-wa", account, target}, "")
		if err := keychainCommandStatus(status, true); err != nil {
			return err
		}
		if len(out) > ResponseLimit {
			return ErrKeystoreData
		}
		value = string(out)
		return nil
	})
	if err != nil {
		return "", err
	}
	return decodeKeychainValue(value)
}

func (api *keychainAPI) set(path, service, account, value string) error {
	encoded := "go-keyring-base64:" + base64.StdEncoding.EncodeToString([]byte(value))
	command := "add-generic-password -U -s " + shellescape.Quote(service) + " -a " + shellescape.Quote(account) + " -w " + shellescape.Quote(encoded)
	if len(command)+1 > 4096 {
		return keyring.ErrSetDataTooBig
	}
	return api.withKeychain(path, func(target string) error {
		// security -i accepts one command per line; reject embedded delimiters
		// even inside quoted identifiers. The encoded value cannot contain them.
		if strings.ContainsAny(service+account+target, "\x00\r\n") {
			return ErrKeystoreUnavailable
		}
		input := command + " " + shellescape.Quote(target) + "\n"
		if len(input) > 4096 {
			return keyring.ErrSetDataTooBig
		}
		_, status := api.command([]string{"-i"}, input)
		// security's interactive mode can hide a command failure. OSStore.Set
		// always verifies persistence by reading back through the same gate.
		return keychainCommandStatus(status, false)
	})
}

func nativeKeychainGet(path, service, account string) (string, error) {
	if keychainACLCandidate {
		return hardenedKeychainGet(path, service, account)
	}
	api, err := nativeKeychain()
	if err != nil {
		return "", err
	}
	return api.get(path, service, account)
}

func nativeKeychainSet(path, service, account, value string) error {
	if keychainACLCandidate {
		return hardenedKeychainSet(path, service, account, value)
	}
	api, err := nativeKeychain()
	if err != nil {
		return err
	}
	return api.set(path, service, account, value)
}

func platformKeychainGet(path, service, account string) (string, error) {
	if path == "" {
		return keyring.Get(service, account)
	}
	return nativeKeychainGet(path, service, account)
}

func platformKeychainSet(path, service, account, value string) error {
	if path == "" {
		return keyring.Set(service, account, value)
	}
	return nativeKeychainSet(path, service, account, value)
}
