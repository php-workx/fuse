---
id: fus-p50r
status: closed
deps: []
links: []
created: 2026-03-31T06:09:43Z
type: task
priority: 2
assignee: Ronny Unger
tags: [ci, windows, phase-4]
---
# Add GOOS=windows golangci-lint step to CI

The justfile has a 'just lint-windows' target (added in Phase 4) but the CI workflow .github/workflows/ci.yml was not updated to call it. The windows-check job at ci.yml:100-119 only has 'go vet' and 'go build' steps with GOOS=windows. No golangci-lint step is present.

This means ~500 lines of Windows-specific code across 5+ files are not linted in CI. Lint errors (unused parameters, cognitive complexity, type inconsistencies) accumulate undetected.

Ticket fus-p3yf (closed) had AC-1: 'CI runs GOOS=windows golangci-lint run' — this criterion was not met. The justfile target was added but CI was not updated.

Found by: our spec-verification explorer.

Files: .github/workflows/ci.yml

Test cases:
- GOOS=windows golangci-lint run ./... exits 0 locally
- CI windows-check job logs show golangci-lint output

## Acceptance Criteria

1. CI windows-check job runs GOOS=windows golangci-lint run ./...
2. Current Windows code passes the linter (fix issues first if needed)
3. ci.yml includes the lint step alongside existing go vet and go build


## Notes

**2026-03-31T06:24:53Z**

Fixed: added golangci-lint install + GOOS=windows lint step to CI windows-check job.
