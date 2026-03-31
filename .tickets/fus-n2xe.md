---
id: fus-n2xe
status: closed
deps: []
links: []
created: 2026-03-30T13:00:00Z
type: task
priority: 2
assignee:
parent:
tags: [windows, testing, phase-4]
---
# Extract interpretWaitError to shared code or add Windows test file

## Problem

`interpretWaitError` in `runner_exec_windows.go:169-178` is pure logic with no platform dependencies — it takes an `error` and returns `(int, error)`. The Unix version at `runner_exec_unix.go:172-184` has an extra branch for `WaitStatus.Signaled()` but the core logic is shared.

Currently neither version has direct unit tests. The Windows version could be tested cross-platform if extracted, or a `//go:build windows` test file could provide coverage on Windows CI runners.

**Source:** Go Dev review finding #8 (NOTE)

## Where it surfaces

- `internal/adapters/runner_exec_windows.go:169-178` — Windows `interpretWaitError`
- `internal/adapters/runner_exec_unix.go:172-184` — Unix `interpretWaitError`

## Risk if unfixed

Low — the function is simple. But it's an easy win for test coverage and the shared logic pattern reduces platform drift.

## Acceptance Criteria

Option A (extract):
1. Common `interpretWaitError` logic lives in a platform-agnostic file
2. Unix file adds the `WaitStatus.Signaled()` branch on top
3. Unit tests in a non-tagged test file

Option B (Windows test):
1. `runner_exec_windows_test.go` with `//go:build windows` exists
2. Table-driven tests for nil error, ExitError, other error cases
3. Tests run on `windows-latest` CI (when available)

## Test Cases

1. `interpretWaitError(nil)` → `(0, nil)`
2. `interpretWaitError(&exec.ExitError{ProcessState: ...})` → `(exitCode, nil)`
3. `interpretWaitError(fmt.Errorf("unknown"))` → `(-1, error)`
