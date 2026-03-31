---
id: fus-l9vc
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
# Document known limitations: race window, UAC elevation, nested jobs

Groups three documentation findings about inherent Windows limitations that users and future maintainers should understand.

## Problem

1. **Race window (runner_exec_windows.go:53-59):** Between `cmd.Start()` and `job.assign()`, the child process is not tracked by the job object. A crafted command could spawn a grandchild in this window that escapes containment. The window is sub-millisecond and the risk is mitigated by classification (malicious commands are caught before execution), but it should be documented.

2. **UAC elevation (general):** If a child process triggers UAC elevation (e.g., `runas`, `Start-Process -Verb RunAs`), the elevated process runs at a higher integrity level and the job object cannot control it. This is an inherent Windows security boundary.

3. **Nested job objects (job_windows.go):** When fuse runs inside a CI runner's job object, child job creation works (Windows 8+) but assignment may fail with `ERROR_ACCESS_DENIED` if the parent job has restrictive limits. Current code handles this gracefully (debug log + untracked child) — see also fus-e3pw for log level promotion.

**Source:** Security review findings #2 (WARN), #4 (WARN), #10 (NOTE — positive)

## Where it surfaces

- `internal/adapters/runner_exec_windows.go:53-59` — race window
- `internal/adapters/job_windows.go:25-47` — nested job behavior
- General — UAC elevation escape

## Risk if unfixed

Users may assume fuse provides stronger containment guarantees than it actually does. Future maintainers may not understand why certain edge cases exist.

## Acceptance Criteria

1. Comment in `executeShellCommand` near `job.assign()` documents the race window and why it's accepted
2. Comment in `newJobObject` documents nested job behavior and potential `ERROR_ACCESS_DENIED`
3. Comment or doc note explains UAC elevation escapes job containment (inherent Windows limitation)
4. `GOOS=windows go build ./...` passes

## Test Cases

1. **Code inspection:** `grep -c 'race window\|sub-millisecond\|race between' internal/adapters/runner_exec_windows.go` >= 1
2. **Code inspection:** `grep -c 'nested job\|ERROR_ACCESS_DENIED\|CI runner' internal/adapters/job_windows.go` >= 1
3. **Code inspection:** `grep -c 'UAC\|elevated\|integrity level' internal/adapters/job_windows.go` >= 1
