# Phase 1 Windows Foundation -- Security Engineer Review

**Reviewer:** Security engineer (Windows-primary, evaluating for team adoption)
**Date:** 2026-03-28
**Documents reviewed:**
- `.agents/plans/2026-03-28-windows-foundation.md` (Phase 1 implementation plan)
- `specs/windows-support-plan.md` (overall 5-phase roadmap)
- Source files: `approve/prompt.go`, `approve/manager.go`, `adapters/hook.go`, `adapters/runner.go`, `adapters/codexshell.go`, `core/classify.go`, `policy/builtins_security.go`, `policy/hardcoded.go`, `core/normalize.go`, `core/compound.go`, `core/safecmds.go`

---

## Verdict: CONDITIONAL PASS

Phase 1 is mechanically sound as a compilation milestone. The fail-closed behavior is correctly preserved -- I traced the call paths and confirmed that stubbed Windows code results in BLOCKED, not ALLOW. However, Phase 1 produces a `.exe` that ships with zero Windows-specific threat coverage, a shell parser that does not understand PowerShell, and no code signing. This creates a window (Phases 1 through 4) where the tool exists on Windows but cannot meaningfully protect against the attacks that actually happen on Windows. The plan acknowledges this, but the gap must be communicated with more force than a line in CLAUDE.md.

Conditions for passing:
1. Findings 1 and 2 below must be addressed before shipping the `.exe`.
2. Finding 3 must have a concrete plan before the Phase 1 PR merges.
3. The README or installation docs must include the disclaimer described in Finding 4.

---

## Findings

### 1. BLOCKER -- The shell parser is Bash-only and this is not documented as a limitation

The entire classification pipeline depends on `mvdan.cc/sh/v3/syntax` configured with `syntax.Variant(syntax.LangBash)`. This parser is used in three critical locations:

- `core/compound.go:21` -- `SplitCompoundCommand` parses with `LangBash`
- `core/normalize.go:814` and `:847` -- classification normalization uses `LangBash`

On Windows, Claude Code and Codex will likely invoke commands through `cmd.exe` or PowerShell. When a PowerShell command like `Get-ChildItem -Recurse | Remove-Item -Force` is fed into a Bash parser, it will either fail to parse (triggering the fail-closed APPROVAL path, which is safe but noisy) or parse incorrectly (the more dangerous case -- imagine `Invoke-WebRequest` being tokenized by a Bash parser and not matching any rules).

The Phase 1 plan states "fuse hook evaluate -- full classification pipeline" under "What Phase 1 Delivers to Windows Users." This is technically true in that the code runs, but the classification output is unreliable for non-Bash syntax. The plan must explicitly document that **classification accuracy is only valid for Bash-syntax commands on Windows** (e.g., commands invoked through Git Bash or WSL). PowerShell and CMD commands will either fail-closed or produce meaningless results.

**Recommendation:** Add a visible warning to both `fuse doctor` output and `fuse hook evaluate` stderr output on Windows stating that the classification pipeline is designed for Bash syntax. Phase 2 is where this gets addressed -- that is fine -- but Phase 1 users should not assume coverage they do not have.

### 2. BLOCKER -- Code signing and SmartScreen are unresolved prerequisites for distribution

The plan lists code signing under "Ask First" but then proceeds to add Windows to GoReleaser and CI. This means Phase 1 will produce an unsigned `.exe` in GitHub Releases.

On a typical enterprise Windows workstation:
- Windows Defender SmartScreen will flag the binary with "Windows protected your PC" on first run.
- Some organizations have AppLocker or WDAC policies that block unsigned executables entirely.
- Even without AppLocker, the SmartScreen warning requires clicking "More info" then "Run anyway" -- a step that many non-technical users will refuse to take for a security tool they did not choose to install.

The irony of a security tool triggering a security warning is not lost on anyone. This is a significant adoption barrier.

**Recommendation:** Do not add the Windows `.exe` to public GitHub Releases until at minimum one of these is resolved: (a) the binary is signed with an EV code signing certificate, (b) distribution is through a package manager that handles trust (Scoop bucket, winget manifest, or Chocolatey package), or (c) the release notes include a clear SHA256 verification workflow. Option (c) is the minimum viable approach for Phase 1. Option (a) should be on the Phase 2 timeline. The `.exe` can still be built in CI for testing -- just do not publish it as a release artifact until the trust story is resolved.

### 3. WARN -- The safe-command list is entirely Unix-centric

`core/safecmds.go` contains the `unconditionalSafe` map with commands like `ls`, `cat`, `grep`, `awk`, `sed`, `du`, `df`, `mount`, `lsof`, `lsblk`, etc. None of these exist natively on Windows (they exist in Git Bash and WSL, but not in a PowerShell or CMD context).

This has two consequences on Windows:
- **False negatives:** Common safe Windows commands (`dir`, `type`, `findstr`, `Get-Content`, `Get-ChildItem`, `Test-Path`) are not recognized as safe. They will fall through to policy/builtin evaluation and produce unnecessary CAUTION or APPROVAL decisions.
- **False positives (minor risk):** If someone installs Unix tools on Windows (GnuWin32, MSYS2), the safe-command list will work, but the semantics may differ (e.g., `mount` on Windows has very different implications than on Linux).

This is acceptable for Phase 1 given the "more restrictive, not less" framing, but it will make the tool annoying to use on Windows in hook mode. Every `dir` command will be classified as unknown, which may train users to ignore fuse output.

**Recommendation:** Phase 2 must include a Windows-specific safe-command list. The Phase 1 plan should note this as a known UX degradation with a cross-reference to Phase 2.

### 4. WARN -- No documentation for the "security gap window" between Phase 1 and Phase 5

The dependency graph shows that real Windows security intelligence arrives in Phase 5, which depends on Phase 2. The plan correctly identifies that Phase 1 is "more restrictive than Unix" -- but this framing obscures the actual situation.

Between Phase 1 and Phase 5, fuse on Windows will:
- Not detect PowerShell `-EncodedCommand` obfuscation
- Not detect `Invoke-Expression` or `Invoke-WebRequest` abuse
- Not detect registry persistence via `reg add ... /v ... /t REG_SZ /d ...`
- Not detect scheduled task creation via `schtasks /create`
- Not detect Defender exclusion tampering via `Add-MpPreference -ExclusionPath`
- Not detect LOLBin abuse (`certutil -decode`, `bitsadmin /transfer`, `mshta`, `regsvr32`)
- Not detect credential access via `cmdkey`, `vaultcmd`, or `dpapi` tools

The "more restrictive" framing is only true for commands that the Bash parser can parse and that match existing rules. For Windows-native attack patterns, fuse is not restrictive -- it is blind.

**Recommendation:** The Phase 1 release notes and `fuse doctor` output on Windows must include a clear statement: "Windows security rules are not yet implemented. Fuse classifies commands using Unix-oriented rules and a Bash syntax parser. Windows-native attack patterns (PowerShell, CMD, registry, scheduled tasks) are not detected. Full Windows threat coverage is planned for Phase 5." This is honest and sets the right expectations.

### 5. WARN -- `fuse install claude` claims to work on Windows but hook invocation is untested

The plan lists `fuse install claude` under "Works" on Windows. This command writes to `~/.claude/settings.json`. However:
- The hook command is `fuse hook evaluate`, which reads JSON from stdin and writes to stderr. On Windows, this should work if Claude Code invokes hooks the same way.
- But the plan contains no information about **how Claude Code actually invokes hooks on Windows**. Does it use `cmd.exe /c fuse hook evaluate`? Does it use `CreateProcess` directly? Does the stdin/stdout/stderr piping work the same way?
- The `rejectSymlinkedClaudeSettingsPath` function in `cli/install.go` walks filesystem paths looking for symlinks. On Windows, the path is `%USERPROFILE%\.claude\settings.json` -- the code uses `os.UserHomeDir()` which should work, but this is untested.

**Recommendation:** Before claiming `fuse install claude` works on Windows, add an integration test that runs on `windows-latest` in CI: install, verify settings.json contents, then run a hook evaluation with piped JSON input.

### 6. WARN -- `trustedPath()` on Windows returns empty string

The proposed `runner_windows.go` stub returns `""` for `trustedPath()`:

```go
func trustedPath() string {
    return "" // Windows PATH is managed by the OS; no hardcoded safe PATH.
}
```

This means `BuildChildEnv` in `runner.go` will set `PATH=""` in the child environment, which effectively means no command can be found by the OS. While `fuse run` is stubbed to return an error on Windows (so this code path should not be reached), the function is also called from `executeCapturedShellCommand` which is used by the codex-shell adapter. If someone manages to reach this code path (e.g., through future changes), the empty PATH would break command resolution silently.

**Recommendation:** Return a sensible Windows default: `C:\Windows\System32;C:\Windows;C:\Windows\System32\Wbem`. Even if the code path is currently unreachable, defense in depth says the stub should not produce a broken environment.

### 7. WARN -- The cross-compilation gate test is a good idea but insufficient

`TestCrossBuild_WindowsCompiles` runs `go build ./...` with `GOOS=windows`. This catches compilation failures but does not catch:
- Runtime panics from nil pointer dereferences in Windows stubs
- Incorrect behavior from platform-specific path handling (e.g., `filepath.Separator` is `\` on Windows)
- Unicode path issues (Windows paths can contain non-ASCII characters)

**Recommendation:** In addition to the cross-compilation gate, add a `_test.go` file with `//go:build windows` that runs on the `windows-latest` CI runner. This should exercise: (a) `PromptUser` returns `errNonInteractive`, (b) `executeShellCommand` returns the correct error message, (c) `ForegroundChildProcessGroupIfTTY` returns `(nil, nil)`, (d) `BuildChildEnv` with a Windows-like environment produces valid output.

### 8. NOTE -- Self-protection rules use Unix paths exclusively

The hardcoded BLOCKED rules in `policy/hardcoded.go` protect paths like `.claude/settings.json` and `~/.fuse/`. On Windows, these paths use backslashes: `.claude\settings.json`. The regex patterns use forward slashes only:

```go
regexp.MustCompile(`(>|>>|tee|cp|mv|sed\s+-i|cat\s+.*>)\s*.*\.claude/settings\.json`)
```

A command like `type .claude\settings.json > malicious.json` would not match. However, since the Bash parser is being used and Windows-native commands are not parsed correctly anyway, this is a theoretical issue for Phase 1. It becomes a real issue in Phase 2 when CMD/PowerShell commands are supported.

**Recommendation:** Track this as a Phase 2 requirement. Self-protection rules must be duplicated or generalized to handle both path separator conventions.

### 9. NOTE -- The `catastrophicPaths` map is Unix-only

`policy/hardcoded.go` defines catastrophic paths as `/`, `/etc`, `/usr`, `/var`, etc. On Windows, the catastrophic paths would be `C:\`, `C:\Windows`, `C:\Windows\System32`, `C:\Program Files`, `C:\Users`. Since `fuse run` is stubbed, this is not an active risk in Phase 1, but it is another item for the Phase 2/5 checklist.

**Recommendation:** Add a comment in the code and a tracking item for Phase 5.

### 10. NOTE -- WSL is not mentioned anywhere in the plan

WSL (Windows Subsystem for Linux) is a significant part of the Windows developer ecosystem. When Claude Code runs inside WSL, it is effectively running on Linux -- fuse should work there without any changes. But when Claude Code runs on native Windows and the user has WSL available, the agent could execute `wsl.exe -e bash -c "<command>"` to escape to a Linux environment where different rules apply.

The `wsl.exe` command should eventually be treated as a wrapper binary (like `sudo`) that requires inspection of the inner command. This is not a Phase 1 concern, but it should be documented as a threat vector for Phase 5.

**Recommendation:** Add `wsl.exe` / `wsl` to the Phase 5 threat model as a wrapper/escape vector.

### 11. NOTE -- Approval timeout UX on Windows will be confusing

When a command requires APPROVAL on Windows Phase 1, the flow is:
1. `PromptUser` returns `errNonInteractive` immediately
2. `handlePromptError` waits for DB poll (up to 2 minutes or context deadline)
3. If no external approval: returns `DecisionBlocked`

During step 2, the agent receives `pendingApprovalMsg` which says "wait 30-60 seconds, then retry." But there is no `fuse monitor` TUI on Windows yet (Phase 3), so the user has no way to approve. The agent will retry, hit the same wall, and retry again -- potentially for several cycles before giving up.

**Recommendation:** The `pendingApprovalMsg` text should be platform-aware. On Windows Phase 1, it should say something like: "This command requires approval but the approval terminal is not yet available on Windows. The command has been blocked. On Unix, this would prompt the user for approval."

---

## Gaps in Protection

### Gaps that are acceptable at Phase 1

| Gap | Why it is acceptable |
|-----|---------------------|
| No PowerShell/CMD rule coverage | Phase 5 scope. Fail-closed behavior means unrecognized patterns are not silently allowed. |
| No Windows safe-command list | Results in over-classification (CAUTION for safe commands), not under-classification. Annoying but not dangerous. |
| No approval prompt | Returns BLOCKED. More restrictive than Unix, not less. |
| No `fuse run` execution | Stubbed with clear error. Users cannot accidentally execute through fuse. |
| No Windows path convention in rules | Bash parser cannot parse Windows-native commands anyway. Becomes relevant in Phase 2. |

### Gaps that need tracking and timelines

| Gap | When it must be addressed | Risk if delayed |
|-----|--------------------------|-----------------|
| PowerShell `-EncodedCommand` detection | Phase 5, but should be one of the first rules | An agent could base64-encode arbitrary commands to bypass all rule matching. This is the #1 PowerShell evasion technique. |
| `wsl.exe` as escape vector | Phase 5 | Agent can drop to Linux shell, bypassing Windows-specific rules (when they exist). |
| Self-protection for Windows paths | Phase 2 | Agent could modify `.claude\settings.json` using Windows-native tools once Phase 2 enables CMD/PowerShell. |
| Windows-specific catastrophic paths | Phase 5 | `Remove-Item -Recurse -Force C:\` would not be caught. |
| Code signing | Before public `.exe` distribution | Adoption blocker for enterprise teams. |

### Gaps that are NOT addressed in any phase

| Gap | Concern |
|-----|---------|
| Named pipe / COM object abuse | Not mentioned in Phase 5 scope. PowerShell can create COM objects (`New-Object -ComObject ...`) for privilege escalation and lateral movement. |
| WMI command execution | `Get-WmiObject`, `Invoke-WmiMethod`, `wmic` are not mentioned. These are common living-off-the-land techniques. |
| .NET method invocation via PowerShell | `[System.Net.WebClient]::new().DownloadString(...)` is functionally equivalent to `curl | bash` but uses .NET types, not commands. |
| Windows Event Log tampering | `wevtutil cl Security` clears the security event log. Not listed in Phase 5 scope. |
| DLL sideloading via PATH manipulation | Windows DLL search order is different from Unix library loading. `PATH=` manipulation on Windows can cause DLL sideloading. The `trustedPath` mechanism partially addresses this but is currently stubbed. |

---

## Adoption Risks

### Would I trust this tool to protect my team at Phase 1?

No. Phase 1 is explicitly a compilation milestone and I would not deploy it as a security control on Windows. It is useful as:
- A signal that Windows support is coming
- A way for early adopters to test the classification pipeline in Git Bash / WSL scenarios
- A foundation for the engineering team to build on

### What would need to be true before I recommend it?

1. **Phase 2 + Phase 3 complete:** The tool must understand Windows shell syntax and have a working approval prompt. Without these, it is a classification engine that cannot classify Windows commands and cannot prompt for human approval.

2. **Phase 5 delivers PowerShell rule parity:** The Unix rule set covers reverse shells, data exfiltration, credential access, persistence, obfuscation, and privilege escalation. The Windows rule set must cover equivalent threats in Windows-native tooling (PowerShell, CMD, WMI, COM, .NET).

3. **Code signing resolved:** The binary must not trigger SmartScreen warnings.

4. **At least one package manager distribution channel:** Scoop or winget. Telling engineers to download a `.exe` from GitHub is not how Windows security tools get adopted.

5. **`fuse doctor --security` works on Windows:** Must validate the actual security posture, not just skip all checks. If every check returns "SKIP," the doctor command provides a false sense that things were checked and passed.

### What would actively prevent adoption?

- **Agent retry loops from approval stubs.** If Claude Code retries APPROVAL-blocked commands 5+ times because the error message says "wait and retry" but there is nothing to wait for on Windows, users will uninstall fuse within the first hour.
- **Over-classification noise.** If every `dir`, `type`, and `findstr` command triggers CAUTION because it is not in the safe-command list, users will switch to dry-run mode permanently -- which defeats the purpose.
- **SmartScreen warnings.** A security tool that your OS warns you about is a hard sell, especially to team members who did not choose to install it.

---

## Summary

The Phase 1 engineering plan is well-structured. The fail-closed analysis is correct and thorough -- I independently verified the call path from `PromptUser` through `handlePromptError` to `DecisionBlocked`. The build-tag strategy is clean and the conformance checks are specific. The cross-compilation gate test is a good defensive measure.

The concerns are not about Phase 1's code quality but about what Phase 1's output represents to users. A `.exe` in GitHub Releases looks like a product. The plan must ensure that the gap between "it compiles" and "it protects" is communicated clearly at every touchpoint: installation, doctor output, hook error messages, and release notes.

Address the two blockers (shell parser limitation documentation, code signing / distribution strategy), and this plan is good to execute.
