# 2026-03-11 Spec Conformance Matrix

## Scope

This is a grouped first-pass release-readiness audit against:

- `specs/functional.md`
- `specs/technical_v1.1.md`
- `specs/testplan.md`

Status meanings:

- `implemented` - current code and tests provide direct evidence
- `partial` - implementation exists, but proof is incomplete or confidence is thin
- `gap` - missing, contradicted, or not reproducibly proven
- `accepted-limit` - limitation is explicitly documented and matches implementation

## Baseline

| Metric | Result |
|---|---|
| `go test ./...` in original checkout | pass |
| `go test ./...` in clean worktree | pass on `release-readiness-audit` branch |
| Golden command fixtures | `182` |
| Built-in rule IDs | `225` |
| Hardcoded blocked rules | `22` |
| Go test files | `24` |
| Top-level Go tests | `177` |

## Matrix

| ID | Requirement Area | Source | Status | Evidence | Notes |
|---|---|---|---|---|---|
| `FUNC-9.1` / `TECH-5` | Shell mediation pipeline exists and classifies before execution | `functional.md` §9.1, `technical_v1.1.md` §5 | `partial` | `internal/core/normalize.go`, `internal/core/classify.go`, `internal/core/compound.go`, `internal/adapters/hook.go`, `internal/adapters/runner.go`, `internal/core/classify_test.go`, `internal/core/normalize_test.go`, `internal/core/compound_test.go`, `internal/adapters/hook_test.go` | Core path exists, clean-worktree reproducibility is restored on this branch, and self-protection regression coverage was strengthened. Full per-spec proof is still incomplete. |
| `FUNC-9.2` / `FUNC-12` / `TECH-7` | Referenced file inspection for supported languages | `functional.md` §9.2, §12, `technical_v1.1.md` §7 | `partial` | `internal/core/inspect.go`, `internal/inspect/python.go`, `internal/inspect/shell.go`, `internal/inspect/javascript.go`, `internal/core/inspect_test.go`, `internal/inspect/python_test.go`, `internal/inspect/shell_test.go`, `internal/inspect/javascript_test.go` | Supported scanner families exist. Test surface covers safe/dangerous/truncated/symlinked cases, but full test-plan traceability is incomplete. |
| `FUNC-9.3` / `FUNC-13` / `TECH-11` | MCP proxy mediates tool calls and denies risky actions | `functional.md` §9.3, §13, `technical_v1.1.md` §11 | `partial` | `internal/adapters/mcpproxy.go`, `internal/adapters/mcpproxy_test.go`, `internal/adapters/hook_test.go` | Sensitive resource read blocking and malformed frame handling are covered. Real downstream end-to-end confidence is still thinner than release-ready. |
| `FUNC-9.4` / `FUNC-11` / `TECH-8` / `TECH-10` | Approval flow, binding, invalidation, and execution semantics | `functional.md` §9.4, §11, `technical_v1.1.md` §8, §10 | `partial` | `internal/approve/manager.go`, `internal/approve/hmac.go`, `internal/approve/prompt.go`, `internal/db/approvals.go`, `internal/adapters/runner.go`, `internal/approve/manager_test.go`, `internal/approve/hmac_test.go`, `internal/adapters/runner_test.go` | Single-use binding and re-verification exist. Real prompt-path behavior is still mostly tested through non-interactive denial rather than full interactive flows. |
| `FUNC-9.5` / `FUNC-15` / `TECH-9` / `TECH-14` | Local persistence, HMAC, and privacy behavior | `functional.md` §9.5, §15, `technical_v1.1.md` §9, §14 | `partial` | `internal/db/db.go`, `internal/db/schema.go`, `internal/db/events.go`, `internal/db/secret.go`, `internal/db/db_test.go` | Persistence stack exists and is exercised. Need explicit release decision on retention/privacy claims after audit finishes. |
| `FUNC-14.1` / `TECH-3.1` / `TECH-4.1` | Claude Code hook integration is primary, fail-closed on supported paths | `functional.md` §14.1, `technical_v1.1.md` §3.1, §4.1 | `partial` | `internal/adapters/hook.go`, `internal/cli/install.go`, `internal/cli/doctor.go`, `internal/adapters/hook_test.go`, `internal/cli/install_test.go`, `internal/cli/doctor_test.go` | Claude path has the strongest evidence today, but no completed conformance matrix yet proves every hook requirement. |
| `FUNC-14.2` / `TECH-3.2` | Codex integration is available and release-worthy | `functional.md` §14.2, `technical_v1.1.md` §3.2 | `partial` | `internal/adapters/codexshell.go`, `internal/adapters/codexshell_test.go`, `README.md` | Implementation exists and enabled-mode `SAFE`, `BLOCKED`, approval-required handling, and JSON-RPC shell-server behavior are now tested, but the path still lacks enough end-to-end and dogfood evidence for GA-level confidence. |
| `FUNC-17` / `TECH-1.2` / `testplan.md` §6 | Performance claims and low-friction hot path are proven | `functional.md` §17, `technical_v1.1.md` §1.2, `specs/testplan.md` §6 | `gap` | `specs/testplan.md`, `README.md`, current repo test inventory | No benchmark or release-gate performance harness was found in the current test surface. |
| `FUNC-19` / `TECH-12.3` / `testplan.md` §4 | Rule corpus has sufficient golden fixtures | `functional.md` §19, `technical_v1.1.md` §12.3, `specs/testplan.md` §4 | `gap` | `testdata/fixtures/commands.yaml`, `internal/core/classify_test.go`, `internal/core/fixture_coverage_test.go`, `internal/policy/hardcoded.go`, `internal/policy/builtins_core.go`, `internal/policy/builtins_security.go` | `182` fixtures and new guard tests materially improve confidence, but that is still below the implied `494` minimum if every built-in and hardcoded rule needs a positive and near-miss row. |
| `REL-BASELINE-001` | Clean-checkout/build reproducibility | release-readiness baseline | `implemented` | `.gitignore`, `cmd/fuse/main.go`, `integration_test.go`, clean-worktree `go test ./...` pass on this branch | The clean-checkout release blocker is fixed on `release-readiness-audit`. |
| `FUNC-4.3` / `FUNC-4.4` / `TECH-16.1` / `TECH-16.2` | TOCTOU and obfuscation limitations are honestly documented | `functional.md` §4.3-§4.4, `technical_v1.1.md` §16.1-§16.2 | `accepted-limit` | `README.md`, `specs/functional.md`, `specs/technical_v1.1.md` | Limitation language is present and consistent with the current product shape. |
| `FUNC-7` / `FUNC-21` | Enforced scope matches marketed scope | `functional.md` §7, §21 | `partial` | `README.md`, `internal/adapters/hook.go`, `internal/adapters/mcpproxy.go`, `internal/adapters/codexshell.go` | The product story is coherent, but release wording still needs to decide whether Codex is GA, beta, or deferred. |

## First-Pass Conclusion

The codebase is not in a "spec-proven release-ready" state yet.

Current confidence by slice:

- Claude shell/hook path: strongest evidence
- file inspection and approval semantics: promising, still partial
- MCP proxy: useful, still partial
- Codex: materially stronger, still under-proven
- test-plan conformance: currently below release confidence

This matrix is grouped by requirement area for first-pass triage. It is not yet a full per-normative-statement extraction.
