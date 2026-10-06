//go:build windows

package client

import "os"

// Native route, pool, receive, swap and deferred facts remain available. Trust
// documents need a separately reviewed Windows ownership/ACL implementation.
func openPrivacyTrustDocument(string) (*os.File, error) { return nil, ErrInvalid }
