# fuse quality gate
# Single source of truth for "my code is clean" — hooks and CI delegate here.

# Pinned tool versions — keep in sync with CI (.github/workflows/ci.yml)
golangci_lint_ver := "v2.11.3"
gofumpt_ver       := "v0.7.0"
govulncheck_ver   := "v1.1.4"
actionlint_ver    := "v1.7.11"
gotestfmt_ver     := "v2.5.0"

version    := `git describe --tags --always --dirty 2>/dev/null || echo "dev"`
commit     := `git rev-parse --short HEAD 2>/dev/null || echo "unknown"`
build_date := `date -u +"%Y-%m-%dT%H:%M:%SZ"`
ldflags    := "-X github.com/runger/fuse/internal/cli.Version=" + version + " -X github.com/runger/fuse/internal/cli.GitCommit=" + commit + " -X github.com/runger/fuse/internal/cli.BuildDate=" + build_date

default:
    @just --list

# --- Quality gate (pre-commit: fast checks) ---

# Run all pre-commit checks (format, vet, lint, build, mod-tidy, workflow lint, secrets)
pre-commit: fmt vet lint build-check mod-tidy actionlint gitleaks

# Run full quality gate (pre-push: pre-commit + tests + vuln)
check: pre-commit test vuln

# Run everything including release checks
dev: check
    @echo "All checks passed!"

# --- Individual checks ---

# Check formatting with gofumpt (detect-only, no auto-fix)
fmt:
    @command -v gofumpt >/dev/null 2>&1 || (echo "gofumpt not installed (run: just install-dev)" && exit 1)
    @test -z "$(gofumpt --extra -l .)" || (echo "gofumpt: unformatted files:" && gofumpt --extra -l . && exit 1)

# Go vet
vet:
    go vet ./...

# Lint with golangci-lint
lint:
    @command -v golangci-lint >/dev/null 2>&1 || (echo "golangci-lint not installed (run: just install-dev)" && exit 1)
    golangci-lint run

# Lint GitHub Actions workflows
actionlint:
    @if [ -d .github/workflows ]; then \
        command -v actionlint >/dev/null 2>&1 || (echo "actionlint not installed (run: just install-dev)" && exit 1); \
        actionlint .github/workflows/*.yml; \
    fi

# Scan for leaked secrets
gitleaks:
    @if command -v gitleaks >/dev/null 2>&1; then \
        gitleaks git --no-banner; \
    else \
        echo "warning: gitleaks not installed, skipping secret scan"; \
    fi

# Verify the project compiles (fast, no binary output)
build-check:
    go build ./...

# Verify go.mod and go.sum are tidy (detect-only)
mod-tidy:
    @cp go.mod go.mod.bak
    @if [ -f go.sum ]; then cp go.sum go.sum.bak; fi
    @go mod tidy
    @DIRTY=0; \
        diff -q go.mod go.mod.bak >/dev/null 2>&1 || DIRTY=1; \
        if [ -f go.sum.bak ]; then diff -q go.sum go.sum.bak >/dev/null 2>&1 || DIRTY=1; \
        elif [ -f go.sum ]; then DIRTY=1; fi; \
        mv go.mod.bak go.mod; \
        if [ -f go.sum.bak ]; then mv go.sum.bak go.sum; elif [ -f go.sum ]; then rm go.sum; fi; \
        if [ "$$DIRTY" = "1" ]; then echo "go.mod/go.sum not tidy — run 'go mod tidy'" && exit 1; fi

# Run all tests with race detector
test:
    go test -race -count=1 ./...

# Run tests with coverage report
cover:
    go test -race -coverprofile=coverage.out -covermode=atomic ./...
    go tool cover -html=coverage.out -o coverage.html
    @echo "Coverage report: coverage.html"

# Scan for known vulnerabilities
vuln:
    @command -v govulncheck >/dev/null 2>&1 || (echo "govulncheck not installed (run: just install-dev)" && exit 1)
    govulncheck ./...

# Enforce suppression budgets (nolint/nosec counts)
budgets nolint_budget="4" nosec_budget="0":
    #!/usr/bin/env bash
    set -euo pipefail
    nolint_count="$( (git grep -n '//nolint' -- '*.go' || true) | wc -l | tr -d '[:space:]' )"
    nosec_count="$( (git grep -n '#nosec' -- '*.go' || true) | wc -l | tr -d '[:space:]' )"
    echo "//nolint: ${nolint_count} (budget: {{nolint_budget}})"
    echo "#nosec:  ${nosec_count} (budget: {{nosec_budget}})"
    if [ "${nolint_count}" -gt "{{nolint_budget}}" ]; then
        echo "Error: //nolint count exceeded budget."
        exit 1
    fi
    if [ "${nosec_count}" -gt "{{nosec_budget}}" ]; then
        echo "Error: #nosec count exceeded budget."
        exit 1
    fi

# --- Build targets ---

# Build the fuse binary
build:
    mkdir -p bin
    go build -ldflags '{{ldflags}}' -o bin/fuse ./cmd/fuse

# Install fuse to $GOPATH/bin
install:
    go install -ldflags '{{ldflags}}' ./cmd/fuse

# Cross-build for all platforms (linux + macOS, no Windows — fuse uses Unix TTY)
build-all: build-linux build-darwin

build-linux:
    GOOS=linux GOARCH=amd64 go build -ldflags '{{ldflags}}' -o bin/fuse-linux-amd64 ./cmd/fuse
    GOOS=linux GOARCH=arm64 go build -ldflags '{{ldflags}}' -o bin/fuse-linux-arm64 ./cmd/fuse

build-darwin:
    GOOS=darwin GOARCH=amd64 go build -ldflags '{{ldflags}}' -o bin/fuse-darwin-amd64 ./cmd/fuse
    GOOS=darwin GOARCH=arm64 go build -ldflags '{{ldflags}}' -o bin/fuse-darwin-arm64 ./cmd/fuse

# --- Setup ---

# Install development tools and git hooks
install-dev:
    @echo "Installing Go tools..."
    go install github.com/golangci/golangci-lint/cmd/golangci-lint@{{golangci_lint_ver}}
    go install golang.org/x/vuln/cmd/govulncheck@{{govulncheck_ver}}
    go install mvdan.cc/gofumpt@{{gofumpt_ver}}
    go install golang.org/x/tools/cmd/goimports@latest
    go install golang.org/x/tools/cmd/deadcode@latest
    go install gotest.tools/gotestsum@v1.12.1
    @echo "Installing git hooks..."
    @bash scripts/install-hooks.sh
    @echo "Done! Development environment ready."

# Remove build artifacts
clean:
    rm -rf bin/ coverage.out coverage.html
    go clean

# Download and tidy dependencies
deps:
    go mod download
    go mod tidy

# Build and run with arguments
run *args: build
    ./bin/fuse {{args}}
