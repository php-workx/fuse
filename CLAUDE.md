# CLAUDE.md — Fuse

## What is Fuse?

Fuse is a local safety runtime for AI coding agents (Claude Code, Codex CLI). It classifies shell commands and MCP tool calls into four decision levels — `SAFE`, `CAUTION`, `APPROVAL`, `BLOCKED` — and gates execution accordingly. Everything runs locally with no cloud dependencies.

## Tech Stack

- **Language:** Go 1.24.12
- **Build:** `just` (not Make) — see `justfile` for all targets
- **Platforms:** macOS (darwin/arm64, amd64), Linux (linux/amd64, arm64). No Windows — uses Unix TTY.
- **Dependencies:** Minimal. Pure-Go SQLite (`modernc.org/sqlite`), shell parser (`mvdan.cc/sh`), Cobra CLI, YAML. No RPC/cloud SDKs.

## Project Layout

```
cmd/fuse/              Entry point (thin main.go)
internal/
  cli/                 Cobra commands: install, doctor, run, proxy, hook, events, dryrun, enable/disable
  core/                Classification pipeline, normalization, safe-command detection, sanitization
  policy/              Built-in preset rules (git, aws, gcp, k8s, security) + user YAML policy evaluation
  adapters/            Runtime adapters:
    hook.go              Claude Code PreToolUse hook (classify-only)
    codexshell.go        Codex shell MCP server (concurrent JSON-RPC)
    mcpproxy.go          Generic MCP proxy with tool interception
    runner.go            Shell execution wrapper (run mode)
  approve/             Approval lifecycle: HMAC-signed records, TUI prompt via /dev/tty
  config/              YAML config loading, XDG paths
  db/                  SQLite state storage (WAL mode, schema migrations v1-v4, event log)
  inspect/             File inspection for scripts (shell, python, javascript)
  events/              Event types and formatting
  releasecheck/        Shell compatibility release checks (bash/zsh/fish)
testdata/fixtures/     Test fixtures including commands.yaml
specs/                 Technical and functional specifications
```

## Development Workflow

### Setup

```bash
just setup          # Install tools + configure git hooks
```

### Quality Gates

```bash
just dev            # Full local quality gate (fmt, vet, lint, test, vuln, semgrep, budgets)
just pre-commit     # Fast checks only (~15s)
just check          # Full gate including SonarQube
```

### Common Commands

```bash
just test           # Tests with race detector + coverage report
just build          # Build binary to bin/fuse
just format         # Auto-fix formatting (when `just fmt` fails)
just lint           # golangci-lint
just vuln           # govulncheck
just budgets        # Enforce suppression budgets (max 6 //nolint, 0 #nosec)
```

### Running

```bash
just run -- <args>  # Build and run with arguments
```

## Architecture

### Classification Pipeline

1. **Normalize** — Two-level normalization (display string vs classification string)
2. **BLOCKED check** — Hardcoded self-protection rules (e.g., modifying `.claude/settings.json`)
3. **User policy** — Evaluate rules from user's `policy.yaml`
4. **Built-in presets** — Evaluate preset rules (git, aws, gcp, k8s, security, etc.)
5. **File inspection** — Inspect script contents when rules trigger it
6. **Decision** — Output SAFE/CAUTION/APPROVAL/BLOCKED

### Adapter Pattern

Three runtime modes, all feeding the same classification core:
- **Hook mode** (`fuse hook`) — Claude Code `PreToolUse` hook. Classify-only, no execution.
- **Codex shell** (`fuse codex-shell`) — MCP server for Codex CLI. Concurrent goroutine-per-request.
- **MCP proxy** (`fuse proxy`) — Generic MCP proxy managing downstream server lifecycle.
- **Run mode** (`fuse run`) — Direct shell execution with TOCTOU checks and sanitization.

### Approval System

- HMAC-signed approval records stored in SQLite
- Scopes: `once`, `command`, `session`, `forever`
- TUI prompt rendered via `/dev/tty` (can't be spoofed by piped input)
- Context-aware timeouts (25s hook mode, 5min run mode)

### Tag System

Built-in rules are organized by tags (e.g., `git`, `aws`, `security`). Users can override rule behavior per-tag via `tag_overrides` in `policy.yaml`, including enabling/disabling entire tag groups and overriding decision levels.

## Testing

- **Integration tests** — `integration_test.go` at repo root (full hook/run flow)
- **Unit tests** — Per-package `*_test.go` files throughout `internal/`
- **Release checks** — `internal/releasecheck/` tests shell compatibility (bash/zsh/fish) gated by `FUSE_RELEASE_CHECK=1`
- **Fixture coverage** — `internal/core/fixture_coverage_test.go` ensures test fixtures in `testdata/` are exercised
- Run with: `just test` (includes race detector and coverage)

## CI/CD

GitHub Actions (`.github/workflows/ci.yml`) with three parallel jobs:
1. **Quality Gate** — fmt, vet, lint, build, mod-tidy, test, vuln, budgets, codecov
2. **Shell Compatibility** — Ubuntu + macOS matrix testing bash/zsh/fish
3. **Cross-build** — linux/darwin x amd64/arm64 (depends on gate + compat passing)

## Code Quality Standards

- **Formatter:** gofumpt (stricter than gofmt)
- **Linter:** golangci-lint v2 (config in `.golangci.yml`)
- **Suppression budgets:** Max 6 `//nolint` directives, 0 `#nosec` — enforced in CI
- **Security scanning:** semgrep (SAST), gitleaks (secrets), govulncheck (dependencies)
- **Pre-commit hooks:** Configured via `scripts/` directory

## Key Conventions

- All source is under `internal/` — nothing is exported as a library
- Config files live in XDG directories (`~/.config/fuse/`)
- State (SQLite DB) lives in XDG data (`~/.local/share/fuse/`)
- Platform-specific code uses build tags (e.g., `ioctl_darwin.go`, `ioctl_linux.go`)
- TTY handling for approval prompts uses `/dev/tty` directly
