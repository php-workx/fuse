---
id: fus-tvat
status: closed
deps: []
links: []
created: 2026-03-31T06:09:31Z
type: task
priority: 2
assignee: Ronny Unger
tags: [windows, reliability, phase-4]
---
# Evaluate CreateJobObject hard-fail vs warn-and-continue

runner_exec_windows.go:33-36 hard-fails when newJobObject() returns an error — no command execution, no fallback. By contrast, proxyChildCleanup in mcpproxy_cleanup_windows.go:14-21 warns and falls back to direct kill.

This asymmetry means fuse run is completely unusable on Windows CI runners or containers where the host job object prohibits child job creation (ERROR_ACCESS_DENIED). The doctor check would show FAIL, but fuse run gives no actionable guidance.

Found by: our reliability explorer.

Files: internal/adapters/runner_exec_windows.go

Test cases:
- Simulate CreateJobObject failure: verify fuse run either (A) warns and executes without containment or (B) returns error with actionable message mentioning fuse doctor
- Verify executeCapturedShellCommandWithStdin has the same behavior as executeShellCommand

## Design

Design decision: executeShellCommand hard-fails on CreateJobObject failure (returns error, no execution). proxyChildCleanup warns-and-continues. The asymmetry means fuse run is unusable in restrictive Windows CI/containers where parent job objects prohibit child job creation. Options: (A) warn-and-continue like mcpproxy — child runs without containment. (B) keep fail-closed but improve the error message to say 'run fuse doctor --security to diagnose'. Option A is more pragmatic. Option B is more secure.

## Acceptance Criteria

1. Decision documented in code comment at runner_exec_windows.go:33
2. If warn-and-continue: child runs without containment, slog.Warn emitted, error message mentions 'fuse doctor --security'
3. If keep fail-closed: error message includes actionable guidance (run fuse doctor)
4. Behavior consistent between executeShellCommand and executeCapturedShellCommandWithStdin


## Notes

**2026-03-31T06:25:15Z**

Decision: keep fail-closed (security tool). Improved error message to include 'fuse doctor --security' guidance. Applied to both executeShellCommand and executeCapturedShellCommandWithStdin.
