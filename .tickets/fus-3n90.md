---
id: fus-3n90
status: open
deps: [fus-j81l, fus-28ni]
links: []
created: 2026-03-31T15:13:14Z
type: feature
priority: 1
assignee: Ronny Unger
tags: [windows, security, phase-5, wave-3]
---
# Batch file scanner (.bat/.cmd)

Issue 5.8. Create internal/inspect/batch.go implementing ScanBatch(content []byte) []Signal. Follow ScanShell() pattern with CMD-specific comment handling. Key subtlety: 'REM' needs trailing space or end-of-line check to avoid false-matching 'REMOVE'. '::' only valid at line start (after whitespace). See plan for full pattern list.

## Acceptance Criteria

1. ScanBatch() in internal/inspect/batch.go follows ScanShell() pattern
2. Comment handling: skip lines where TrimSpace starts with 'REM ' (case-insensitive, with trailing space) or '::'
3. REM without trailing space does NOT skip (avoids matching REMOVE)
4. Patterns scan for: lolbin, registry_modify, persistence, destructive_fs, user_modify, firewall_modify
5. Unit tests in batch_test.go with clean content, REM comments, :: comments, and attack patterns
6. Known limitation documented: ^ line continuation not tracked

