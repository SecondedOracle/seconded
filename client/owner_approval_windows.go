package client

import (
	"crypto/ed25519"
	"os"
)

// Windows has no administrator-protected approval key; use the caller's TTY confirmation.
func ownerApprovalKey() (ed25519.PublicKey, error) { return nil, os.ErrNotExist }
