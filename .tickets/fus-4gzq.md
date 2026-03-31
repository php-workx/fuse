---
id: fus-4gzq
status: closed
deps: []
links: []
created: 2026-03-31T06:10:54Z
type: chore
priority: 3
assignee: Ronny Unger
tags: [windows, documentation, phase-4]
---
# Add race window comment to mcpproxy_cleanup_windows.go

mcpproxy_cleanup_windows.go:24 calls job.assign() after cmd.Start() — functionally correct but lacks the race window comment present in runner_exec_windows.go:57-59. Both have the same sub-millisecond race between cmd.Start() and job.assign(). The runner_exec version documents this explicitly; the mcpproxy version does not.

One-line comment addition for consistency.

Found by: our spec-verification explorer, security-config explorer.

Files: internal/adapters/mcpproxy_cleanup_windows.go

## Acceptance Criteria

1. Comment near job.assign() in mcpproxy_cleanup_windows.go documents the race window
2. Pattern matches the comment in runner_exec_windows.go for consistency

