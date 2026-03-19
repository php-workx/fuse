# fuse quality gate
# Single source of truth for "my code is clean" — hooks and CI delegate here.

# Pinned tool versions — keep in sync with CI (.github/workflows/ci.yml)
golangci_lint_ver := "v2.11.3"
gofumpt_ver       := "v0.7.0"
govulncheck_ver   := "v1.1.4"
actionlint_ver    := "v1.7.11"

version    := `git describe --tags --always --dirty 2>/dev/null || echo "dev"`
commit     := `git rev-parse --short HEAD 2>/dev/null || echo "unknown"`
build_date := `date -u +"%Y-%m-%dT%H:%M:%SZ"`
ldflags    := "-X github.com/runger/fuse/internal/cli.Version=" + version + " -X github.com/runger/fuse/internal/cli.GitCommit=" + commit + " -X github.com/runger/fuse/internal/cli.BuildDate=" + build_date

default:
    @just --list

# --- Quality gates ---

# Pre-commit: fast local checks (~15s)
pre-commit: fmt vet lint build-check mod-tidy actionlint shellcheck gitleaks

# Local quality gate: pre-commit + tests + vuln + semgrep (no SonarQube)
check-local: pre-commit test vuln semgrep budgets

# Full quality gate: local + SonarQube
check: check-local sonar

# Developer shorthand: full local gate
dev: check-local
    @echo "All checks passed!"

# --- Static analysis ---

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

# Lint shell scripts with shellcheck
shellcheck:
    @if command -v shellcheck >/dev/null 2>&1; then \
        find scripts -name '*.sh' -o -name 'pre-commit' -o -name 'pre-push' | xargs shellcheck --; \
    else \
        echo "warning: shellcheck not installed, skipping"; \
    fi

# --- Security ---

# Scan for leaked secrets
gitleaks:
    @if command -v gitleaks >/dev/null 2>&1; then \
        gitleaks git --no-banner; \
    else \
        echo "warning: gitleaks not installed, skipping secret scan"; \
    fi

# SAST scan with semgrep (auto-config for Go patterns)
semgrep:
    @if command -v semgrep >/dev/null 2>&1; then \
        semgrep scan --config auto --error --quiet --exclude='testdata' .; \
    else \
        echo "warning: semgrep not installed, skipping SAST scan"; \
    fi

# Scan for known vulnerabilities in dependencies
vuln:
    @command -v govulncheck >/dev/null 2>&1 || (echo "govulncheck not installed (run: just install-dev)" && exit 1)
    govulncheck ./...

# Enforce suppression budgets (nolint/nosec counts)
budgets nolint_budget="6" nosec_budget="0":
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

# --- Testing ---

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

# Run all tests with race detector and coverage
test:
    go test -race -count=1 -coverprofile=coverage.out -covermode=atomic ./...
    go tool cover -html=coverage.out -o coverage.html
    @echo "Coverage report: coverage.html"

# --- SonarQube ---

# Run SonarQube: scan → terminal report → quality gate (fails if gate doesn't pass)
sonar:
    #!/usr/bin/env bash
    set -euo pipefail
    SONAR_URL="http://localhost:9000"
    PROJECT_KEY="fuse"

    if ! command -v sonar-scanner >/dev/null 2>&1; then
        echo "sonar-scanner not installed, skipping"
        exit 0
    fi
    if [ ! -f .env ]; then
        echo ".env missing, skipping sonar scan (run: just sonar-setup)"
        exit 0
    fi
    TOKEN=$(grep -E '^SONAR_TOKEN=[A-Za-z0-9_]+$' .env | cut -d= -f2)
    if [ -z "$TOKEN" ]; then
        echo "error: SONAR_TOKEN not found or invalid in .env"
        exit 1
    fi
    AUTH=(-H "Authorization: Bearer $TOKEN")

    # 1. Run sonar-scanner (coverage.out produced by `just test` in check-local)
    printf '\n=== SonarQube Scan ===\n'
    SONAR_TOKEN="$TOKEN" sonar-scanner -Dsonar.qualitygate.wait=true || true

    # 3. Report
    printf '\n=== Quality Gate ===\n'
    QG=$(curl -sf "${AUTH[@]}" "$SONAR_URL/api/qualitygates/project_status?projectKey=$PROJECT_KEY")
    STATUS=$(echo "$QG" | jq -r '.projectStatus.status')
    if [ "$STATUS" = "OK" ]; then
        printf '  Status: PASSED\n'
    elif [ "$STATUS" = "ERROR" ]; then
        printf '  Status: FAILED\n'
        echo "$QG" | jq -r '.projectStatus.conditions[] | select(.status == "ERROR") | "  ! \(.metricKey): \(.actualValue) (threshold: \(.errorThreshold))"'
    else
        printf '  Status: %s\n' "$STATUS"
    fi

    printf '\n=== Metrics ===\n'
    METRICS="bugs,vulnerabilities,code_smells,coverage,duplicated_lines_density,ncloc"
    curl -sf "${AUTH[@]}" "$SONAR_URL/api/measures/component?component=$PROJECT_KEY&metricKeys=$METRICS" \
        | jq -r '.component.measures[] | "  \(.metric): \(.value)"'

    printf '\n=== Open Issues (top 15) ===\n'
    ISSUES=$(curl -sf "${AUTH[@]}" "$SONAR_URL/api/issues/search?componentKeys=$PROJECT_KEY&statuses=OPEN,CONFIRMED&ps=15&s=SEVERITY&asc=false")
    TOTAL=$(echo "$ISSUES" | jq '.total')
    printf '  Total open: %s\n\n' "$TOTAL"
    echo "$ISSUES" | jq -r '.issues[] | "  \(.severity | ascii_downcase) | \(.component | split(":")[1] // .component):\(.line // "?") | \(.message | .[0:100])"'

    printf '\n  Full report: %s/dashboard?id=%s\n' "$SONAR_URL" "$PROJECT_KEY"

    # 4. Gate — exit non-zero if quality gate failed
    if [ "$STATUS" != "OK" ]; then
        exit 1
    fi

# Provision SonarQube: wait for server, create project, generate token
sonar-setup:
    #!/usr/bin/env bash
    set -euo pipefail
    SONAR_URL="http://localhost:9000"
    PROJECT_KEY="fuse"
    printf "Waiting for SonarQube at %s ..." "$SONAR_URL"
    for i in $(seq 1 30); do
        if curl -sf "$SONAR_URL/api/system/status" | grep -q '"status":"UP"'; then
            printf " ready.\n"
            break
        fi
        printf "."
        sleep 2
        if [ "$i" -eq 30 ]; then
            printf "\nSonarQube not reachable after 60s. Start it with:\n"
            printf "  docker run -d -p 9000:9000 sonarqube:community\n"
            exit 1
        fi
    done
    # Create project (idempotent).
    curl -sf -u admin:admin -X POST \
        "$SONAR_URL/api/projects/create?name=$PROJECT_KEY&project=$PROJECT_KEY" \
        -o /dev/null || true
    # Revoke old token, generate new one.
    curl -sf -u admin:admin -X POST \
        "$SONAR_URL/api/user_tokens/revoke?name=fuse-local" -o /dev/null || true
    TOKEN=$(curl -sf -u admin:admin -X POST \
        "$SONAR_URL/api/user_tokens/generate?name=fuse-local" \
        | grep -o '"token":"[^"]*"' | cut -d'"' -f4)
    if [ -z "$TOKEN" ]; then
        echo "Failed to generate token. Check SonarQube credentials (default: admin/admin)."
        exit 1
    fi
    # Write token to .env.
    if [ -f .env ]; then
        grep -v '^SONAR_TOKEN=' .env > .env.tmp || true
        mv .env.tmp .env
    fi
    echo "SONAR_TOKEN=$TOKEN" >> .env
    # Set new code period to main branch.
    curl -sf -H "Authorization: Bearer $TOKEN" -X POST \
        "$SONAR_URL/api/new_code_periods/set?project=$PROJECT_KEY&type=REFERENCE_BRANCH&value=main"
    echo "Done. Token written to .env, new code period set to main. Run: just sonar"

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

# Full developer setup: install tools, configure git hooks
setup: install-dev
    git config core.hooksPath scripts
    @echo "Git hooks configured (scripts/)"

# Install development tools and git hooks
install-dev:
    @echo "Installing Go tools..."
    go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@{{golangci_lint_ver}}
    go install golang.org/x/vuln/cmd/govulncheck@{{govulncheck_ver}}
    go install mvdan.cc/gofumpt@{{gofumpt_ver}}
    go install github.com/rhysd/actionlint/cmd/actionlint@{{actionlint_ver}}
    go install golang.org/x/tools/cmd/goimports@latest
    go install golang.org/x/tools/cmd/deadcode@latest
    @echo "Installing git hooks..."
    @bash scripts/install-hooks.sh
    @echo "Done! Development environment ready."

# Format all Go files in-place (use when `just fmt` fails)
format:
    gofumpt --extra -w .

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
