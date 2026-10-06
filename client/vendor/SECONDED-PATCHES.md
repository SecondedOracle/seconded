# Local credential-store patches

SECONDED pins go-keyring v0.2.6 and purego v0.10.0 in `client/go.mod` and
`client/go.sum`. The vendored go-keyring sources have two application patches:

- `SetProvider` installs the application's noninteractive macOS backend during
  initialization. Existing keyring mocks continue to override that provider in
  tests. The native implementation lives in `client/keychain_darwin.go` and uses
  purego so release binaries still build with `CGO_ENABLED=0`.
- Secret Service's prompt handler rejects every interaction-required object
  without calling `org.freedesktop.Secret.Prompt.Prompt` or waiting for its signal.

Preserve these patches when updating or regenerating vendor. Run the no-prompt
unit tests, mutation controls, and opt-in native keychain acceptance after changes.
The native implementation retains the existing encoded credential format and
read-back verification; it does not alter keychain ACLs or unlock keychains.

Reference: Apple's `SecKeychainSetUserInteractionAllowed` controls whether native
Keychain Services may display UI. It must be called in the process performing
the credential operation, so setting it in the parent does not protect a
`/usr/bin/security` subprocess.

https://developer.apple.com/documentation/security/seckeychainsetuserinteractionallowed(_:)
https://github.com/apple-oss-distributions/Security/blob/main/SecurityTool/macOS/security.c
https://github.com/ebitengine/purego/tree/v0.10.0

The upgrade-compatibility correction uses the native bridge only for metadata:
interaction is disabled, the target is opened/resolved, and SecKeychainGetStatus
must report unlocked before each read/write through `/usr/bin/security`. That
stable executable remains trusted across client rebuilds. The parent UI policy
does not cover the CLI; a lock between the check and subprocess access can still
prompt. The CLI has a deadline and OSStore retains read-back verification.
Vendor regeneration and surplus purego-file cleanup were deliberately not performed; no
dependency or vendored source was regenerated.
