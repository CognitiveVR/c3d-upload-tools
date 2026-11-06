GOLANGCI_LINT_VERSION ?= v2.12.2

BINARY_NAME = cognitive3d-cli-upload
VERSION ?= $(shell git describe --tags --exact-match 2>/dev/null | sed "s/^v//" | grep . || echo "0.0.0-dev")
COMMIT = $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
DATE = $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
PKG = github.com/cognitivevr/c3d-upload-tools/internal/version
LDFLAGS = -ldflags "-s -w -X $(PKG).Version=$(VERSION) -X $(PKG).Commit=$(COMMIT) -X $(PKG).Date=$(DATE)"

.PHONY: help build clean test lint fmt vet install-hooks

help: ## Show this help message
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  %-15s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

build: ## Build the binary for the current platform
	go build $(LDFLAGS) -o $(BINARY_NAME) .

clean: ## Remove build artifacts
	rm -f $(BINARY_NAME) coverage.out
	rm -rf dist/

test: ## Run tests with the race detector
	go test -race -shuffle=on -coverprofile=coverage.out ./...

lint: ## Run golangci-lint (pinned version in GOLANGCI_LINT_VERSION)
	@if command -v golangci-lint > /dev/null; then \
		golangci-lint run ./...; \
	else \
		echo "golangci-lint not installed. Install with: go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)"; \
		exit 1; \
	fi

fmt: ## Format code
	go fmt ./...

vet: ## Run go vet
	go vet ./...

install-hooks: ## Install the git pre-commit hook
	git config core.hooksPath .githooks
