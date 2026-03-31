---
id: fus-t4vn
status: closed
deps: []
links: []
created: 2026-03-30T11:30:00Z
type: bug
priority: 2
assignee: ""
tags: [windows, phase-3, review-finding]
---
# shouldColorize emits ANSI on legacy Windows conhost without VT support

`shouldColorize()` in `help.go:58-65` uses `isTerminal()` to decide whether to emit ANSI color codes. On Windows, `isTerminal` now returns `true` via `GetConsoleMode` succeeding — but this doesn't mean the console supports ANSI/VT processing. On legacy `conhost.exe` (pre-Windows 10 1511), ANSI escape sequences print as raw text.

The approval prompt handles this correctly via `renderPrompt` → `renderPromptPlain` fallback (probes `ENABLE_VIRTUAL_TERMINAL_PROCESSING` before emitting ANSI). The help rendering path does not.

Found by: CodeRabbit (follow-up on isTerminal review).

Files: internal/cli/help.go, internal/cli/help_width_windows.go

## Fix

Probe VT support in `shouldColorize` on Windows:

```go
// In help_width_windows.go, add:
func supportsANSI() bool {
    conOut, err := windows.GetStdHandle(windows.STD_OUTPUT_HANDLE)
    if err != nil || conOut == windows.InvalidHandle {
        return false
    }
    var mode uint32
    if err := windows.GetConsoleMode(conOut, &mode); err != nil {
        return false
    }
    // Try enabling VT processing — if it fails, console doesn't support ANSI
    if err := windows.SetConsoleMode(conOut, mode|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING); err != nil {
        return false
    }
    // Restore original mode (don't leave VT permanently set)
    _ = windows.SetConsoleMode(conOut, mode)
    return true
}
```

Then in `help.go`, change `shouldColorize`:
```go
func shouldColorize() bool {
    if os.Getenv("NO_COLOR") != "" { return false }
    if os.Getenv("TERM") == "dumb" { return false }
    if !isTerminal(int(os.Stdout.Fd())) { return false }
    return supportsANSI() // platform-specific probe
}
```

On Unix, `supportsANSI` returns `true` unconditionally (all modern terminals support ANSI).

## Acceptance Criteria

1. Help output on legacy conhost shows plain text (no raw escape codes)
2. Help output on Windows Terminal / modern conhost shows colors
3. Unix behavior unchanged
4. `GOOS=windows go vet ./internal/cli/...` passes

## Notes

**2026-03-31T06:20:42Z**

Closed: duplicate of fus-wrx7 (supportsANSI VT restore is the root cause).
