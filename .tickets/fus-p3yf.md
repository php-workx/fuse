---
id: fus-p3yf
status: closed
deps: []
links: []
created: 2026-03-30T13:00:00Z
type: task
priority: 2
assignee:
parent:
tags: [windows, ci, phase-4]
---
# Add GOOS=windows golangci-lint to CI quality gate

## Problem

The CI conformance checks include `GOOS=windows go build` and `GOOS=windows go vet` but not `GOOS=windows golangci-lint run`. The linter catches issues that vet doesn't — unused parameters, cognitive complexity, type inconsistencies (like `bytes.Buffer` vs `strings.Builder`), and dead code.

Currently there are 5 Windows-specific files with ~500 lines of code that are not linted.

**Source:** Go Dev review missing item #5

## Where it surfaces

- `.github/workflows/ci.yml` — quality gate job
- `justfile` — `just lint` target

## Risk if unfixed

Windows code accumulates style and correctness issues that are caught on Unix but not on Windows. Platform drift becomes harder to detect.

## Acceptance Criteria

1. CI runs `GOOS=windows golangci-lint run ./...` (or equivalent)
2. Current Windows code passes the linter (fix any issues first)
3. `just lint` or a new `just lint-windows` target runs the check locally

## Test Cases

1. `GOOS=windows golangci-lint run ./...` exits 0
2. CI job includes the Windows lint step
