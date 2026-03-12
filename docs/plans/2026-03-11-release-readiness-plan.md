# Release Readiness Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Move `fuse` from "promising and useful for dogfooding" to a stable, releasable product for Claude-first usage, with Codex and cloud workflows validated to a clearly stated confidence level.

**Architecture:** Treat release readiness as a staged risk-reduction program. First prove the implementation matches the accepted specs, then close the highest-severity correctness and test gaps, then validate real-world friction and cloud-safety behavior under dogfooding, and only then cut a release candidate with explicit go/no-go gates.

**Tech Stack:** Go 1.24+, Cobra CLI, Bubble Tea/Lip Gloss, SQLite, `go test`, `go test -race`, golden YAML fixtures, Claude Code hook integration, Codex MCP shell integration, `bd` issue tracking

---

## Release Verdict We Are Targeting

The target is not "perfect security." The target is:

1. Claude integration is safe enough for daily developer use on local and non-production cloud workflows.
2. Dangerous destructive actions on mediated paths are blocked or approval-gated with no known P0 silent bypasses in the documented v1 scope.
3. Safe day-to-day commands do not create unacceptable friction.
4. Codex integration is either:
   - fully validated and included in the release, or
   - explicitly marked beta/fast-follow with clear boundaries.

## Current State

Current honest state:

- Core implementation exists and `go test ./...` passes.
- Claude shell and MCP mediation paths appear materially stronger than Codex validation today.
- The product likely works as a useful guardrail for controlled dogfooding.
- We do not yet have proof that implementation fully matches [functional.md](/Users/runger/workspaces/fuse/specs/functional.md), [technical_v1.1.md](/Users/runger/workspaces/fuse/specs/technical_v1.1.md), and [testplan.md](/Users/runger/workspaces/fuse/specs/testplan.md).
- We do not yet have enough real-world evidence that friction is low enough for daily ops-heavy use.

## Files to Create

| File | Change |
|------|--------|
| `docs/plans/2026-03-11-release-readiness-plan.md` | **NEW** overall roadmap from current state to release |
| `docs/audits/2026-03-11-spec-conformance-matrix.md` | **NEW** implementation/spec traceability artifact |
| `docs/audits/2026-03-11-testplan-traceability.md` | **NEW** test-plan coverage artifact |
| `docs/audits/2026-03-11-gap-register.md` | **NEW** prioritized remediation inventory |
| `docs/audits/2026-03-11-review-summary.md` | **NEW** audit summary with release blockers called out |
| `docs/release/rc1-readiness-report.md` | **NEW** release-candidate go/no-go report |
| `docs/release/dogfood-results.md` | **NEW** dogfood findings, friction notes, and recommended defaults |

## Files to Inspect and Likely Modify

| File | Why |
|------|-----|
| `docs/plans/2026-03-11-spec-conformance-review-plan.md` | prerequisite audit plan |
| `README.md` | release positioning and limitation language |
| `specs/functional.md` | release contract |
| `specs/technical_v1.1.md` | implementation contract |
| `specs/testplan.md` | coverage contract |
| `internal/core/*` | shell classification correctness |
| `internal/policy/*` | rule coverage and precedence |
| `internal/inspect/*` | script inspection correctness |
| `internal/approve/*` | approval semantics and prompt behavior |
| `internal/db/*` | approval/event persistence integrity |
| `internal/adapters/hook.go` | Claude integration path |
| `internal/adapters/mcpproxy.go` | MCP cloud mediation path |
| `internal/adapters/codexshell.go` | Codex integration path |
| `internal/cli/*` | install/doctor/run/proxy usability |
| `testdata/fixtures/commands.yaml` | golden fixture depth |
| `testdata/scripts/*` | scanner fixture depth |

### Task 1: Execute The Spec Conformance Audit

**Files:**
- Use: `docs/plans/2026-03-11-spec-conformance-review-plan.md`
- Create: `docs/audits/2026-03-11-spec-conformance-matrix.md`
- Create: `docs/audits/2026-03-11-testplan-traceability.md`
- Create: `docs/audits/2026-03-11-gap-register.md`
- Create: `docs/audits/2026-03-11-review-summary.md`

**Step 1: Execute the existing audit plan**

Run the audit already defined in:
`docs/plans/2026-03-11-spec-conformance-review-plan.md`

Expected result:
- every requirement from the functional and technical specs is marked `implemented`, `partial`, `gap`, or `intentional divergence`
- every test-plan item is marked `covered`, `partial`, `missing`, or `obsolete`

**Step 2: Identify release blockers**

Mark all findings in `gap-register.md` as one of:
- `release-blocker`
- `must-fix-before-rc`
- `post-rc`
- `document-and-accept`

Release-blocker definition:
- silent destructive bypass inside documented mediation scope
- incorrect approval semantics
- incorrect hook/MCP block behavior
- severe cloud/Codex regression on intended workflows

**Step 3: Create `bd` issues for all must-fix findings**

Run:

```bash
bd create "Release blocker: <short title>" --description "<spec section, evidence, user impact>" -t bug -p 0 --json
bd create "Release readiness: <short title>" --description "<evidence, expected behavior, acceptance criteria>" -t task -p 1 --json
```

**Step 4: Commit**

```bash
git add docs/audits/
git commit -m "docs: record release-readiness audit results"
```

### Task 2: Close P0 and P1 Correctness Gaps

**Files:**
- Modify: `internal/core/*`
- Modify: `internal/policy/*`
- Modify: `internal/inspect/*`
- Modify: `internal/approve/*`
- Modify: `internal/db/*`
- Modify: `internal/adapters/*`
- Modify: `internal/cli/*`
- Modify: `*_test.go`
- Modify: `testdata/fixtures/commands.yaml`
- Modify: `testdata/scripts/*`

**Step 1: Fix P0 silent-bypass or wrong-decision behavior first**

Examples of P0-class issues:
- destructive command becomes `SAFE`
- hook path returns a non-blocking error when it should block
- MCP destructive call reaches downstream without approval
- file-backed approval can be reused incorrectly

For each issue:
1. Write the failing test or fixture first.
2. Run the focused test and confirm it fails.
3. Implement the smallest fix.
4. Re-run the focused test.
5. Re-run the broader package tests.

**Step 2: Fix P1 confidence gaps next**

Examples of P1-class issues:
- missing near-miss fixture coverage for major rule families
- unsupported but claimed-safe behavior in Codex path
- scanner ambiguity that falls the wrong way
- daily developer safe-command false positives on common workflows

**Step 3: Keep the spec honest**

If the implementation is intentionally better or narrower than the spec:
- either align the code to the spec
- or update the spec/README to reflect reality

Do not leave unresolved drift between behavior and claims.

**Step 4: Verification**

Run after each merged remediation batch:

```bash
go test ./...
go test ./internal/core ./internal/policy ./internal/inspect ./internal/approve ./internal/db -race -v
```

**Step 5: Commit**

```bash
git add .
git commit -m "fix: close release-readiness correctness gaps"
```

### Task 3: Raise Test and Fixture Confidence To Release Level

**Files:**
- Modify: `testdata/fixtures/commands.yaml`
- Modify: `testdata/scripts/*`
- Modify: `internal/core/classify_test.go`
- Modify: `internal/core/normalize_test.go`
- Modify: `internal/core/inspect_test.go`
- Modify: `internal/adapters/hook_test.go`
- Modify: `internal/adapters/mcpproxy_test.go`
- Modify: `internal/adapters/codexshell_test.go`
- Modify: `internal/db/db_test.go`
- Modify: `integration_test.go`

**Step 1: Fill rule-fixture coverage gaps**

Target:
- every hardcoded blocked rule has at least one positive and one near-miss fixture
- every built-in rule ID in scope for v1 has at least one positive and one near-miss fixture

If this target proves too large for immediate release, narrow it explicitly:
- require full fixture coverage for hardcoded rules and highest-risk cloud/IaC/security families
- document lower-priority families as post-release work

Do not silently ship with an unachieved coverage claim.

**Step 2: Strengthen Codex-path tests**

Add focused tests for:
- `SAFE` Codex command flow
- `APPROVAL` Codex command denial on non-interactive terminal
- `BLOCKED` Codex command handling when enabled
- event logging and exit-code behavior

The current Codex suite is not deep enough for strong release confidence.

**Step 3: Add seam tests for friction-sensitive safe flows**

Add explicit tests covering:
- `git status`
- `git diff`
- `go test`
- `pytest`
- `terraform plan`
- `kubectl get`
- common `aws/gcloud/az` read-only commands

Goal: prove the commands people use every day remain unprompted.

**Step 4: Add release-gate commands**

Run:

```bash
go test ./...
go test -race ./...
```

If `-race ./...` is too slow or flaky, define the exact required package subset and write that into `rc1-readiness-report.md`.

**Step 5: Commit**

```bash
git add .
git commit -m "test: raise release-readiness coverage"
```

### Task 4: Dogfood Real Claude, Codex, and Cloud Workflows

**Files:**
- Create: `docs/release/dogfood-results.md`
- Modify: `README.md`
- Modify: install/doctor docs if needed

**Step 1: Define dogfood scenarios**

Minimum scenario groups:
- Claude local coding workflow
- Claude cloud/infra workflow
- Codex local coding workflow
- Codex cloud/infra workflow
- MCP cloud delete/read/update workflow

For each scenario record:
- command/tool called
- expected classification
- actual classification
- whether a prompt appeared
- whether the prompt was justified
- whether the flow felt disruptive

**Step 2: Run a fixed dogfood matrix**

At minimum cover:
- 20 SAFE daily commands
- 10 CAUTION commands
- 10 APPROVAL cloud/destructive commands
- 5 BLOCKED catastrophic/self-protection commands
- 10 script-backed commands with mixed safe/dangerous content
- 10 MCP tool calls across read/update/delete classes

**Step 3: Measure friction**

Define two simple release thresholds:
- SAFE path should feel effectively invisible on normal coding loops
- approval prompts should be mostly "obviously justified," not noisy or surprising

Any recurring false-positive prompt on normal workflows becomes a P1 release issue.

**Step 4: Decide Codex release posture**

After dogfooding, choose exactly one:
- `Claude GA, Codex beta`
- `Claude GA, Codex excluded from v1 release`
- `Claude GA, Codex GA`

Do not ship ambiguous language.

**Step 5: Commit**

```bash
git add docs/release/dogfood-results.md README.md
git commit -m "docs: record dogfood findings and release posture"
```

### Task 5: Cut A Release Candidate And Make A Go/No-Go Decision

**Files:**
- Create: `docs/release/rc1-readiness-report.md`
- Modify: `README.md`
- Modify: release notes or changelog if the repo has them

**Step 1: Write the readiness report**

The report must answer:
- what is in scope for the release
- what is proven by tests
- what is proven by dogfooding
- what limitations remain
- whether Codex is GA, beta, or deferred
- whether cloud usage is recommended for prod or only non-prod

**Step 2: Apply explicit release gates**

RC1 requires all of the following:
- no open P0 issues
- no unowned P1 issues in mediated destructive paths
- full test suite green
- required race suite green
- audit artifacts completed
- dogfood results recorded
- README/limitations aligned with reality

**Step 3: Decide product wording**

Use one of these positions:
- `release-ready for Claude local-first workflows; Codex beta`
- `release-ready for Claude and Codex local-first workflows`
- `not release-ready; continue dogfood only`

Do not claim stronger safety than the evidence supports.

**Step 4: Commit**

```bash
git add docs/release/rc1-readiness-report.md README.md
git commit -m "docs: add rc1 readiness report"
```

## Verification

Run these commands before declaring the product release-ready:

1. `go test ./...`
2. `go test -race ./...`
3. `rg -n '^  - command:' testdata/fixtures/commands.yaml | wc -l`
4. `rg -o 'builtin:[A-Za-z0-9:-]+' internal/policy | sort -u | wc -l`
5. `rg -n 'regexp\\.MustCompile' internal/policy/hardcoded.go | wc -l`
6. `git diff -- docs/audits docs/release README.md`

## Exit Criteria

We are at a stable, releasable point only when all are true:

1. The spec-conformance audit is complete.
2. All release-blocking gaps are fixed or explicitly documented and accepted.
3. Test-plan coverage claims match reality.
4. Daily SAFE workflows have been dogfooded and judged low-friction.
5. Dangerous mediated workflows have been dogfooded and judged correctly blocked or approval-gated.
6. The release statement in README and release docs matches the actual evidence.

## Recommended Execution Order

1. Execute the spec-conformance audit.
2. Fix P0/P1 correctness issues.
3. Raise fixture and integration-test confidence.
4. Dogfood Claude/Codex/cloud workflows.
5. Cut RC1 and make a go/no-go call.
