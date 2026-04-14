# Security Hardening Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Close the biggest practical security gaps between `fuse`'s current mediated-command guardrail and a safer day-to-day Claude/Codex integration by adding secure host configuration, MCP trust controls, native-tool path/secret protections, and stronger diagnostics.

**Architecture:** Keep `fuse` focused on local guardrails rather than turning it into a sandbox. Strengthen the product in three layers: installation-time host configuration, runtime mediation/trust checks, and diagnostic/audit visibility. Reuse the existing `install`, `doctor`, policy, and adapter seams rather than introducing a daemon or external service.

**Tech Stack:** Go 1.24+, Cobra CLI, existing Claude `settings.json` merge logic, Codex `config.toml` merge logic, MCP proxy/runtime adapters, YAML config, golden tests, `go test`, `tk`

---

## Scope And Non-Goals

This plan intentionally covers:

1. Secure-by-default Claude/Codex install flows.
2. Detection of unsafe host configuration and unmediated MCP paths.
3. Better protection for sensitive local files and common secret locations.
4. Optional companion checks for Claude native file tools where shell mediation is insufficient.
5. Honest docs and diagnostics so users know what is protected.

This plan intentionally does **not** cover:

1. Building a full OS sandbox, seccomp layer, or network firewall.
2. Re-implementing Claude/Codex permission systems inside `fuse`.
3. Full DLP or comprehensive prompt/output redaction.
4. Defending against a fully hostile agent with arbitrary local-code execution outside mediated paths.
5. Team workflows, centralized policy servers, or cloud IAM integration.

## Expected Deliverables

1. A new secure install mode for Claude and a stronger doctor/security audit path.
2. Runtime and config-level warnings or failures when MCP integration bypasses `fuse`.
3. Expanded rule coverage for secret-path access and security-sensitive local files.
4. Companion native-tool protections for high-risk read/write paths.
5. Updated README/spec/release docs that accurately describe the hardened posture and remaining limits.

## Files To Inspect

| File | Why |
|------|-----|
| `internal/cli/install.go` | Current Claude/Codex install logic and merge behavior |
| `internal/cli/doctor.go` | Current diagnostics and live checks |
| `internal/cli/install_test.go` | Install-path regression coverage |
| `internal/cli/doctor_test.go` | Doctor-path regression coverage |
| `internal/adapters/hook.go` | Claude hook entrypoint and hook-mode assumptions |
| `internal/adapters/mcpproxy.go` | MCP trust boundary and downstream mediation |
| `internal/core/mcpclassify.go` | Unknown-tool behavior and argument scanning |
| `internal/policy/hardcoded.go` | High-priority hard blocks for self-protection and sensitive paths |
| `internal/policy/builtins_security.go` | Credential, exfiltration, package, and secret-related built-ins |
| `README.md` | User-facing install and security posture |
| `specs/functional.md` | Product boundary and non-goals |
| `specs/technical_v1.1.md` | Technical contract for install/doctor/runtime behavior |

## Files To Create

| File | Change |
|------|--------|
| `docs/plans/2026-03-12-security-hardening-plan.md` | **NEW** implementation roadmap |
| `docs/release/2026-03-12-security-posture.md` | **NEW** shipped posture, residual gaps, and recommended setup |
| `internal/cli/security_config.go` | **NEW** shared helpers for secure config analysis/merge |
| `internal/cli/security_config_test.go` | **NEW** focused tests for secure config logic |

### Task 1: Define The Secure Contract Before Code

**Files:**
- Modify: `README.md`
- Modify: `specs/functional.md`
- Modify: `specs/technical_v1.1.md`
- Create: `docs/release/2026-03-12-security-posture.md`

**Step 1: Write the contract delta**

Document the new supported posture:
- `fuse install claude --secure` configures the hook plus recommended Claude safety settings.
- `fuse doctor --security` validates that posture and reports drift.
- `fuse` protects mediated shell/MCP paths and selected native file-tool paths, but is still not a sandbox.

**Step 2: Write the failing doc expectations as checklist items**

Expected checklist:
- secure install mode documented
- native-tool limitations documented
- MCP trust model documented
- explicit non-goals preserved

**Step 3: Update the docs minimally**

Keep the spec honest. Do not claim:
- full filesystem mediation
- full network mediation
- hostile-agent resistance

**Step 4: Verify docs reference the same contract**

Run:

```bash
rg -n "secure|doctor --security|sandbox|MCP trust|native tool" README.md specs/functional.md specs/technical_v1.1.md docs/release/2026-03-12-security-posture.md
```

**Step 5: Commit**

```bash
git add README.md specs/functional.md specs/technical_v1.1.md docs/release/2026-03-12-security-posture.md
git commit -m "docs: define hardened security contract"
```

### Task 2: Add Secure Claude Install Mode

**Files:**
- Modify: `internal/cli/install.go`
- Create: `internal/cli/security_config.go`
- Test: `internal/cli/install_test.go`
- Test: `internal/cli/security_config_test.go`

**Step 1: Write the failing tests**

Add tests covering:
- `fuse install claude --secure` sets the hook and secure Claude config keys without clobbering unrelated settings
- re-running secure install is idempotent
- insecure existing values are upgraded only for the `fuse`-managed keys

Focus on exact JSON merge behavior for:
- hook matchers
- sandbox/permission-related keys
- MCP-related safe defaults that belong in `settings.json`

**Step 2: Run the focused tests and confirm failure**

Run:

```bash
go test ./internal/cli -count=1 -run 'TestInstallClaude|TestSecureClaude'
```

Expected: failure because secure mode and secure-config helpers do not exist yet.

**Step 3: Add the CLI surface**

Implement:
- `fuse install claude --secure`
- internal helpers that merge recommended secure defaults into Claude settings
- machine-readable comments/constants for which keys are `fuse`-managed

Do not add interactive prompts in v1. The command should be deterministic.

**Step 4: Re-run focused tests**

Run:

```bash
go test ./internal/cli -count=1 -run 'TestInstallClaude|TestSecureClaude'
```

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/cli/install.go internal/cli/security_config.go internal/cli/install_test.go internal/cli/security_config_test.go
git commit -m "feat(cli): add secure Claude install mode"
```

### Task 3: Add Security Diagnostics To Doctor

**Files:**
- Modify: `internal/cli/doctor.go`
- Modify: `internal/cli/doctor_test.go`
- Modify: `internal/cli/security_config.go`
- Test: `internal/cli/security_config_test.go`

**Step 1: Write the failing tests**

Add tests for:
- `fuse doctor --security` warns when Claude hook exists but secure settings are missing
- `fuse doctor --security` warns when Codex still has built-in shell enabled or `fuse-shell` is missing
- `fuse doctor --security` warns when risky/unmediated MCP server config is detected
- standard `fuse doctor` remains backward-compatible

**Step 2: Run the focused tests and confirm failure**

Run:

```bash
go test ./internal/cli -count=1 -run 'TestDoctor'
```

Expected: failure because the new security checks and flag do not exist yet.

**Step 3: Implement security checks**

Add:
- `--security` flag
- Claude secure-config validation
- Codex shell mediation validation
- warnings for suspicious PTY/TTY posture if the current doctor can detect it safely
- warnings for missing mediation on known MCP servers

Prefer `WARN` over `FAIL` when `fuse` cannot safely auto-prove a risky state.

**Step 4: Re-run focused tests**

Run:

```bash
go test ./internal/cli -count=1 -run 'TestDoctor'
```

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/cli/doctor.go internal/cli/doctor_test.go internal/cli/security_config.go internal/cli/security_config_test.go
git commit -m "feat(cli): add security posture diagnostics"
```

### Task 4: Enforce MCP Trust Controls

**Files:**
- Modify: `internal/cli/install.go`
- Modify: `internal/cli/doctor.go`
- Modify: `internal/adapters/mcpproxy.go`
- Modify: `internal/adapters/mcpproxy_test.go`
- Modify: `internal/core/mcpclassify.go`

**Step 1: Write the failing tests**

Add tests for:
- doctor warns on configured MCP servers that are not routed through `fuse proxy mcp` or `fuse proxy codex-shell`
- unknown/untrusted downstream MCP server names are surfaced clearly in diagnostics
- MCP proxy rejects or warns on obviously unsafe local resource reads and risky tool metadata where applicable

**Step 2: Run focused tests**

Run:

```bash
go test ./internal/adapters ./internal/cli -count=1 -run 'TestMCP|TestDoctor'
```

Expected: failure because trust-control diagnostics are incomplete.

**Step 3: Implement the minimum trust layer**

Implement:
- config inspection helpers that identify mediated vs unmediated MCP configuration
- stronger doctor output for unsafe MCP posture
- optional runtime warnings/events when a downstream server exposes unexpected destructive capabilities

Do not block all unknown MCP servers by default in this slice. Diagnose first, then tighten later if evidence shows low-friction enforcement is viable.

**Step 4: Re-run focused tests**

Run:

```bash
go test ./internal/adapters ./internal/cli -count=1 -run 'TestMCP|TestDoctor'
```

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/cli/install.go internal/cli/doctor.go internal/adapters/mcpproxy.go internal/adapters/mcpproxy_test.go internal/core/mcpclassify.go
git commit -m "feat(mcp): add mediation trust controls"
```

### Task 5: Expand Sensitive Path And Secret Access Protections

**Files:**
- Modify: `internal/policy/hardcoded.go`
- Modify: `internal/policy/builtins_security.go`
- Modify: `internal/policy/policy_test.go`
- Modify: `internal/core/classify_test.go`
- Modify: `testdata/fixtures/commands.yaml`

**Step 1: Write the failing fixtures and tests**

Add positive and near-miss coverage for:
- reads of `.env`, `.env.*`, `*.pem`, `*.key`, kubeconfig, `~/.aws`, `~/.config/gcloud`, `~/.azure`, and common token files
- dangerous edits to `.git/config`, `.git/hooks/*`, CI secret files, and Claude/Codex config files
- exfiltration-adjacent packaging/install flows where secret-bearing paths are referenced

Classify with care:
- catastrophic secret-file deletion or mutation: `BLOCKED`
- secret-file bulk reads/dumps: `APPROVAL` or `CAUTION` depending on sensitivity and commonality
- harmless near-miss developer files: `SAFE`

**Step 2: Run focused tests and confirm failure**

Run:

```bash
go test ./internal/policy ./internal/core -count=1
```

Expected: failure because the new rules/fixtures are not present yet.

**Step 3: Implement the smallest rule set that closes the biggest gaps**

Prefer:
- hardcoded rules for self-protection and obviously sensitive managed files
- built-ins for broader secret-path patterns and credential access categories

Do not add expansive wildcard rules that would make normal coding noisy.

**Step 4: Re-run focused tests**

Run:

```bash
go test ./internal/policy ./internal/core -count=1
```

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/policy/hardcoded.go internal/policy/builtins_security.go internal/policy/policy_test.go internal/core/classify_test.go testdata/fixtures/commands.yaml
git commit -m "feat(policy): harden sensitive path protections"
```

### Task 6: Add Native File-Tool Guardrails For Claude

**Files:**
- Modify: `internal/adapters/hook.go`
- Modify: `internal/adapters/hook_test.go`
- Modify: `internal/cli/install.go`
- Modify: `README.md`
- Modify: `specs/technical_v1.1.md`

**Step 1: Write the failing tests**

Add hook tests for Claude native tool names such as:
- `Read`
- `Write`
- `Edit`
- `MultiEdit`

Cover:
- blocked access to obviously sensitive files
- approval-required access to high-risk secret locations
- safe access to normal project files

**Step 2: Run focused tests and confirm failure**

Run:

```bash
go test ./internal/adapters -count=1 -run 'TestRunHook|TestHook'
```

Expected: failure because hook handling currently focuses on `Bash` and `mcp__*`.

**Step 3: Implement minimal native-tool classification**

Implement:
- extraction of target file paths from Claude native file-tool payloads
- classification using a narrow sensitive-path policy
- secure-install hook matcher updates if Claude requires explicit file-tool hook matchers

Keep this slice deliberately narrow: path-based protections only. Do not attempt semantic diff review or full content scanning in v1.

**Step 4: Re-run focused tests**

Run:

```bash
go test ./internal/adapters -count=1 -run 'TestRunHook|TestHook'
```

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/adapters/hook.go internal/adapters/hook_test.go internal/cli/install.go README.md specs/technical_v1.1.md
git commit -m "feat(hook): protect sensitive native file tools"
```

### Task 7: Add Release-Gate Verification And Final Docs

**Files:**
- Modify: `.github/workflows/ci.yml`
- Modify: `README.md`
- Modify: `docs/release/2026-03-12-security-posture.md`
- Modify: `docs/release/rc1-readiness-report.md`

**Step 1: Write the failing expectations**

Add CI/documentation expectations for:
- secure install/doctor coverage
- native file-tool coverage
- MCP trust warnings and policy checks

**Step 2: Update CI minimally**

Add only the checks that are stable and deterministic:
- CLI tests
- hook tests
- policy/core tests

Do not add flaky live external-security scanners in this slice.

**Step 3: Update public docs**

Document:
- recommended secure setup
- what `fuse` now protects
- what still requires Claude/Codex host permissions and human judgment

**Step 4: Run full verification**

Run:

```bash
go test -count=1 ./...
go test -race -count=1 ./...
golangci-lint run
govulncheck ./...
```

Expected: PASS.

**Step 5: Commit**

```bash
git add .github/workflows/ci.yml README.md docs/release/2026-03-12-security-posture.md docs/release/rc1-readiness-report.md
git commit -m "docs: publish hardened security guidance"
```

## Recommended Implementation Order

1. Task 1 and Task 2 first, because secure install is the highest-leverage gap.
2. Task 3 and Task 4 second, because diagnostics and MCP trust make the posture observable.
3. Task 5 third, because sensitive-path protection is high value and low architecture risk.
4. Task 6 after that, because native file-tool mediation is useful but the most integration-sensitive.
5. Task 7 last, once the behavior is stable enough to document and gate in CI.

## Open Questions To Resolve During Implementation

1. Which Claude settings keys are stable enough to let `fuse install claude --secure` manage automatically?
2. Whether native file-tool hooks can be matched directly in Claude settings or need broader `PreToolUse` coverage.
3. Whether Codex config exposes enough structure to reliably detect all shell bypass paths.
4. Which sensitive-path reads should be `CAUTION` versus `APPROVAL` to avoid making daily use annoying.

## tk Follow-Up

Create or update linked `tk` tickets before implementation begins:

```bash
tk create "Add secure Claude install mode" -d "Merge recommended Claude security defaults into settings.json via fuse install claude --secure." -t task -p 1
tk create "Add security posture diagnostics" -d "Teach fuse doctor to validate secure Claude/Codex settings and mediated MCP posture." -t task -p 1
tk create "Protect sensitive native file tools" -d "Add narrow Claude native file-tool path protections for secret and config files." -t task -p 1
```
