---
id: fus-g5ry
status: closed
deps: []
links: []
created: 2026-03-30T13:00:00Z
type: bug
priority: 1
assignee:
parent:
tags: [windows, reliability, phase-4]
---
# Replace doctor probe `timeout` command with non-interactive-safe alternative

## Problem

`doctor_live_windows.go:103` uses `cmd.exe /c timeout /t 30 /nobreak >nul` as the probe process for the job object diagnostic. The `timeout` command requires a console — in non-interactive contexts (CI runners, SSH sessions, Windows containers, Windows Services), it fails with "ERROR: Input redirection is not supported, exiting the process immediately."

This means `fuse doctor --security` shows FAIL for the job object check in exactly the environments where process management matters most (CI, remote sessions).

**Source:** Go Dev review finding #5 (WARN), Security review finding #8 (NOTE)

## Where it surfaces

- `internal/cli/doctor_live_windows.go:103` — `exec.Command("cmd.exe", "/c", "timeout /t 30 /nobreak >nul")`

## Risk if unfixed

False FAIL in `fuse doctor` output on CI runners and non-interactive Windows sessions. Users and automation see a diagnostic failure that has nothing to do with job object support.

## Acceptance Criteria

1. Probe command works in non-interactive contexts (no console required)
2. Probe command runs long enough for job assignment to complete (~1 second minimum)
3. Probe command is universally available on Windows (no optional features)
4. `fuse doctor --security` shows PASS for job object check on both interactive terminals and CI runners
5. `GOOS=windows go vet ./internal/cli/...` passes

## Test Cases

1. **Headless context:** Run probe command in a non-interactive CMD session (`cmd.exe /c <probe>` piped from stdin) — should not fail
2. **Cross-compile:** `GOOS=windows go build ./...` succeeds
3. **Suggested replacement:** `ping -n 30 127.0.0.1 >nul` (universally available, no console dependency, ~29 second runtime)
