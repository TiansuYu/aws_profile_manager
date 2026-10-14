BINARY := aws-profile-manager
BIN_DIR := bin
PKG := ./cmd/aws-profile-manager
VERSION ?= $(shell git describe --tags --always --dirty)
LDFLAGS := -X github.com/tiansuyu/aws_profile_manager/internal/cli.Version=$(VERSION)
GO_BIN := $(shell go env GOBIN)

ifeq ($(GO_BIN),)
GO_BIN := $(shell go env GOPATH)/bin
endif

.PHONY: build test vet fmt run install go-install uninstall clean

build:
	go build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/$(BINARY) $(PKG)

test:
	go test ./...

vet:
	go vet ./...

fmt:
	go fmt ./...

run:
	go run -ldflags "$(LDFLAGS)" $(PKG)

# Install into the Go bin dir (GOBIN or $(go env GOPATH)/bin) using the Go toolchain.
install go-install:
	go install -ldflags "$(LDFLAGS)" $(PKG)
	@echo "installed $(BINARY) -> $(GO_BIN)/$(BINARY)"
	@echo "ensure that dir is on your PATH"

uninstall:
	rm -f "$(GO_BIN)/$(BINARY)"

clean:
	rm -rf $(BIN_DIR)
