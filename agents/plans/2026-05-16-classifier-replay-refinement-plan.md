---
id: plan-2026-05-16-classifier-replay-refinement
type: plan
date: 2026-05-16
source: "Fuse local event DB noise analysis and classifier reliability discussion"
ticket: "epo-build-classifier-replay-audit-fo-tycm"
---

# Plan: Classifier Replay and Reliability Refinement

## Context

Fuse is intended to be the last-resort safety boundary. It should interrupt for catastrophic or meaningfully risky commands, not for routine development workflows. Recent classifier work reduced known false-positive clusters around developer commands, `epos`, trusted-workspace `cd && test/lint`, canonical localhost URLs, and exact configured `just` recipes.

That work was targeted, but it does not prove the larger goal: reducing thousands of approval prompts to a small number of genuinely critical prompts while preserving malicious-command detection. The next phase needs measurement before more rule tuning.

Baseline from the local Fuse event DB on 2026-05-16:

| Decision | Events |
|----------|--------|
| SAFE | 86,807 |
| CAUTION | 10,454 |
| APPROVAL | 2,510 |
| BLOCKED | 229 |

Combined `APPROVAL + CAUTION`: 12,964 events, 11,360 distinct commands.

## Goal

Make Fuse substantially less noisy for developers while becoming more trustworthy for catastrophic-command detection.

The target is not a specific exception count. The target is evidence-backed behavior:

- Routine read-only developer workflows should be `SAFE` or log-only `CAUTION`.
- Mutating but local/day-to-day workflows should usually be `CAUTION`, not approval prompts.
- Catastrophic, destructive, evasive, privilege-escalating, exfiltrating, remote-execution, deploy/release/destroy, and opaque script execution should remain `APPROVAL` or `BLOCKED`.
- Changes must be measured against historical commands and adversarial regressions.

## Non-Goals

- Do not reduce noise by adding broad string exceptions.
- Do not silently downgrade commands that are opaque, evasive, or high-impact.
- Do not optimize for raw event count alone; estimate actual user interruption using deduped prompt equivalents.
- Do not treat local historical commands as automatically safe.

## Strategy

### 1. Build Replay Measurement

Add a replay/audit path that reads historical `events.command` rows and reclassifies them with the current classifier without executing anything.

Required outputs:

- Old-to-new decision matrix.
- Counts by old decision, new decision, reason, rule ID, source, agent, workspace, and command fingerprint.
- Remaining `APPROVAL` and high-volume `CAUTION` clusters.
- Estimated deduped prompt equivalents using a stable classifier-derived key.
- JSON output for reproducible analysis.
- Human summary for quick local iteration.

The replay tool must support a local DB path and a row limit so it can run safely against `~/.fuse/state/fuse.db` and small synthetic test DBs.

### 2. Keep a Regression Safety Set

Replay analysis must be paired with an adversarial corpus that should not downgrade:

- Destructive filesystem commands: `rm -rf /`, recursive deletes outside build/cache dirs.
- Credential reads and exfiltration: private keys, tokens, cloud credentials, `curl | sh`.
- Privilege escalation: `sudo`, persistence, shell profile modification.
- Remote execution and opaque scripts: encoded payloads, heredocs that execute shell, downloaded scripts.
- Evasive forms: percent-encoded URLs, ANSI-C quoting, non-canonical loopback, path traversal.
- Deploy/release/destroy workflows unless explicitly configured.

Any broad refactor must show both replay improvement and adversarial pass.

### 3. Refine by Cluster, Not by Anecdote

Use replay output to pick the highest-volume noisy clusters. For each cluster:

1. Identify the true intent and risk boundary.
2. Decide whether the fix is a small rule adjustment, a parser improvement, tool-specific semantic classifier, or a deeper architecture change.
3. Add tests that pin both benign and dangerous variants.
4. Re-run replay and adversarial tests.
5. Record the measured effect.

### 4. Prefer Structural Understanding

Where string rules are too blunt, refactor toward command semantics:

- Shell command decomposition before policy matching.
- Tool-specific classifiers for common developer tools (`git`, `gh`, `npm`, `pnpm`, `yarn`, `just`, `docker`, `kubectl`, `terraform`, cloud CLIs).
- Explicit operation classes: read-only, local mutation, remote mutation, deploy/release, destructive, exfiltration, privilege escalation, opaque execution.
- Target sensitivity: local workspace, localhost, trusted build/cache dirs, system paths, credentials, remote hosts, production-like contexts.

## First Execution Slice

Ticket: `epo-build-classifier-replay-audit-fo-tycm`

Implement a replay audit command or internal tool that:

- Opens an events DB.
- Reads historical commands with prior decisions and metadata.
- Reclassifies each command through the current classifier.
- Emits a decision transition matrix.
- Groups remaining `APPROVAL` and `CAUTION` clusters.
- Has synthetic tests and does not require the developer's real DB.

## First Replay Result

After implementing the first replay command, the local 100,000-event DB replay produced:

| Metric | Value |
|--------|-------|
| Historical `APPROVAL` events | 2,436 |
| Current classifier `APPROVAL` events | 1,107 |
| Deduped current approval prompt keys | 1,026 |
| Historical `CAUTION` events | 10,455 |
| Current classifier `CAUTION` events | 12,388 |

Top remaining approval causes were not simple read-only developer commands. They clustered around:

- Compound split failures on malformed or redacted deployment commands.
- Security-sensitive env assignments such as `PATH=...`, `HOME=...`, and cloud/deploy environment setup.
- File inspection drift where historical script paths no longer exist at replay time.
- PowerShell inner-command extraction failures.

This result is directionally good but not close to the desired "last resort only" behavior. The next pass should focus on structural fixes:

- Replay-aware handling for stale file references so historical DB analysis does not overstate live approval prompts.
- Better parsing/diagnostics for malformed commands created by redaction or shell quoting.
- More nuanced env-assignment classification that distinguishes routine scoped developer environment setup from binary injection or config redirection.
- Tool-specific semantics for common `build`, `typecheck`, and validation commands that currently remain `CAUTION`.

## Success Criteria

The first slice is done when:

- A developer can run a replay command against `~/.fuse/state/fuse.db`.
- The output answers: "How many historical approvals would still prompt today?"
- Remaining noisy clusters are visible and rankable.
- The command has tests using temporary event data.
- `go test ./...` passes.

The broader refinement phase is done when:

- Replay shows a dramatic reduction in deduped prompt equivalents.
- The remaining prompts are explainable and high-signal.
- The adversarial corpus remains strict.
- Each major downgrade has a documented reason and regression tests.

## Reporting Template

Each refinement pass should produce:

```text
Replay window: <all events | last N days | limit>
Historical APPROVAL events: <n>
Current APPROVAL events: <n>
Deduped prompt equivalents: <n>
Top remaining approval clusters:
  1. <count> <reason/rule> <example>
  2. ...
Top downgraded clusters:
  1. <old> -> <new> <count> <reason>
Adversarial corpus: PASS|FAIL
Tests: <commands>
```

## Open Questions

- Should replay live under `fuse test`, `fuse events`, or a new `fuse audit` command?
- Should deduped prompt equivalents reuse the exact approval decision key, or use a replay-specific fingerprint that avoids policy side effects?
- How much metadata should be included in JSON output before it risks leaking sensitive historical commands?
