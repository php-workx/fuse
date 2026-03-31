---
id: fus-kyal
status: closed
deps: []
links: []
created: 2026-03-31T06:08:53Z
type: bug
priority: 2
assignee: Ronny Unger
tags: [reliability, phase-4]
---
# Remove redundant downstreamIn.Close() in mcpproxy.go

mcpproxy.go calls downstreamIn.Close() three times:
- Line 74: outer defer (pre-existing)
- Line 86: inner defer (added in Phase 4)
- Line 98: goroutine (pre-existing, intentional — signals EOF to downstream before Wait)

The outer defer at line 74 is now redundant since the inner defer at line 86 covers the same close. Multiple closes on io.PipeWriter return os.ErrClosed which is swallowed, so no crash — but it's vestigial dead code that confuses readers.

Found by: our reliability explorer, kody (#6 partial).

Files: internal/adapters/mcpproxy.go

Test cases:
- Code inspection: grep -c 'downstreamIn.Close' internal/adapters/mcpproxy.go returns 2 (not 3)
- go test ./internal/adapters/... -race passes

## Acceptance Criteria

1. downstreamIn.Close() called exactly twice: inner defer + goroutine
2. Outer defer at line 74 removed
3. go test ./internal/adapters/... -race passes

