---
id: fus-w2ht
status: closed
deps: []
links: []
created: 2026-03-28T18:00:04Z
type: feature
priority: 2
assignee: ""
parent: fus-r7km
tags: [windows, phase-3, wave-1]
---
# Implement Windows terminal width and isTerminal

Replace default `80`/`false` returns in `help_width_windows.go` with real Windows Console API calls.

Files: internal/cli/help_width_windows.go
Wave 1 — no dependencies.

## Design

Use `GetConsoleScreenBufferInfo` for terminal width and `GetConsoleMode` for terminal detection. Mirror the logic of `help_width_unix.go` using Windows equivalents.

## Code Specification

```go
//go:build windows

package cli

import "golang.org/x/sys/windows"

func terminalWidth() int {
    conOut, err := windows.GetStdHandle(windows.STD_OUTPUT_HANDLE)
    if err != nil || conOut == windows.InvalidHandle {
        return 80
    }
    var info windows.ConsoleScreenBufferInfo
    if err := windows.GetConsoleScreenBufferInfo(conOut, &info); err != nil {
        return 80
    }
    width := int(info.Window.Right - info.Window.Left + 1)
    if width > 0 {
        return width
    }
    return 80
}

func isTerminal(fd int) bool {
    var mode uint32
    return windows.GetConsoleMode(windows.Handle(fd), &mode) == nil
}
```

Note: `GetStdHandle` is acceptable here (unlike the approval prompt) because terminal width detection doesn't have anti-spoofing requirements. We're just measuring the output buffer, not reading security-sensitive input.

## Acceptance Criteria

1. `terminalWidth()` returns actual console width when running in a console
2. `terminalWidth()` returns 80 when console is unavailable (pipes, CI)
3. `isTerminal()` returns true for console handles, false for pipes
4. `GOOS=windows go vet ./internal/cli/...` passes

## Conformance Checks

- content_check: {file: "internal/cli/help_width_windows.go", pattern: "GetConsoleScreenBufferInfo"}
- content_check: {file: "internal/cli/help_width_windows.go", pattern: "isTerminal"}
- tests: `GOOS=windows go vet ./internal/cli/...`

## Notes

**2026-03-31T06:20:38Z**

Closed: implemented in Phase 3/4 commits on feat/windows-terminal-approval branch.
