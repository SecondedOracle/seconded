package client

import (
	"sync"
	"time"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
)

var localAuthentication = sync.OnceValues(func() (uintptr, error) {
	return purego.Dlopen("/System/Library/Frameworks/LocalAuthentication.framework/LocalAuthentication", purego.RTLD_NOW|purego.RTLD_LOCAL)
})

func platformOwnerPresence(reason string) (err error) {
	// Framework/symbol failures must not authorize the action.
	defer func() {
		if recover() != nil {
			err = ErrTerminalPolicyRequired
		}
	}()
	if _, err := localAuthentication(); err != nil {
		return errPresenceUnavailable
	}
	context := objc.ID(objc.GetClass("LAContext")).Send(objc.RegisterName("alloc")).Send(objc.RegisterName("init"))
	if context == 0 {
		return errPresenceUnavailable
	}
	defer context.Send(objc.RegisterName("release"))
	defer context.Send(objc.RegisterName("invalidate"))
	// Biometrics only: no password typed through a scriptable terminal, no reuse
	// of another application's authentication, and a fresh context per action.
	const biometricPolicy = 1
	context.Send(objc.RegisterName("setTouchIDAuthenticationAllowableReuseDuration:"), float64(0))
	var authError objc.ID
	if !objc.Send[bool](context, objc.RegisterName("canEvaluatePolicy:error:"), int64(biometricPolicy), &authError) {
		return errPresenceUnavailable
	}
	text := objc.ID(objc.GetClass("NSString")).Send(objc.RegisterName("alloc")).Send(objc.RegisterName("initWithUTF8String:"), reason)
	if text == 0 {
		return ErrTerminalPolicyRequired
	}
	defer text.Send(objc.RegisterName("release"))
	result := make(chan bool, 1)
	reply := objc.NewBlock(func(_ objc.Block, success bool, _ objc.ID) {
		select {
		case result <- success:
		default:
		}
	})
	defer reply.Release()
	context.Send(objc.RegisterName("evaluatePolicy:localizedReason:reply:"), int64(biometricPolicy), text, reply)
	timer := time.NewTimer(90 * time.Second)
	defer timer.Stop()
	select {
	case success := <-result:
		if success {
			return nil
		}
	case <-timer.C:
	}
	return ErrTerminalPolicyRequired
}
