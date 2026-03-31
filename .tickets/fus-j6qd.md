---
id: fus-j6qd
status: closed
deps: [fus-p3cw, fus-d8fn]
links: []
created: 2026-03-28T18:00:05Z
type: task
priority: 3
assignee: ""
parent: fus-r7km
tags: [windows, phase-3, wave-2]
---
# Delete Phase 1 scaffolding constants files

Delete two files containing dummy constants that were Phase 1 compilation scaffolding, now rendered obsolete by Phase 3's Console API implementation:

1. `internal/approve/ioctl_windows.go` — `ioctlGetTermios = 0`, `ioctlSetTermios = 0` (unused by new prompt_windows.go)
2. `internal/cli/doctor_termios_windows.go` — `doctorIoctlGetTermios = 0`, `doctorIoctlSetTermios = 0` (unused by new doctor_live_windows.go)

Files: internal/approve/ioctl_windows.go (DELETE), internal/cli/doctor_termios_windows.go (DELETE)
Wave 2 — depends on fus-p3cw and fus-d8fn (must confirm constants are unused after new implementations land).

## Pre-Deletion Verification

Before deleting, run:
```bash
# Verify ioctl constants unused on Windows
grep -r 'ioctlGetTermios\|ioctlSetTermios' internal/ --include='*_windows.go'
# Must return only ioctl_windows.go itself

# Verify doctor termios constants unused on Windows
grep -r 'doctorIoctlGetTermios\|doctorIoctlSetTermios' internal/cli/ --include='*_windows.go'
# Must return only doctor_termios_windows.go itself
```

If any other Windows file references these constants, do NOT delete that file.

## Acceptance Criteria

1. `internal/approve/ioctl_windows.go` does not exist
2. `internal/cli/doctor_termios_windows.go` does not exist
3. No other `_windows.go` file references `ioctlGetTermios`, `ioctlSetTermios`, `doctorIoctlGetTermios`, or `doctorIoctlSetTermios`
4. `GOOS=windows go build ./...` succeeds
5. `GOOS=windows go vet ./...` succeeds

## Conformance Checks

- command: `test ! -f internal/approve/ioctl_windows.go && test ! -f internal/cli/doctor_termios_windows.go`
- tests: `GOOS=windows go build ./...`

## Notes

**2026-03-31T06:20:38Z**

Closed: implemented in Phase 3/4 commits on feat/windows-terminal-approval branch.
