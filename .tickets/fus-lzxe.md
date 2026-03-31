---
id: fus-lzxe
status: closed
deps: []
links: []
created: 2026-03-31T06:10:05Z
type: task
priority: 2
assignee: Ronny Unger
tags: [windows, security, phase-4]
---
# Promote job.close() CloseHandle failure from Debug to Warn

job_windows.go:95 logs CloseHandle failure at slog.Debug: 'slog.Debug("close job object handle", "err", err)'. Ticket fus-e3pw promoted job.assign() failures from Debug to Warn because silent containment degradation is unacceptable for a security tool. But job.close() was missed.

A CloseHandle failure means the job object handle leaked. With JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE, a leaked handle means children may not be killed on parent exit (the OS will eventually close it on process exit, but timing is unpredictable during crashes).

One-line fix: change slog.Debug to slog.Warn.

Found by: our error-handling explorer, reliability explorer.

Files: internal/adapters/job_windows.go

Test cases:
- Code inspection: grep 'slog.Warn.*close job' job_windows.go returns 1 match
- Code inspection: grep 'slog.Debug.*close job' job_windows.go returns 0 matches

## Acceptance Criteria

1. job.close() logs CloseHandle failure at slog.Warn (not slog.Debug)
2. Message includes context: 'children may not be terminated'
3. GOOS=windows go build ./... passes

