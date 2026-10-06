//go:build !privacy_companion

package client

import (
	"context"
	"sync"
)

type privacyProcess struct {
	native *privacyNative
	once   sync.Once
	gate   chan struct{}
	closed bool
}

func privacyCompanionEnabled() bool                                 { return false }
func (p *privacyProcess) stopCompanion()                            {}
func (p *privacyProcess) companionCall(context.Context, []byte) any { return privacyRejected() }
