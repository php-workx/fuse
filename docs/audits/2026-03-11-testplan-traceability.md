# 2026-03-11 Test Plan Traceability

## Scope

This is an ID-level first-pass mapping for `specs/testplan.md`.

Status meanings:

- `covered` - direct evidence exists and is reasonably aligned
- `partial` - related tests exist, but not yet at the exact depth or shape the test plan asks for
- `missing` - no direct evidence found

## ID-Level Mapping

| ID | Current Evidence | Status | Notes |
|---|---|---|---|
| `UNIT-NORM-001` | `internal/core/normalize_test.go`, `internal/core/classify_test.go` | `partial` | Normalization/input validation is covered, but not yet proven one-to-one against this exact case. |
| `UNIT-NORM-002` | `internal/core/normalize_test.go` | `partial` | Unicode normalization coverage exists. |
| `UNIT-NORM-003` | `internal/core/normalize_test.go`, `internal/core/classify_test.go` | `partial` | Bidi/control stripping intent is partially represented. |
| `UNIT-NORM-004` | `internal/core/normalize_test.go` | `partial` | ANSI stripping coverage exists. |
| `UNIT-NORM-005` | `internal/core/compound_test.go`, `internal/core/classify_test.go` | `partial` | Compound splitting and restrictive-wins behavior exist. |
| `UNIT-NORM-006` | `internal/core/compound_test.go`, `internal/core/classify_test.go` | `partial` | Heredoc/suspicious-inline behavior needs exact case matching. |
| `UNIT-NORM-007` | `internal/core/normalize_test.go` | `partial` | Basename extraction is covered. |
| `UNIT-NORM-008` | `internal/core/normalize_test.go` | `partial` | Wrapper stripping is covered. |
| `UNIT-NORM-009` | `internal/core/normalize_test.go` | `partial` | `bash -c` and `ssh` extraction are covered. |
| `UNIT-NORM-010` | `internal/core/normalize_test.go` | `partial` | Extraction-failure fail-closed path exists. |
| `UNIT-NORM-011` | `internal/core/classify_test.go`, `internal/core/mcpclassify_test.go` | `partial` | Suspicious interpreter heuristics exist, but exact per-language cases remain to be matched. |
| `UNIT-RULE-001` | `internal/policy/policy_test.go` | `partial` | Rule precedence is covered. |
| `UNIT-RULE-002` | `internal/core/sanitize_test.go`, `internal/core/classify_test.go` | `partial` | Sanitization and quoted-content behavior are partially covered. |
| `UNIT-RULE-003` | `internal/core/classify_test.go`, `internal/core/normalize_test.go` | `partial` | Escalation behavior exists. |
| `UNIT-RULE-004` | `internal/policy/policy_test.go`, `internal/policy/hardcoded.go` | `partial` | Hardcoded immutability is represented indirectly. |
| `UNIT-RULE-005` | `internal/core/safecmds_test.go` | `partial` | Safe-command behavior exists. |
| `UNIT-RULE-006` | `internal/core/mcpclassify_test.go` | `partial` | MCP rule classification exists. |
| `UNIT-FILE-001` | `internal/core/inspect_test.go`, `internal/inspect/python_test.go` | `partial` | Python inspection covered. |
| `UNIT-FILE-002` | `internal/core/inspect_test.go`, `internal/inspect/shell_test.go` | `partial` | Shell inspection covered. |
| `UNIT-FILE-003` | `internal/core/inspect_test.go`, `internal/inspect/javascript_test.go` | `partial` | JS/TS inspection covered. |
| `UNIT-FILE-004` | `internal/core/inspect_test.go` | `partial` | Symlink/truncated/unsupported cases exist. |
| `UNIT-FILE-005` | `internal/core/inspect_test.go` | `partial` | Unknown extension and decision inference are covered. |
| `UNIT-APP-001` | `internal/approve/manager_test.go`, `internal/db/db_test.go` | `partial` | Approval creation/consumption exists. |
| `UNIT-APP-002` | `internal/approve/hmac_test.go` | `covered` | HMAC verification is directly covered. |
| `UNIT-APP-003` | `internal/db/db_test.go` | `partial` | Expiry behavior is covered. |
| `UNIT-APP-004` | `internal/db/db_test.go`, `internal/approve/manager_test.go` | `partial` | Scope and reuse behavior are covered. |
| `UNIT-APP-005` | `internal/adapters/runner_test.go` | `covered` | Changed-script reverify is directly covered. |
| `UNIT-APP-006` | `internal/db/db_test.go` | `partial` | Cleanup/pruning logic is covered. |
| `UNIT-APP-007` | `internal/db/db_test.go` | `partial` | Retention behavior exists, but exact mapping still needs confirmation. |
| `UNIT-APP-008` | `internal/approve/manager_test.go`, `internal/approve/hmac_test.go` | `partial` | Integrity and manager behavior exist. |
| `UNIT-SAFE-001` | `internal/core/safecmds_test.go` | `covered` | Unconditional safe command coverage exists. |
| `UNIT-SAFE-002` | `internal/core/safecmds_test.go` | `covered` | Conditional safe command coverage exists. |
| `UNIT-SCRUB-001` | `internal/db/db_test.go` | `covered` | Credential scrubbing is directly tested. |
| `UNIT-SCRUB-002` | `internal/db/db_test.go`, `internal/core/sanitize_test.go` | `partial` | Scrubbing/sanitization evidence exists. |
| `UNIT-RULE-007` | `internal/core/mcpclassify_test.go`, `internal/policy/policy_test.go` | `partial` | Late rule-edge coverage exists, but exact case mapping needs completion. |
| `INT-HOOK-001` | `internal/adapters/hook_test.go`, `integration_test.go` | `partial` | Safe path exists. |
| `INT-HOOK-002` | `internal/adapters/hook_test.go`, `integration_test.go` | `partial` | Block path exists. |
| `INT-HOOK-003` | `internal/adapters/hook_test.go`, `integration_test.go` | `partial` | Approval-required non-interactive denial exists. |
| `INT-HOOK-004` | `internal/adapters/hook_test.go`, `integration_test.go` | `partial` | Schema validation/fail-closed evidence exists. |
| `INT-INSTALL-001` | `internal/cli/install_test.go`, `internal/cli/doctor_test.go` | `partial` | Install and doctor coverage exist. |
| `INT-MCP-001` | `internal/adapters/mcpproxy_test.go`, `integration_test.go` | `partial` | Proxy routing exists. |
| `INT-MCP-002` | `internal/adapters/mcpproxy_test.go` | `partial` | Error-path routing exists. |
| `INT-MCP-003` | `internal/adapters/mcpproxy_test.go` | `partial` | Sensitive resource blocking exists. |
| `INT-MCP-004` | `internal/adapters/mcpproxy_test.go`, `internal/adapters/hook_test.go` | `partial` | Destructive-path denial exists. |
| `INT-MCP-005` | `internal/adapters/mcpproxy_test.go` | `partial` | Downstream lifecycle coverage exists. |
| `INT-E2E-001` | `integration_test.go` | `partial` | E2E tests exist, but clean-checkout reproducibility currently breaks confidence. |
| `INT-RUN-001` | `internal/adapters/runner_test.go`, `integration_test.go` | `partial` | Run-mode safe path exists. |
| `INT-RUN-002` | `internal/adapters/runner_test.go`, `integration_test.go` | `partial` | Reverify/approval behavior exists. |
| `INT-POLICY-001` | `internal/policy/policy_test.go` | `covered` | Policy load/evaluation behavior is directly tested. |
| `INT-CLI-001` | `internal/cli/run_test.go`, `internal/cli/doctor_test.go` | `partial` | CLI entrypoint behavior exists. |
| `INT-CLI-002` | `internal/cli/install_test.go`, `internal/cli/doctor_test.go` | `partial` | Install/doctor coverage exists. |
| `GOLD-CMD-001` | `testdata/fixtures/commands.yaml`, `internal/core/classify_test.go` | `partial` | Golden command suite exists. |
| `GOLD-CMD-002` | `testdata/fixtures/commands.yaml`, `internal/policy/*` | `gap` | Fixture volume is too low for full per-rule confidence. |
| `GOLD-SCRIPT-001` | `testdata/scripts/*`, `internal/inspect/*_test.go`, `internal/core/inspect_test.go` | `partial` | Script fixtures exist, but breadth is limited. |
| `GOLD-MCP-001` | no dedicated MCP golden corpus found | `missing` | MCP is tested via code, not dedicated golden fixtures. |
| `ADV-NORM-001` | `internal/core/normalize_test.go`, `internal/core/classify_test.go` | `partial` | Some adversarial normalization exists. |
| `ADV-NORM-002` | `internal/core/compound_test.go`, `internal/core/normalize_test.go` | `partial` | Some parser-edge adversarial coverage exists. |
| `ADV-RULE-002` | `internal/policy/policy_test.go`, `internal/core/classify_test.go` | `partial` | Rule hardening exists. |
| `ADV-RULE-003` | `internal/core/classify_test.go`, `internal/core/mcpclassify_test.go` | `partial` | Edge classification behavior exists. |
| `ADV-RULE-004` | `internal/core/safecmds_test.go` | `partial` | Safe-path abuse checks exist indirectly. |
| `ADV-SELF-001` | `internal/policy/hardcoded.go`, `internal/policy/policy_test.go` | `partial` | Self-protection logic exists, but exact dedicated test mapping remains incomplete. |
| `ADV-APP-001` | `internal/approve/hmac_test.go`, `internal/db/db_test.go` | `partial` | Approval tamper/reuse coverage exists. |
| `ADV-APP-002` | `internal/adapters/runner_test.go`, `internal/db/db_test.go` | `partial` | Changed-script and lifecycle coverage exist. |
| `ADV-APP-003` | `internal/adapters/hook_test.go` | `partial` | Non-interactive denial exists. |
| `ADV-MCP-001` | `internal/adapters/mcpproxy_test.go` | `partial` | MCP malformed/oversize coverage exists. |
| `ADV-MCP-002` | `internal/adapters/mcpproxy_test.go` | `partial` | Sensitive resource handling exists. |
| `ADV-FILE-001` | `internal/core/inspect_test.go`, `internal/inspect/*_test.go` | `partial` | File-edge coverage exists. |
| `ADV-FILE-002` | `internal/core/inspect_test.go` | `partial` | Unsupported/truncated/symlink behavior exists. |
| `PERF-001` | no harness found | `missing` | No explicit performance test evidence found. |
| `PERF-002` | no harness found | `missing` | No explicit performance test evidence found. |
| `PERF-002A` | no harness found | `missing` | No explicit performance test evidence found. |
| `PERF-002B` | no harness found | `missing` | No explicit performance test evidence found. |
| `PERF-003` | no harness found | `missing` | No explicit performance test evidence found. |
| `PERF-004` | no harness found | `missing` | No explicit performance test evidence found. |
| `PERF-005` | no harness found | `missing` | No explicit performance test evidence found. |
| `COMPAT-001` | no compatibility harness found | `missing` | No explicit compatibility matrix found. |
| `COMPAT-002` | no compatibility harness found | `missing` | No explicit compatibility matrix found. |
| `COMPAT-003` | no compatibility harness found | `missing` | No explicit compatibility matrix found. |
| `COMPAT-004` | no compatibility harness found | `missing` | No explicit compatibility matrix found. |
| `COMPAT-005` | no compatibility harness found | `missing` | No explicit compatibility matrix found. |
| `COMPAT-006` | no compatibility harness found | `missing` | No explicit compatibility matrix found. |
| `COMPAT-007` | no compatibility harness found | `missing` | No explicit compatibility matrix found. |
| `REG-001` | `go test ./...`, package tests, `integration_test.go` | `partial` | Regression surface exists. |
| `REG-002` | `go test ./...`, golden fixtures | `partial` | Regression intent exists. |
| `REG-003` | package tests, fixture tests | `partial` | Regression evidence exists. |
| `REG-004` | package tests, root integration tests | `partial` | Regression coverage exists, but release-level structure is incomplete. |

## High-Signal Gaps

1. `GOLD-CMD-002` - fixture depth is below the written target
2. `GOLD-MCP-001` - no dedicated MCP golden fixture corpus found
3. `PERF-*` - no explicit performance proof found
4. `COMPAT-*` - no explicit compatibility proof found
5. `INT-E2E-001` - clean-checkout reproducibility issue reduces confidence in end-to-end claims

