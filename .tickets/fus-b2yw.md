---
id: fus-b2yw
status: closed
deps: []
links: []
created: 2026-03-30T09:00:04Z
type: task
priority: 2
assignee: ""
parent: fus-r7km
tags: [windows, phase-3, review-finding]
---
# Update stale Phase 3 skip in doctor_test.go

`TestRunDoctorLive_ReportsTerminalCapabilityChecks` at `doctor_test.go:430` skips on Windows with:
```
t.Skip("terminal capability checks not yet supported on Windows (planned: Phase 3)")
```

Phase 3 is now implemented. The skip prevents CI from exercising the new Windows doctor checks.

Found by: internal review (low).

Files: internal/cli/doctor_test.go

## Fix

Either:
- Remove the Windows skip entirely (if the test can run without a real console)
- Replace with a `skipIfNoConsole` guard that skips only when no console is available (CI), not when on Windows

## Acceptance Criteria

1. Skip comment no longer references "planned: Phase 3"
2. Test runs on Windows when console is available
3. Test skips gracefully in CI (no console)

## Notes

**2026-03-31T06:20:38Z**

Closed: implemented in Phase 3/4 commits on feat/windows-terminal-approval branch.
