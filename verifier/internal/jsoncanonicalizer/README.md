# jsoncanonicalizer (vendored copy)

Unmodified copy of `github.com/cyberphone/json-canonicalization` at
`v0.0.0-20241213102144-19d51d7fe467`, path
`go/src/webpki.org/jsoncanonicalizer`, Apache-2.0 (see `LICENSE`).

It implements RFC 8785 (JSON Canonicalization Scheme). The SECONDED client vendors the
same version, so the verifier and the client canonicalize an envelope byte for byte the
same way. The copy lives under `internal/` so the verifier module has no external
dependencies and builds with `GOPROXY=off`.
