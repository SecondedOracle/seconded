package client

import (
	"bufio"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"time"
)

var errPresenceUnavailable = errors.New("owner_presence_unavailable")

// ConfirmOwnerPresence adds biometrics or administrator-configured approval to
// the caller's existing TTY confirmation. With neither available, that TTY
// confirmation is the fallback and does not stop an agent with shell access.
var authenticateOwnerPresence = platformOwnerPresence
var loadOwnerPresenceApprovalKey = ownerApprovalKey

func ConfirmOwnerPresence(reason string, input io.Reader, output io.Writer) error {
	return confirmOwnerPresence(reason, input, output, authenticateOwnerPresence, loadOwnerPresenceApprovalKey)
}

func confirmOwnerPresence(reason string, input io.Reader, output io.Writer, presence func(string) error, key func() (ed25519.PublicKey, error)) error {
	err := presence(reason)
	if err == nil {
		return nil
	}
	if !errors.Is(err, errPresenceUnavailable) {
		return ErrTerminalPolicyRequired
	}
	public, err := key()
	if errors.Is(err, os.ErrNotExist) && len(public) == 0 {
		return nil // The caller already required the six-character TTY confirmation.
	}
	if err != nil || len(public) != ed25519.PublicKeySize {
		return ErrTerminalPolicyRequired
	}
	return signedOwnerApproval(reason, input, output, public, time.Now)
}

func signedOwnerApproval(reason string, input io.Reader, output io.Writer, public ed25519.PublicKey, now func() time.Time) error {
	nonce := make([]byte, 32)
	if _, err := rand.Read(nonce); err != nil {
		return ErrTerminalPolicyRequired
	}
	expires := now().Add(2 * time.Minute)
	challenge, err := json.Marshal(struct {
		Domain  string `json:"domain"`
		Reason  string `json:"reason"`
		Nonce   string `json:"nonce"`
		Expires int64  `json:"expires"`
	}{"seconded-owner-approval-v1", reason, base64.RawStdEncoding.EncodeToString(nonce), expires.Unix()})
	if err != nil {
		return ErrTerminalPolicyRequired
	}
	if _, err = fmt.Fprintf(output, "Offline owner approval required. Verify the action on your separate signing device.\nChallenge: %s\nApproval signature: ", base64.RawStdEncoding.EncodeToString(challenge)); err != nil {
		return ErrTerminalPolicyRequired
	}
	answer, err := bufio.NewReader(io.LimitReader(input, 128)).ReadString('\n')
	if err != nil || len(answer) < 2 {
		return ErrTerminalPolicyRequired
	}
	signature, err := base64.RawStdEncoding.DecodeString(answer[:len(answer)-1])
	if err != nil || !now().Before(expires) || !ed25519.Verify(public, challenge, signature) {
		return ErrTerminalPolicyRequired
	}
	return nil
}
