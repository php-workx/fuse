Reviewer Role: Kernel-level Systems Programmer & `UNIX` Shell Expert
Date: March 9, 2026
Target: `specs/technical.md` (`v3.0`), `specs/functional.md` (`v3.0`)

Executive Summary

The specification describes a robust "sudo-like" tool for agent mediation. However, the proposed process execution model in fuse run (Section `10.1`) contains critical defects related to `UNIX` `TTY` group management. As currently specified, fuse run will inevitably cause interactive child processes (shells, `REPLs`, editors) to hang immediately due to `SIGTTIN` signals.

Additionally, standard signal propagation for job control (`SIGTSTP`) and window resizing (`SIGWINCH`) is missing, which will degrade the user experience in a terminal environment.

Findings

1. Interactive Process Hang (`SIGTTIN`) via Process Groups
 * Severity: Critical
 * Title: Child processes will hang on input due to missing `TTY` foreground transfer.
 * Spec Section: `10.1` Execution model (fuse run)
 * What is wrong: The implementation sets Setpgid: true for the child process to create a new process group but fails to transfer terminal control to this new group using tcsetpgrp.
 * Why it matters: In `POSIX` systems, only the foreground process group can read from the controlling terminal. If a background process group tries to read, the kernel sends `SIGTTIN`, suspending the process.
 * Failure Scenario:
     1. User runs fuse run `--` python.
     2. fuse starts, becoming the foreground group.
     3. fuse spawns python in a new process group (via Setpgid: true).
     4. python tries to read the banner/prompt from stdin (the `TTY`).
     5. Kernel detects python is not in the foreground group.
     6. Kernel sends `SIGTTIN` to python.
     7. python suspends. fuse waits indefinitely. The terminal appears dead.
 * Recommended Fix:
     * fuse must call `unix.Tcsetpgrp` to promote the child process group to the foreground before cmd.Wait().
     * fuse must restore itself to the foreground (using Tcsetpgrp) after cmd.Wait() returns.
     * fuse must ignore `SIGTTOU` during these transitions to avoid stopping itself.

2. Broken Job Control (Ctrl+Z)
 * Severity: High
 * Title: `SIGTSTP` is not handled, creating orphaned terminal states.
 * Spec Section: `10.1` Execution model
 * What is wrong: The signal forwarding logic only lists `SIGINT`, `SIGTERM`, and `SIGHUP`. It omits `SIGTSTP` (Ctrl+Z).
 * Why it matters: Users habitually use Ctrl+Z to background long-running tasks.
 * Failure Scenario:
     1. User runs fuse run `--` `long_task`.
     2. User hits Ctrl+Z.
     3. The `TTY` driver sends `SIGTSTP` to the foreground group (fuse).
     4. fuse (default Go behavior) usually exits or stops without forwarding.
     5. If fuse stops, the child keeps running but loses `TTY` state.
     6. If fuse exits (unlikely for `TSTP` but possible if unhandled), the child becomes orphaned.
 * Recommended Fix:
     * Intercept `SIGTSTP`.
     * Forward `SIGTSTP` to the child process group.
     * Self-suspend fuse using syscall.Kill(os.Getpid(), `syscall.SIGSTOP`).
     * Handle `SIGCONT` to resume: send `SIGCONT` to the child process group and re-assert tcsetpgrp if necessary.

3. Missing Window Resize Propagation
 * Severity: Medium
 * Title: `SIGWINCH` is not forwarded to the child.
 * Spec Section: `10.1` Execution model
 * What is wrong: `SIGWINCH` is absent from the signal forwarding list.
 * Why it matters: Interactive `CLI` tools (vim, htop, less, `TUI` dashboards) rely on this signal to redraw when the terminal is resized.
 * Failure Scenario:
     1. User runs fuse run `--` top.
     2. User resizes the terminal window.
     3. fuse receives `SIGWINCH` (or ignores it).
     4. Child top never receives it and continues rendering at the old size, causing visual corruption.
 * Recommended Fix: Add `syscall.SIGWINCH` to the `signal.Notify` list and forward it to the child process group.

4. Incorrect Exit Code for Signal Termination
 * Severity: Medium
 * Title: Signal termination returns -1 instead of 128+n.
 * Spec Section: `10.1` Execution model
 * What is wrong: The code snippet return exitErr.ExitCode(), nil relies on Go's `ExitCode()` behavior, which returns -1 if the process was terminated by a signal.
 * Why it matters: `POSIX` shells expect a process killed by signal N to return 128 + N. Scripts checking $? (`e.g`., checking for code 130 for Ctrl+C interruption) will fail if they see 255 (unsigned -1).
 * Recommended Fix:

1 if status, ok := exitErr.Sys().(syscall.WaitStatus); ok {
2 if status.Signaled() {
3 return 128 + `int(status.Signal()`), nil
4 }
5 }
6 return exitErr.ExitCode(), nil

5. Stdio Buffering Risks in `MCP` Proxy
 * Severity: Medium
 * Title: Potential Deadlock on Large `MCP` Payloads.
 * Spec Section: `11.1` Proxy architecture
 * What is wrong: The spec implies parsing tools/call `JSON` payloads to classify them. It implies full message buffering.
 * Why it matters: There is no explicitly defined max size for `MCP` messages. If a tool call includes a 100MB string argument, fuse attempting to read/buffer this before forwarding could lead to high memory usage or deadlocks if the pipes fill up (standard pipe buffer is 64KB).
 * Failure Scenario: An agent sends a massive text update via `MCP`. fuse tries to read it all into memory for inspection.
 * Recommended Fix: Enforce a strict limit on `MCP` message size (`e.g`., matching the 64KB limit for shell commands, or slightly higher like 1MB) and fail-close if exceeded, or implement a streaming parser that does not require buffering the full payload if possible (though `JSON` verification requires full validity check).

Open Questions

1. `TTY` Race Conditions: In hook mode, fuse (a child of the agent) opens `/dev/tty`. Does the agent (which owns the `PTY` master) pause its own rendering while waiting for the hook? If not, the `TUI` prompt will fight with the agent's output for cursor position.
2. isatty checks: Does fuse verify that stdin is actually a `TTY` before attempting to use it as one for tcsetpgrp operations?

Suggested Additional Tests

1. Interactive Smoke Test: Run fuse run `--` python and verify the `REPL` accepts input. (Will fail currently).
2. Resize Test: Run fuse run `--` top, resize window, verify layout updates.
3. Job Control Test: Run fuse run `--` sleep 100, press Ctrl+Z, verify jobs in parent shell shows stopped job, fg resumes it.
4. Signal Exit Code: Run fuse run `--` sleep 100, kill -9 the sleep process from another terminal. Verify
`echo $? prints 137.`

Overall Risk Assessment

High. While the rule engine and classification logic (Section 5 & 6) are well-thought-out, the process execution wrapper (Section 10) uses a naive implementation that ignores standard `POSIX` terminal handling rules. This will make fuse run unusable for any interactive command, which is a core use case. Fixing the `TTY` group handling is a prerequisite for a working prototype.


##########################


1. Command Injection via Unsafe Argument Reassembly
   * Severity: Critical (Security Bypass)
   * Spec Section: `10.1` Execution model & `4.2` fuse run
   * What is wrong: The spec says "Parse command from argv" but does not define how fuse reconstructs the command string from the argument list to pass to `/bin/sh` -c.
   * Why it matters: Shells consume quotes. If the user runs fuse run `--` git commit -m "fix; rm -rf /", fuse receives the arguments as ["git", "commit", "-m", "fix; rm -rf /"].
       * If fuse simply joins these with spaces (a common Go mistake: strings.Join(args, " ")), the resulting string passed to sh -c is git commit -m fix; rm -rf /.
       * The shell will execute git commit -m fix and then execute rm -rf /.
   * Concrete Failure Scenario:
       1. User: fuse run `--` echo "Hello; rm -rf /"
       2. fuse sees arg: Hello; rm -rf / (quotes are gone).
       3. fuse constructs: echo Hello; rm -rf /
       4. fuse classifies: echo -> `SAFE`.
       5. fuse executes: `/bin/sh` -c "echo Hello; rm -rf /"
       6. Disaster.
   * Recommended Fix:
       * fuse must quote every argument when reconstructing the command string for sh -c. Use printf %q semantics or a Go library like shellescape.Quote() for every element of argv.
       * Alternatively, bypass sh -c and use exec.Command(args[0], args[1:]...) directly if shell features (pipes, redirects) aren't strictly required for the fuse run wrapper mode (though the spec implies they might be). Since the spec demands `/bin/sh` -c to support the user's intent, escaping is mandatory.

2. Environment Variable Poisoning (Inherited Context)
 * Severity: High (Security Bypass)
 * Spec Section: `10.1` Execution model (`cmd.Env` = os.Environ()) & `5.3` Normalization
 * What is wrong: The classification engine checks for dangerous environment variables in the command string (`e.g`., PATH=... cmd), but fuse run blindly inherits the current process environment (os.Environ()) and passes it to the child.
 * Why it matters: An attacker (or a compromised agent script) can export a dangerous variable before calling fuse. fuse sees a clean command string, but the process executes in a poisoned environment.
 * Concrete Failure Scenario:
     1. User/Agent: export `PATH=/tmp/malware:$PATH`
     2. User/Agent: fuse run `--` ls
     3. fuse classifies ls -> `SAFE`.
     4. fuse executes `/bin/sh` -c ls with the inherited dirty `PATH`.
     5. Shell resolves ls to `/tmp/malware/ls`.
     6. Malware executes.
 * Recommended Fix:
     * fuse must inspect os.Environ() during the classification phase, not just the command string.
     * Or, fuse must sanitize the environment it passes to the child (allowlist `TERM`, `SSH_*`, etc., and strict-check `PATH`, `LD_*`, `PYTHONPATH`). The spec's default `inherit_all` is insecure for a security tool.

3. `TTY` State Corruption on Timeout/Signal
 * Severity: High (UX/Stability)
 * Spec Section: `8.3` Input handling & `18.3` Crash resilience
 * What is wrong: fuse hook evaluate puts the `TTY` in raw mode. Section `18.3` states that Claude Code's timeout "kills the process" (likely `SIGTERM` or `SIGKILL`) if it takes too long.
 * Why it matters: If fuse is killed while in raw mode, the terminal attributes are not restored. The user is returned to a terminal that doesn't echo characters, process newlines correctly, or handle signals (Ctrl+C). This "breaks" the terminal, requiring the user to blindly type reset.
 * Concrete Failure Scenario:
     1. Agent triggers fuse hook.
     2. fuse sets `TTY` to raw mode and waits for user input.
     3. User is away or slow (30s timeout elapses).
     4. Agent kills fuse.
     5. fuse dies immediately.
     6. User returns to a broken terminal cursor and invisible text.
 * Recommended Fix:
     * Implement a signal handler in hook mode to catch SIGTERM/SIGINT and restore `TTY` state before exiting.
     * Use defer to ensure restoration on panic.
     * (Note: `SIGKILL` cannot be caught; this is an inherent risk of the agent-hook architecture, but catching `SIGTERM` covers the timeout case).

4. Orphaned Process Cleanup (`Double-Fork/Death`)
 * Severity: Medium
 * Spec Section: `10.1` Execution model
 * What is wrong: fuse creates a new process group for the child but doesn't guarantee the child dies if fuse dies (`e.g`., via kill -9).
 * Why it matters: In a "sudo-like" wrapper, if the parent dies, the child should not continue running detached. This creates "zombie" flows where the agent thinks the command failed (pipe closed), but the destructive command (`e.g`., a `DB` migration) keeps running in the background.
 * Recommended Fix:
     * Linux: Use `syscall.Prctl(syscall.PR_SET_PDEATHSIG`, `syscall.SIGTERM`) on the child process to ensure it receives a signal when fuse exits.
     * `macOS`: Use kqueue with `EVFILT_PROC` / `NOTE_EXIT` to monitor the parent, or accept that this is harder on `macOS` (best effort via signal forwarding).
