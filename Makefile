# Kapibara Makefile.
GO ?= go
BIN ?= bin/kapibara
PKG := github.com/orcinustools/kapibara
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
LDFLAGS := -X $(PKG)/pkg/version.Version=$(VERSION) -X $(PKG)/pkg/version.GitCommit=$(COMMIT)

.PHONY: all build build-go ui test e2e tidy lint clean run

all: build

# Build the React SPA (output embedded into pkg/webui/dist) then the Go binary.
build: ui build-go

ui:
	cd web && npm install && npm run build

build-go:
	CGO_ENABLED=0 $(GO) build -ldflags "$(LDFLAGS)" -o $(BIN) ./cmd/kapibara

test:
	$(GO) test ./...

# End-to-end tests that talk to a live orcinus API (set KAPIBARA_E2E=1).
e2e:
	KAPIBARA_E2E=1 $(GO) test ./test/e2e/... -v -timeout 20m

tidy:
	$(GO) mod tidy

clean:
	rm -rf bin

run: build
	./$(BIN) serve
