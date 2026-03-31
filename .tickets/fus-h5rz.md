---
id: fus-h5rz
status: closed
deps: []
links: []
created: 2026-03-30T09:00:06Z
type: task
priority: 3
assignee: ""
parent: fus-r7km
tags: [windows, phase-3, review-finding]
---
# Strengthen TestRenderPromptPlain assertions

`TestRenderPromptPlain_DoesNotPanic` only checks `info.Size() > 0` — a single newline would satisfy this. The test doesn't verify that the command, reason, or header are actually rendered.

Found by: internal review (low).

Files: internal/approve/prompt_windows_test.go

## Fix

Read the file content after writing and assert it contains key strings:
- `"echo hello"` (the command)
- `"test reason"` (the reason)
- `"fuse: approval required"` (the header)

## Acceptance Criteria

1. Test verifies prompt content, not just non-zero size
2. `GOOS=windows go vet ./internal/approve/...` passes

## Notes

**2026-03-31T06:30:20Z**

Already done: TestRenderPromptPlain_RendersContent asserts 'echo hello', 'test reason', and 'fuse: approval required' in output.
