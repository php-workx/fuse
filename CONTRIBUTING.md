# Contributing to Fuse

## Reporting Bugs

Open a [bug report](https://github.com/php-workx/fuse/issues/new?template=bug_report.md) with:

- Fuse version (`fuse --version` or `go version`)
- Operating system and architecture
- Steps to reproduce
- Expected vs actual behavior
- Output of `fuse doctor`

## Suggesting Features

Open a [feature request](https://github.com/php-workx/fuse/issues/new?template=feature_request.md) with:

- Problem statement (what's painful today)
- Proposed solution
- Alternatives considered

## Development Setup

```bash
# Clone the repo
git clone https://github.com/php-workx/fuse.git
cd fuse

# Install tools and configure git hooks
just setup

# Verify everything works
just dev
```

### Prerequisites

- Go 1.25+
- [just](https://github.com/casey/just) (command runner)
- golangci-lint v2
- gofumpt

## Quality Gates

```bash
just dev          # Full local quality gate (fmt, vet, lint, test, vuln, semgrep, budgets)
just pre-commit   # Fast checks only (~15s)
just test         # Tests with race detector + coverage
just lint         # golangci-lint
```

All PRs must pass `just dev` before merge. The pre-commit hook runs `just pre-commit` automatically.

## PR Conventions

- **One feature per PR** — keep changes focused
- **Conventional commits** — `feat:`, `fix:`, `test:`, `docs:`, `chore:`
- **Tests required** for new functionality
- **Update CHANGELOG.md** for user-facing changes

## Code Style

- Formatter: gofumpt (stricter than gofmt)
- Linter: golangci-lint v2 (config in `.golangci.yml`)
- Suppression budgets: max 6 `//nolint`, 0 `#nosec` — enforced in CI
- All source is under `internal/` — nothing is exported as a library

## Code of Conduct

This project follows the [Contributor Covenant](CODE_OF_CONDUCT.md). Be respectful.
