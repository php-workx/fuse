---
id: fus-wrx7
status: closed
deps: []
links: []
created: 2026-03-31T06:08:02Z
type: bug
priority: 1
assignee: Ronny Unger
tags: [windows, correctness, phase-3]
---
# Fix supportsANSI() to leave VT processing enabled after probe

supportsANSI() in help_width_windows.go:31-44 enables ENABLE_VIRTUAL_TERMINAL_PROCESSING to probe support, then immediately restores the original mode. When shouldColorize() returns true and help output uses ANSI escape sequences, VT processing is no longer active — escape sequences render as raw garbage on older Windows 10 conhost where VT is supported but not pre-enabled. Windows Terminal users are unaffected (VT already in original mode).

Found by: our correctness explorer, Gemini, Qodo, CodeRabbit — all 4 external reviewers flagged this independently.

Files: internal/cli/help_width_windows.go

Test cases:
- On Windows conhost (not Windows Terminal): run 'fuse --help', verify ANSI colors render correctly
- On Windows Terminal: same test, verify no regression
- Code inspection: grep 'SetConsoleMode.*mode)' help_width_windows.go returns 0 matches (restore removed)
- Code inspection: grep 'sync.Once' help_width_windows.go returns 1 match

## Design

Remove the SetConsoleMode restore call at help_width_windows.go:43. Wrap the probe in sync.Once so it runs exactly once per process. The function name changes meaning from 'probe and restore' to 'probe and enable'. supportsANSI restore error logging (line 43) becomes moot after this fix since there is no restore.

## Acceptance Criteria

1. supportsANSI() leaves VT enabled when probe succeeds (no restore)
2. Result cached via sync.Once (probe runs exactly once)
3. fuse --help on Windows shows colored output, not raw ANSI codes
4. GOOS=windows go vet ./internal/cli/... passes


## Notes

**2026-03-31T06:22:20Z**

Fixed: VT left enabled after probe, cached in sync.Once.
