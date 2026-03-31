---
id: fus-tssy
status: closed
deps: []
links: []
created: 2026-03-31T06:10:17Z
type: chore
priority: 3
assignee: Ronny Unger
tags: [testing, windows, phase-4]
---
# Update stale Windows skip messages in test files

19 test skip guards across 2 files use the stale message 'shell execution not yet supported on Windows':
- internal/adapters/runner_test.go: 4 instances (lines 435, 457, 472, 488)
- internal/adapters/codexshell_test.go: 15 instances (lines 162, 195, 227, 261, 283, 303, 442, 477, 512, 547, 586, 628, 700, 736, 861)

Phase 4 fully implements Windows shell execution. The tests still need to skip on Windows because they use Unix-only commands (echo, printf, cat, rm), but the stated reason is factually incorrect and misleads anyone reading the tests into thinking the feature is still missing.

Found by: our test-adequacy explorer.

Files: internal/adapters/runner_test.go, internal/adapters/codexshell_test.go

Suggested replacement: t.Skip("test uses Unix-specific shell commands")

## Acceptance Criteria

1. No test file contains 'shell execution not yet supported on Windows'
2. Skip messages accurately reflect the real reason (Unix-specific test commands)
3. go test ./internal/adapters/... -race passes

