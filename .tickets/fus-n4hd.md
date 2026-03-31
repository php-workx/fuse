---
id: fus-n4hd
status: closed
deps: []
links: []
created: 2026-03-30T09:00:03Z
type: bug
priority: 2
assignee: ""
parent: fus-r7km
tags: [windows, phase-3, review-finding]
---
# Handle WaitForSingleObject WAIT_FAILED in approval prompt

`readApprovalDecision` and `readScope` discard the error return from `WaitForSingleObject`. When the console handle becomes invalid (WAIT_FAILED = 0xFFFFFFFF), the code loops at 100ms intervals until the deadline (up to 5 minutes) instead of failing fast.

Found by: internal review (low), Kody (high), Qodo (bug), CodeRabbit (major).

Files: internal/approve/prompt_windows.go

## Fix

```go
// Line 143 (readApprovalDecision) and line 210 (readScope):
event, waitErr := windows.WaitForSingleObject(inHandle, 100)
if event == 0xFFFFFFFF { // WAIT_FAILED
    return false, "", fmt.Errorf("console wait failed: %w", waitErr)
}
if event != windows.WAIT_OBJECT_0 {
    continue
}
```

## Acceptance Criteria

1. WAIT_FAILED causes immediate error return, not continued polling
2. Same fix applied in both `readApprovalDecision` and `readScope`
3. `GOOS=windows go vet ./internal/approve/...` passes

## Notes

**2026-03-31T06:20:42Z**

Closed: duplicate of fus-n4d6 (readScope errors as denials — more comprehensive).
