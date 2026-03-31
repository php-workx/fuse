---
id: fus-v9mr
status: closed
deps: []
links: []
created: 2026-03-30T09:00:00Z
type: bug
priority: 1
assignee: ""
parent: fus-r7km
tags: [windows, phase-3, review-finding]
---
# Restore CONOUT$ console mode after approval prompt

`renderPrompt()` enables `ENABLE_VIRTUAL_TERMINAL_PROCESSING` on the CONOUT$ handle but never restores the original output mode. `PromptUser`'s `restoreConsole` defer only restores CONIN$ (input). The VT processing flag is left permanently set on the console buffer for the process lifetime.

Found by: internal review (medium), Qodo (bug), CodeRabbit (related).

Files: internal/approve/prompt_windows.go

## Fix

**Important:** The VT mode must remain active during the entire prompt (not just rendering) because `renderPromptANSI` writes ANSI codes and the user sees them while the read loop runs. So restore must happen in `PromptUser`, not in `renderPrompt`.

Save output mode in `PromptUser` before calling `renderPrompt`, restore alongside input mode:

```go
// In PromptUser, after opening conOut (around line 50):
outHandle := windows.Handle(conOut.Fd())
var origOutMode uint32
if err := windows.GetConsoleMode(outHandle, &origOutMode); err == nil {
    // Will be restored in restoreConsole defer
}

// Expand restoreConsole defer (line 80-83):
restoreConsole := func() {
    _ = windows.SetConsoleMode(inHandle, origMode)
    _ = windows.SetConsoleMode(outHandle, origOutMode) // ← ADD
}
defer restoreConsole()
```

Then `renderPrompt` can set VT processing knowing it will be restored by the caller.

## Acceptance Criteria

1. CONOUT$ console mode is restored to its original value after PromptUser returns
2. Both CONIN$ and CONOUT$ modes restored on all exit paths (normal, panic, signal)
3. `GOOS=windows go vet ./internal/approve/...` passes

## Notes

**2026-03-31T06:20:38Z**

Closed: implemented in Phase 3/4 commits on feat/windows-terminal-approval branch.
