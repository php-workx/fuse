---
id: fus-p3cw
status: closed
deps: []
links: []
created: 2026-03-28T18:00:01Z
type: feature
priority: 1
assignee: ""
parent: fus-r7km
tags: [windows, phase-3, wave-1]
---
# Implement Windows Console API approval prompt

Replace the 12-line `errNonInteractive` stub in `prompt_windows.go` with a full Console API implementation (~200 lines). Port all 6 functions from `prompt_unix.go`, using Windows Console API equivalents.

Files: internal/approve/prompt_windows.go, internal/approve/prompt_windows_test.go (NEW)
Wave 1 — no dependencies.

## Design

Mirror `prompt_unix.go` structure using Windows Console API:

- **`openConsole(nonInteractive bool)`** — opens `CONIN$`/`CONOUT$` via `os.OpenFile` (NOT `GetStdHandle`, which doesn't bypass stdin redirection). Returns `*os.File` handles. Returns `errNonInteractive` if `FUSE_NON_INTERACTIVE` set or `CONIN$` open fails.

- **Raw mode** — `windows.GetConsoleMode(handle, &origMode)` to save, then clear `ENABLE_LINE_INPUT | ENABLE_ECHO_INPUT | ENABLE_PROCESSED_INPUT | ENABLE_MOUSE_INPUT | ENABLE_WINDOW_INPUT` (see pm-201). Restore via `defer windows.SetConsoleMode(handle, origMode)`. Add panic recovery that also restores (mirror `prompt_unix.go:56-64`).

- **Keystroke reading** — `windows.WaitForSingleObject(windows.Handle(conIn.Fd()), 100)` for 100ms timeout (matches Unix VTIME=1). If `WAIT_OBJECT_0`: `conIn.Read(buf)`. If `WAIT_TIMEOUT`: continue polling loop. Note: `conIn.Fd()` returns `uintptr`, must cast to `windows.Handle` (see pm-203).

- **Console buffer flush** — Call `windows.FlushConsoleInputBuffer(windows.Handle(conIn.Fd()))` after opening CONIN$ and before rendering prompt. Prevents stale keystrokes from auto-approving (see pm-202).

- **Signal handling** — `signal.Notify(sigCh, os.Interrupt)` only. SIGTERM/SIGHUP don't exist on Windows. Ctrl+C also arrives as byte `\x03` with ENABLE_PROCESSED_INPUT cleared.

- **ANSI output** — try `ENABLE_VIRTUAL_TERMINAL_PROCESSING` on `CONOUT$`. If fails (pre-Windows 10), use plain text `renderPromptPlain()`.

- **Concurrency** — same `ttyMu.TryLock()` pattern as Unix.

## Code Specification

```go
//go:build windows

package approve

import (
    "context"
    "fmt"
    "os"
    "os/signal"
    "sync"
    "time"

    "golang.org/x/sys/windows"
)

var ttyMu sync.Mutex

func PromptUser(ctx context.Context, command, reason string, hookMode, nonInteractive bool) (bool, string, error) { ... }
func openConsole(nonInteractive bool) (conIn, conOut *os.File, err error) { ... }
func readApprovalDecision(ctx context.Context, conIn, conOut *os.File, deadline time.Time, sigCh <-chan os.Signal) (bool, string, error) { ... }
func readScope(ctx context.Context, conIn, conOut *os.File, deadline time.Time, sigCh <-chan os.Signal) (string, bool) { ... }
func renderPrompt(conOut *os.File, command, reason string) { ... }
func renderPromptPlain(conOut *os.File, command, reason string) { ... }
```

Key helper — `openConsole`:
```go
func openConsole(nonInteractive bool) (conIn, conOut *os.File, err error) {
    if nonInteractive || os.Getenv("FUSE_NON_INTERACTIVE") != "" {
        return nil, nil, errNonInteractive
    }
    conIn, err = os.OpenFile("CONIN$", os.O_RDWR, 0)
    if err != nil {
        return nil, nil, errNonInteractive
    }
    conOut, err = os.OpenFile("CONOUT$", os.O_RDWR, 0)
    if err != nil {
        _ = conIn.Close()
        return nil, nil, errNonInteractive
    }
    return conIn, conOut, nil
}
```

Key references:
- `prompt_unix.go:21-96` — `PromptUser()` flow to mirror
- `prompt_unix.go:98-151` — `readApprovalDecision()` loop
- `prompt_unix.go:166-211` — `readScope()` function
- `prompt_unix.go:214-234` — `renderPrompt()` with ANSI codes
- `prompt_shared.go:16` — `sanitizePrompt()` (already shared)
- BubbleTea's `tty_windows.go` — validates `os.OpenFile("CONIN$", os.O_RDWR, 0o644)` pattern

## Acceptance Criteria

1. `PromptUser()` opens CONIN$/CONOUT$ directly (anti-spoofing)
2. Console enters raw mode (no echo, no line input, no processed input, no mouse input, no window input)
3. Console input buffer flushed before rendering prompt (anti-spoofing)
4. Console mode always restored: normal exit, panic recovery, signal, context cancel
5. Keystrokes read individually: a/y → approve, d/n → deny, Ctrl-C → deny
6. Scope selection UI: o/c/s/f after approval
7. Timeout: 25s hook mode, 5min run mode (matches Unix)
8. `GOOS=windows go vet ./internal/approve/...` passes
9. Tests: `TestOpenConsole_NonInteractiveEnv`, `TestOpenConsole_NonInteractiveFlag`

## Tests

**`internal/approve/prompt_windows_test.go`** — **NEW**:
- `TestOpenConsole_NonInteractiveEnv`: Set `FUSE_NON_INTERACTIVE=1`, expect `errNonInteractive` (portable — no console needed)
- `TestOpenConsole_NonInteractiveFlag`: Pass `nonInteractive=true`, expect `errNonInteractive` (portable)

Console-specific tests require a real console and should be skipped in CI:
```go
func skipIfNoConsole(t *testing.T) {
    t.Helper()
    f, err := os.OpenFile("CONIN$", os.O_RDWR, 0)
    if err != nil {
        t.Skip("no console available (CI environment)")
    }
    _ = f.Close()
}
```

## Pre-Mortem Fixes

**pm-20260328-201: Clear ENABLE_MOUSE_INPUT and ENABLE_WINDOW_INPUT**

Raw mode must also clear mouse and window input flags, otherwise WaitForSingleObject fires on mouse movements and window resizes, causing spurious reads. Proven in cancelreader (BubbleTea dep).

```go
rawMode := origMode
rawMode &^= windows.ENABLE_LINE_INPUT |
    windows.ENABLE_ECHO_INPUT |
    windows.ENABLE_PROCESSED_INPUT |
    windows.ENABLE_MOUSE_INPUT |    // prevent mouse events triggering wait
    windows.ENABLE_WINDOW_INPUT     // prevent resize events triggering wait
```

**pm-20260328-202: Flush console input buffer before reading**

CONIN$ shares a global console input buffer. Stale keystrokes could auto-approve without user seeing the prompt. Flush after opening, before rendering.

```go
if err := windows.FlushConsoleInputBuffer(windows.Handle(conIn.Fd())); err != nil {
    fmt.Fprintf(conOut, "  warning: could not flush console input: %v\n", err)
}
```

**pm-20260328-203: Handle type conversion**

`conIn.Fd()` returns `uintptr`, `WaitForSingleObject` accepts `windows.Handle`. Cast explicitly:

```go
handle := windows.Handle(conIn.Fd())
event, err := windows.WaitForSingleObject(handle, 100)
```

## Conformance Checks

- content_check: {file: "internal/approve/prompt_windows.go", pattern: "func PromptUser"}
- content_check: {file: "internal/approve/prompt_windows.go", pattern: "CONIN\\$"}
- content_check: {file: "internal/approve/prompt_windows.go", pattern: "GetConsoleMode"}
- content_check: {file: "internal/approve/prompt_windows.go", pattern: "WaitForSingleObject"}
- content_check: {file: "internal/approve/prompt_windows.go", pattern: "FlushConsoleInputBuffer"}
- content_check: {file: "internal/approve/prompt_windows.go", pattern: "ENABLE_MOUSE_INPUT"}
- tests: `GOOS=windows go vet ./internal/approve/...`

## Notes

**2026-03-31T06:20:37Z**

Closed: implemented in Phase 3/4 commits on feat/windows-terminal-approval branch.
