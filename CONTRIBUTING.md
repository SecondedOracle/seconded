# Contributing

Thanks for looking. This repository is the public surface of SECONDED: the Go MCP
client, the offline receipt verifier, the shipped tool and price catalogs, and the
documentation. The service itself is not developed here.

## What we take

- **Bug reports** against the client or verifier, with a reproduction.
- **Documentation fixes** where the docs disagree with the code or catalogs.
- **Verifier improvements**: more negative test vectors, more host or language ports,
  clearer output. The verifier is deliberately small; keep it that way.
- **Host configuration recipes** under `examples/` for MCP hosts we do not cover yet.

## What we do not take here

- Changes to prices, product catalogs, receipt formats or key pins. Those files are
  generated from the service and are replaced wholesale on each release. A pull
  request that edits `client/products.json`, `client/tools-list.json` or
  `client/privacy-tools.json` by hand will be closed with a pointer to this paragraph.
- New paid products or server behaviour. Open an issue describing the need instead.

## Building and testing

```sh
cd client   && go build ./... && go test ./...
cd verifier && go build ./... && go test ./...
```

The client vendors its dependencies, so both commands run with `GOPROXY=off`.
The `go.mod` files name the Go toolchain they were tested with.

## Pull requests

- One change per pull request. Say what you verified and how.
- Do not commit wallet profiles, ledgers, keys, receipts from your own paid checks, or
  anything under a `dist/` directory.
- CI builds the client and runs the verifier tests on `ubuntu-latest`. A red build is a
  red build; please do not ask for a merge around it.

## Licence

There is no licence file in this repository yet. Until one is added, contributions are
accepted on the understanding that the maintainers may publish them under the licence
eventually chosen for the repository.
