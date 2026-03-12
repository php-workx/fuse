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
- **Status:** `fixed on release-readiness-audit branch`
- **Evidence:**
  - `integration_test.go` builds `./cmd/fuse`
  - `.gitignore:6` ignores `fuse`
  - original checkout contains ignored local `cmd/fuse/main.go`
  - fresh worktree created from `HEAD` does not contain `cmd/fuse/`
  - `go test ./...` fails in the worktree because `./cmd/fuse` is missing
- **Impact:** The repository is not reproducibly buildable/testable from a clean checkout, which invalidates release confidence.
- **Recommended fix:** Make the integration path build the actual tracked CLI target from a clean checkout. That may mean tracking the entrypoint correctly in git, changing the integration test to build the actual tracked target, or both.
- **Resolution:** Fixed in commit `408bc5f` by tracking `cmd/fuse/main.go` and narrowing `.gitignore` from `fuse` to `/fuse`, which restores clean-worktree reproducibility for `go test ./...`.

### REL-002

- **Title:** Codex integration exists but is under-proven for release confidence
- **Severity:** `must-fix-before-rc`
- **Type:** `test-gap`
- **Status:** `partially addressed on release-readiness-audit branch`
- **Evidence:**
  - `internal/adapters/codexshell.go`
  - `internal/adapters/codexshell_test.go`
  - direct Codex tests now cover stdin isolation, disabled-mode bypass, event pruning, enabled-mode `SAFE`, enabled-mode `BLOCKED`, approval-required handling without a TTY, and JSON-RPC shell-server request handling
- **Impact:** The product may be usable with Codex, but current evidence is too thin to label the path stable or GA.
- **Recommended fix:** Add stronger end-to-end and dogfood evidence around the Codex shell execution seam before release posture is finalized.
- **Progress:** Commit `b0edc47` adds enabled-mode `SAFE`, `BLOCKED`, and approval-without-TTY tests in `internal/adapters/codexshell_test.go`. Commit `65e5038` hardens those tests by isolating `FUSE_HOME` and using harmless blocked and approval fixtures. This branch also covers `RunCodexShellServer` at the MCP/JSON-RPC boundary for `initialize`, `tools/list`, and `tools/call` success/error paths. Remaining work is real dogfood evidence and final release posture.

### REL-003

- **Title:** Golden fixture depth is materially below the written test-plan target
- **Severity:** `must-fix-before-rc`
- **Type:** `test-gap`
- **Status:** `partially addressed on release-readiness-audit branch`
- **Evidence:**
  - `testdata/fixtures/commands.yaml` now has `182` fixture rows
  - current code contains `225` built-in IDs and `22` hardcoded blocked rules
  - `specs/testplan.md` expects positive + near-miss coverage per hardcoded/built-in rule family
  - `internal/core/fixture_coverage_test.go` now enforces hardcoded blocked coverage and minimum coverage for several high-risk command families
  - `internal/core/classify_test.go` now implements the `UNIT-RULE-006` sentinel matrix across built-in sections `§6.3.1` through `§6.3.21`
- **Impact:** Current golden tests do not yet justify strong claims about full rule-corpus regression protection.
- **Recommended fix:** Continue expanding fixtures for highest-risk families and either complete the full target or narrow the release claim before RC1.
- **Progress:** Commit `65e5038` adds high-risk fixture coverage guards and expands `testdata/fixtures/commands.yaml`. Commit `e6d3793` closes the classifier regression exposed by that fixture work: hardcoded self-protection rules now still win for inline interpreter payloads and unclosed heredoc writes targeting `~/.fuse`, rather than silently downgrading to `APPROVAL`. This branch also fixes inline pipeline handling so commands such as `curl ... | bash` no longer degrade to `SAFE` after compound splitting, and adds the full built-in-section sentinel matrix from `UNIT-RULE-006`. That sentinel slice also tightened rule behavior for `find . -delete`, `pip install https://...`, and `nc -zv ...` near-miss handling. Remaining work is broader per-rule corpus depth and final contract alignment.

### REL-007

- **Title:** Hardcoded self-protection rules could be bypassed by inline interpreter handling and heredoc parse failure
- **Severity:** `release-blocker`
- **Type:** `implementation-gap`
- **Status:** `fixed on release-readiness-audit branch`
- **Evidence:**
  - `python -c "import shutil; shutil.rmtree('~/.fuse/config')"` was classified `APPROVAL`
  - `cat > ~/.fuse/config/policy.yaml << EOF` was classified `APPROVAL`
  - `internal/core/classify.go` evaluated hardcoded rules on sanitized commands and skipped them entirely on compound parse failure
- **Impact:** Commands targeting fuse-managed config could fall back to approval-required instead of non-overridable `BLOCKED`, weakening self-protection semantics.
- **Recommended fix:** Evaluate hardcoded rules on the normalized unsanitized command and before parse-error fallback approval.
- **Resolution:** Fixed in commit `e6d3793` by checking hardcoded rules before parse-failure fallback in `Classify` and before sanitization-based rule evaluation in `classifySingleCommand`, with regression tests for inline interpreter and heredoc cases.

### REL-004

- **Title:** Performance and compatibility gates are not currently proven
- **Severity:** `must-fix-before-rc`
- **Type:** `test-gap`
- **Status:** `partially addressed on release-readiness-audit branch`
- **Evidence:**
  - `internal/releasecheck/releasecheck_test.go`
  - `scripts/run-release-checks.sh`
  - current `darwin/arm64` smoke run shows `PERF-001`, `PERF-002`, and `PERF-002A` comfortably under their stated thresholds
  - cross-builds now pass for `darwin/arm64`, `darwin/amd64`, `linux/amd64`, and `linux/arm64`
  - shell wrapper compatibility passes locally for `bash`, `zsh`, and `fish`
  - locale invariance passes locally for `LC_ALL=C`, `en_US.UTF-8`, and `ja_JP.UTF-8`
  - `GOTOOLCHAIN=go1.21.13 go test ./...` fails immediately because `go.mod` requires `go >= 1.25.0`
  - `PERF-003` currently fails on large unmatched uppercase inputs (`32 KB` p95 about `104 ms`, `64 KB` p95 about `208 ms`)
  - `specs/testplan.md` defines `PERF-*` and `COMPAT-*` sections
- **Impact:** We can now make several measured current-machine claims, but we still cannot honestly claim the written Go-version floor or full pathological-input performance target at release confidence.
- **Recommended fix:** Keep the new release-check harness, then either lower the actual Go/dependency floor and optimize long-input classification or narrow the written compatibility/performance contract before RC1.

### REL-008

- **Title:** Supported minimum Go version no longer matches the spec and test plan
- **Severity:** `must-fix-before-rc`
- **Type:** `compatibility-gap`
- **Status:** `open`
- **Evidence:**
  - `specs/technical_v1.1.md` §1.1 states minimum Go `1.21`
  - `specs/testplan.md` `COMPAT-002` explicitly requires validating `go1.21.x`
  - `go.mod` declares `go 1.25.0`
  - `docs/release/2026-03-12-perf-compat-smoke.md` shows `GOTOOLCHAIN=go1.21.13 go test -count=1 ./...` fails immediately
- **Impact:** The current repo cannot honestly claim the documented minimum Go version.
- **Recommended fix:** Either lower the repo/toolchain floor and prove `go1.21.x` support, or update the spec, test plan, and release scope to the newer supported minimum.

### REL-009

- **Title:** Pathological uppercase-input performance misses the written `PERF-003` threshold
- **Severity:** `must-fix-before-rc`
- **Type:** `performance-gap`
- **Status:** `open`
- **Evidence:**
  - `internal/releasecheck/releasecheck_test.go`
  - `docs/release/2026-03-12-perf-compat-smoke.md`
  - current darwin/arm64 baseline records `uppercase-32k` p95 about `103 ms` and `uppercase-64k` p95 about `205 ms`
- **Impact:** The repo now has proof that `PERF-003` is not currently satisfied for at least one pathological input family.
- **Recommended fix:** Reduce display/classification overhead on long inert inputs, or narrow the published `PERF-003` claim before RC1.

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

1. `REL-008` Go-version compatibility floor
2. `REL-009` pathological-input performance threshold
3. `REL-002` Codex release confidence
4. `REL-003` golden fixture depth and remaining corpus alignment
5. `REL-005` dogfood friction evidence
