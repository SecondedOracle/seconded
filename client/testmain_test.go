//go:build darwin || linux

package client

import (
	"os"
	"syscall"
	"testing"
)

func TestMain(m *testing.M) {
	// Go's TempDir children inherit the umask. Storage fixtures must be private,
	// just like directories created by the client, regardless of the caller's mask.
	syscall.Umask(0077)
	os.Exit(m.Run())
}
