# Review: Phase 1 -- Windows Foundation

**Reviewer persona:** Senior Go developer, cross-platform CLI specialist
**Date:** 2026-03-28
**Artifact:** `.agents/plans/2026-03-28-windows-foundation.md`
**Supporting spec:** `specs/windows-support-plan.md`

## Verdict: CONDITIONAL PASS

The plan is thorough, correctly identifies the cascade root cause, and proposes a sound build-tag strategy. The fail-safe analysis is exemplary -- tracing the `errNonInteractive` path all the way to `DecisionBlocked` is exactly the kind of due diligence this change requires. However, there are several concrete issues that would cause implementation to stall or produce subtly incorrect results if not addressed before coding begins.

---

## Findings

### 1. NOTE: `syscall.SIGHUP` reference in `prompt.go` will be extracted to `prompt_unix.go`, but the plan should verify it

**File:** `internal/approve/prompt.go:80`

The plan proposes renaming `prompt.go` to `prompt_unix.go` with `//go:build unix`, which inherently fixes this. However, the plan's audit table lists `unix.*` usages (20 occurrences) but does not separately track `syscall.SIGHUP` as a potential concern. On Windows, `syscall.SIGHUP` is actually defined (Go defines "invented" signal values on Windows for portability), so this is not a real blocker. But the plan should document this explicitly to prevent a future implementer from mistakenly worrying about it.

**Severity:** NOTE
**Recommendation:** Add a note to Issue 1 that `syscall.SIGINT`, `syscall.SIGTERM`, and `syscall.SIGHUP` are all defined on Windows as invented constants, so any shared code referencing these compiles fine. The signals that are NOT defined on Windows are `SIGTSTP`, `SIGWINCH`, `SIGTTOU`, `SIGQUIT` (minus the ones Go invents). The current extraction correctly avoids leaving these in shared code.

---

### 2. NOTE: `runner_tty_windows.go` stub for `ForegroundChildProcessGroupIfTTY` should show full implementation

**File:** Plan Issue 2

The plan mentions creating both `runner_exec_windows.go` (for shell execution stubs) and `runner_tty_windows.go` (for `ForegroundChildProcessGroupIfTTY` no-op). However, the plan's description for `runner_tty_windows.go` says just "no-op stub" without specifying the return value explicitly in the code block. The only concrete code shown is:

> `runner_tty_windows.go` — Windows stub returns `(nil, nil)`.

This is correct behavior -- callers guard with `if restore != nil` before invoking the restore function. But since `ForegroundChildProcessGroupIfTTY` is an exported symbol used by `doctor.go`, this must be crystal clear.

**Severity:** NOTE
**Recommendation:** Add the full stub implementation to the plan:
```go
//go:build windows

package adapters

func ForegroundChildProcessGroupIfTTY(pid int) (restore func(), err error) {
    return nil, nil
}
```

---

### 3. WARN: `executeCapturedShellCommand` stays in `runner.go` but its chain must be verified

**File:** `internal/adapters/runner.go:395-397`

The plan correctly extracts `executeCapturedShellCommandWithStdin` to `runner_exec_unix.go` and provides a Windows stub. The thin wrapper `executeCapturedShellCommand` stays in `runner.go` and calls `executeCapturedShellCommandWithStdin`. On Windows, this means `executeCapturedShellCommand` calls the stub which returns an error immediately. This is correct.

However, `codexshell.go:265` and `codexshell.go:340` call `executeCapturedShellCommand` directly. On Windows, these code paths will silently fail with a generic error. The error message from the stub is "shell execution is not yet supported on Windows" which is technically correct, but `codexshell.go` formats its own error from this. The implementer should verify that the error propagation through `codexshell.go` produces a sensible user-facing message, not a cryptic internal error.

**Severity:** WARN
**Recommendation:** After implementation, manually trace what `codexshell.go` returns to the MCP client when the shell stub returns an error. It should return a proper JSON-RPC error, not crash or return malformed JSON.

---

### 4. WARN: Plan claims `WaitStatus.Signaled()` is "not available on Windows" -- this is incorrect

**File:** Plan table in Issue 2

The plan's table says `interpretWaitError` fails because "Method not available on Windows." In fact, `syscall.WaitStatus` on Windows is a struct with an `ExitCode uint32` field, and it does have a `Signaled()` method (always returns `false`). The function would compile on Windows. It is being extracted anyway because `interpretWaitError` is only called from `waitForManagedCommand` (which is Unix-only), so it is not a problem in practice. But the stated reason is technically wrong and could mislead a future developer who reads the plan for context.

**Severity:** NOTE
**Recommendation:** Correct the table entry to: "Always returns false on Windows, making the signal exit code branch dead code. Extracted to Unix file for correctness."

---

### 5. WARN: `runner_windows.go` stub for `platformSysProcAttr` should not return empty struct

**File:** Plan Issue 2 -- `runner_windows.go`

The plan shows:
```go
func platformSysProcAttr() *syscall.SysProcAttr {
    return &syscall.SysProcAttr{}
}
```

This compiles, but returning an empty `SysProcAttr` on Windows allocates a struct with all zero values, which is a valid no-op. However, the caller (`executeShellCommand`) is also being stubbed to return an error immediately on Windows, so `platformSysProcAttr` is never actually called on Windows in Phase 1. Despite this, the function must exist because it is referenced from `executeCapturedShellCommandWithStdin` -- wait, no. `executeCapturedShellCommandWithStdin` is also being extracted to Unix-only. And `executeShellCommand` is also being extracted.

So `platformSysProcAttr` is only called from Unix-extracted functions. On Windows, it is only called if someone adds new code in `runner.go` that uses it. Returning `&syscall.SysProcAttr{}` is safe because Windows `SysProcAttr` has fields like `HideWindow`, `CmdLine`, `CreationFlags` -- all zero-valued, which is benign.

**Severity:** NOTE
**Recommendation:** No change needed, but add a comment in the stub: `// Not called in Phase 1; all callers are in Unix-only files.`

---

### 6. WARN: `golang.org/x/term` for `help_width_windows.go` needs `go.mod` promotion

**File:** Plan Issue 4

The plan uses `golang.org/x/term` in `help_width_windows.go` and notes "If `golang.org/x/term` is not already a direct dependency, add it to `go.mod`." Currently, `golang.org/x/term` is NOT in `go.mod` at all -- not even as indirect. However, `github.com/charmbracelet/x/term v0.2.2` IS an indirect dependency. These are different packages (`golang.org/x/term` vs `github.com/charmbracelet/x/term`).

Running `go get golang.org/x/term` will add it as a direct dependency. This is a minor dependency addition but the plan should be explicit about it since the project emphasizes minimal dependencies.

**Severity:** WARN
**Recommendation:** Either: (a) explicitly call out the `go get golang.org/x/term` step in Issue 4, or (b) use a simpler approach for Phase 1 and just return hardcoded values (80, false) without the `golang.org/x/term` import, deferring the real terminal detection to Phase 3 when the Console API work happens. Option (b) avoids a new dependency for a feature that is marginally useful on Windows at this stage.

---

### 7. WARN: Cross-build gate test uses `cmd.Environ()` which may behave differently across Go versions

**File:** Plan Issue 6 -- `crossbuild_test.go`

```go
cmd.Env = append(cmd.Environ(), "GOOS=windows", "GOARCH=amd64")
```

`cmd.Environ()` returns the parent process environment when `cmd.Env` is nil. Appending `GOOS=windows` to an environment that already contains `GOOS=darwin` or `GOOS=linux` results in duplicate entries. Go's `exec.Cmd` uses the last value for duplicate keys on Unix, but this behavior is platform-dependent and not guaranteed by the spec. On Windows runners (where this test is skipped, but still), duplicate env vars behave differently.

**Severity:** WARN
**Recommendation:** Use `os.Environ()` filtered to remove existing `GOOS`/`GOARCH` entries, then append the desired values:
```go
env := os.Environ()
filtered := make([]string, 0, len(env))
for _, e := range env {
    if !strings.HasPrefix(e, "GOOS=") && !strings.HasPrefix(e, "GOARCH=") {
        filtered = append(filtered, e)
    }
}
cmd.Env = append(filtered, "GOOS=windows", "GOARCH=amd64")
```

---

### 8. WARN: Cross-build gate test uses `go build ./...` from wrong directory

**File:** Plan Issue 6 -- `crossbuild_test.go`

The test runs `exec.Command("go", "build", "./...")` which builds relative to the test's working directory (`internal/adapters/`), not the repo root. This means it would only test the `adapters` package, not the entire binary. To verify the full binary compiles, it should build `./cmd/fuse` from the repo root.

**Severity:** WARN
**Recommendation:** Set `cmd.Dir` to the repo root (derive from the test file path), or change the build target:
```go
// Find the module root.
root := filepath.Join("..", "..")
cmd := exec.Command("go", "build", "./cmd/fuse")
cmd.Dir = root
```
Alternatively, move the test to the repo root `integration_test.go` where it can naturally build `./...`.

---

### 9. WARN: `doctor.go` will have an unused `syscall` import after extraction

**File:** `internal/cli/doctor.go`

After extracting the four live-check functions, `doctor.go` will no longer reference `syscall.*` (the only usage was `syscall.SysProcAttr{Setpgid: true}` on line 935). The `syscall` import on line 14 will become unused, causing a compilation error. Similarly, the `unix` import on line 17, and potentially the `io` import (used in `startForegroundProbeProcess`'s parameter list) and `os/exec` import (used in `startForegroundProbeProcess`'s return type).

The plan's post-extraction verification correctly says `doctor.go` must not contain `unix.*`, but does not mention cleaning up the now-unused `syscall` import or other potentially orphaned imports.

**Severity:** WARN
**Recommendation:** Add to Issue 3's extraction checklist: "After extraction, run `goimports` on `doctor.go` to remove unused imports (`syscall`, `unix`, possibly `io` and `os/exec`). Verify with `go vet ./internal/cli/`." The plan's verification command already includes `go vet` which would catch this, but calling it out prevents confusion.

---

### 10. WARN: `doctor.go` also uses `io` and `os/exec` only in extracted functions

**File:** `internal/cli/doctor.go`

Let me be precise. `startForegroundProbeProcess` (line 930) uses `exec.Command` and takes `io.Reader`/`io.Writer` parameters. `checkLiveForegroundProcessGroup` (line 889) calls `startForegroundProbeProcess` and references `io.Discard`. After extracting all four functions to `doctor_live_unix.go`:

- `io` is used on line 899 (`io.Discard`) -- inside `checkLiveForegroundProcessGroup`, which is extracted
- `os/exec` is used on line 930 (`exec.Command`) -- inside `startForegroundProbeProcess`, which is extracted
- `syscall` is used on line 935 -- inside `startForegroundProbeProcess`, which is extracted

If these are the ONLY uses of `io`, `os/exec`, and `syscall` in `doctor.go`, all three imports become orphaned. The `go vet` step will catch this, but the implementer should know in advance that the import block needs cleanup.

**Severity:** NOTE (subsumed by Finding 9)
**Recommendation:** Verify all imports after extraction. The `doctor_live_unix.go` file will need its own import block including `fmt`, `io`, `os`, `os/exec`, `syscall`, `golang.org/x/sys/unix`, and `github.com/php-workx/fuse/internal/adapters`.

---

### 11. WARN: GoReleaser `format_overrides` syntax should be verified for v2

**File:** `.goreleaser.yml`

The plan proposes:
```yaml
archives:
  - id: default
    formats:
      - tar.gz
    format_overrides:
      - goos: windows
        formats:
          - zip
```

GoReleaser v2 changed the archive format configuration. The current `.goreleaser.yml` uses `version: 2` already. In GoReleaser v2, the syntax for format overrides uses `formats` (plural, a list) at the top level and `format_overrides` with `goos`/`formats` entries. The plan's syntax looks correct for GoReleaser v2, but it should be validated against the specific GoReleaser version pinned in the project (or used by the release workflow).

**Severity:** NOTE
**Recommendation:** Validate the exact GoReleaser v2 `format_overrides` syntax before implementation. Run `goreleaser check` after making the change.

---

### 12. WARN: CI workflow needs careful scoping of Windows job

**File:** `.github/workflows/ci.yml`

The plan says to add `windows-latest` to the quality gate OS matrix. The current CI has three jobs: `check` (quality gate on ubuntu-latest), `compat` (shell compatibility on ubuntu+macos), and `build` (cross-build on ubuntu-latest). The plan's wording "Add `windows-latest` to the quality gate OS matrix" is ambiguous because the `check` job currently runs on a single OS, not a matrix.

Converting `check` to a matrix (`ubuntu-latest` + `windows-latest`) means every step runs on both OSes. But several steps will fail on Windows:
- `semgrep` -- not supported on Windows
- `shellcheck` -- may not be available (the current recipe handles this with `command -v` but `find scripts` may behave differently)
- `betterleaks` -- may be slow or unavailable
- `actionlint` -- should work
- `golangci-lint` -- needs Windows-compatible cache paths
- `sonar` -- skipped already (local only, not in CI)
- Coverage upload -- should work

**Severity:** WARN
**Recommendation:** Do NOT add Windows to the existing `check` job matrix. Instead, create a dedicated `windows-check` job with a reduced scope: `go build ./...`, `go vet ./...`, `go test ./... -race`, and optionally `golangci-lint`. Skip semgrep, shellcheck, betterleaks, budgets (which use bash-specific syntax). This is cleaner and avoids littering the existing job with `if: runner.os != 'Windows'` conditionals.

---

### 13. WARN: `doctor.go` imports `adapters` package -- extraction ordering is critical

**File:** `internal/cli/doctor.go:21`

`doctor.go` imports `github.com/php-workx/fuse/internal/adapters` to call `adapters.ForegroundChildProcessGroupIfTTY`. After extraction, this call moves to `doctor_live_unix.go`. The question is: does `doctor.go` still need the `adapters` import?

If `adapters.ForegroundChildProcessGroupIfTTY` is the ONLY reference to the `adapters` package in `doctor.go`, then the import becomes unused and must be removed from `doctor.go` (and added to `doctor_live_unix.go`). Let me verify.

Looking at the actual file: `adapters` is imported on line 21. It is used on line 912 inside `checkLiveForegroundProcessGroup`, which is being extracted. If this is the only use of `adapters` in `doctor.go`, the import must be cleaned up.

The plan correctly identifies the dependency (`Issue 3 depends on Issue 2 because doctor calls `adapters.ForegroundChildProcessGroupIfTTY``) but does not mention the import cleanup in `doctor.go`.

**Severity:** WARN (not a true blocker since `go vet` catches it, but it can confuse the implementer)
**Recommendation:** Explicitly note that after extraction, `doctor.go`'s import block needs cleanup. The `doctor_live_unix.go` file needs to import both `golang.org/x/sys/unix` and the `adapters` package.

---

### 14. NOTE: Case-insensitive path matching on Windows

**File:** `internal/adapters/native_file_policy.go`

The native file policy uses `filepath.ToSlash` for normalization, which handles backslashes correctly. However, Windows file paths are case-insensitive (`C:\Users` == `c:\users`). The current path matching uses exact string comparison (`==`, `strings.HasSuffix`). On Windows, a path like `.Claude/Settings.json` (case variant) would NOT be caught by the `.claude/settings.json` check.

This is a Phase 2+ concern (the classification pipeline and file policy already compile on Windows), but it represents a security gap that should be flagged now.

**Severity:** NOTE
**Recommendation:** Add to the Phase 5 (Windows Security Intelligence) backlog: "File policy path matching needs case-insensitive comparison on Windows. Use `strings.EqualFold` or normalize to lowercase."

---

### 15. NOTE: `trustedPath()` returns empty string on Windows

**File:** Plan Issue 2 -- `runner_windows.go`

```go
func trustedPath() string {
    return "" // Windows PATH is managed by the OS; no hardcoded safe PATH.
}
```

`BuildChildEnv` (which stays in `runner.go`) sets `PATH=` + `trustedPath()`. On Windows, this would set `PATH=` (empty), which would break any command execution. In Phase 1 this is harmless because `executeShellCommand` is stubbed. But when Phase 2 implements shell execution, an empty PATH will be a nasty surprise.

**Severity:** WARN
**Recommendation:** Return a minimal Windows PATH instead of empty:
```go
func trustedPath() string {
    systemRoot := os.Getenv("SystemRoot")
    if systemRoot == "" {
        systemRoot = `C:\Windows`
    }
    return systemRoot + `\System32;` + systemRoot + `;` + systemRoot + `\System32\Wbem`
}
```
Production callers of `BuildChildEnv` (`executeShellCommand` and `executeCapturedShellCommandWithStdin`) are being extracted to Unix-only, so the empty PATH is not a runtime risk in Phase 1. However, `BuildChildEnv` is an exported function and its test `TestBuildChildEnv_ResetsPathToTrusted` will run on Windows. The test passes (it asserts `PATH == trustedPath()`, and empty equals empty), but it validates useless behavior -- confirming PATH is set to empty. This is confusing and masks a latent bug that will surface in Phase 2.

---

### 16. NOTE: `BuildChildEnv` strips `DYLD_*` on Windows -- harmless but sloppy

**File:** `internal/adapters/runner.go:61`

`BuildChildEnv` strips environment variables prefixed with `DYLD_` (macOS dynamic linker). On Windows, no such variables exist, so this is harmless dead code. But it would be cleaner to guard this with a runtime check or build tag in a future cleanup pass.

**Severity:** NOTE
**Recommendation:** No action for Phase 1. Consider `runtime.GOOS` guard in Phase 2.

---

## Missing Items

### M1. No `GOOS=windows go vet ./...` in the existing pre-commit/CI flow

The plan mentions `GOOS=windows go vet` in per-issue verification, but does not propose adding it as a permanent quality gate. Without this, a developer could add Unix-specific code to a non-tagged file and break Windows compilation without knowing. The cross-build gate test (Issue 6) partially addresses this, but it only runs `go build`, not `go vet`. `go vet` catches additional issues like unreachable code and incorrect format strings that `go build` misses.

**Recommendation:** Add `GOOS=windows GOARCH=amd64 go vet ./...` to the CI Windows job and to the cross-build gate test.

---

### M2. No plan for test file handling on Windows

The plan does not address what happens to test files when `GOOS=windows go test ./...` runs. Specific issues:

- `internal/approve/prompt_test.go` tests `sanitizePrompt`, which moves to `prompt_shared.go` (no build tag). This test is fine on Windows as-is.
- `internal/adapters/runner_test.go` has tests like `TestExecuteCommand_SafeCommand` (line 219) and `TestExecuteCommand_DisabledPassesThrough` (line 240) that call `ExecuteCommand`, which in the disabled-mode path calls `executeShellCommand` directly. On Windows, the stub returns an error, so these tests would FAIL. They need `//go:build unix` or `if runtime.GOOS == "windows" { t.Skip(...) }`.
- `internal/adapters/codexshell_test.go` line 177 calls `executeCapturedShellCommand` directly. This would hit the Windows stub and fail.
- `internal/cli/doctor_test.go` line 441 references `/dev/tty` in expected test output. This would need conditional expectations on Windows.

**Recommendation:** Audit all test files in the affected packages. For tests that exercise shell execution, add `t.Skip("shell execution not supported on Windows")` guards. This is required before the Windows CI job can run `go test ./...` without failures.

---

### M3. No mention of `go.sum` changes

Adding `golang.org/x/term` as a direct dependency (Issue 4) will modify both `go.mod` and `go.sum`. The plan mentions `go.mod` but not `go.sum`. This is minor but completeness matters for code review.

---

### M4. No Windows-specific error message standardization

The plan uses different error message patterns:
- `"fuse run is not yet supported on Windows"` (runner stub)
- `"shell execution is not yet supported on Windows"` (captured shell stub)
- `"Windows Console API support planned (Phase 3)"` (doctor stub)
- `"Windows job object support planned (Phase 4)"` (doctor stub)

These messages are user-facing. They should follow a consistent pattern and ideally reference a URL or docs page for tracking.

**Recommendation:** Standardize to a pattern like `"not yet supported on Windows (planned: Phase N); see https://..."` or at minimum ensure all messages include "Windows" and a phase reference.

---

### M5. Integration test file at repo root

The plan does not mention `integration_test.go` at the repo root. If it runs shell commands (e.g., `fuse hook evaluate`), it may need `//go:build unix` or conditional skips for Windows.

**Recommendation:** Audit `integration_test.go` for Windows compatibility. Add `if runtime.GOOS == "windows" { t.Skip("...") }` where needed.

---

## Risk Assessment

### Most likely to go wrong during implementation

1. **Orphaned imports after extraction (HIGH probability, LOW severity).** Every extraction (Issues 1, 2, 3) will leave behind unused imports in the original file. `go vet` catches these, but if the implementer doesn't run it after each extraction, they'll hit confusing errors when they try to compile the whole binary and multiple files fail simultaneously. Mitigation: run `goimports -w` after each file extraction.

2. **Test files referencing extracted functions (MEDIUM probability, MEDIUM severity).** If `prompt_test.go` or `runner_test.go` call functions that are now behind `//go:build unix`, those tests will fail to compile on Windows. The plan does not audit test files. Mitigation: grep all test files for references to extracted function names and tag them appropriately.

3. **Empty `trustedPath()` on Windows reaching production before Phase 2 (LOW probability, HIGH severity).** If any code path calls `BuildChildEnv` on Windows in a non-stubbed context (e.g., the MCP proxy, which uses `buildProxyEnv` -- a different function), the empty PATH could cause failures. Current inspection shows `BuildChildEnv` is only called from `executeShellCommand` and `executeCapturedShellCommandWithStdin`, both of which are stubbed. But future changes could add a call. Mitigation: return a real Windows PATH now.

4. **CI Windows job hitting unexpected tooling failures (HIGH probability, LOW severity).** golangci-lint, semgrep, shellcheck, and betterleaks all have Windows-specific quirks. The plan handwaves this with "may need cache path adjustment." In practice, the first CI run will likely fail on tooling setup, not code compilation. Mitigation: create a minimal Windows CI job (build + vet + test only) and add tooling incrementally.

5. **GoReleaser v2 syntax mismatch (MEDIUM probability, LOW severity).** The proposed `format_overrides` YAML may not match the exact GoReleaser v2 schema. Mitigation: run `goreleaser check` locally before merging.
