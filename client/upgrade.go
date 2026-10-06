package client

import (
	"bufio"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
)

var errFingerprintMismatch = errors.New("fingerprint_mismatch")

func newerVersion(candidate, enrolled string) bool {
	return minimumVersionOK(candidate, enrolled) && !minimumVersionOK(enrolled, candidate)
}

// Each enrollment stages a separate credential. install.json is the sole commit
// point: a crash exposes either the complete old pair or the complete new pair.
// Retained records are never consulted as a fallback for an unavailable store.
type enrollmentSecret struct {
	store SecretStore
	key   string
}

func (s enrollmentSecret) Get(k string) (string, error) {
	if k != "vault" {
		return "", ErrInvalid
	}
	return s.store.Get(s.key)
}
func (s enrollmentSecret) Set(k, value string) error {
	if k != "vault" {
		return ErrInvalid
	}
	return s.store.Set(s.key, value)
}

func enrollmentStore(files *Files, osStore SecretStore, i Installation) (SecretStore, error) {
	if !hexDigest.MatchString(i.EnrollmentSlot) {
		return nil, ErrStorage
	}
	if i.Backend == "file" {
		return namedWalletStore{files, walletFilename(i)}, nil
	}
	if i.Backend == "os_keystore" {
		return enrollmentSecret{osStore, "vault-enrollment-" + i.EnrollmentSlot}, nil
	}
	return nil, ErrStorage
}

func selectedEnrollmentStore(files *Files, store SecretStore, i Installation) (SecretStore, error) {
	if active, ok := store.(activeWalletStore); ok {
		store = active.osStore
	}
	if i.EnrollmentSlot != "" {
		return enrollmentStore(files, store, i)
	}
	if i.Backend == "file" {
		return namedWalletStore{files, walletFilename(i)}, nil
	}
	if i.Backend != "os_keystore" {
		return nil, ErrStorage
	}
	return store, nil
}

// The caller holds the profile lock, including signature verification and commit.
func checkEnrollmentLocked(files *Files, store SecretStore, exe string, rollback bool) error {
	i, err := readInstallation(files)
	if err != nil {
		return err
	}
	hash, err := enrollmentFingerprint(exe)
	if err != nil {
		return err
	}
	if hash != i.Fingerprint {
		// Authenticate before touching a credential store, including on rollback.
		if verified, err := verifyAutomaticReleaseDigest(exe, true); err != nil || verified != hash {
			return errFingerprintMismatch
		}
		if (!rollback && !newerVersion(Version, i.Version)) || (rollback && !newerVersion(i.Version, Version)) {
			return errFingerprintMismatch
		}
	}
	selected, err := selectedEnrollmentStore(files, store, i)
	if err != nil {
		return err
	}
	v, err := LoadVault(selected)
	if err != nil {
		return err
	}
	if i.Address != v.Address || i.Fingerprint != v.Fingerprint || i.Version != v.Version {
		return errFingerprintMismatch
	}
	if err = migrateLegacyPolicy(selected, &v); err != nil {
		return err
	}
	if hash == i.Fingerprint {
		return nil
	}
	return commitEnrollment(files, store, i, v, exe, hash)
}

func commitEnrollment(files *Files, store SecretStore, i Installation, v Vault, exe, hash string) error {
	previous := i.Fingerprint
	v.PreviousFingerprint, v.Fingerprint, v.Version = previous, hash, Version
	i.PreviousFingerprint, i.Fingerprint, i.Version, i.Executable = previous, hash, Version, exe
	// The transition identifies a slot distinct from the currently selected
	// digest's slot. Retrying the same transition overwrites its staged copy.
	// Include direction: A->B and B->A must not share storage.
	// The profile lock serializes retries; install.json alone selects the slot.
	// No retained record is automatically removed.
	id := sha256.Sum256([]byte(previous + ":" + hash))
	i.EnrollmentSlot = fmt.Sprintf("%x", id)
	if active, ok := store.(activeWalletStore); ok {
		store = active.osStore
	}
	target, err := enrollmentStore(files, store, i)
	if err != nil {
		return err
	}
	if err = SaveVault(target, v); err != nil {
		return err
	}
	saved, err := LoadVault(target)
	if err != nil {
		return err
	}
	if saved.Key != v.Key || saved.Address != v.Address || saved.Fingerprint != hash || saved.Version != Version || saved.PreviousFingerprint != previous {
		return ErrStorage
	}
	if err = saveInstallation(files, i); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "SECONDED wallet enrollment updated to %s; previous fingerprint %s retained.\n", Version, previous)
	return nil
}

// RollbackEnrollment is a terminal-only entry point, never exposed as an MCP tool.
// The profile lock binds the confirmation to the exact address and versions.
func RollbackEnrollment(dir, exe string) error {
	files, err := OpenFiles(dir)
	if err != nil {
		return err
	}
	unlock, err := files.Lock()
	if err != nil {
		return err
	}
	defer unlock()
	i, err := readInstallation(files)
	if err != nil {
		return err
	}
	if !addressPattern.MatchString(i.Address) || !newerVersion(i.Version, Version) {
		return errFingerprintMismatch
	}
	if _, err := verifyAutomaticReleaseDigest(exe, true); err != nil {
		return errFingerprintMismatch
	}
	terminal := "/dev/tty"
	if runtime.GOOS == "windows" {
		terminal = "CONIN$"
	}
	tty, err := os.OpenFile(terminal, os.O_RDWR, 0)
	if err != nil {
		return ErrTerminalPolicyRequired
	}
	defer tty.Close()
	info, err := tty.Stat()
	if err != nil || info.Mode()&os.ModeCharDevice == 0 {
		return ErrTerminalPolicyRequired
	}
	output := tty
	if runtime.GOOS == "windows" {
		output, err = os.OpenFile("CONOUT$", os.O_WRONLY, 0)
		if err != nil {
			return ErrTerminalPolicyRequired
		}
		defer output.Close()
	}
	if _, err = fmt.Fprintf(output, "Rollback SECONDED %s to %s for wallet %s. Type rollback %s to confirm: ", i.Version, Version, i.Address, i.Address[len(i.Address)-6:]); err != nil {
		return err
	}
	answer, err := bufio.NewReader(io.LimitReader(tty, 64)).ReadString('\n')
	if err != nil || strings.TrimSpace(answer) != "rollback "+i.Address[len(i.Address)-6:] {
		return ErrTerminalPolicyRequired
	}
	if err = ConfirmOwnerPresence(fmt.Sprintf("Rollback wallet %s from %s to %s. Executable: %s. Profile: %s", i.Address, i.Version, Version, exe, dir), tty, output); err != nil {
		return ErrTerminalPolicyRequired
	}
	return checkEnrollmentLocked(files, ProfileOSStore(dir), exe, true)
}

// Migration runs under the enrollment/profile lock and is durably marked once.
// Owner-approved policies survive subsequent starts after this first review.
func migrateLegacyPolicy(store SecretStore, v *Vault) error {
	if v.PolicyVersion >= 1 {
		return nil
	}
	if v.Settings.Limits.Day == nil || *v.Settings.Limits.Day > ChatDailyCeiling {
		v.Settings.Frozen = true
	}
	v.PolicyVersion = 1
	return SaveVault(store, *v)
}
