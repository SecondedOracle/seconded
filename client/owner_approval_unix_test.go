//go:build !windows

package client

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

type approvalFileInfo struct {
	mode os.FileMode
	uid  uint32
}

func (i approvalFileInfo) Name() string       { return "fixture" }
func (i approvalFileInfo) Size() int64        { return 64 }
func (i approvalFileInfo) Mode() os.FileMode  { return i.mode }
func (i approvalFileInfo) ModTime() time.Time { return time.Time{} }
func (i approvalFileInfo) IsDir() bool        { return i.mode.IsDir() }
func (i approvalFileInfo) Sys() any           { return &syscall.Stat_t{Uid: i.uid} }

func TestOfflineApprovalTrustRequiresAdministrator(t *testing.T) {
	for _, tc := range []struct {
		mode            os.FileMode
		uid             uint32
		directory, want bool
	}{
		{0644, 0, false, true}, {0600, 0, false, true}, {0664, 0, false, false}, {0600, 501, false, false},
		{os.ModeSymlink | 0777, 0, false, false}, {os.ModeDir | 0755, 0, true, true}, {os.ModeDir | 0775, 0, true, false},
	} {
		if rootProtected(approvalFileInfo{tc.mode, tc.uid}, tc.directory) != tc.want {
			t.Fatal(tc)
		}
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "owner-approval.pub")
	if err := os.WriteFile(path, []byte("0000000000000000000000000000000000000000000000000000000000000000\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := readOwnerApprovalKey(path); err == nil || errors.Is(err, os.ErrNotExist) {
		t.Fatal("agent-owned key enabled fallback")
	}
	if _, err := readOwnerApprovalKey(filepath.Join(dir, "absent")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("absent key did not allow TTY fallback", err)
	}
}
