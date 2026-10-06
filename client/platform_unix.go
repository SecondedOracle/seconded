//go:build darwin || linux

package client

import (
	"os"
	"syscall"

	"golang.org/x/sys/unix"
)

func createPrivateDirectory(p string) error { return os.MkdirAll(p, 0700) }

func securePath(p string, dir bool) error {
	s, e := os.Lstat(p)
	if os.IsNotExist(e) {
		return ErrNotFound
	}
	if e != nil || s.Mode()&os.ModeSymlink != 0 || s.IsDir() != dir || (!dir && !s.Mode().IsRegular()) || s.Mode().Perm()&0077 != 0 {
		return ErrStorage
	}
	if !ownedByCurrentUser(s) {
		return ErrStorage
	}
	return nil
}

func ownedByCurrentUser(info os.FileInfo) bool {
	owner, ok := info.Sys().(*syscall.Stat_t)
	return ok && owner.Uid == uint32(os.Geteuid())
}
func lockFile(f *os.File) error     { return unix.Flock(int(f.Fd()), unix.LOCK_EX) }
func unlockFile(f *os.File)         { _ = unix.Flock(int(f.Fd()), unix.LOCK_UN) }
func replaceFile(a, b string) error { return os.Rename(a, b) }
func syncDir(p string) error {
	d, e := os.Open(p)
	if e != nil {
		return ErrStorage
	}
	defer d.Close()
	if e = d.Sync(); e != nil {
		return ErrStorage
	}
	return nil
}
