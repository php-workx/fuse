---
id: fus-e3pw
status: closed
deps: []
links: []
created: 2026-03-30T13:00:00Z
type: bug
priority: 0
assignee:
parent:
tags: [windows, security, phase-4]
---
# Promote job object containment failures from debug to warn

## Problem

When `job.assign()` fails in `runner_exec_windows.go:57-59`, the failure is logged at `slog.Debug` level. This means a user running fuse normally never sees that process tree containment is degraded — the child runs untracked, and `TerminateJobObject` won't reach it or its descendants on timeout.

Similarly, `GenerateConsoleCtrlEvent` failures at `runner_exec_windows.go:160` are logged at debug level.

For a security tool, silent containment degradation is a blocker. If fuse cannot track a child in a job object, the user should know.

**Source:** Security review finding #1 (BLOCKER), finding #9 (NOTE — no audit trail)

## Where it surfaces

- `internal/adapters/runner_exec_windows.go:57-59` — `job.assign()` failure
- `internal/adapters/runner_exec_windows.go:91-93` — `job.assign()` failure (captured variant)
- `internal/adapters/runner_exec_windows.go:160` — `GenerateConsoleCtrlEvent` failure

## Risk if unfixed

A restrictive parent job object (CI runners, Windows containers) or AV interference could cause `OpenProcess` / `AssignProcessToJobObject` to fail. The child runs without containment — timeout kills only the direct child, not grandchildren. The user has zero visibility.

## Acceptance Criteria

1. `job.assign()` failure logs at `slog.Warn` (not `slog.Debug`)
2. `GenerateConsoleCtrlEvent` failure logs at `slog.Warn` (not `slog.Debug`)
3. Log messages include pid, error, and note that containment is degraded
4. `GOOS=windows go vet ./internal/adapters/...` passes

## Test Cases

1. **Code inspection:** `grep -n 'slog.Debug.*job.*assign\|slog.Debug.*ConsoleCtrl' internal/adapters/runner_exec_windows.go` returns 0 matches
2. **Code inspection:** `grep -n 'slog.Warn.*job.*assign\|slog.Warn.*ConsoleCtrl' internal/adapters/runner_exec_windows.go` returns 2+ matches
3. **Cross-compile:** `GOOS=windows go build ./...` succeeds
