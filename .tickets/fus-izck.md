---
id: fus-izck
status: closed
deps: []
links: []
created: 2026-03-31T06:10:45Z
type: chore
priority: 3
assignee: Ronny Unger
tags: [testing, windows, phase-3]
---
# Add f.Sync() before os.ReadFile in prompt_windows_test.go

prompt_windows_test.go:79 writes to a temp file via renderPromptPlain then reads it via os.ReadFile (which opens a new fd). Data may still be in the OS buffer. While this works reliably in practice (kernel page cache), an explicit f.Sync() is defensive and eliminates any theoretical flakiness.

One-line fix: add f.Sync() after renderPromptPlain and before os.ReadFile.

Found by: CodeRabbit (#17).

Files: internal/approve/prompt_windows_test.go

## Acceptance Criteria

1. f.Sync() called after renderPromptPlain and before os.ReadFile
2. go test ./internal/approve/... -race passes

