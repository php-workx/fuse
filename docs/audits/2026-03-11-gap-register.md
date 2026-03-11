# 2026-03-11 Gap Register

## Classification Rules

- `release-blocker` - must be fixed or explicitly removed from release scope
- `must-fix-before-rc` - high confidence gap that should be closed before RC1
- `post-rc` - acceptable to defer with documented scope
- `accepted-limit` - not a bug; limitation is already part of the product contract

## Findings

### REL-001

- **Title:** Clean checkout does not contain the CLI entrypoint expected by integration tests
- **Severity:** `release-blocker`
- **Type:** `implementation-gap`
- **Status:** `open`
- **Evidence:**
  - `integration_test.go` builds `./cmd/fuse`
  - `.gitignore:6` ignores `fuse`
  - original checkout contains ignored local `cmd/fuse/main.go`
  - fresh worktree created from `HEAD` does not contain `cmd/fuse/`
  - `go test ./...` fails in the worktree because `./cmd/fuse` is missing
- **Impact:** The repository is not reproducibly buildable/testable from a clean checkout, which invalidates release confidence.
- **Recommended fix:** Track the real CLI entrypoint in git and remove the ignored-local-file dependency from the root integration path.

### REL-002

- **Title:** Codex integration exists but is under-proven for release confidence
- **Severity:** `must-fix-before-rc`
- **Type:** `test-gap`
- **Status:** `open`
- **Evidence:**
  - `internal/adapters/codexshell.go`
  - `internal/adapters/codexshell_test.go`
  - current direct Codex tests cover stdin isolation, disabled-mode bypass, and event pruning
- **Impact:** The product may be usable with Codex, but current evidence is too thin to label the path stable or GA.
- **Recommended fix:** Add explicit tests for enabled-mode SAFE, BLOCKED, and approval-required Codex command handling, then dogfood Codex workflows before release posture is finalized.

### REL-003

- **Title:** Golden fixture depth is materially below the written test-plan target
- **Severity:** `must-fix-before-rc`
- **Type:** `test-gap`
- **Status:** `open`
- **Evidence:**
  - `testdata/fixtures/commands.yaml` has `162` fixture rows
  - current code contains `225` built-in IDs and `22` hardcoded blocked rules
  - `specs/testplan.md` expects positive + near-miss coverage per hardcoded/built-in rule family
- **Impact:** Current golden tests do not yet justify strong claims about full rule-corpus regression protection.
- **Recommended fix:** Expand fixtures for highest-risk families immediately and either complete the full target or narrow the release claim.

### REL-004

- **Title:** Performance and compatibility gates are not currently proven
- **Severity:** `must-fix-before-rc`
- **Type:** `test-gap`
- **Status:** `open`
- **Evidence:**
  - no benchmark or compatibility harness found in current test surface
  - `specs/testplan.md` defines `PERF-*` and `COMPAT-*` sections
- **Impact:** We cannot honestly claim low-friction hot paths or platform support at release confidence.
- **Recommended fix:** Add explicit release-gate commands for performance and a lightweight compatibility matrix, then record results in the readiness report.

### REL-005

- **Title:** Daily-usage friction is not yet validated by dogfooding
- **Severity:** `must-fix-before-rc`
- **Type:** `release-readiness-gap`
- **Status:** `open`
- **Evidence:**
  - safe-command baselines exist in `internal/core/safecmds.go`
  - no current dogfood report exists under `docs/release/`
- **Impact:** The product may be technically correct but still too noisy or disruptive for real usage.
- **Recommended fix:** Run a fixed dogfood matrix across Claude, Codex, and cloud workflows and log actual prompt/friction findings.

### REL-006

- **Title:** Hook-mode TOCTOU remains a documented limitation
- **Severity:** `accepted-limit`
- **Type:** `intentional-divergence`
- **Status:** `accepted`
- **Evidence:**
  - `README.md`
  - `specs/functional.md` §4.3
  - `specs/technical_v1.1.md` §16.1
- **Impact:** Hook mode is a guardrail, not full containment.
- **Recommended fix:** None for v1 beyond honest documentation and stronger positioning of `fuse run` where appropriate.

## Immediate Priority Order

1. `REL-001` clean-checkout/build reproducibility
2. `REL-002` Codex release confidence
3. `REL-003` golden fixture depth
4. `REL-004` performance/compatibility proof
5. `REL-005` dogfood friction evidence

