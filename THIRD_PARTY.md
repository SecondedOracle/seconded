# Third-party licences

SECONDED's Apache-2.0 licence does not replace the licences of its dependencies.
The following inventory covers every module in `client/vendor/modules.txt`.
Licence names were checked against each vendored module's own LICENSE file;
the linked files preserve the upstream copyright notices and terms.

| Module | Vendored version | Licence | Upstream licence in this tree |
| --- | --- | --- | --- |
| `al.essio.dev/pkg/shellescape` | `v1.5.1` | MIT | [LICENSE](client/vendor/al.essio.dev/pkg/shellescape/LICENSE) |
| `github.com/cyberphone/json-canonicalization` | `v0.0.0-20241213102144-19d51d7fe467` | Apache-2.0 | [LICENSE](client/vendor/github.com/cyberphone/json-canonicalization/LICENSE) |
| `github.com/danieljoos/wincred` | `v1.2.2` | MIT | [LICENSE](client/vendor/github.com/danieljoos/wincred/LICENSE) |
| `github.com/decred/dcrd/dcrec/secp256k1/v4` | `v4.4.0` | ISC | [LICENSE](client/vendor/github.com/decred/dcrd/dcrec/secp256k1/v4/LICENSE) |
| `github.com/ebitengine/purego` | `v0.10.0` | Apache-2.0 | [LICENSE](client/vendor/github.com/ebitengine/purego/LICENSE) |
| `github.com/godbus/dbus/v5` | `v5.1.0` | BSD-2-Clause | [LICENSE](client/vendor/github.com/godbus/dbus/v5/LICENSE) |
| `github.com/zalando/go-keyring` | `v0.2.6` | MIT | [LICENSE](client/vendor/github.com/zalando/go-keyring/LICENSE) |
| `golang.org/x/crypto` | `v0.56.0` | BSD-3-Clause | [LICENSE](client/vendor/golang.org/x/crypto/LICENSE) |
| `golang.org/x/sys` | `v0.47.0` | BSD-3-Clause | [LICENSE](client/vendor/golang.org/x/sys/LICENSE) |

The standalone verifier also includes a copy of the JSON canonicalizer under
`verifier/internal/jsoncanonicalizer`, licensed under Apache-2.0, copyright 2018
Anders Rundgren. See its [LICENSE](verifier/internal/jsoncanonicalizer/LICENSE)
and [provenance](verifier/internal/jsoncanonicalizer/README.md).

The purego module includes Go-derived code with its own retained BSD notices;
see [internal/fakecgo](client/vendor/github.com/ebitengine/purego/internal/fakecgo).

## Vendored documentation links

The vendored source and attribution are preserved from the released client. Some
upstream documentation links have aged: the shellescape gocover badge/coverage links,
purego's rules_go issue link and the godbus workflow badge returned 404 in the
2026-10-19 audit. The godbus contribution guide references `README.markdown`, while
the vendored file is [README.md](client/vendor/github.com/godbus/dbus/v5/README.md).
The Sourcegraph badge timed out; the Decred SignMessage and Zalando security-form
fragments were not confirmed in returned HTML. Those checks do not establish that
the underlying packages or services are unavailable.

Use the module identities and retained licence files above for attribution. Repairing
upstream link text belongs in a reviewed dependency-documentation refresh; it does
not require or justify changing the frozen runtime sources.
