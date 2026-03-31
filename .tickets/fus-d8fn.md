---
id: fus-d8fn
status: closed
deps: []
links: []
created: 2026-03-28T18:00:03Z
type: feature
priority: 2
assignee: ""
parent: fus-r7km
tags: [windows, phase-3, wave-1]
---
# Implement Windows doctor live checks

Replace SKIP stubs in `doctor_live_windows.go` with real console checks for `checkLiveTTYAccess` and `checkLiveRawMode`. Keep `checkLiveForegroundProcessGroup` as SKIP (Phase 4).

Files: internal/cli/doctor_live_windows.go
Wave 1 — no dependencies.

## Design

Mirror the patterns in `doctor_live_unix.go` using Windows Console API. Import `golang.org/x/sys/windows`.

**`checkLiveTTYAccess`** (replaces `doctor_live_unix.go:17-31`):
- Open `CONIN$` via `os.OpenFile("CONIN$", os.O_RDWR, 0)`
- Call `windows.GetConsoleMode(handle, &mode)` to verify it's a real console
- Close the handle
- Return PASS if both succeed, WARN if either fails

**`checkLiveRawMode`** (replaces `doctor_live_unix.go:34-81`):
- Open `CONIN$`, save mode via `GetConsoleMode`
- Set raw mode: clear `ENABLE_LINE_INPUT | ENABLE_ECHO_INPUT` via `SetConsoleMode`
- Restore original mode via `SetConsoleMode`
- Close handle
- Return PASS if all three operations succeed, WARN on any failure

**`checkLiveForegroundProcessGroup`** — no change, stays SKIP with "Phase 4" detail.

## Acceptance Criteria

1. `checkLiveTTYAccess` returns PASS/WARN (not SKIP) on Windows
2. `checkLiveRawMode` returns PASS/WARN (not SKIP) on Windows
3. `checkLiveForegroundProcessGroup` still returns SKIP with Phase 4 message
4. `GOOS=windows go vet ./internal/cli/...` passes
5. No import of `golang.org/x/sys/unix` (Windows-only file)

## Tests (optional, CI-limited)

**`internal/cli/doctor_live_windows_test.go`** — **NEW** (optional):
- Guard all tests with `skipIfNoConsole` (same pattern as fus-p3cw)
- `TestCheckLiveTTYAccess_Pass`: On real console, returns PASS
- `TestCheckLiveRawMode_Pass`: On real console, enters and restores raw mode

These tests cannot run in CI (no interactive console on `windows-latest`). They are for manual verification on Windows machines.

## Conformance Checks

- content_check: {file: "internal/cli/doctor_live_windows.go", pattern: "GetConsoleMode"}
- content_check: {file: "internal/cli/doctor_live_windows.go", pattern: "CONIN\\$"}
- tests: `GOOS=windows go vet ./internal/cli/...`

## Notes

**2026-03-31T06:20:38Z**

Closed: implemented in Phase 3/4 commits on feat/windows-terminal-approval branch.
