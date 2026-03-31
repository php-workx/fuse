---
id: fus-k8ub
status: closed
deps: []
links: []
created: 2026-03-30T13:00:00Z
type: bug
priority: 1
assignee:
parent:
tags: [windows, correctness, phase-3]
---
# Remove hardcoded Windows APPROVAL block gate in runner.go

## Problem

`runner.go:135-139` contains a hardcoded gate that blocks all APPROVAL-level commands on Windows:

```go
if runtime.GOOS == "windows" {
    fmt.Fprintf(os.Stderr, "fuse: BLOCKED — approval not yet supported on Windows\n")
    rc.logWithVerdict("blocked")
    cleanupExecutionState(rc.database, rc.cfg)
    return 1, nil
}
```

Phase 3 implemented the Windows Console API approval prompt (`approve/prompt_windows.go`). The spec marks Phase 3 as DONE. But this gate was never removed, so APPROVAL commands in `fuse run` mode are still unconditionally blocked on Windows.

The codex-shell path (`codexshell.go`) does NOT have this gate, creating an inconsistency: codex-shell can approve commands on Windows but `fuse run` cannot.

**Source:** Go Dev review finding #4 (WARN)

## Where it surfaces

- `internal/adapters/runner.go:135-139` — hardcoded `runtime.GOOS == "windows"` block

## Risk if unfixed

- `fuse run` is unusable for APPROVAL commands on Windows
- Phase 4's process management (job objects, signal forwarding) is untestable for APPROVAL commands in `fuse run`
- Users get inconsistent behavior between run mode and codex-shell mode

## Acceptance Criteria

1. The `runtime.GOOS == "windows"` block at runner.go:135-139 is removed
2. `fuse run <APPROVAL-level-command>` on Windows shows the approval prompt (same as codex-shell)
3. Existing Unix approval flow unchanged
4. `go test ./... -race` passes
5. `GOOS=windows go build ./...` passes

## Test Cases

1. **Code inspection:** `grep -n 'runtime.GOOS.*windows' internal/adapters/runner.go` returns only the `COMPLUS_` env stripping (line ~74), not an APPROVAL block
2. **Manual (Windows):** `fuse run "git push"` — should show approval prompt, not "BLOCKED — approval not yet supported"
3. **Integration:** Existing approval tests pass on Unix
