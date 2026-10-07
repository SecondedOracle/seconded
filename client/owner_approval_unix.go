//go:build !windows

package client

import (
	"crypto/ed25519"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// Configuration lives outside the agent's account. An environment variable or
// writable profile setting would let the same agent turn the gate off.
func ownerApprovalKey() (ed25519.PublicKey, error) {
	return readOwnerApprovalKey("/etc/seconded/owner-approval.pub")
}

func rootProtected(info os.FileInfo, directory bool) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && stat.Uid == 0 && info.Mode().Perm()&0022 == 0 && (directory && info.IsDir() || !directory && info.Mode().IsRegular())
}

func readOwnerApprovalKey(path string) (ed25519.PublicKey, error) {
	// Resolve system symlinks such as /etc -> /private/etc before checking every
	// ancestor; none may be replaceable by an unprivileged process.
	if _, err := os.Lstat(path); os.IsNotExist(err) {
		return nil, os.ErrNotExist
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return nil, errPresenceUnavailable
	}
	for dir := filepath.Dir(resolved); ; dir = filepath.Dir(dir) {
		info, err := os.Lstat(dir)
		if err != nil || !rootProtected(info, true) {
			return nil, errPresenceUnavailable
		}
		if filepath.Dir(dir) == dir {
			break
		}
	}
	original, err := os.Lstat(path)
	if err != nil || !rootProtected(original, false) {
		return nil, errPresenceUnavailable
	}
	file, err := os.Open(resolved)
	if err != nil {
		return nil, errPresenceUnavailable
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !os.SameFile(original, info) || !rootProtected(info, false) {
		return nil, errPresenceUnavailable
	}
	raw, err := io.ReadAll(io.LimitReader(file, 67))
	if err != nil {
		return nil, errPresenceUnavailable
	}
	key, err := hex.DecodeString(strings.TrimSpace(string(raw)))
	if err != nil || len(key) != ed25519.PublicKeySize {
		return nil, errPresenceUnavailable
	}
	return ed25519.PublicKey(key), nil
}
