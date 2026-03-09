# fuse — Technical Specification Review

Version: 1.1
Date: 2026-03-08
Reviewers: Agent Internals Engineer, Security Red Teamer, Go Systems Engineer
Status: **All findings addressed in technical.md v3.0 and functional.md v3.0**

---

## Overview

Three persona-based reviews were conducted against `specs/technical.md` (v2.0) and `specs/functional.md` (v2.0). This document collects all findings for triage and resolution.

**Totals:** 66 findings (10 CRITICAL, 22 HIGH, 22 MEDIUM, 12 LOW)
**Status:** All P0 and P1 findings resolved. P2/P3 findings addressed where straightforward.

---

## Reviewer 1: Agent Internals Engineer

Focus: Claude Code hook protocol correctness, Codex MCP integration, agent lifecycle assumptions.

### AI-1 [CRITICAL] Hook JSON schema is wrong (§3.1)

The spec uses `{"hooks": {"Bash": {"pre": [...]}}}` but the correct Claude Code schema is:
```json
{
  "hooks": {
    "PreToolUse": [
      {
        "matcher": "Bash",
        "hooks": [
          {
            "type": "command",
            "command": "fuse hook"
          }
        ]
      }
    ]
  }
}
```
**Fix:** Rewrite the entire §3.1 hook configuration example to match the actual Claude Code hooks schema.

### AI-2 [CRITICAL] hooks.json does not exist (§3.1)

The spec references `.claude/hooks.json` but hooks are configured in `settings.json` (either project-level `.claude/settings.json` or user-level `~/.claude/settings.json`).

**Fix:** Replace all references to `hooks.json` with `settings.json`. Update `fuse install` to write to the correct file.

### AI-3 [HIGH] Timeout units are wrong (§3.1)

The spec says `"timeout": 30000` implying milliseconds, but Claude Code hook timeouts are in seconds. 30000 seconds = 8+ hours.

**Fix:** Change to `"timeout": 30` (30 seconds).

### AI-4 [HIGH] stdin JSON schema not verified (§3.1)

The spec assumes the hook receives `{"tool_name": "Bash", "tool_input": {"command": "..."}}` on stdin, but does not cite the exact Claude Code hook input schema. If the schema differs, fuse silently misparses.

**Fix:** Verify against Claude Code documentation. Add schema validation with fail-closed on parse error.

### AI-5 [HIGH] Exit code semantics may be inverted (§3.1)

The spec says exit 0 = allow, exit 2 = block. Claude Code PreToolUse hooks use: exit 0 = allow, non-zero = block. Verify exit 2 specifically is handled correctly vs generic non-zero.

**Fix:** Verify against Claude Code docs. Use exit 1 for block if that is the standard convention.

### AI-6 [HIGH] stderr output format unspecified (§3.1)

The spec says fuse writes JSON to stderr for Claude Code to display, but does not specify the exact format Claude Code expects. If Claude Code expects plain text or a specific JSON schema, fuse's output may be silently dropped.

**Fix:** Verify Claude Code's stderr handling for hooks. Specify the exact output format.

### AI-7 [HIGH] `fuse install` modifies settings without conflict resolution (§4)

If the user already has hooks configured in `settings.json`, `fuse install` must merge rather than overwrite.

**Fix:** Implement JSON-merge logic that preserves existing hooks while adding fuse's hook entry.

### AI-8 [MEDIUM] No hook for MCP tool calls (§3.1)

The spec only hooks `Bash` tool calls. Claude Code also has `mcp__*` tool names for MCP tool calls. Without hooking these, MCP tool calls bypass fuse entirely in hook mode.

**Fix:** Add `PreToolUse` hook matchers for MCP tool patterns, or use a wildcard matcher with fuse handling the routing internally.

### AI-9 [MEDIUM] Codex integration not field-tested (§3.2)

The Codex shell MCP integration path assumes Codex supports `--shell-tool=none` and will register fuse as an MCP server. These are speculative features.

**Fix:** Mark as "pending Codex CLI support" and add a fallback integration path.

### AI-10 [MEDIUM] `fuse doctor` does not verify hook is actually active (§4)

`fuse doctor` checks config file existence but doesn't verify Claude Code actually loads and invokes the hook on each Bash call.

**Fix:** Add a self-test: `fuse doctor --live` that triggers a synthetic hook invocation and verifies the round-trip.

### AI-11 [MEDIUM] No versioning on hook protocol (§3.1)

If Claude Code changes its hook input schema, fuse breaks silently. No version negotiation exists.

**Fix:** Add a version field check in the stdin JSON. Log warnings for unrecognized schema versions. Fail-closed on unknown schemas.

### AI-12 [LOW] `fuse uninstall` cleanup is incomplete (§4)

The spec doesn't specify whether `fuse uninstall` removes the SQLite database, config files, and log files, or just the hook registration.

**Fix:** Specify exact cleanup scope. Default to removing hook registration only; `--purge` flag for full cleanup.

### AI-13 [LOW] No graceful degradation if bubbletea fails (§8.3)

If the TUI library crashes or `/dev/tty` behaves unexpectedly, the spec says auto-deny. But the error path is not specified in detail.

**Fix:** Add explicit panic recovery around TUI code. Log the error and exit with deny code.

---

## Reviewer 2: Security Red Teamer

Focus: Bypass vectors, TOCTOU attacks, normalization gaps, approval system attacks, self-protection.

### SR-1 [CRITICAL] Shell metacharacter bypass via command chaining (§5.2)

The classification pipeline does not split on `;`, `&&`, `||`, `|`. An agent can chain `echo safe; terraform destroy`.

**Fix:** Split compound commands before classification. Classify each sub-command. Most restrictive wins.

### SR-2 [CRITICAL] Newline injection in command string (§5.2, §3.1)

JSON `\n` in `tool_input.command` creates multi-line shell commands. Display normalization collapses whitespace but doesn't handle newlines as command separators.

**Fix:** Treat `\n` as a command separator. Split and classify each line independently.

### SR-3 [CRITICAL] TOCTOU gap in hook mode (§5.2, §10.1)

In hook mode, fuse classifies then exits. The agent can modify referenced files between classification and execution. File hash binding is useless because fuse is not the executor.

**Fix:** Document as fundamental hook-mode limitation. Recommend `fuse run` for stronger guarantees. Consider post-execution audit hook.

### SR-4 [CRITICAL] `fuse disable` allows agent to disable protection (§18.1)

`fuse disable` causes exit 0 on all subsequent calls. No detection rule prevents an agent from running this command.

**Fix:** Add `fuse disable`, `fuse uninstall`, `fuse enable` to hardcoded BLOCKED rules.

### SR-5 [CRITICAL] Policy file manipulation (§16.2, §6.1)

Agent can write to `~/.fuse/config/policy.yaml` to add `action: safe, command_regex: [".*"]` and use `disabled_builtins` to remove all protections.

**Fix:** Add hardcoded BLOCKED rules for writes to `~/.fuse/config/*`. Reject overly broad user rules on load.

### SR-6 [CRITICAL] Hooks configuration tampering (§3.1)

Agent can overwrite `.claude/settings.json` to remove the fuse hook entirely.

**Fix:** Add hardcoded BLOCKED rules for writes to `.claude/settings.json`, `~/.claude/settings.json`.

### SR-7 [HIGH] Symlink race on referenced files (§5.5, §7.1)

Agent creates symlink to benign file at classify time, retargets to destructive file before execution.

**Fix:** Resolve symlinks via `filepath.EvalSymlinks()` before inspection. Hash canonical path.

### SR-8 [HIGH] Environment variable injection bypasses classification (§5.3)

`PATH=/evil/dir terraform plan` classifies as `terraform plan` (SAFE) but runs attacker-controlled binary.

**Fix:** Flag `PATH=`, `LD_PRELOAD=`, `LD_LIBRARY_PATH=`, `PYTHONPATH=`, `NODE_PATH=` prefixes as APPROVAL.

### SR-9 [HIGH] Argument injection via flag variations (§5.3, §6.2)

Hardcoded `rm` regex only matches combined `-rf`. Split flags (`rm -r -f /`), long flags (`rm --recursive --force /`), and path-before-flags all bypass.

**Fix:** Expand regex to cover split flags, long-form flags, mixed flags. Consider proper flag parsing.

### SR-10 [HIGH] Unicode and encoding normalization gaps (§5.3)

Zero-width characters, fullwidth Latin, homoglyphs evade regex word boundary matching.

**Fix:** Strip non-ASCII control characters, apply NFKC normalization as Stage 0 in §5.2.

### SR-11 [HIGH] Wrapper stripping hides privilege escalation (§5.3)

`sudo git push --force` stripped to `git push --force` -> CAUTION (no prompt). sudo runs it as root.

**Fix:** If sudo/doas was stripped, escalate classification by one level (SAFE->CAUTION, CAUTION->APPROVAL).

### SR-12 [HIGH] `bash -c` inner command extraction is incomplete (§5.3)

Nested `bash -c`, ANSI-C quoting, command substitution inside `-c` strings all defeat simple regex extraction.

**Fix:** Use `go-shellwords` for proper extraction. Fail-closed (APPROVAL) when extraction fails. Handle nested cases recursively.

### SR-13 [HIGH] MCP tool name spoofing (§6.6)

Malicious MCP server registers `get_data_then_delete_all` — matches safe `^get_` prefix.

**Fix:** Check all patterns, not just first prefix match. Escalate if name matches both safe prefix and contains destructive verb.

### SR-14 [HIGH] MCP message injection from downstream (§11.2)

Malicious downstream server can inject fake `tools/list` responses or prompt-injection content.

**Fix:** Validate downstream responses. Restrict which tool names downstream can expose. Track in-flight request IDs.

### SR-15 [HIGH] SQLite direct manipulation for approval forgery (§9.2, §8.1)

Agent runs `sqlite3 ~/.fuse/state/fuse.db "INSERT INTO approvals ..."` to forge approvals.

**Fix:** Add `sqlite3` targeting fuse.db to BLOCKED rules. HMAC-sign approval records with per-install secret. Set DB to mode 0600.

### SR-16 [HIGH] Config file manipulation weakens inspection (§16.1)

Agent sets `max_inspect_file_bytes: 1` in config.yaml to defeat file inspection.

**Fix:** Protect config files. Enforce minimum values for security-critical settings.

### SR-17 [MEDIUM] Context sanitization ordering (§6.4)

If single-quote masking is applied before compound command splitting, quotes can hide split operators.

**Fix:** Specify exact order of operations. Split before sanitize.

### SR-18 [MEDIUM] Approval hash collision via null bytes (§8.1)

Null bytes in command strings create ambiguous hash input boundaries.

**Fix:** Strip null bytes before hashing. Use length-prefixed fields instead of delimiter-separated.

### SR-19 [MEDIUM] File inspection evasion via dynamic code generation (§7.3-7.5)

String concatenation, `importlib`, `base64` decode, computed `require()` evade regex scanners.

**Fix:** Add heuristic patterns for `importlib`, `__import__`, `getattr`, computed `require()`, `base64` decode + eval.

### SR-20 [MEDIUM] Oversized file inspection boundary (§7.1)

Destructive code placed after the `max_inspect_file_bytes` boundary evades scanning.

**Fix:** Strictly enforce: any truncated file MUST classify as APPROVAL regardless of signals found.

### SR-21 [MEDIUM] DoS via stdin flooding (§4.1)

10MB command string forces regex engine to process excessive input within the 30s timeout.

**Fix:** Add max command length check (64KB) early in pipeline. Exceed -> APPROVAL.

### SR-22 [MEDIUM] Race condition in approval consumption (§8.1, §9.2)

Two concurrent hook invocations can both read and consume the same unconsumed approval.

**Fix:** Use atomic SQL: `UPDATE ... WHERE consumed = 0 RETURNING id`. No rows returned = already consumed.

### SR-23 [MEDIUM] MCP proxy doesn't validate JSON-RPC message IDs (§11.2)

Malicious downstream sends forged response IDs causing response confusion.

**Fix:** Track in-flight request IDs per connection. Drop unsolicited responses.

### SR-24 [MEDIUM] `fuse run` uses relative `sh` path (§10.1)

`exec.Command("sh", ...)` resolves via potentially-poisoned PATH.

**Fix:** Use absolute path: `exec.Command("/bin/sh", "-c", command)`.

### SR-25 [MEDIUM] `/dev/tty` could be controlled by agent (§8.3)

If fuse runs inside agent-created `screen`/`expect` session, approvals are auto-granted.

**Fix:** Verify `/dev/tty` is a real terminal. Warn in `fuse doctor` if pty detected.

### SR-26 [LOW] RE2 guarantee must be enforced (§1.1)

If patterns are later ported using PCRE library, ReDoS becomes possible.

**Fix:** Add spec note: all regex must use Go `regexp` (RE2). Add compilation test.

### SR-27 [LOW] Sensitive data in event log (§9.2)

Commands containing inline passwords/API keys are stored in plaintext.

**Fix:** Add credential-pattern scrubbing to event logging.

### SR-28 [HIGH] Absolute path to binary bypasses all rules (§6.2, §6.3)

Rules use `\brm\s+`, `\bterraform\s+`, etc. `/usr/bin/rm -rf /`, `/usr/local/bin/terraform destroy` match no rule. Agents resolving full paths (e.g., after `which terraform`) is natural behavior.

**Fix:** During classification normalization, extract the basename of the first token (the executable). If the token contains `/`, strip the path prefix before applying rules.

### SR-29 [HIGH] `$HOME` / `$PWD` variable targets bypass rm rules (§6.2)

`rm -rf $HOME` bypasses the `[/~]` character class in the hardcoded BLOCKED rule. `$HOME` starts with `$`, not `/` or `~`. Agents frequently use `$HOME` instead of `~`.

**Fix:** Expand the target character class to include `$HOME`, `${HOME}`, `$PWD`, `${PWD}`. Better: resolve known shell variables before classification.

### SR-30 [HIGH] MCP argument content not classified (§6.6)

MCP classification only matches on tool name prefix. A tool named `execute_query` called with `{"query": "DROP DATABASE production"}` bypasses all rules. Generic tools with command/query arguments are common.

**Fix:** For MCP tools, also scan argument values against shell command and database rule patterns.

### SR-31 [HIGH] MCP fallback for unmatched tool names is unclear (§6.6)

Tools with prefixes not in the three regex groups (`cleanup_`, `nuke_`, `wipe_`, `reset_`, `flush_`) fall through to an undefined default — likely SAFE.

**Fix:** Expand destructive verb prefix list. Classify unmatched MCP tool names as CAUTION rather than SAFE.

### SR-32 [HIGH] Environment variables not shown in approval prompt (§8.2)

The approval prompt shows the command but not the active environment. `terraform destroy` looks identical whether `AWS_PROFILE=prod` or `AWS_PROFILE=staging`. User approves with no context.

**Fix:** Show relevant environment variables (AWS_PROFILE, AWS_REGION, TF_WORKSPACE, KUBECONFIG, etc.) in the approval prompt.

### SR-33 [HIGH] Many dangerous commands match no rule (§6.2, §6.3)

Commands classified SAFE by default that are destructive: `ssh prod 'rm -rf /data'` (inner command invisible), `mysql -e 'DROP DATABASE prod'` (single-quote masked), `redis-cli FLUSHALL`, `systemctl stop critical-service`, `pkill -9 production-app`, `kill -9 1`, `iptables -F`, PaaS CLIs (`heroku apps:destroy`, `flyctl destroy`).

**Fix:** Add rules for `ssh` (extract inner command), database CLIs, `systemctl stop/disable`, `kill/pkill`, `iptables -F`, PaaS CLIs. Address single-quote masking blind spot for SQL arguments.

### SR-34 [CRITICAL] Single-quote masking ordering ambiguity with bash -c (§5.3, §6.4)

Context sanitization masks single-quoted strings. `bash -c 'terraform destroy'` — if sanitization happens BEFORE `bash -c` inner extraction, the inner command becomes `[MASKED]` and is invisible to classification. The spec does not define whether extraction or sanitization happens first. This also affects `ssh server 'rm -rf /'` and `mysql -e 'DROP TABLE users'`.

**Fix:** Explicitly specify that inner command extraction from `bash -c`/`sh -c`/`ssh` happens BEFORE context sanitization.

### SR-35 [HIGH] exec/eval bypass in Python file inspection (§7.3)

`exec("import subprocess; subprocess.run(['rm', '-rf', '/data'])")` — the dangerous code is inside a string argument to `exec()`, not at the start of a line. Python scanner has no patterns for `exec` or `eval`.

**Fix:** Add `\bexec\s*\(` and `\beval\s*\(` as suspicious signals in the Python scanner.

### SR-36 [MEDIUM] CWD not in approval hash or prompt (§8.1, §8.2)

`rm -rf ./data` in `/tmp/test` vs `/home/user/production` — same hash, same prompt, very different effect.

**Fix:** Display cwd in the approval prompt. Optionally include in hash.

### SR-37 [MEDIUM] Pipe to python/node/ruby not detected (§5.4)

Suspicious patterns only detect pipe to `(ba)?sh`. `cat script.py | python3 -` bypasses detection.

**Fix:** Add `| python`, `| node`, `| ruby`, `| perl` to suspicious pipe patterns.

### SR-38 [MEDIUM] PATH manipulation as two-step attack (§5.3)

Agent runs `export PATH=/tmp/evil:$PATH` (classified SAFE), writes `/tmp/evil/ls` that runs `rm -rf /data`, then runs `ls` (classified SAFE).

**Fix:** Flag `export PATH=` modifications as CAUTION.

### SR-39 [MEDIUM] Detected-but-unsupported file types default wrong (§5.5, §7)

Ruby and Perl files are detected as referenced files (§5.5) but have no scanner (§7). If no scanner runs, the file may be classified SAFE despite unknown content.

**Fix:** For detected-but-unsupported file types, default to CAUTION.

### SR-40 [LOW] ANSI escape sequences break word boundaries (§5.3)

`\x1b[0mterraform destroy` — ANSI codes between characters break `\b` regex matching.

**Fix:** Strip ANSI escape sequences during display normalization.

---

## Reviewer 3: Go Systems Engineer

Focus: Performance, concurrency, resource management, error handling, Go idioms.

### GE-1 [HIGH] 50ms latency target is unrealistic (§1, Functional §17)

SQLite open + WAL setup + regex compilation + file I/O + hash computation in 50ms for cold start is ambitious. Pure-Go SQLite (`modernc.org/sqlite`) is ~3-5x slower than CGo SQLite.

**Fix:** Benchmark early. Consider: pre-compiled regex cache, connection pooling daemon, lazy SQLite open (skip if SAFE classification), or raise target to 100-150ms for cold start.

### GE-2 [HIGH] SQLite per-invocation overhead (§9.3)

Each hook invocation opens a new SQLite connection, sets WAL mode, creates tables if needed, queries/inserts, then closes. This is the dominant latency contributor.

**Fix:** Consider a long-running fuse daemon with Unix socket for hot-path queries. Alternatively, use a lightweight state file (JSON/gob) for the approval cache with SQLite only for event logging.

### GE-3 [HIGH] Signal handling gaps (§10.1)

`fuse run` subprocess management doesn't specify signal forwarding. If user sends SIGINT to fuse, the child process may be orphaned.

**Fix:** Forward SIGINT, SIGTERM, SIGHUP to child process group. Use `cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}` for process group management. Handle SIGCHLD for zombie prevention.

### GE-4 [HIGH] Error handling gaps in pseudocode (§10.1, §8.1)

Pseudocode uses bare `if err != nil` without specifying error wrapping, logging, or user-facing messages. Production Go code needs `fmt.Errorf("context: %w", err)` for debuggability.

**Fix:** Add error wrapping conventions to the spec. Specify which errors are user-facing vs internal.

### GE-5 [MEDIUM] Regex compilation should be package-level (§5, §7)

If regex patterns are compiled per-invocation (`regexp.MustCompile` in hot path), each hook call recompiles 60+ patterns.

**Fix:** Specify that all regex must be compiled at package init time as `var` declarations. Use `regexp.MustCompile` at package level only.

### GE-6 [MEDIUM] No context.Context propagation (§10.1)

The spec doesn't mention `context.Context` for timeouts and cancellation. In hook mode, fuse has a 30s timeout enforced by Claude Code, but fuse itself doesn't enforce any internal timeout.

**Fix:** Accept a `--timeout` flag. Use `context.WithTimeout` for the entire pipeline. Cancel all operations (SQLite queries, file reads, subprocess) on timeout. Exit with deny code.

### GE-7 [MEDIUM] WAL checkpoint strategy unspecified (§9.3)

WAL mode grows the `-wal` file indefinitely without checkpointing. With many events per session, the WAL file can grow large.

**Fix:** Run `PRAGMA wal_checkpoint(TRUNCATE)` after maintenance operations. Add checkpoint to `fuse maintenance` command.

### GE-8 [LOW] `modernc.org/sqlite` version pinning (§2.2)

The spec lists `modernc.org/sqlite` without version. This package has had breaking changes between versions.

**Fix:** Pin to a specific version in go.mod. Document minimum version requirements.

### GE-9 [LOW] Missing `defer` patterns in pseudocode (§9.3, §10.1)

Pseudocode opens files and DB connections without showing `defer Close()` patterns, inviting resource leaks.

**Fix:** Add `defer` to all resource acquisition in pseudocode.

### GE-10 [LOW] No structured logging specification (§13)

Error handling section mentions logging but doesn't specify a logging library or structured logging format.

**Fix:** Specify `log/slog` (stdlib since Go 1.21) with JSON output for machine-parseable logs.

### GE-11 [LOW] Test fixtures path not specified (§12)

40+ golden fixtures are listed but the directory structure for test data is not specified.

**Fix:** Specify `testdata/` directory convention per Go standards. Define fixture file format.

### GE-12 [LOW] No build tags or platform constraints (§2)

The spec targets macOS and Linux but doesn't mention build constraints for platform-specific code (e.g., `/dev/tty` handling, signal management).

**Fix:** Specify build tags for platform-specific files. Document Windows as explicitly unsupported.

---

## Cross-Cutting Themes

### Theme 1: Self-Protection is Missing

Multiple reviewers independently identified that fuse has no protection against agents modifying fuse's own configuration, disabling fuse, or manipulating the approval database. This is the single most critical gap.

**Findings:** SR-4, SR-5, SR-6, SR-15, SR-16, AI-7
**Priority:** P0 — must fix before v1.0

**Recommended resolution:**
- Add hardcoded BLOCKED rules for: `fuse disable`, `fuse uninstall`, writes to `~/.fuse/config/*`, writes to `.claude/settings.json`, `sqlite3` commands targeting `fuse.db`
- HMAC-sign approval records
- Enforce minimum values for security-critical config settings

### Theme 2: Hook Protocol Specification is Incorrect

The Claude Code hook integration — fuse's primary integration path — has fundamental errors in the spec.

**Findings:** AI-1, AI-2, AI-3, AI-4, AI-5, AI-6
**Priority:** P0 — must fix before implementation begins

**Recommended resolution:**
- Rewrite §3.1 against actual Claude Code documentation
- Verify stdin schema, exit code semantics, stderr format, timeout units
- Use `settings.json` not `hooks.json`

### Theme 3: Command Normalization Has Bypass Vectors

The normalization pipeline does not handle compound commands, newlines, Unicode, complex flag combinations, absolute paths, or shell variable targets.

**Findings:** SR-1, SR-2, SR-9, SR-10, SR-12, SR-17, SR-28, SR-29, SR-34, SR-40
**Priority:** P0 — must fix before v1.0

**Recommended resolution:**
- Add compound command splitting (`;`, `&&`, `||`, `|`, `\n`)
- Add Unicode NFKC normalization, ANSI stripping, control character stripping
- Expand flag patterns for hardcoded rules (split flags, long flags)
- Extract basename from absolute paths before rule matching
- Expand target patterns to include `$HOME`, `$PWD` shell variables
- Specify exact pipeline stage ordering (extraction before sanitization)
- Define context sanitization interaction with `bash -c`/`ssh` extraction

### Theme 4: Performance Model Needs Validation

The 50ms target may be unrealistic given pure-Go SQLite overhead and per-invocation costs.

**Findings:** GE-1, GE-2, GE-5, GE-6
**Priority:** P1 — validate with prototype before committing to architecture

**Recommended resolution:**
- Build a minimal prototype to measure cold-start latency
- If >100ms, consider daemon mode or lightweight approval cache
- Pre-compile regex at package level
- Add internal timeout enforcement

### Theme 5: TOCTOU is Fundamental in Hook Mode

File-backed commands cannot be atomically classified and executed in hook mode.

**Findings:** SR-3, SR-7
**Priority:** P1 — document limitation, mitigate where possible

**Recommended resolution:**
- Document as known limitation in both specs
- Recommend `fuse run` for high-security contexts
- Resolve symlinks before inspection
- In `fuse run` mode, re-verify file hash immediately before execution

### Theme 6: Rule Coverage Has Major Gaps

Many destructive commands match no rule and fall through to SAFE. The single-quote masking creates blind spots for SQL/SSH arguments. MCP classification only checks tool name prefixes, ignoring argument content.

**Findings:** SR-30, SR-31, SR-33, SR-35, SR-37, SR-38, SR-39
**Priority:** P1 — expand before v1.0

**Recommended resolution:**
- Add rules for `ssh` (extract and classify remote command), database CLIs, `systemctl`, `kill/pkill`, `iptables`, PaaS CLIs
- Scan MCP tool arguments against command/SQL patterns
- Expand MCP destructive verb prefix list; default unmatched to CAUTION
- Add `exec`/`eval` to Python file inspection scanner
- Add pipe-to-interpreter detection for python/node/ruby
- Flag `export PATH=` as CAUTION

### Theme 7: Approval Prompt Lacks Context

Users approve commands without seeing environment variables, working directory, or other contextual information that changes the command's effect.

**Findings:** SR-32, SR-36
**Priority:** P1 — important for informed human decisions

**Recommended resolution:**
- Show cwd in approval prompt
- Show relevant environment variables (AWS_PROFILE, TF_WORKSPACE, KUBECONFIG, etc.)

---

## Action Items Summary

| Priority | Count | Description |
|----------|-------|-------------|
| P0 | 3 themes, ~28 findings | Self-protection rules, hook protocol rewrite, normalization hardening |
| P1 | 4 themes, ~16 findings | Performance validation, TOCTOU documentation, rule coverage gaps, approval context |
| P2 | ~13 findings | MCP proxy hardening, error handling, Go idioms, logging |
| P3 | ~9 findings | Test fixtures, build tags, version pinning, edge cases |
