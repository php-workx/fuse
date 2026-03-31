---
id: fus-iviw
status: closed
deps: []
links: []
created: 2026-03-31T06:09:16Z
type: bug
priority: 2
assignee: Ronny Unger
tags: [testing, phase-3]
---
# Add clearTrackedVars to TestGetContextVars_MultipleVars

prompt_test.go:70 TestGetContextVars_MultipleVars calls t.Setenv without first calling clearTrackedVars(t). The other two tests in the same file (TestGetContextVars_Empty at line 51, TestGetContextVars_SingleVar at line 60) do call it. Host environment variables (e.g. AWS_REGION, KUBECONTEXT) bleed into the output. The test passes anyway because it uses strings.Contains, but it lacks isolation.

One-line fix: add clearTrackedVars(t) as the first line.

Found by: every reviewer — our review, CodeRabbit, Kody, security-dataflow, api-contract, correctness, reliability. Most-duplicated finding in the entire review.

Files: internal/approve/prompt_test.go

Test cases:
- Set AWS_REGION=us-east-1 in host env, run test — output should NOT contain AWS_REGION
- Code inspection: grep -A1 'func TestGetContextVars_MultipleVars' prompt_test.go shows clearTrackedVars(t)

## Acceptance Criteria

1. clearTrackedVars(t) is the first call in TestGetContextVars_MultipleVars
2. go test ./internal/approve/ -run TestGetContextVars -v passes
3. Test is hermetic — same result regardless of host environment

