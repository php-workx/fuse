---
id: fus-j7ta
status: closed
deps: []
links: []
created: 2026-03-30T13:00:00Z
type: task
priority: 1
assignee:
parent:
tags: [windows, documentation, phase-4]
---
# Add safety comments to job_windows.go (unsafe.Pointer and signal flow)

Groups three documentation findings that share a root cause: non-obvious Windows API patterns lacking inline justification.

## Problem

1. **unsafe.Pointer (job_windows.go:39):** `unsafe.Pointer` is used for `SetInformationJobObject` struct parameter. No `// SAFETY:` comment explains why this is sound (struct is stack-allocated, pointer valid for syscall duration). Given the project's `#nosec` budget of 0, future reviewers need documented invariants.

2. **Signal flow (runner_exec_windows.go:131-143):** The interaction between `signal.Notify(os.Interrupt)`, `forwardConsoleCtrl`, and `CTRL_BREAK_EVENT` is non-obvious. Fuse survives Ctrl+C because `signal.Notify` suppresses the default handler, then forwards `CTRL_BREAK_EVENT` to the child's process group. This isn't clear to future maintainers.

3. **CTRL_BREAK_EVENT semantics (runner_exec_windows.go:151-166):** `CTRL_BREAK_EVENT` triggers immediate termination by default (unlike Unix SIGINT which can be caught). The comment explains why we use it over `CTRL_C_EVENT` but doesn't note this behavioral difference from Unix.

**Source:** Go Dev review findings #1 (WARN), #7 (NOTE); Security review finding #6 (WARN)

## Where it surfaces

- `internal/adapters/job_windows.go:39` — `unsafe.Pointer` usage
- `internal/adapters/runner_exec_windows.go:131-143` — signal notification flow
- `internal/adapters/runner_exec_windows.go:151-166` — `CTRL_BREAK_EVENT` semantics

## Risk if unfixed

Future maintainer misunderstands the signal flow, removes `signal.Notify` thinking it's redundant (making fuse terminate on Ctrl+C instead of forwarding), or adds `#nosec`/`//nolint` to suppress an `unsafe` warning without understanding the invariant.

## Acceptance Criteria

1. `// SAFETY:` comment near `unsafe.Pointer` usage explains struct lifetime and pointer validity
2. Comment in `waitForManagedCommand` explains why fuse survives Ctrl+C (signal.Notify suppresses default)
3. Comment in `forwardConsoleCtrl` notes that `CTRL_BREAK_EVENT` triggers immediate termination by default (behavioral difference from Unix SIGINT)
4. `GOOS=windows go build ./...` passes

## Test Cases

1. **Code inspection:** `grep -c 'SAFETY' internal/adapters/job_windows.go` >= 1
2. **Code inspection:** `grep 'signal.Notify.*suppresses\|survives Ctrl' internal/adapters/runner_exec_windows.go` returns a match
3. **Code inspection:** `grep 'immediate termination\|unlike.*SIGINT\|default handler' internal/adapters/runner_exec_windows.go` returns a match
