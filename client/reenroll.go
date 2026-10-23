package client

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// ReenrollWallet explicitly trusts a separately verified running-image digest.
// It never creates a wallet, changes backend or policy, or authorizes a downgrade.
func ReenrollWallet(dir string, store SecretStore, exe, digest string) (result Installation, err error) {
	defer func() { err = setupStorageError(err, dir) }()
	if !filepath.IsAbs(dir) {
		return result, ErrStorage
	}
	if _, err = os.Lstat(filepath.Join(dir, "install.json")); errors.Is(err, os.ErrNotExist) {
		return result, errors.New("reenroll_wallet_missing")
	} else if err != nil {
		return result, ErrStorage
	}
	files, err := OpenFiles(dir)
	if err != nil {
		return result, err
	}
	unlock, err := files.Lock()
	if err != nil {
		return result, err
	}
	defer unlock()
	digest = strings.ToLower(strings.TrimSpace(digest))
	if !hexDigest.MatchString(digest) {
		return result, errors.New("release_digest_required")
	}
	hash, err := enrollmentFingerprint(exe)
	if err != nil {
		return result, err
	}
	if digest != hash {
		return result, errors.New("release_digest_mismatch")
	}
	i, err := readInstallation(files)
	if err != nil {
		return result, err
	}
	if !minimumVersionOK(Version, i.Version) {
		return result, errors.New("reenroll_downgrade_refused")
	}
	if err = confirmReenrollment(i, exe, dir, hash); err != nil {
		return result, err
	}
	selected, err := selectedEnrollmentStore(files, store, i)
	if err != nil {
		return result, err
	}
	v, err := LoadVault(selected)
	if errors.Is(err, ErrNotFound) {
		return result, errors.New("reenroll_wallet_missing")
	}
	if err != nil {
		return result, err
	}
	if i.Address != v.Address || i.Fingerprint != v.Fingerprint || i.Version != v.Version {
		return result, ErrStorage
	}
	if err = migrateLegacyPolicy(selected, &v); err != nil {
		return result, err
	}
	if hash == i.Fingerprint {
		return i, nil
	}
	if err = commitEnrollment(files, store, i, v, exe, hash); err != nil {
		return result, err
	}
	return readInstallation(files)
}

type fingerprintMismatchError struct{ command string }

func (e *fingerprintMismatchError) Error() string { return errFingerprintMismatch.Error() }
func (e *fingerprintMismatchError) Unwrap() error { return errFingerprintMismatch }

const FingerprintMismatchHint = "This executable could not open the wallet's enrolled fingerprint. Its build may differ, or the enrollment records may disagree. A human must verify the build against the published SHA-256SUMS, then run setup --reenroll --release-sha256 <published hash> with the same executable and --profile in their own terminal. Agents must not run this step. For an older signed release, use rollback in a human terminal instead. Preserve the profile if its records disagree."

func enrollmentCommand(dir, exe, suffix string) string {
	quote := func(s string) string {
		if runtime.GOOS == "windows" {
			return "'" + strings.ReplaceAll(s, "'", "''") + "'"
		}
		return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
	}
	prefix := ""
	if runtime.GOOS == "windows" {
		prefix = "& "
	}
	return prefix + quote(exe) + " --profile " + quote(dir) + " " + suffix
}

func actionableFingerprintError(err error, dir, exe string) error {
	// The mismatched process is not a source of trusted recovery commands.
	return err
}

// FingerprintMismatchMessage exposes fixed human guidance only, never a computed
// digest, local command, store error text or key. The reason remains fingerprint_mismatch.
func FingerprintMismatchMessage(err error) string {
	if !errors.Is(err, errFingerprintMismatch) {
		return ""
	}
	return FingerprintMismatchHint
}

// Hold the profile lock through confirmation, as rollback does, so the owner
// approves exactly the selection that will be committed. Tests inject this gate.
var confirmReenrollment = confirmReenrollmentTerminal

func confirmReenrollmentTerminal(i Installation, exe, dir, hash string) error {
	if !addressPattern.MatchString(i.Address) {
		return ErrStorage
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
	return confirmReenrollmentPrompt(i, exe, dir, hash, tty, output)
}

// The terminal entry point validates the controlling device before calling this.
func confirmReenrollmentPrompt(i Installation, exe, dir, hash string, input io.Reader, output io.Writer) error {
	phrase := "reenroll " + i.Address[len(i.Address)-6:]
	reason := fmt.Sprintf("Re-enroll SECONDED wallet %s from version %s to %s. Old fingerprint: %s. New fingerprint: %s. Executable: %s. Profile: %s", i.Address, i.Version, Version, i.Fingerprint, hash, exe, dir)
	if _, err := fmt.Fprintf(output, "%s\nType %s to confirm: ", reason, phrase); err != nil {
		return ErrTerminalPolicyRequired
	}
	answer, err := bufio.NewReader(io.LimitReader(input, 64)).ReadString('\n')
	if err != nil || strings.TrimSpace(answer) != phrase {
		return ErrTerminalPolicyRequired
	}
	if err = ConfirmOwnerPresence(reason, input, output); err != nil {
		return ErrTerminalPolicyRequired
	}
	return nil
}
