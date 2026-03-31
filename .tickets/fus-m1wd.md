---
id: fus-m1wd
status: closed
deps: []
links: []
created: 2026-03-30T13:00:00Z
type: feature
priority: 2
assignee:
parent:
tags: [windows, reliability, phase-4]
---
# Add job object wrapping to mcpproxy.go for grandchild cleanup

## Problem

`mcpproxy.go:87-89` uses `cmd.Process.Kill()` for downstream server cleanup. On Windows, this only kills the direct child process. If the downstream MCP server spawns grandchildren (e.g., a Python MCP server forking subprocesses), those grandchildren are orphaned on shutdown.

The pattern for job object wrapping is already established in `runner_exec_windows.go` and would add consistent process tree management across all adapters.

**Source:** Go Dev review finding #10 (NOTE), Security review finding #7 (NOTE)

## Where it surfaces

- `internal/adapters/mcpproxy.go:81-91` — `cmd.Start()`, `cmd.Process.Kill()`, `cmd.Wait()` sequence

## Risk if unfixed

Orphaned downstream server children on Windows. Low probability (most MCP servers are single-process) but the pattern exists and is easy to apply.

## Implementation note

This is more invasive than "~15 lines" because `mcpproxy.go` is platform-agnostic. Requires either:
- Build-tagged helper files (`mcpproxy_cleanup_unix.go` / `mcpproxy_cleanup_windows.go`) with a `cleanupChildProcess(cmd)` function
- Or an interface/function variable set by platform-specific `init()`

## Acceptance Criteria

1. On Windows, downstream MCP server child is assigned to a job object
2. Cleanup kills the entire process tree (not just direct child)
3. On Unix, behavior unchanged (no job objects)
4. `GOOS=windows go build ./...` passes
5. `go test ./... -race` passes
6. No changes to mcpproxy.go's public API

## Test Cases

1. **Code inspection:** Windows-tagged file exists with job object wrapping for proxy child
2. **Cross-compile:** `GOOS=windows go vet ./internal/adapters/...` passes
3. **Manual (Windows):** Start a downstream MCP server that spawns a child, kill fuse, verify all processes are terminated
