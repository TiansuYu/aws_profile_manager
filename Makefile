.DEFAULT_GOAL := help

BINARY := aws-profile-manager
BIN_DIR := bin
PKG := ./cmd/aws-profile-manager
VERSION ?= $(shell git describe --tags --always --dirty)
LDFLAGS := -X github.com/tiansuyu/aws_profile_manager/internal/cli.Version=$(VERSION)
GO_BIN := $(shell go env GOBIN)

ifeq ($(GO_BIN),)
GO_BIN := $(shell go env GOPATH)/bin
endif

.PHONY: help build test test-race vet fmt fmt-check lint lint-go secrets hooks-install check run install go-install uninstall clean

help: ## Show available commands (also the default target)
	@awk 'BEGIN {FS = ":.*## "} /^[a-zA-Z_-]+:.*## / {printf "  %-16s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

build: ## Build the binary in bin/ with Git version metadata
	go build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/$(BINARY) $(PKG)

test: ## Run unit and compiled-CLI integration tests
	go test ./...

test-race: ## Run all tests with the race detector
	go test -race ./...

vet: ## Run go vet
	go vet ./...

fmt: ## Format Go sources
	gofmt -w cmd internal

fmt-check: ## Check Go formatting without changing files
	@unformatted=$$(gofmt -l cmd internal) || exit $$?; \
	if [ -n "$$unformatted" ]; then \
		printf 'Unformatted Go files:\n%s\nRun make fmt to fix them.\n' "$$unformatted"; \
		exit 1; \
	fi

lint: ## Run all prek checks, including a working-tree secret scan
	prek run --all-files --hook-stage manual

lint-go: ## Run golangci-lint (install v2.14 separately)
	golangci-lint run

secrets: ## Scan the working tree for secrets with managed Gitleaks
	prek run gitleaks-tree --all-files --hook-stage manual

hooks-install: ## Install prek's pre-commit hook in this clone
	prek install

check: lint test-race ## Run basic lint, secret scanning, and race tests

run: ## Start the interactive picker
	go run -ldflags "$(LDFLAGS)" $(PKG)

install: go-install ## Install the binary into GOBIN or GOPATH/bin

go-install:
	go install -ldflags "$(LDFLAGS)" $(PKG)
	@echo "installed $(BINARY) -> $(GO_BIN)/$(BINARY)"
	@echo "ensure that dir is on your PATH"

uninstall: ## Remove the installed binary
	rm -f "$(GO_BIN)/$(BINARY)"

clean: ## Remove local build and release artifacts
	rm -rf $(BIN_DIR) dist
