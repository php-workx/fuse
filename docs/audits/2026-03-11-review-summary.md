# 2026-03-11 Review Summary

## Verdict

`fuse` is not yet release-ready by evidence standard.

Current honest position:

- good dogfood candidate
- strongest on Claude shell/hook path
- promising on file inspection and MCP mediation
- under-proven on Codex
- under-proven on full test-plan conformance
- partially measured on performance/compatibility, with two confirmed misses

## First-Pass Grouped Counts

| Status | Count |
|---|---|
| `implemented` | `1` |
| `partial` | `9` |
| `gap` | `1` |
| `accepted-limit` | `1` |

These counts are grouped first-pass row counts from the current audit matrix. They are useful for triage, not a claim that the audit is fully exhaustive yet.

## Release-Blocker Candidates

### 1. Clean-checkout reproducibility was the first blocker and is now fixed on this branch

The biggest initial finding was not a classifier edge case. It was build reproducibility:

- root integration tests expect `./cmd/fuse`
- a clean worktree created from `HEAD` originally did not contain `cmd/fuse`
- the original checkout passed because ignored local files existed on disk
- commit `408bc5f` fixes this by tracking `cmd/fuse/main.go` and narrowing `.gitignore`

This blocker is resolved on `release-readiness-audit`; it still needs to be carried through normal integration.

### 2. Self-protection classification is materially stronger on this branch

Fixture expansion exposed a real correctness bug in classifier precedence:

- inline interpreter payloads such as `python -c ... ~/.fuse/config ...` could fall through to `APPROVAL`
- unclosed heredoc writes into `~/.fuse/config` could also fail closed to `APPROVAL`
- inline pipe-script payloads such as `curl ... | bash` could degrade to `SAFE` after compound splitting

That behavior is now fixed on this branch by evaluating hardcoded self-protection rules before sanitized inline-script approval, preserving those rules on compound parse-error paths, and preserving inline pipe-script approval across compound splitting. This closes a meaningful gap in the repo's ability to prevent unintended tampering with Fuse-managed state.

### 3. Codex should not be called GA yet, but the test baseline improved again on this branch

Codex support exists in code and docs. This branch now includes explicit enabled-mode `SAFE`, `BLOCKED`, and approval-without-TTY executor tests plus MCP/JSON-RPC shell-server tests for `initialize`, `tools/list`, and `tools/call`. That is much stronger evidence than before, but it still falls short of a full GA claim without real dogfood evidence.

### 4. The built-in rule surface is better pinned down, but the exhaustive contract is still open

This branch now has two stronger regression layers for rule behavior:

- high-risk golden coverage guards for hardcoded blocked rules and major command families
- a passing sentinel matrix across built-in sections `§6.3.1` through `§6.3.21`

That materially improves confidence and exposed/fixed real bugs, but it is still not the same as the written exhaustive per-rule positive + near-miss contract for all `225` built-in IDs.

### 5. We now have a real perf/compat harness, and it exposed two concrete release blockers

This branch now has a repeatable release-check runner in `scripts/run-release-checks.sh` and env-gated perf/compat tests in `internal/releasecheck/releasecheck_test.go`.

Current measured results on this `darwin/arm64` machine are strong for the hot paths:

- shell warm path and cold path are well under the stated thresholds
- MCP hot-path classification is far under the stated threshold
- shell wrapper compatibility passes locally for `bash`, `zsh`, and `fish`
- locale invariance passes locally
- cross-builds pass for the four declared `GOOS/GOARCH` targets

But the same harness also found two hard mismatches:

- the repo’s declared minimum Go version had drifted and needed to be aligned to the real `1.24` dependency floor
- pathological long unmatched inputs currently miss the `PERF-003` `<100 ms` target on this machine

The Go-floor mismatch is now fixed on this branch:

- `go.mod` now requires Go `1.24.0`
- the current spec and test plan now name Go `1.24`
- `GOTOOLCHAIN=go1.24.0 go test -count=1 ./...` passes

Recommended current posture:

- `Claude: primary`
- `Codex: beta or deferred until strengthened`

## Release Recommendation Today

If a release had to happen today, the honest statement would be:

> release-ready for internal dogfooding only; not yet validated for a stable public release across Claude, Codex, and cloud workflows

## Next Actions

1. Resolve the remaining `PERF-003` long-input miss now that it is a measured release blocker.
2. Expand or explicitly narrow the remaining per-rule golden-fixture contract; `182` rows is still below the written full-rule target for `225` built-in IDs.
3. Add stronger Codex end-to-end and dogfood evidence, then add the remaining prompt/sqlite/memory proof before RC1.

---

## Posture Update: 2026-03-24

**Updated posture: public beta.**

Since the original audit (2026-03-11), the following changes have been made:

- LLM judge feature (shadow + active modes) with full test coverage
- 4 security hardening rounds (path traversal, symlink resolution, credential scrubbing, context propagation)
- Integration tests isolated from production state
- Pre-existing lint violations resolved
- Test suite runs clean with `-race` across all 13 packages
- Module path corrected to `github.com/php-workx/fuse`
- goreleaser release pipeline added

**Current posture:**

| | Status |
|---|--------|
| Platforms | macOS, Linux |
| Claude Code | primary integration |
| Codex CLI | beta |
| Windows | planned, not supported in v1 |

The project is past internal-only use. Fuse is a guardrail, not a sandbox.
See [docs/TRUST_MODEL.md](../TRUST_MODEL.md) for security boundaries.
