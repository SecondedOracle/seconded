package client

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
)

// Set only by the release builder from the reviewed SSH Ed25519 public key.
// Never import trust from a downloaded allowed_signers file or environment.
var releaseSigner string

const releaseMetadataLimit = 1024 * 1024

func readReleaseMetadata(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > releaseMetadataLimit {
		return nil, errors.New("release_signature_required")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, errors.New("release_signature_required")
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, releaseMetadataLimit+1))
	if err != nil || len(data) > releaseMetadataLimit {
		return nil, errors.New("release_signature_invalid")
	}
	return data, nil
}

func verifyReleaseSignature(sums, signature []byte) error {
	return verifyReleaseSignatureWithHelper(sums, signature, systemSSHKeygen())
}

func verifyReleaseSignatureWithHelper(sums, signature []byte, helper string) error {
	key, err := base64.StdEncoding.DecodeString(releaseSigner)
	prefix := []byte("\x00\x00\x00\x0bssh-ed25519\x00\x00\x00\x20")
	if err != nil || len(key) != len(prefix)+32 || !bytes.HasPrefix(key, prefix) {
		return errors.New("release_signer_not_configured")
	}
	// The verifier consumes snapshots, preventing sidecar changes between
	// signature verification and checksum parsing. These files contain no secrets.
	dir, err := os.MkdirTemp("", "seconded-release-")
	if err != nil {
		return errors.New("release_signature_unavailable")
	}
	defer os.RemoveAll(dir)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if helper == "" || runSSHVerify(ctx, helper, dir, sshCapabilitySigner, []byte(sshCapabilityMessage), []byte(sshCapabilitySignature)) != nil {
		return errors.New("release_signature_unavailable")
	}
	if err = runSSHVerify(ctx, helper, dir, releaseSigner, sums, signature); err != nil {
		var exited *exec.ExitError
		if !errors.As(err, &exited) || ctx.Err() != nil {
			return errors.New("release_signature_unavailable")
		}
		return errors.New("release_signature_invalid")
	}
	return nil
}

func runSSHVerify(ctx context.Context, helper, dir, signer string, sums, signature []byte) error {
	allowed := filepath.Join(dir, "allowed_signers")
	sig := filepath.Join(dir, "SHA-256SUMS.sig")
	line := "release@seconded namespaces=\"seconded-release\" ssh-ed25519 " + signer + "\n"
	if os.WriteFile(allowed, []byte(line), 0600) != nil || os.WriteFile(sig, signature, 0600) != nil {
		return errors.New("release_signature_unavailable")
	}
	cmd := exec.CommandContext(ctx, helper, "-Y", "verify", "-f", allowed,
		"-I", "release@seconded", "-n", "seconded-release", "-s", sig)
	cmd.Stdin = bytes.NewReader(sums)
	cmd.Env = []string{"PATH=/usr/bin:/bin"}
	if runtime.GOOS == "windows" {
		cmd.Env = []string{"SystemRoot=" + filepath.Dir(filepath.Dir(filepath.Dir(helper)))}
	}
	return cmd.Run()
}

var releaseSumLine = regexp.MustCompile(`^([0-9a-f]{64})  ([A-Za-z0-9._+-]+)$`)

const releaseVersionPrefix = "# seconded-release-version: "

// VerifyAutomaticReleaseDigest authenticates the manifest before consulting any
// checksum. Platform asset names remain stable even if the owner renames the binary.
func VerifyAutomaticReleaseDigest(exe string) (string, error) {
	return verifyAutomaticReleaseDigest(exe, false)
}

func verifyAutomaticReleaseDigest(exe string, requireVersion bool) (string, error) {
	dir := filepath.Dir(exe)
	sums, err := readReleaseMetadata(filepath.Join(dir, "SHA-256SUMS"))
	if err != nil {
		return "", err
	}
	signature, err := readReleaseMetadata(filepath.Join(dir, "SHA-256SUMS.sig"))
	if err != nil {
		return "", err
	}
	if err = verifyReleaseSignature(sums, signature); err != nil {
		return "", err
	}
	asset := "seconded-mcp_" + runtime.GOOS + "_" + runtime.GOARCH
	if runtime.GOOS == "windows" {
		asset += ".exe"
	}
	seen := map[string]bool{}
	want := ""
	version := ""
	for index, line := range strings.Split(strings.TrimSuffix(string(sums), "\n"), "\n") {
		if index == 0 && strings.HasPrefix(line, releaseVersionPrefix) {
			version = strings.TrimPrefix(line, releaseVersionPrefix)
			if !minimumVersionOK(version, version) {
				return "", errors.New("release_signature_invalid")
			}
			continue
		}
		match := releaseSumLine.FindStringSubmatch(line)
		if match == nil || seen[match[2]] {
			return "", errors.New("release_signature_invalid")
		}
		seen[match[2]] = true
		if match[2] == asset {
			want = match[1]
		}
	}
	if requireVersion && (version == "" || version != Version) {
		return "", errors.New("release_version_mismatch")
	}
	hash := Fingerprint
	if requireVersion {
		hash = enrollmentFingerprint
	}
	got, err := hash(exe)
	if err != nil {
		return "", err
	}
	if want == "" || got != want {
		return "", errors.New("release_digest_mismatch")
	}
	return want, nil
}

// Enrollment authenticates the running image. On Linux the proc descriptor
// remains bound to that image even when its pathname is renamed or replaced.
var enrollmentFingerprint = func(exe string) (string, error) {
	if runtime.GOOS == "linux" {
		return Fingerprint("/proc/self/exe")
	}
	return Fingerprint(exe)
}
