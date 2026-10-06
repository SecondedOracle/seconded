//go:build !windows

package client

import (
	"os"
	"syscall"
)

func openPrivacyTrustDocument(path string) (*os.File, error) {
	return openPrivacyFile(path, 0022, 32768)
}
func openPrivacyFile(path string, forbidden os.FileMode, limit int64) (*os.File, error) {
	file, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&forbidden != 0 || !ownedByCurrentUser(info) || info.Size() > limit {
		_ = file.Close()
		return nil, ErrInvalid
	}
	return file, nil
}
