---
id: fus-h6sz
status: closed
deps: []
links: []
created: 2026-03-30T13:00:00Z
type: task
priority: 1
assignee:
parent:
tags: [windows, correctness, phase-4]
---
# Align Windows executor to use strings.Builder (match Unix)

## Problem

The Unix `executeCapturedShellCommandWithStdin` at `runner_exec_unix.go:85-86` uses `strings.Builder` for stdout/stderr capture, while the Windows version at `runner_exec_windows.go:94-95` uses `bytes.Buffer`. Both work, but the divergence:

1. Obscures structural comparison between the two files during review
2. `strings.Builder` is more efficient for append-only string construction (the exact use case)
3. Signals platform drift that could accumulate over time

**Source:** Go Dev review finding #2 (WARN)

## Where it surfaces

- `internal/adapters/runner_exec_windows.go:94-95` — `var stdoutBuf bytes.Buffer` / `var stderrBuf bytes.Buffer`

## Risk if unfixed

Low — both types work. But platform divergence in parallel files is a maintenance anti-pattern that leads to behavior discrepancies when someone assumes they're identical.

## Acceptance Criteria

1. Windows executor uses `strings.Builder` for stdout/stderr capture (matching Unix)
2. `bytes` import removed if no longer needed
3. `GOOS=windows go build ./...` passes
4. `go test ./... -race` passes (Unix regression)

## Test Cases

1. **Code inspection:** `grep 'bytes.Buffer' internal/adapters/runner_exec_windows.go` returns 0 matches
2. **Code inspection:** `grep 'strings.Builder' internal/adapters/runner_exec_windows.go` returns 2 matches
3. **Cross-compile:** `GOOS=windows go vet ./internal/adapters/...` passes
