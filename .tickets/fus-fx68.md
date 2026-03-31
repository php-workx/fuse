---
id: fus-fx68
status: closed
deps: []
links: []
created: 2026-03-31T06:09:04Z
type: task
priority: 2
assignee: Ronny Unger
tags: [windows, reliability, phase-4]
---
# Reduce doctor probe duration from 30 seconds to 2 seconds

doctor_live_windows.go:105 uses 'ping -n 30 127.0.0.1 >nul' as the probe command (~29 seconds runtime). The job object assignment check needs the process alive for microseconds. If cmd.Process.Kill() fails silently (error discarded at line 119), cmd.Wait() blocks for the full 29 seconds, making 'fuse doctor' appear hung.

Found by: our reliability explorer.

Files: internal/cli/doctor_live_windows.go

Test cases:
- Code inspection: grep 'ping -n 2' doctor_live_windows.go returns 1 match
- Manual (Windows): fuse doctor --security completes job object check in <3s

## Acceptance Criteria

1. Probe command uses ping -n 2 (not ping -n 30)
2. fuse doctor --security completes the job object check in under 3 seconds
3. GOOS=windows go vet ./internal/cli/... passes

