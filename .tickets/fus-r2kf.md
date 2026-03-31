---
id: fus-r2kf
status: closed
deps: []
links: []
created: 2026-03-30T09:00:08Z
type: task
priority: 3
assignee: ""
parent: fus-r7km
tags: [windows, phase-3, review-finding]
---
# Verify isTerminal fd-to-Handle casting is correct

CodeRabbit flagged that `isTerminal(fd int)` in `help_width_windows.go` casts the fd parameter directly to `windows.Handle`:

```go
func isTerminal(fd int) bool {
    var mode uint32
    return windows.GetConsoleMode(windows.Handle(fd), &mode) == nil
}
```

On Windows, file descriptors (small integers from C runtime) and Windows HANDLEs (kernel object pointers) are different concepts. `os.Stdout.Fd()` returns a `uintptr` that is a Windows HANDLE, but callers passing a C-style fd (0, 1, 2) would get the wrong result.

Found by: CodeRabbit (major).

Files: internal/cli/help_width_windows.go

## Investigation needed

All callers pass `int(os.File.Fd())`:
- `help.go:65` — `isTerminal(int(os.Stdout.Fd()))`
- `monitor.go:31` — `isTerminal(int(os.Stdin.Fd()))` and `isTerminal(int(os.Stdout.Fd()))`

On Windows, `os.File.Fd()` returns a `uintptr` that IS a Windows HANDLE (Go runtime uses native handles). So the roundtrip `uintptr → int → windows.Handle` should preserve the value. But verify:

1. Confirm no callers pass literal fd numbers (0, 1, 2) — these would be C runtime fds, NOT handles
2. Confirm `int` doesn't truncate the `uintptr` on 64-bit Windows (HANDLE is a pointer-sized value)
3. Check what the Unix implementation receives to ensure the interface is consistent

## Resolution

**No change needed.** All callers pass `int(os.File.Fd())`:
- `help.go:65` — `isTerminal(int(os.Stdout.Fd()))`
- `monitor.go:31` — `isTerminal(int(os.Stdin.Fd()))` and `int(os.Stdout.Fd())`

On Windows, `os.File.Fd()` returns the Windows HANDLE as `uintptr`. Go's `int` and `uintptr` are the same width on all platforms (64-bit on amd64, 32-bit on 386). The `uintptr → int → windows.Handle(uintptr)` roundtrip is lossless. No literal fd numbers (0, 1, 2) are ever passed.

## Acceptance Criteria

1. ~~Verify all callers pass compatible handle values~~ Verified: all pass os.File.Fd()
2. ~~Fix or document if there's a mismatch~~ No mismatch found

## Notes

**2026-03-31T06:30:20Z**

Verified: all callers pass int(os.File.Fd()) which returns a Windows HANDLE on Windows (not a C fd). The cast to windows.Handle is correct. No change needed.
