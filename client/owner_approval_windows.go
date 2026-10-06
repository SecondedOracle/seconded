package client

import "crypto/ed25519"

// Windows fails closed until an administrator-protected trust store is supplied.
func ownerApprovalKey() (ed25519.PublicKey, error) { return nil, errPresenceUnavailable }
