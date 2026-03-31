---
id: fus-n4d6
status: closed
deps: []
links: []
created: 2026-03-31T06:08:20Z
type: bug
priority: 1
assignee: Ronny Unger
tags: [windows, correctness, phase-3]
---
# Fix readScope to propagate failures as errors, not denials

readScope() in prompt_windows.go:201-260 returns ('', true) (denied) on timeout, WAIT_FAILED, and read errors. The caller readApprovalDecision() treats denied=true as approved=false, err=nil, which the approval manager interprets as an explicit user denial — clearing the pending approval and NOT triggering the fallback path (DB poll, TUI).

This is a semantic bug: a console failure during scope selection is not the same as the user pressing 'D'. The manager's fallback path should handle transient failures, but it never fires because the error is masked as a denial.

Also: WaitForSingleObject error is discarded with _ at line 222, unlike readApprovalDecision which properly wraps it at line 152-154.

Found by: CodeRabbit (#18, major), our error-handling explorer (surface only — found the _ discard but missed the semantic impact on the approval manager's fallback path).

Files: internal/approve/prompt_windows.go

Test cases:
- When WaitForSingleObject returns WAIT_FAILED during scope selection: readScope returns error, not denial
- When read times out during scope selection: readScope returns error, not denial
- When user presses 'D': readScope returns ('', true, nil) — still a denial
- readApprovalDecision propagates scope errors: returns (false, '', error)

## Design

Change readScope signature from (string, bool) to (string, bool, error). On timeout/WAIT_FAILED/read error, return ('', false, err) instead of ('', true). Update readApprovalDecision at prompt_windows.go:182-187 to check the new error return and propagate it. The approval manager's handlePromptError already handles errors correctly — it falls back to DB poll. The current code bypasses that fallback by masking errors as denials.

## Acceptance Criteria

1. readScope returns an error on WAIT_FAILED, read failure, and timeout
2. readApprovalDecision propagates scope errors to the caller
3. Approval manager receives an error (not denial) on console failures — triggers fallback path
4. Explicit user 'D' press still returns denial (not error)
5. WaitForSingleObject error captured and logged (not discarded with _)
6. GOOS=windows go vet ./internal/approve/... passes


## Notes

**2026-03-31T06:22:59Z**

Fixed: readScope returns (string, bool, error). User Ctrl-C = denial. Timeout/WAIT_FAILED/read error/signal/ctx cancel = error propagated to approval manager fallback.
