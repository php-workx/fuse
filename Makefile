# Makefile for fuse

BINARY_NAME=fuse
VERSION?=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
GIT_COMMIT?=$(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE?=$(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS=-ldflags "-X github.com/runger/fuse/internal/cli.Version=$(VERSION) -X github.com/runger/fuse/internal/cli.GitCommit=$(GIT_COMMIT) -X github.com/runger/fuse/internal/cli.BuildDate=$(BUILD_DATE)"

.PHONY: all build install install-dev install-hooks clean test cover fmt format-check workflow-lint lint vuln check-fast check-pre-push dev deps run help build-all build-linux build-darwin

all: build

## build: Build the fuse binary
build:
	go build $(LDFLAGS) -o bin/fuse ./cmd/fuse

## install: Install fuse to $GOPATH/bin
install:
	go install $(LDFLAGS) ./cmd/fuse

## install-dev: Install development dependencies
install-dev:
	@echo "Installing Go tools..."
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	go install golang.org/x/vuln/cmd/govulncheck@latest
	go install golang.org/x/tools/cmd/goimports@latest
	go install golang.org/x/tools/cmd/deadcode@latest
	go install github.com/rhysd/actionlint/cmd/actionlint@latest
	go install gotest.tools/gotestsum@v1.12.1
	@echo "Done! Development environment ready."

## install-hooks: Configure repository-managed git hooks
install-hooks:
	bash ./scripts/install-git-hooks.sh

## clean: Remove build artifacts
clean:
	rm -rf bin/
	go clean

## test: Run all tests with race detector
test:
	@if command -v gotestsum >/dev/null 2>&1; then \
		gotestsum --format testdox -- -race ./...; \
	else \
		go test -race -v ./...; \
	fi

## cover: Run all tests with coverage
cover:
	go test -race -coverprofile=coverage.out -covermode=atomic ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

## fmt: Format code
fmt:
	go fmt ./...

## format-check: Verify gofmt/goimports formatting
format-check:
	bash ./scripts/check-format.sh

## workflow-lint: Validate GitHub Actions workflows
workflow-lint:
	bash ./scripts/check-workflows.sh

## lint: Run linter
lint:
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run; \
	else \
		echo "Error: golangci-lint not installed."; \
		echo "Install with: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"; \
		echo "Or run: make install-dev"; \
		exit 1; \
	fi

## vuln: Scan for vulnerabilities
vuln:
	@if command -v govulncheck >/dev/null 2>&1; then \
		govulncheck ./...; \
	else \
		echo "Error: govulncheck not installed."; \
		echo "Install with: go install golang.org/x/vuln/cmd/govulncheck@latest"; \
		echo "Or run: make install-dev"; \
		exit 1; \
	fi

## check-fast: Run fast local hygiene checks
check-fast:
	bash ./scripts/check-fast.sh

## check-pre-push: Run the slow pre-push verification suite
check-pre-push:
	bash ./scripts/check-pre-push.sh

## dev: Run all checks (fmt, lint, test, vuln)
dev: check-pre-push
	@echo "All checks passed!"

## deps: Download dependencies
deps:
	go mod download
	go mod tidy

## run: Build and run with arguments
run: build
	./bin/$(BINARY_NAME) $(ARGS)

## help: Show this help message
help:
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/## /  /'

# Cross-compilation targets

## build-all: Build for all platforms (linux + macOS, no Windows — fuse uses Unix TTY)
build-all: build-linux build-darwin

build-linux:
	GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o bin/$(BINARY_NAME)-linux-amd64 ./cmd/fuse
	GOOS=linux GOARCH=arm64 go build $(LDFLAGS) -o bin/$(BINARY_NAME)-linux-arm64 ./cmd/fuse

build-darwin:
	GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) -o bin/$(BINARY_NAME)-darwin-amd64 ./cmd/fuse
	GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o bin/$(BINARY_NAME)-darwin-arm64 ./cmd/fuse
