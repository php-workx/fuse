---
id: fus-r7km
status: closed
deps: []
links: []
created: 2026-03-28T18:00:00Z
type: epic
priority: 1
assignee: ""
tags: [windows, phase-3]
---
# Phase 3: Windows Terminal & Approval

Implement interactive approval prompts on Windows via Console API. Replace Phase 1 stubs with real CONIN$/CONOUT$ handle management, raw mode via GetConsoleMode/SetConsoleMode, and keystroke reading via WaitForSingleObject. Also implement doctor diagnostics and terminal width detection.

Source: .agents/plans/2026-03-28-windows-terminal-approval.md
Research: .agents/research/2026-03-28-windows-terminal-approval.md

## Acceptance Criteria

1. `PromptUser()` on Windows opens CONIN$/CONOUT$, enters raw mode, reads keystrokes, shows approval/scope UI
2. Anti-spoofing preserved: piped stdin cannot auto-approve commands
3. Console mode always restored on exit, panic, signal, context cancellation
4. `fuse doctor --security` reports PASS for console access and raw mode on Windows
5. `terminalWidth()` returns actual console width on Windows
6. All existing Unix tests pass unchanged
7. `GOOS=windows go build ./...` and `GOOS=windows go vet ./...` succeed

## Cross-Cutting Constraints

These apply to ALL child tickets:

**Always:**
- All existing Unix tests must continue to pass unchanged — `just test` green before and after
- Anti-spoofing: open CONIN$/CONOUT$ directly, never os.Stdin/os.Stdout
- Fail-safe: if console unavailable, return errNonInteractive (DB poll fallback → blocked if no TUI)
- Console mode must always be restored on exit, panic, signal, context cancellation
- `GOOS=windows go build ./...` and `GOOS=windows go vet ./...` must succeed after every issue
- Prompt interface unchanged: `PromptUser(ctx, command, reason, hookMode, nonInteractive) (bool, string, error)`
- No new dependencies — `golang.org/x/sys/windows` is already available

**Ask First:**
- Whether to support ANSI colors on pre-Windows-10 (plain text fallback adds ~20 lines)
- Whether `fuse doctor --security` should FAIL (not WARN) when console unavailable on Windows

**Never:**
- No process group / job object work (Phase 4)
- No signal forwarding to child processes (Phase 4)
- No changes to runner_tty_windows.go or runner_exec_windows.go (Phase 4)
- No changes to manager.go, hmac.go, or any platform-agnostic code
- No new dependencies

## Verification (after all children closed)

1. Cross-compile: `GOOS=windows go build ./...`
2. Vet: `GOOS=windows go vet ./...`
3. Unix regression: `just test`
4. Shared tests: `go test ./internal/approve/ -run "TestSanitizePrompt|TestGetContextVars" -v`
5. Manual on Windows:
   ```bash
   go build -o bin/fuse.exe ./cmd/fuse/
   bin/fuse.exe run "echo hello"              # Should show approval prompt
   echo "a" | bin/fuse.exe run "echo hello"   # Must NOT auto-approve
   bin/fuse.exe doctor --security             # Console checks should show PASS
   ```

## Post-Merge Cleanup

- `goimports -w internal/approve/ internal/cli/`
- `GOOS=windows go build ./...` — full cross-compile check
- `just test` — Unix regression check
- Update specs/windows-support-plan.md Phase 3 section with "Done" status

## Notes

**2026-03-31T06:20:38Z**

Closed: implemented in Phase 3/4 commits on feat/windows-terminal-approval branch.
