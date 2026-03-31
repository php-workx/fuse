---
id: fus-f4qx
status: closed
deps: []
links: []
created: 2026-03-30T13:00:00Z
type: task
priority: 0
assignee:
parent:
tags: [windows, security, phase-4]
---
# Add SECURITY comment prohibiting JOB_OBJECT_LIMIT_BREAKAWAY_OK

## Problem

`job_windows.go` correctly does NOT set `JOB_OBJECT_LIMIT_BREAKAWAY_OK`, but there is no comment explaining why. This flag would allow child processes to call `CreateProcess` with `CREATE_BREAKAWAY_FROM_JOB` and escape the job object entirely, defeating the containment guarantee.

A future maintainer might add this flag thinking it improves compatibility with nested job objects. Without an explicit prohibition comment, the security invariant is undocumented.

**Source:** Security review finding #3 (WARN)

## Where it surfaces

- `internal/adapters/job_windows.go:28-35` — `newJobObject()` where `LimitFlags` is configured

## Risk if unfixed

A future change could accidentally enable process escape from fuse's containment. Low probability but catastrophic impact — the entire process management guarantee evaporates.

## Acceptance Criteria

1. A `// SECURITY:` comment near the `LimitFlags` assignment explicitly states `JOB_OBJECT_LIMIT_BREAKAWAY_OK` must never be set and why
2. `GOOS=windows go build ./...` passes

## Test Cases

1. **Code inspection:** `grep -A2 'SECURITY.*BREAKAWAY' internal/adapters/job_windows.go` returns the prohibition comment
