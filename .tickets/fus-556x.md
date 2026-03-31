---
id: fus-556x
status: closed
deps: []
links: []
created: 2026-03-31T06:08:44Z
type: bug
priority: 1
assignee: Ronny Unger
tags: [correctness, phase-3]
---
# Fix inconsistent error return between ctx cancellation and signal in prompt_unix.go

In prompt_unix.go readApprovalDecision, the select statement has two interruption branches with inconsistent error returns:
- ctx.Done() case: returns false, '', fmt.Errorf('approval interrupted: %%w', ctx.Err())
- sigCh case: returns false, '', nil

Both are interruptions, but only one returns an error. Callers cannot distinguish 'user explicitly denied' from 'signal interrupted' when the signal case returns nil — it looks identical to a clean denial.

The same pattern likely exists in prompt_windows.go and should be checked.

Found by: CodeRabbit (#16). Our review missed this — identified as a gap in our error-handling explorer's branch-level symmetry checking.

Files: internal/approve/prompt_unix.go, internal/approve/prompt_windows.go (check for same pattern)

Test cases:
- When context is cancelled during approval: error returned contains 'interrupted'
- When signal received during approval: error returned contains 'interrupted' (currently returns nil)
- When user presses 'D': returns (false, '', nil) — clean denial, no error

## Design

In readApprovalDecision's select statement, change the signal case from 'return false, "", nil' to 'return false, "", fmt.Errorf("approval interrupted by signal")'. This matches the ctx.Done() case which returns fmt.Errorf("approval interrupted: %%w", ctx.Err()). If the nil return is intentional (signal = treat as denial), add an explicit comment explaining the asymmetry — but this seems unintentional since both are interruptions.

## Acceptance Criteria

1. Both interruption branches in readApprovalDecision return consistent error semantics
2. Signal receipt returns an error (not nil) so callers can distinguish 'user denied' from 'interrupted'
3. go test ./internal/approve/... -race passes
4. Prompt behavior unchanged for actual user input (approve/deny keystrokes)


## Notes

**2026-03-31T06:23:33Z**

Fixed: signal case now returns error on both Unix and Windows, matching ctx.Done() behavior.
