# Spec Conformance Review Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Systematically verify that the current codebase implements `specs/functional.md` and `specs/technical_v1.1.md`, and that the existing tests and fixtures satisfy `specs/testplan.md`.

**Architecture:** Run the review as a traceability audit, not an ad hoc code walk. First build a requirement matrix from the three spec documents, then audit the codebase in subsystem slices, then map every required behavior to tests and fixtures, and finally turn confirmed gaps into a prioritized issue list.

**Tech Stack:** Go 1.21+, Cobra CLI, Bubble Tea/Lip Gloss TUI, SQLite, `rg`, `go test`, `go test -race`, YAML golden fixtures in `testdata/fixtures/commands.yaml`, script fixtures in `testdata/scripts/`

---

## Baseline Audit

Run these commands before the audit starts and record the results in the audit artifacts. These numbers anchor the review and make completion measurable.

| Metric | Command | Current Result |
|---|---|---|
| Full test suite | `go test ./...` | pass |
| Internal source files | `find internal -type f | wc -l` | `72` |
| Spec files | `find specs -type f | wc -l` | `11` |
| Script fixtures | `find testdata/scripts -type f | wc -l` | `7` |
| Go test files | `find . -name '*_test.go' | wc -l` | `24` |
| Top-level Go tests | `rg '^func Test' -g '*_test.go' | wc -l` | `177` |
| Test defs incl. subtests | `rg -n 'func Test|t\\.Run\\(' internal | wc -l` | `220` |
| Golden command fixtures | `rg -n '^  - command:' testdata/fixtures/commands.yaml | wc -l` | `162` |
| Built-in rule IDs | `rg -o 'builtin:[A-Za-z0-9:-]+' internal/policy | sort -u | wc -l` | `225` |
| Hardcoded blocked rules | `rg -n 'regexp\\.MustCompile' internal/policy/hardcoded.go | wc -l` | `22` |
| Existing spec audit plan | `find agents/plans -type f | sed -n '1,80p'` | `agents/plans/2026-03-10-spec-audit-fixes.md` |

Initial implication to verify during the audit: `specs/testplan.md` expects at least `2 x (builtins + hardcoded)` golden fixture rows. With the current baseline that threshold is `494`, while `commands.yaml` currently has `162` rows. Treat this as a likely gap until proven otherwise.

## Files to Create

| File | Change |
|------|--------|
| `docs/audits/2026-03-11-spec-conformance-matrix.md` | **NEW** master traceability matrix for functional + technical requirements |
| `docs/audits/2026-03-11-testplan-traceability.md` | **NEW** mapping of test-plan IDs to real tests, fixtures, and gaps |
| `docs/audits/2026-03-11-gap-register.md` | **NEW** prioritized findings log with evidence and remediation owner |
| `docs/audits/2026-03-11-review-summary.md` | **NEW** executive summary with pass/partial/gap counts |

## Files to Inspect

| File | Why |
|------|-----|
| `specs/functional.md` | product contract and user-visible behavior |
| `specs/technical_v1.1.md` | implementation contract and testing requirements |
| `specs/testplan.md` | required test inventory and coverage criteria |
| `agents/plans/2026-03-10-spec-audit-fixes.md` | prior audit evidence to reuse and challenge |
| `README.md` | public surface sanity check against spec |
| `internal/core/normalize.go` | normalization pipeline |
| `internal/core/compound.go` | compound command splitting |
| `internal/core/classify.go` | decision engine |
| `internal/core/mcpclassify.go` | MCP classification |
| `internal/core/inspect.go` | referenced-file detection and inspection dispatch |
| `internal/core/sanitize.go` | context sanitization |
| `internal/core/safecmds.go` | safe command baselines |
| `internal/policy/hardcoded.go` | non-overridable BLOCKED rules |
| `internal/policy/builtins_core.go` | built-in operational rules |
| `internal/policy/builtins_security.go` | built-in security rules |
| `internal/policy/policy.go` | policy load/evaluation behavior |
| `internal/inspect/python.go` | Python scanner |
| `internal/inspect/shell.go` | shell scanner |
| `internal/inspect/javascript.go` | JS/TS scanner |
| `internal/approve/manager.go` | approval lifecycle |
| `internal/approve/hmac.go` | approval integrity |
| `internal/approve/prompt.go` | TUI prompt behavior |
| `internal/db/db.go` | DB open/connection behavior |
| `internal/db/schema.go` | schema and migrations |
| `internal/db/approvals.go` | atomic approval consumption |
| `internal/db/events.go` | event persistence and scrubbing |
| `internal/adapters/hook.go` | Claude hook semantics |
| `internal/adapters/runner.go` | `fuse run` execution flow |
| `internal/adapters/mcpproxy.go` | MCP proxy path |
| `internal/adapters/codexshell.go` | Codex shell MCP behavior |
| `internal/cli/install.go` | install/uninstall integration |
| `internal/cli/doctor.go` | diagnostics contract |
| `internal/cli/run.go` | CLI entrypoint semantics |
| `internal/cli/proxy.go` | proxy command wiring |
| `testdata/fixtures/commands.yaml` | golden rule corpus |
| `testdata/scripts/*` | scanner fixture coverage |
| `integration_test.go` | top-level end-to-end coverage |

### Task 1: Build The Master Requirement Matrix

**Files:**
- Create: `docs/audits/2026-03-11-spec-conformance-matrix.md`
- Inspect: `specs/functional.md`
- Inspect: `specs/technical_v1.1.md`
- Inspect: `specs/testplan.md`

**Step 1: Create the matrix skeleton**

Use one row per normative requirement. Columns must be:
`Requirement ID | Source | Requirement text | Implementation files | Existing tests/fixtures | Status | Evidence | Notes`

Use these requirement ID prefixes:
- `FUNC-<section>-<n>` for `specs/functional.md`
- `TECH-<section>-<n>` for `specs/technical_v1.1.md`
- `TEST-<section>-<id>` for `specs/testplan.md`

**Step 2: Extract functional requirements**

Read and decompose these sections into rows:
- `functional.md` §7 through §19
- treat §3 and §4 as boundary and threat-model checks
- treat §20 and §21 as delivery/success-criteria checks, not implementation details

Expected result: every item in `functional.md` §9-§19 appears in the matrix at least once.

**Step 3: Extract technical requirements**

Read and decompose these sections into rows:
- `technical_v1.1.md` §3 through §13
- `technical_v1.1.md` §16 for explicit limitation checks

Expected result: every pipeline stage, rule-engine rule family, approval/storage requirement, execution mode, MCP proxy behavior, and testing requirement has a row.

**Step 4: Cross-link the test plan**

For every `specs/testplan.md` case or coverage rule, add the related matrix row IDs in a `Covers` column inside `docs/audits/2026-03-11-testplan-traceability.md`.

**Step 5: Commit checkpoint**

```bash
git add docs/audits/2026-03-11-spec-conformance-matrix.md docs/audits/2026-03-11-testplan-traceability.md
git commit -m "docs: add spec conformance audit matrix skeleton"
```

### Task 2: Audit Shell Classification And Policy Behavior

**Files:**
- Inspect: `internal/core/normalize.go`
- Inspect: `internal/core/compound.go`
- Inspect: `internal/core/classify.go`
- Inspect: `internal/core/sanitize.go`
- Inspect: `internal/core/safecmds.go`
- Inspect: `internal/core/mcpclassify.go`
- Inspect: `internal/policy/hardcoded.go`
- Inspect: `internal/policy/builtins_core.go`
- Inspect: `internal/policy/builtins_security.go`
- Inspect: `internal/policy/policy.go`
- Test: `internal/core/normalize_test.go`
- Test: `internal/core/compound_test.go`
- Test: `internal/core/classify_test.go`
- Test: `internal/core/sanitize_test.go`
- Test: `internal/core/safecmds_test.go`
- Test: `internal/core/mcpclassify_test.go`
- Test: `internal/policy/policy_test.go`

**Step 1: Verify the pipeline order**

Check the implementation against `technical_v1.1.md` §5.2-§5.5:
- input validation
- display normalization
- compound splitting
- classification normalization
- inner-command extraction
- suspicious inline detection
- sanitization
- referenced-file detection
- rule evaluation
- escalation modifier
- decision-key generation

Expected result: each stage is implemented once, in the intended order, or logged as a gap.

**Step 2: Verify decision precedence**

Confirm:
- hardcoded BLOCKED rules are not overrideable
- user policy overrides built-ins but not hardcoded rules
- built-ins beat fallback heuristics
- most-restrictive-wins behavior exists for both rule matches and compound commands

Run:

```bash
go test ./internal/core ./internal/policy -run 'Test(Classify|DisplayNormalize|ClassificationNormalize|SplitCompound|Policy|MCPClassify)' -v
```

Expected: pass, with enough evidence to cite exact tests per requirement row.

**Step 3: Verify rule-corpus coverage against the spec**

Build a subsection checklist for:
- hardcoded blocked rules
- Git
- AWS
- GCP
- Azure
- Terraform/CDK/Pulumi
- Kubernetes
- Containers
- Databases
- Remote execution
- Local filesystem
- interpreter launches
- credential access
- exfiltration
- persistence/privesc
- package managers
- reconnaissance

Expected result: every technical spec subsection in §6.2-§6.5 maps to concrete code and at least one test or fixture, or is marked missing.

### Task 3: Audit Referenced-File Inspection And Approval/Storage Semantics

**Files:**
- Inspect: `internal/core/inspect.go`
- Inspect: `internal/inspect/python.go`
- Inspect: `internal/inspect/shell.go`
- Inspect: `internal/inspect/javascript.go`
- Inspect: `internal/approve/manager.go`
- Inspect: `internal/approve/hmac.go`
- Inspect: `internal/approve/prompt.go`
- Inspect: `internal/db/db.go`
- Inspect: `internal/db/schema.go`
- Inspect: `internal/db/approvals.go`
- Inspect: `internal/db/events.go`
- Test: `internal/core/inspect_test.go`
- Test: `internal/inspect/python_test.go`
- Test: `internal/inspect/shell_test.go`
- Test: `internal/inspect/javascript_test.go`
- Test: `internal/approve/manager_test.go`
- Test: `internal/approve/hmac_test.go`
- Test: `internal/db/db_test.go`

**Step 1: Verify inspection boundaries**

Check the implementation against:
- `functional.md` §9.2 and §12
- `technical_v1.1.md` §5.5 and §7

Specifically confirm:
- supported file types
- single-file boundary only
- canonical symlink resolution
- oversized-file behavior
- parse failure fallback
- decision-key file hash binding

**Step 2: Verify approval semantics**

Confirm:
- `Approve Once` and `Deny` are the only v1 user actions
- decision keys differ correctly for shell, script-backed shell, and MCP
- approvals are single-use
- TTL and invalidation rules match the spec
- hook mode and run mode use the right deny/allow behavior

Run:

```bash
go test ./internal/core ./internal/inspect ./internal/approve ./internal/db -race -v
```

Expected: pass; race coverage specifically validates approval-storage semantics from `testplan.md`.

**Step 3: Verify persistence and privacy**

Check:
- lazy DB open behavior for SAFE/BLOCKED hot paths
- event retention / pruning logic
- HMAC secret handling
- credential scrubbing
- no forbidden data retained beyond the spec

Expected result: all `functional.md` §15 and `technical_v1.1.md` §8-§9, §14 are accounted for in code and tests.

### Task 4: Audit Hook, Run, CLI, And MCP Surfaces

**Files:**
- Inspect: `internal/adapters/hook.go`
- Inspect: `internal/adapters/runner.go`
- Inspect: `internal/adapters/mcpproxy.go`
- Inspect: `internal/adapters/codexshell.go`
- Inspect: `internal/cli/install.go`
- Inspect: `internal/cli/doctor.go`
- Inspect: `internal/cli/run.go`
- Inspect: `internal/cli/proxy.go`
- Test: `internal/adapters/hook_test.go`
- Test: `internal/adapters/runner_test.go`
- Test: `internal/adapters/mcpproxy_test.go`
- Test: `internal/adapters/codexshell_test.go`
- Test: `internal/cli/install_test.go`
- Test: `internal/cli/doctor_test.go`
- Test: `internal/cli/run_test.go`
- Test: `integration_test.go`

**Step 1: Verify Claude hook behavior**

Check `technical_v1.1.md` §3.1, §4.1, and §10.2 against the code:
- input schema validation
- exit code behavior
- stderr text behavior
- `/dev/tty` prompting
- hook timeout behavior
- block semantics on malformed input

**Step 2: Verify `fuse run` behavior**

Check:
- exact single-string `--` parsing
- re-verification before execution
- streamed stdout/stderr behavior
- exit-code semantics
- environment sanitization

**Step 3: Verify MCP proxy and Codex shell behavior**

Check:
- proxy routing
- request ID tracking
- structured deny responses
- resource read handling for sensitive paths
- Codex shell execution model versus hook mode

Run:

```bash
go test ./internal/adapters ./internal/cli ./... -run 'Test(RunHook|BuildChildEnv|ExecuteCommand|Proxy|RunDoctor|Install|ParseRunCommandArg|ExecuteCodexShellCommand)' -v
```

Expected: pass, or exact failing requirement rows captured in the gap register.

### Task 5: Audit Test Plan Coverage And Fixture Depth

**Files:**
- Create: `docs/audits/2026-03-11-testplan-traceability.md`
- Inspect: `specs/testplan.md`
- Inspect: `testdata/fixtures/commands.yaml`
- Inspect: `testdata/scripts/*`
- Inspect: all `*_test.go` files already listed above

**Step 1: Map every test-plan ID to code**

For each section in `specs/testplan.md`, record:
- test-plan ID
- expected behavior
- existing test name(s)
- existing fixture(s)
- status: `covered`, `partial`, `missing`, `obsolete`

Expected result: no test-plan section remains unmapped.

**Step 2: Verify golden fixture sufficiency**

Run:

```bash
rg -n '^  - command:' testdata/fixtures/commands.yaml | wc -l
rg -o 'builtin:[A-Za-z0-9:-]+' internal/policy | sort -u | wc -l
rg -n 'regexp\.MustCompile' internal/policy/hardcoded.go | wc -l
```

Expected:
- actual fixture count recorded
- required minimum fixture count computed
- every hardcoded rule has a positive and near-miss fixture
- every built-in rule ID has a positive and near-miss fixture

If the current baseline still holds, log this as a P1/P0 coverage gap rather than a vague observation.

**Step 3: Verify scanner-fixture depth**

Confirm the test suite covers the states required by `specs/testplan.md`:
- benign
- suspicious
- dangerous
- truncated
- unsupported type
- binary
- empty
- symlinked

Expected result: each scanner family (`python`, `shell`, `javascript`) has explicit evidence or a missing-test entry.

**Step 4: Verify performance and compatibility coverage**

Check whether any current tests or benchmarks prove:
- warm-path latency target
- cold-path latency target
- macOS/Linux compatibility
- hook and MCP end-to-end behavior

If no code-based proof exists, record the exact missing harnesses under `gap-register.md` instead of hand-waving.

### Task 6: Produce Findings, Prioritize Them, And Track Them

**Files:**
- Create: `docs/audits/2026-03-11-gap-register.md`
- Create: `docs/audits/2026-03-11-review-summary.md`

**Step 1: Classify each finding**

Each finding must have:
- `Severity`: `P0`, `P1`, `P2`, `P3`
- `Type`: `implementation-gap`, `test-gap`, `spec-drift`, `doc-drift`, `intentional-divergence`
- `Source`: exact spec section(s)
- `Evidence`: file and test references
- `Recommended fix`: one sentence

**Step 2: Separate code gaps from documentation gaps**

Do not mix:
- code does not implement the spec
- test plan says coverage should exist, but tests do not prove it
- README or comments drift from the accepted spec

This separation matters because the remediation owners differ.

**Step 3: Convert confirmed gaps into `bd` issues**

If you are executing this review for real, track the follow-up work in `bd`:

```bash
bd ready --json
bd create "Spec conformance remediation: <short title>" --description "<spec section, evidence, expected behavior>" -t bug -p 1 --json
```

If the audit is already being tracked by a parent bead, use `--deps discovered-from:<parent-id>`.

**Step 4: Commit checkpoint**

```bash
git add docs/audits/2026-03-11-gap-register.md docs/audits/2026-03-11-review-summary.md docs/audits/2026-03-11-testplan-traceability.md docs/audits/2026-03-11-spec-conformance-matrix.md
git commit -m "docs: record spec conformance audit findings"
```

## Verification

Run these commands before calling the review complete:

1. `go test ./...`
2. `go test ./internal/core ./internal/policy ./internal/inspect ./internal/approve ./internal/db -race -v`
3. `rg -n '^  - command:' testdata/fixtures/commands.yaml | wc -l`
4. `rg -o 'builtin:[A-Za-z0-9:-]+' internal/policy | sort -u | wc -l`
5. `rg -n 'regexp\\.MustCompile' internal/policy/hardcoded.go | wc -l`
6. `git diff -- docs/audits/`

Expected completion state:
- every functional and technical requirement row is marked `implemented`, `partial`, `gap`, or `intentional divergence`
- every test-plan row is marked `covered`, `partial`, `missing`, or `obsolete`
- every gap has evidence and a recommended owner
- all resulting follow-up work is either fixed or tracked in `bd`

## Deliverables

At the end of the review, there should be exactly four durable outputs:

1. `docs/audits/2026-03-11-spec-conformance-matrix.md`
2. `docs/audits/2026-03-11-testplan-traceability.md`
3. `docs/audits/2026-03-11-gap-register.md`
4. `docs/audits/2026-03-11-review-summary.md`

These four files are the handoff package. They should let a new engineer answer:
- what the specs require
- what the code actually does
- what the tests prove
- what remains to fix
