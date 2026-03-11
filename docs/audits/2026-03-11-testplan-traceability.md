# 2026-03-11 Test Plan Traceability

## Summary

This document maps `specs/testplan.md` to current repo evidence. It is intentionally strict: existence of tests is not the same thing as proof that the written test plan is satisfied.

Status meanings:

- `covered` - direct and credible evidence exists
- `partial` - some tests exist, but not at the depth the test plan asks for
- `missing` - no direct evidence found

## Section Mapping

| Test Plan Section | Current Evidence | Status | Notes |
|---|---|---|---|
| Unit normalization (`UNIT-NORM-*`) | `internal/core/normalize_test.go`, `internal/core/compound_test.go`, `internal/core/classify_test.go`, `internal/core/sanitize_test.go` | `partial` | Core normalization path is well represented, but the full test-plan checklist has not been exhaustively matched one-by-one yet. |
| Unit rule engine (`UNIT-RULE-*`) | `internal/policy/policy_test.go`, `internal/core/classify_test.go`, `internal/core/safecmds_test.go`, `internal/core/mcpclassify_test.go` | `partial` | Rule precedence and safe-command logic exist. Full near-miss coverage per rule family is not yet proven. |
| Unit file inspection (`UNIT-FILE-*`) | `internal/core/inspect_test.go`, `internal/inspect/python_test.go`, `internal/inspect/shell_test.go`, `internal/inspect/javascript_test.go` | `partial` | Good direct test surface. Need explicit matrixing against every test-plan case and edge bucket. |
| Unit approval/storage (`UNIT-APP-*`) | `internal/approve/manager_test.go`, `internal/approve/hmac_test.go`, `internal/db/db_test.go`, `internal/adapters/runner_test.go` | `partial` | Single-use approval and reverify-on-script-change are covered. Interactive prompt-path and full lifecycle proof still need explicit release validation. |
| Integration hook path (`INT-HOOK-*`) | `internal/adapters/hook_test.go`, `integration_test.go` | `partial` | Hook behavior exists and is exercised, but root integration reproducibility is currently compromised by ignored local files. |
| Integration MCP path (`INT-MCP-*`) | `internal/adapters/mcpproxy_test.go`, `internal/adapters/hook_test.go` | `partial` | Useful seam tests exist. Full downstream end-to-end confidence remains below the written test-plan ambition. |
| Integration run mode (`INT-RUN-*`) | `internal/adapters/runner_test.go`, `internal/cli/run_test.go`, `integration_test.go` | `partial` | Safe execution and decision-key re-verification are covered. Clean-checkout CLI binary build is not currently reliable. |
| Golden shell fixtures (`GOLD-CMD-*`) | `testdata/fixtures/commands.yaml`, `internal/core/classify_test.go` | `gap` | Volume is too low relative to the written coverage requirement. |
| Golden script fixtures (`GOLD-SCRIPT-*`) | `testdata/scripts/*`, scanner tests | `partial` | Script fixture families exist, but breadth is limited. |
| Golden MCP fixtures (`GOLD-MCP-*`) | no dedicated MCP golden corpus found | `missing` | MCP behavior is tested via unit/integration code, not a dedicated golden fixture set. |
| Adversarial/red-team plan | various normalization, classification, and MCP tests | `partial` | Some adversarial intent is present, but not clearly organized to match the red-team plan in the spec. |
| Performance plan (`PERF-*`) | no benchmark or performance harness found | `missing` | Release readiness cannot currently claim tested performance gates. |
| Compatibility plan (`COMPAT-*`) | no dedicated compatibility harness found | `missing` | Platform support is claimed, but no explicit compatibility matrix was found in current tests. |
| Regression plan (`REG-*`) | broad package tests and root integration tests | `partial` | Regression coverage exists, but not in the structured way the test plan describes. |

## Concrete Evidence

### Strongest Existing Areas

- `internal/core/normalize_test.go`
- `internal/core/classify_test.go`
- `internal/core/inspect_test.go`
- `internal/adapters/hook_test.go`
- `internal/adapters/mcpproxy_test.go`
- `internal/db/db_test.go`

### Thin or Missing Areas

- Codex-specific release confidence
- dedicated MCP golden fixtures
- benchmark/performance gates
- compatibility matrix
- full fixture depth for hardcoded + built-in rule corpus

## Quantitative Gaps

| Item | Current | Target Signal | Assessment |
|---|---|---|---|
| Golden command fixtures | `162` | at least `2 x (225 built-ins + 22 hardcoded)` = `494` | `gap` |
| Codex tests | `2` direct `TestExecuteCodexShellCommand_*` tests plus one stdin isolation test | needs explicit safe/approval/blocked confidence | `gap` |
| Performance harness | `0` found | required by `specs/testplan.md` §6 | `missing` |
| Compatibility harness | `0` found | required by `specs/testplan.md` §7 | `missing` |

## First-Pass Conclusion

The current test surface is real and useful, but it does not yet satisfy the full written test plan at release confidence.

Most likely must-fix test-readiness items:

1. Clean-checkout reproducibility for root integration tests
2. Golden fixture expansion or explicit narrowing of the release claim
3. Stronger Codex-path tests
4. Explicit performance and compatibility gates

