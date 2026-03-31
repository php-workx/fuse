---
id: fus-g4vs
status: closed
deps: []
links: []
created: 2026-03-28T18:00:02Z
type: task
priority: 2
assignee: ""
parent: fus-r7km
tags: [windows, phase-3, wave-1]
---
# Extract getContextVars to prompt_shared.go

Move `getContextVars()` from `prompt_unix.go:238-258` to `prompt_shared.go`. The function has zero Unix dependencies (only `os.Getenv`) and both platforms need it. Add tests.

Files: internal/approve/prompt_shared.go, internal/approve/prompt_unix.go, internal/approve/prompt_test.go
Wave 1 — no dependencies.

## Design

Simple extraction — no behavioral changes. `getContextVars()` builds a comma-separated string of relevant environment variables (AWS_PROFILE, TF_WORKSPACE, KUBECONFIG, etc.) for display in the approval prompt. Both `prompt_unix.go` and `prompt_windows.go` (Issue fus-p3cw) need this function.

## Code Specification

Move this block from `prompt_unix.go:238-258` to `prompt_shared.go`:

```go
// getContextVars returns relevant environment variables for the prompt.
func getContextVars() string {
    relevantVars := []string{
        "AWS_PROFILE", "AWS_REGION", "AWS_DEFAULT_REGION",
        "TF_WORKSPACE", "TF_VAR_environment",
        "KUBECONFIG", "KUBECONTEXT",
        "GCP_PROJECT", "GOOGLE_CLOUD_PROJECT",
        "AZURE_SUBSCRIPTION",
    }
    var result string
    for _, v := range relevantVars {
        val := os.Getenv(v)
        if val != "" {
            if result != "" {
                result += ", "
            }
            result += v + "=" + val
        }
    }
    return result
}
```

Add to `prompt_test.go`:
- `TestGetContextVars_Empty`: No env vars set → returns ""
- `TestGetContextVars_SingleVar`: AWS_PROFILE=prod → returns "AWS_PROFILE=prod"
- `TestGetContextVars_MultipleVars`: Two vars set → comma-separated result

Run `goimports -w` on both `prompt_shared.go` and `prompt_unix.go` after the move.

## Acceptance Criteria

1. `getContextVars()` defined in `prompt_shared.go`
2. `getContextVars()` removed from `prompt_unix.go`
3. No import changes needed (only uses `os` which is already imported)
4. `go test ./internal/approve/ -run TestGetContextVars -v` passes
5. `just test` passes (no Unix regression)

## Conformance Checks

- content_check: {file: "internal/approve/prompt_shared.go", pattern: "func getContextVars"}
- tests: `go test ./internal/approve/ -run TestGetContextVars -v`

## Notes

**2026-03-31T06:20:38Z**

Closed: implemented in Phase 3/4 commits on feat/windows-terminal-approval branch.
