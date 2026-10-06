//go:build darwin || linux

package client

import (
	"os"
	"syscall"
	"testing"
)

type ownerInfo struct {
	os.FileInfo
	stat syscall.Stat_t
}

func (s ownerInfo) Sys() any { return &s.stat }

func TestStorageOwnerCheck(t *testing.T) {
	info, err := os.Stat(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if !ownedByCurrentUser(info) {
		t.Fatal("current owner rejected")
	}
	other := ownerInfo{FileInfo: info, stat: syscall.Stat_t{Uid: uint32(os.Geteuid()) + 1}}
	if ownedByCurrentUser(other) {
		t.Fatal("foreign owner accepted")
	}
}
