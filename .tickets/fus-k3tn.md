---
id: fus-k3tn
status: closed
deps: []
links: []
created: 2026-03-30T09:00:01Z
type: bug
priority: 1
assignee: ""
parent: fus-r7km
tags: [windows, phase-3, review-finding]
---
# Fix flaky TestGetContextVars tests

Two test quality issues in prompt_test.go:

1. **TestGetContextVars_SingleVar** uses exact string equality (`got != "AWS_PROFILE=prod"`) without clearing other tracked env vars. Fails on any machine with KUBECONFIG, AWS_REGION, etc. set.

2. **TestGetContextVars_Empty** uses conflicting dual-cleanup: `t.Setenv(v, "")` + `os.Unsetenv(v)` + manual `t.Cleanup`. Redundant and confusing.

Found by: internal review (medium + low), CodeRabbit (minor).

Files: internal/approve/prompt_test.go

## Fix

**SingleVar**: Either clear all 10 tracked vars first (matching TestGetContextVars_Empty pattern) or switch to `strings.Contains` (matching TestGetContextVars_MultipleVars pattern).

**Empty**: Remove `os.Unsetenv` and manual `t.Cleanup` block. Use only `t.Setenv(v, "")` — getContextVars skips empty values, and t.Setenv handles restoration automatically.

## Acceptance Criteria

1. `TestGetContextVars_SingleVar` passes on machines with cloud env vars set
2. `TestGetContextVars_Empty` uses a single cleanup mechanism
3. `go test ./internal/approve/ -run TestGetContextVars -v -count=10` passes

## Notes

**2026-03-31T06:20:42Z**

Closed: duplicate of fus-iviw (clearTrackedVars).
