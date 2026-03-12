# 2026-03-11 Review Summary

## Verdict

`fuse` is not yet release-ready by evidence standard.

Current honest position:

- good dogfood candidate
- strongest on Claude shell/hook path
- promising on file inspection and MCP mediation
- under-proven on Codex
- under-proven on full test-plan conformance and low-friction claims

## First-Pass Grouped Counts

| Status | Count |
|---|---|
| `implemented` | `0` |
| `partial` | `7` |
| `gap` | `4` |
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

That behavior is now fixed on this branch by evaluating hardcoded self-protection rules before sanitized inline-script approval and also on compound parse-error paths. This closes a meaningful gap in the repo's ability to prevent unintended tampering with Fuse-managed state.

### 3. Codex should not be called GA yet, but the test baseline improved again on this branch

Codex support exists in code and docs. This branch now includes explicit enabled-mode SAFE, BLOCKED, and approval-without-TTY executor tests plus MCP/JSON-RPC shell-server tests for `initialize`, `tools/list`, and `tools/call`. That is much stronger evidence than before, but it still falls short of a full GA claim without real dogfood evidence.

Recommended current posture:

- `Claude: primary`
- `Codex: beta or deferred until strengthened`

## Release Recommendation Today

If a release had to happen today, the honest statement would be:

> release-ready for internal dogfooding only; not yet validated for a stable public release across Claude, Codex, and cloud workflows

## Next Actions

1. Expand or explicitly narrow golden fixture claims.
2. Strengthen Codex tests and decide Codex release posture from evidence.
3. Add dogfood and performance/compatibility evidence before RC1.
