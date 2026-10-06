//go:build windows

package client

import (
	"path/filepath"

	"golang.org/x/sys/windows"
)

func systemSSHKeygen() string {
	// Resolve through the OS, not an agent-controlled PATH or SystemRoot value.
	dir, err := windows.GetSystemDirectory()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "OpenSSH", "ssh-keygen.exe")
}
