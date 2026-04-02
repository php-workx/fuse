---
id: research-2026-04-02-rule-landscape
type: research
date: 2026-04-02
---

# Research: Rule Landscape

**Backend:** inline  
**Scope:** Re-audit `specs/rule-landscape.md` against `main` and identify the real remaining gaps worth implementing now.

## Summary

The spec matrix is stale in a few important places. Two headline gaps it calls out as open are already implemented: safe build-dir allowlisting and container-escape coverage. The highest-value gaps still actually open in the tree are CI/CD protection and fuller indirect-execution handling beyond the current regex-only `xargs` / `find -exec` coverage.

## Key Files

| File | Purpose |
|------|---------|
| `internal/core/safecmds.go` | Safe build-dir allowlist logic |
| `internal/core/classify.go` | Safe build cleanup layer in classification |
| `internal/core/normalize.go` | Wrapper stripping and inner command extraction |
| `internal/policy/builtins_security.go` | Existing indirect execution and security builtins |
| `internal/policy/rule_tags.go` | Builtin tag registry for tag overrides |
| `testdata/fixtures/commands.yaml` | Golden command coverage |

## Baseline Audit

- Safe build cleanup is already implemented:
  - `safeBuildDirs` contains 28 allowlisted directories in `internal/core/safecmds.go:694-704`
  - `IsSafeBuildCleanup` is wired into the classifier at `internal/core/classify.go:699-701`
  - verified with `rg -n 'safeBuildDirs|IsSafeBuildCleanup|safe build directory cleanup' internal/core`
- Container-escape coverage is already present:
  - `nsenter`, `/var/run/docker.sock`, and related rules are covered in fixtures and builtins
  - verified with `rg -n 'nsenter|docker.sock|privileged' internal/policy testdata/fixtures/commands.yaml`
- CI/CD coverage is still effectively zero:
  - no CI/CD builtins in `internal/policy/builtins_core.go`, `internal/policy/builtins_security.go`, or `internal/policy/rule_tags.go`
  - no CI/CD fixtures in `testdata/fixtures/commands.yaml`
  - verified with `python3` content counts and `rg -n 'gh secret|actions/secrets|gitlab-runner unregister|delete-job|circleci' ...`
- Indirect execution handling is partial:
  - wrapper stripping supports `sudo`, `doas`, `env`, `nohup`, `time`, `nice`, `ionice`, `timeout`, `strace`, `ltrace`, `taskset`, `setsid`, `chroot`, `runas` in `internal/core/normalize.go:20-39`
  - inner extraction exists for `bash/sh -c`, `ssh`, `powershell`, `cmd`, and `wsl`, but not `xargs`, `find -exec`, `parallel`, or `watch`
  - security builtins currently cover only regex-level detection for `xargs` and `find -exec shell` in `internal/policy/builtins_security.go:453-462`
  - there are zero indirect-execution fixtures for `xargs`, `find -exec`, `parallel`, or `watch`

## Findings

1. `specs/rule-landscape.md` is directionally useful but no longer an accurate status document. The safe build-dir allowlist it lists as missing is already implemented in `internal/core/safecmds.go:694-735`, and the classifier explicitly short-circuits to `SAFE` for those cleanups in `internal/core/classify.go:699-701`.

2. Container-escape coverage is also no longer missing. The spec still marks it as a gap, but the current policy and fixtures already cover `nsenter`, privileged containers, and docker-socket style escape vectors. That makes it a documentation-drift item, not active engineering work.

3. CI/CD protection remains a true gap. I found no GitHub Actions, GitLab CI, Jenkins, or CircleCI secret/runner/job rules in the builtin policy files and no related golden fixtures. This is the cleanest remaining “new rule family” to add from the spec.

4. Recursive unwrapping is only partly addressed. Fuse already strips a good set of prefix wrappers and extracts several inner-command forms, but it still does not structurally unwrap `xargs`, `find -exec`, `parallel`, or `watch`. Those cases are only caught when a direct regex pattern happens to match the outer shell string.

5. The best implementation slice is therefore:
   - add a first CI/CD builtin family with tags and fixtures
   - add one level of indirect extraction or fail-closed classification for the highest-signal wrappers (`xargs`, `find -exec`, `watch`, `parallel`)
   - leave broader category ports from the matrix as follow-up work

## Recommendation

Implement in one focused wave:

1. Add CI/CD builtins plus `rule_tags` entries and fixtures.
2. Expand normalization/classification for high-signal indirect execution wrappers.
3. Add golden and unit coverage for both.
4. Capture the remaining category ports from `specs/rule-landscape.md` as explicit follow-up work rather than treating the whole matrix as in-scope now.
