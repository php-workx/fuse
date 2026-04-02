# Windows Support Plan

Five work packages toward full Windows support. Nothing is user-ready until all five are complete.

## Phase 1: Foundation

Make the codebase compile on Windows. Produce a `.exe`. No new functionality — stubs and build-tag refactors only.

**Known scope:**
- 8 of 15 packages already compile (`core`, `policy`, `config`, `db`, `inspect`, `judge`, `events`, `sanitize`)
- All failures cascade from `approve/prompt.go` importing `golang.org/x/sys/unix` without build tags
- 10 new `_windows.go` files needed (mostly trivial stubs)
- 3 existing files need Unix code extracted behind `//go:build unix`
- GoReleaser, CI, and justfile need Windows targets added
- Research complete: `.agents/research/2026-03-28-windows-compilation-blockers.md`

**Done when:**
- `GOOS=windows go build ./...` succeeds
- CI cross-compiles and vets with `GOOS=windows` on `ubuntu-latest` (chosen over `windows-latest` for cost and simplicity; runtime behaviour on real Windows is not tested in CI at this phase)
- GoReleaser Windows `.exe` deferred pending code signing resolution (security review required before distributing unsigned Windows binaries)

## Phase 2: Shell Strategy

Decide how fuse executes and classifies commands on Windows. This is the most consequential design decision — everything downstream depends on it.

**Resolved (implemented in PR #10):**
- Both PowerShell and CMD are supported. `DetectShellType` heuristic classifies commands; PowerShell is the default for ambiguous input on Windows.
- Normalization extracts inner commands from `powershell.exe -Command ...` and `cmd.exe /c ...` wrappers, resolves PowerShell aliases (scoped to wrapper context), and handles backslash paths.
- `fuse run` dispatches to `powershell.exe -NoProfile -NonInteractive -Command` or `cmd.exe /c` based on detected shell type.
- Safe-command list includes ~50 PowerShell cmdlets (Get-*, Test-*, Format-*, etc.) and ~20 CMD builtins (dir, type, findstr, etc.).

**Done when:** ~~Shell execution works, commands are classified through the pipeline, safe-command detection covers common Windows workflows.~~ **DONE** — implemented and merged.

## Phase 3: Terminal & Approval

Replace Unix TTY prompts with Windows Console API. This is what makes fuse a firewall instead of a logger.

**Known scope:**
- Replace `/dev/tty` with `CONIN$`/`CONOUT$` console handles
- Replace `unix.IoctlGetTermios`/`IoctlSetTermios` with `SetConsoleMode`/`GetConsoleMode`
- Anti-spoofing property must be preserved (prompt reads from console directly, not stdin)

**Can run in parallel with:** Phase 4, Phase 5

**Done when:** `fuse run <cmd>` prompts for approval on Windows and the prompt cannot be spoofed via piped input.

## Phase 4: Process Management & Proxy

Replace Unix process groups with Windows job objects. Enables `fuse run` robustness and `fuse proxy codex-shell` for Codex support.

**Resolved (implemented on `feat/windows-terminal-approval`):**
- `jobObject` type wraps Windows Job Objects: create, assign process, terminate tree, close.
- `JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE` ensures all children die when fuse exits (replaces `Pdeathsig`).
- `CREATE_NEW_PROCESS_GROUP` in `platformSysProcAttr()` gives each child its own console group (replaces `Setpgid`).
- `cmd.Cancel` calls `TerminateJobObject` to kill the entire process tree on timeout (replaces `Kill(-pid, SIGKILL)`).
- `GenerateConsoleCtrlEvent(CTRL_BREAK_EVENT, childPID)` forwards interrupt to child group (replaces `syscall.Kill(-pid, sig)`).
- `SIGTSTP`/`SIGWINCH`/`SIGHUP` have no Windows equivalents — not forwarded (correct behavior).
- `ForegroundChildProcessGroupIfTTY` remains a no-op (Windows has no foreground process group transfer).
- `fuse doctor` job object check verifies creation and assignment on Windows.
- Both `executeShellCommand` (interactive) and `executeCapturedShellCommandWithStdin` (codex-shell) use job objects.

**Done when:** ~~`fuse run` and `fuse proxy codex-shell` handle child processes correctly on Windows, including cleanup on parent exit.~~ **DONE** — implemented.

## Phase 5: Windows Security Intelligence

Teach fuse to understand Windows-specific threats. Without this, fuse classifies Windows commands but doesn't catch Windows-native attacks.

**Known scope:**
- PowerShell attack patterns: `Invoke-Expression`, `DownloadString`, `Add-MpPreference` (Defender exclusions), `New-Object Net.WebClient`, `IEX`, `.NET` method invocations
- `-EncodedCommand` is already blocked unconditionally in Phase 2 hardcoded rules (base64 payload hides target). Phase 5 may add base64 decode + inspect for audit/reporting, but the block itself is in place.
- CMD patterns: `certutil -decode`, `bitsadmin /transfer`, `reg add`, `schtasks /create`
- LOLBins: `mshta`, `regsvr32`, `rundll32`, `cmstp`, `msiexec` with suspicious arguments
- Windows path rules: `C:\Windows\System32`, `%APPDATA%`, credential stores, registry
- Script inspection for `.ps1`, `.bat`, `.cmd` files
- Existing Unix rules (reverse shells via `/dev/tcp`, `.bash_history`, `/etc/shadow`) are irrelevant on Windows

**Depends on:** Phase 2 (needs to know how commands are parsed)
**Can run in parallel with:** Phase 3, Phase 4

**Done when:** ~~Fuse detects common Windows attack patterns with the same coverage quality as the Unix rule set.~~ **DONE** — Windows hardcoded protections, builtin download/LOLBin/persistence/security rules, PowerShell/CMD alias and safe-command coverage, and `.ps1` / `.bat` / `.cmd` inspection are all implemented and verified.

**Recommended `tag_overrides` for Windows-heavy development workflows:**
- `tag_overrides` accepts `enabled`, `dryrun`, or `disabled` per builtin tag.
- Prefer `dryrun` for noisy-but-legitimate developer activity and keep execution-oriented tags enforced.
- Example:

```yaml
tag_overrides:
  windows:download: dryrun
  windows:registry: dryrun
  windows:lolbin: enabled
  windows:persistence: enabled
```

**Accepted scanner limitations:**
- PowerShell scanning is line-oriented; here-strings, splatting, and commands split across multiple lines are not reconstructed before matching.
- Batch scanning is line-oriented; commands continued with `^` across lines are not reconstructed before matching.

## Dependency Graph

```text
1 Foundation ──→ 2 Shell Strategy ──┬──→ 3 Terminal & Approval ───→ ┐
                                    ├──→ 4 Process Mgmt & Proxy ──→ ├─ User-ready
                                    └──→ 5 Security Intelligence ─→ ┘
```

Phases 3, 4, 5 are independent after Phase 2 completes.
