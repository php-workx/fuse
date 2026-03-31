---
id: fus-c7gm
status: closed
deps: []
links: []
created: 2026-03-30T09:00:05Z
type: task
priority: 3
assignee: ""
parent: fus-r7km
tags: [windows, phase-3, review-finding]
---
# Make errNonInteractive message platform-neutral

`errNonInteractive` in `prompt_shared.go:11` says:
```
Approval requires an interactive terminal (/dev/tty unavailable)
```

This message is now returned on Windows where CONIN$ is the relevant concept. The machine-parsed `NON_INTERACTIVE_MODE` token is unaffected, but the human-readable portion is misleading.

Found by: internal review (low).

Files: internal/approve/prompt_shared.go

## Fix

Change to platform-neutral wording:
```go
var errNonInteractive = fmt.Errorf("fuse:NON_INTERACTIVE_MODE STOP. Approval requires an interactive terminal (console unavailable)")
```

## Acceptance Criteria

1. Error message does not reference `/dev/tty`
2. `NON_INTERACTIVE_MODE` token preserved (callers parse this)
3. All existing tests pass unchanged

## Notes

**2026-03-31T06:20:38Z**

Closed: implemented in Phase 3/4 commits on feat/windows-terminal-approval branch.
