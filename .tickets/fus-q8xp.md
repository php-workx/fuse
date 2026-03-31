---
id: fus-q8xp
status: closed
deps: []
links: []
created: 2026-03-30T09:00:02Z
type: bug
priority: 1
assignee: ""
parent: fus-r7km
tags: [windows, phase-3, review-finding, cross-platform]
---
# Fix ctx cancellation hang in readApprovalDecision (both platforms)

When `ctx.Done()` fires in `readApprovalDecision`, it returns `(false, "", nil)`. The nil error causes `runTTYPrompt` to call `sendApprovalResult(subCtx, ch, ...)`, which also guards on `ctx.Done()` and silently drops the result. Both goroutines exit without sending to `ch`, causing `RequestApproval` to block forever on `r := <-ch`.

This is a **pre-existing bug** in `prompt_unix.go:108` that Phase 3 replicated in `prompt_windows.go:128`. Fix must apply to both platforms for consistency.

Found by: internal review (medium). Not caught by any of the 4 external reviewers.

Files: internal/approve/prompt_unix.go, internal/approve/prompt_windows.go

## Fix

Return `ctx.Err()` instead of `nil` when ctx is done:

```go
// prompt_unix.go:108 and prompt_windows.go:128
case <-ctx.Done():
    fmt.Fprintf(tty, "\n  Denied (shutdown).\n\n")
    return false, "", fmt.Errorf("approval interrupted: %w", ctx.Err())
```

With a non-nil error, `runTTYPrompt` sends the error result to `ch` via the error path before `sendApprovalResult` checks `ctx.Done()`, unblocking `RequestApproval`.

## Scope

The same nil-error pattern also exists in `readScope` (prompt_unix.go:172, prompt_windows.go:~195). Less likely to trigger (only called after initial approval keystroke), but should be fixed for consistency.

## Deadlock trace

1. `readApprovalDecision` returns `(false, "", nil)` on ctx.Done() — prompt_unix.go:108 / prompt_windows.go:130
2. `runTTYPrompt` sees `promptErr==nil, approved==false` → calls `sendApprovalResult(ctx, ch, {DecisionBlocked})` — manager.go:156
3. `sendApprovalResult` selects `<-ctx.Done()` (already cancelled) → returns without sending — manager.go:141
4. `runDBPoll` also exits via ctx.Done() without sending
5. `RequestApproval` blocks on `r := <-ch` — manager.go:105 → **deadlock**

## Acceptance Criteria

1. `readApprovalDecision` returns non-nil error on context cancellation (both platforms)
2. `readScope` returns non-nil error on context cancellation (both platforms)
3. `RequestApproval` returns promptly when outer context is cancelled
4. `go test ./internal/approve/ -v` passes
5. Both prompt_unix.go and prompt_windows.go have the same fix

## Notes

**2026-03-31T06:20:38Z**

Closed: implemented in Phase 3/4 commits on feat/windows-terminal-approval branch.
