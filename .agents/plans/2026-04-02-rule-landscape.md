---
id: plan-2026-04-02-rule-landscape
type: plan
date: 2026-04-02
source: .agents/research/2026-04-02-rule-landscape.md
---

# Plan: Rule Landscape Gap Closure

## Goal

Close the real Priority 1 gaps that remain open from `specs/rule-landscape.md`: add an initial CI/CD protection family and expand indirect-execution handling beyond the current regex-only coverage.

## Scope Decision

This plan intentionally does **not** try to port the entire category matrix from `specs/rule-landscape.md` in one pass. Discovery showed two claimed gaps are already done:

- safe build-dir allowlist
- container-escape coverage

The focused implementation slice is:

1. CI/CD destructive secret/job/runner operations
2. Structural extraction for high-signal indirect wrappers:
   - `find -exec sh -c`
   - `xargs ... sh -c`
   - `watch <command>`
   - `parallel <command>`

## Baseline Audit

- Safe build allowlist already present:
  - `internal/core/safecmds.go:694-735`
  - `internal/core/classify.go:699-701`
- Existing indirect extraction support:
  - wrapper stripping: `internal/core/normalize.go:20-39`
  - inner extraction only for `bash/sh -c`, `ssh`, `powershell`, `cmd`, `wsl`: `internal/core/normalize.go:154-189`
- Existing indirect regex-only rules:
  - `builtin:indirect:xargs-exec`: `internal/policy/builtins_security.go:453-456`
  - `builtin:indirect:find-exec`: `internal/policy/builtins_security.go:459-462`
- Missing today:
  - CI/CD builtins: `0`
  - indirect wrapper fixtures (`xargs`, `find -exec`, `parallel`, `watch`): `0`
- Verification commands:
  - `rg -n 'safeBuildDirs|IsSafeBuildCleanup|safe build directory cleanup' internal/core`
  - `rg -n 'gh secret|actions/secrets|gitlab-runner unregister|delete-job|circleci' internal/policy testdata/fixtures`
  - `rg -n 'xargs|find -exec|parallel|watch' internal/core/normalize.go testdata/fixtures/commands.yaml`

## Files to Modify

| File | Change |
|------|--------|
| `internal/core/normalize.go` | Add indirect-wrapper extraction helpers for `find -exec sh -c`, `xargs ... sh -c`, `watch`, and `parallel` |
| `internal/core/normalize_test.go` | Add focused extraction tests for the new wrappers |
| `internal/core/classify_test.go` | Add end-to-end classification tests proving inner destructive commands now dominate |
| `internal/policy/builtins_ci.go` | **NEW** CI/CD builtin rules |
| `internal/policy/rule_tags.go` | Register tags/keywords for the new CI/CD rule IDs |
| `testdata/fixtures/commands.yaml` | Add positive and near-miss fixtures for CI/CD and indirect wrappers |

## Implementation Specs

### 1. Indirect wrapper extraction

**`internal/core/normalize.go`**

Add four handlers in `classificationNormalizeRecursive(...)` after the existing `ssh` / shell-wrapper extraction checks:

- `handleFindExecShell(tokens, i, depth, result)`
- `handleXargsShell(tokens, i, depth, result)`
- `handleWatchCommand(tokens, i, depth, result)`
- `handleParallelCommand(tokens, i, depth, result)`

Behavior:

- Preserve the full outer command in `result.Outer`.
- Extract one inner command string when the wrapper clearly executes a nested shell command.
- Recurse through `classificationNormalizeRecursive` so nested `rm -rf /`, `terraform destroy`, etc. get fully classified.
- If the wrapper shape strongly implies hidden execution but extraction fails, set `ExtractionFailed = true` so the outer command can fail closed instead of silently becoming `SAFE`.

### 2. CI/CD builtin family

**`internal/policy/builtins_ci.go`**

Add a new builtin file with CAUTION-tier rules for destructive CI/CD control-plane actions:

- GitHub:
  - `gh secret delete`
  - `gh variable delete`
  - `gh api` against `actions/secrets` or `actions/variables` with destructive HTTP verbs
- GitLab:
  - `gitlab-runner unregister`
  - `glab variable delete`
- Jenkins:
  - `jenkins-cli delete-job`
  - `java -jar jenkins-cli.jar delete-job`
- CircleCI:
  - `circleci context remove-secret`

Use targeted regexes and keep them narrow enough that read-only/status commands remain SAFE.

### 3. Rule tags

**`internal/policy/rule_tags.go`**

Register tags and keywords for the new rule IDs so `tag_overrides` and tag validation continue to work.

Suggested tags:

- `cicd`
- `github-actions`
- `gitlab-ci`
- `jenkins`
- `circleci`

### 4. Test additions

**`internal/core/normalize_test.go`** — add:

- `TestClassificationNormalize_FindExecShellExtractsInner`
- `TestClassificationNormalize_XargsShellExtractsInner`
- `TestClassificationNormalize_WatchExtractsInnerCommand`
- `TestClassificationNormalize_ParallelExtractsInnerCommand`

**`internal/core/classify_test.go`** — add:

- `TestClassify_IndirectExecutionInnerBlockedCommand`
- `TestClassify_IndirectExecutionInnerCautionCommand`

**`testdata/fixtures/commands.yaml`** — add positive/near-miss rows for:

- `gh secret delete` / `gh secret list`
- `gh api ... actions/secrets ... DELETE` / `gh api ... actions/secrets ... GET`
- `gitlab-runner unregister` / `gitlab-runner list`
- `jenkins-cli delete-job` / `jenkins-cli list-jobs`
- `circleci context remove-secret` / `circleci context list`
- `find ... -exec sh -c 'rm -rf /'`
- `printf "rm -rf /\n" | xargs -I{} sh -c '{}'`
- `watch "terraform destroy"`
- `parallel "kubectl delete ns prod" ::: 1`

## Verification

1. `go test ./internal/core -run 'TestClassificationNormalize_|TestClassify_IndirectExecution' -count=1`
2. `go test ./internal/policy -count=1`
3. `go test ./internal/core -run 'TestFixtureCoverage|TestClassify_BuiltinSectionSentinels' -count=1`
4. `go test ./...` if runtime permits

## Follow-up Work Explicitly Deferred

- Broader DCG pack ports: CDN, DNS, API gateways, messaging, monitoring, payment, search, backups
- Broader anti-evasion work beyond current normalization
- Multi-event behavioral chains
- Larger PII/data-masking work
