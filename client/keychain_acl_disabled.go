//go:build darwin && !seconded_keychain_acl

package client

// Candidate migration remains gated until the operator completes native tests.
const keychainACLCandidate = false
