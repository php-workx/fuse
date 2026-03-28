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

**Done when:** `GOOS=windows go build ./...` succeeds, CI runs on `windows-latest`, GoReleaser produces `.exe`

## Phase 2: Shell Strategy

Decide how fuse executes and classifies commands on Windows. This is the most consequential design decision — everything downstream depends on it.

**Open questions:**
- PowerShell, CMD, or both?
- How does normalization work for PowerShell syntax? (cmdlets, pipelines, aliases)
- Does `fuse run` invoke `cmd.exe /c` or `powershell.exe -Command`?
- How do Claude Code and Codex invoke shells on Windows?
- What does the safe-command list look like on Windows?

**Done when:** Shell execution works, commands are classified through the pipeline, safe-command detection covers common Windows workflows.

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

**Known scope:**
- Replace `Setpgid` / `Pdeathsig` with job object assignment (child dies when parent dies)
- Replace `SIGTSTP`/`SIGWINCH`/`SIGTTOU` forwarding with `GenerateConsoleCtrlEvent`
- Replace `syscall.Kill(-pid, sig)` (process group signal) with job object termination
- `fuse proxy codex-shell` must work for Codex support on Windows

**Can run in parallel with:** Phase 3, Phase 5

**Done when:** `fuse run` and `fuse proxy codex-shell` handle child processes correctly on Windows, including cleanup on parent exit.

## Phase 5: Windows Security Intelligence

Teach fuse to understand Windows-specific threats. Without this, fuse classifies Windows commands but doesn't catch Windows-native attacks.

**Known scope:**
- PowerShell attack patterns: `Invoke-Expression`, `-EncodedCommand`, `DownloadString`, `Add-MpPreference` (Defender exclusions)
- CMD patterns: `certutil -decode`, `bitsadmin /transfer`, `reg add`, `schtasks /create`
- Windows path rules: `C:\Windows\System32`, `%APPDATA%`, credential stores, registry
- Script inspection for `.ps1`, `.bat`, `.cmd` files
- Existing Unix rules (reverse shells via `/dev/tcp`, `.bash_history`, `/etc/shadow`) are irrelevant on Windows

**Depends on:** Phase 2 (needs to know how commands are parsed)
**Can run in parallel with:** Phase 3, Phase 4

**Done when:** Fuse detects common Windows attack patterns with the same coverage quality as the Unix rule set.

## Dependency Graph

```
1 Foundation ──→ 2 Shell Strategy ──┬──→ 3 Terminal & Approval ───→ ┐
                                    ├──→ 4 Process Mgmt & Proxy ──→ ├─ User-ready
                                    └──→ 5 Security Intelligence ─→ ┘
```

Phases 3, 4, 5 are independent after Phase 2 completes.
