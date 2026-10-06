GO := go
VERSION ?= 0.1.0-test2
export CGO_ENABLED := 0
export GOTOOLCHAIN := go1.26.7
export GOPROXY := off

.PHONY: build test vet cross
build:
	$(GO) build -mod=vendor -trimpath -buildvcs=false -ldflags '-s -w -buildid= -X main.version=$(VERSION) -X seconded.local/client.Version=$(VERSION)' -o dist/seconded-mcp ./cmd/seconded-mcp
test:
	$(GO) test -mod=vendor ./...
vet:
	$(GO) vet -mod=vendor ./...
cross:
	GOOS=linux GOARCH=amd64 $(GO) build -mod=vendor -trimpath -buildvcs=false -ldflags '-s -w -buildid= -X main.version=$(VERSION) -X seconded.local/client.Version=$(VERSION)' -o dist/seconded-mcp_linux_amd64 ./cmd/seconded-mcp
	GOOS=linux GOARCH=arm64 $(GO) build -mod=vendor -trimpath -buildvcs=false -ldflags '-s -w -buildid= -X main.version=$(VERSION) -X seconded.local/client.Version=$(VERSION)' -o dist/seconded-mcp_linux_arm64 ./cmd/seconded-mcp
	GOOS=windows GOARCH=amd64 $(GO) build -mod=vendor -trimpath -buildvcs=false -ldflags '-s -w -buildid= -X main.version=$(VERSION) -X seconded.local/client.Version=$(VERSION)' -o dist/seconded-mcp_windows_amd64.exe ./cmd/seconded-mcp
	GOOS=darwin GOARCH=amd64 $(GO) build -mod=vendor -trimpath -buildvcs=false -ldflags '-s -w -buildid= -X main.version=$(VERSION) -X seconded.local/client.Version=$(VERSION)' -o dist/seconded-mcp_darwin_amd64 ./cmd/seconded-mcp
