Reviewer: Principal Security Engineer
Date: March 8, 2026
Target: `specs/technical.md` (`v3.0`), `specs/functional.md` (`v3.0`)

Executive Summary

The fuse specification describes a security tool that attempts to retrofit authorization onto a chaotic domain (shell execution) using imperfect mechanisms (regex, string splitting, static analysis). While the intent is "tripwire" rather than "sandbox," critical defects in the specification allow trivial bypass of the safety mechanisms.

The most significant failures stem from the disconnect between how fuse analyzes commands (stateless, text-based, split-and-check) and how the shell actually executes them (stateful, environment-dependent, stream-based).

A sophisticated agent or a compromised script can completely bypass fuse protections in its current design.

`---`

1. Critical Findings

`1.1`. "File Missing" Fallback Defaults to `SAFE` (`TOCTOU` Vulnerability)
Severity: Critical
Section: Technical §6.3.14, §7.1

The spec explicitly states that if a referenced file does not exist, inspection is skipped "with no error" (§7.1). It further states that "If inspection finds no signals, the command is `SAFE`" (§6.3.14).
This creates a trivial bypass for any destructive payload:

1. Agent constructs command: echo "rm -rf /" > `pwn.sh` && bash `pwn.sh`
2. fuse splits this into echo ... (`SAFE`) and bash `pwn.sh` (inspects `pwn.sh`).
3. At classification time, `pwn.sh` does not exist.
4. Inspection is skipped. No signals are found.
5. fuse classifies bash `pwn.sh` as `SAFE`.
6. Command executes. `pwn.sh` is created and immediately executed.

Fix: If a referenced file is missing during classification, the decision must be `APPROVAL` (or `BLOCKED`), never `SAFE`. You cannot assert safety for unknown content.

`1.2`. Incorrect Compound Command Splitting
Severity: Critical
Section: Technical §5.3

The spec requires "Compound command splitting" on operators ; && || | \n while "respecting quoting". It suggests using `mattn/go-shellwords`.
go-shellwords parses arguments, it does not parse shell control operators. Implementing a correct shell parser that handles nesting, subshells, and quoting is notoriously difficult and error-prone.
If the custom splitter fails to identify a separator (`e.g`., inside a subshell or obscure quoting syntax), fuse will treat `safe_cmd`; `dangerous_cmd` as a single command/argument. If this combined string doesn't match a specific `BLOCKED` regex, it falls through to `SAFE` (unrecognized).

Fix: Do not attempt to parse and split compound commands. If a command contains shell control characters that cannot be definitively parsed as safe arguments, classify the entire block as `APPROVAL`.

`1.3`. `CWD` State Desync in Compound Commands
Severity: Critical
Section: Technical §5.1, §7.1

fuse classifies sub-commands "independently," but shell execution is stateful.
Scenario: cd `/tmp` && python `destructive_script.py`
 1. fuse analyzes cd `/tmp` -> `SAFE`.
 2. fuse analyzes python `destructive_script.py`. It resolves `destructive_script.py` relative to the initial `CWD` of the fuse process.
 3. Since the script exists in `/tmp` (not `CWD`), file resolution fails.
 4. Inspection is skipped (see `1.1`). Result: `SAFE`.
 5. Execution proceeds. The shell changes directory to `/tmp` and executes the destructive script.

Fix: Any command containing directory changes (cd, pushd) must force subsequent sub-commands (or the whole block) to `APPROVAL` if they involve file inspection, as the resolution context is lost.

`---`

2. High Severity Findings

`2.1`. Environment Poisoning Inheritance
Severity: High
Section: Technical §10.1, §5.3

fuse run inherits the parent environment (os.Environ()) and passes it to the child `/bin/sh`.
The spec detects `LD_PRELOAD=` assignments in the command string (§5.3), but fails to sanitize the inherited environment.
Exploit:
 1. Agent runs export `LD_PRELOAD=/tmp/evil.so` (Classified SAFE/CAUTION depending on rules, or done via
`echo ... >> `~/.bashrc` then new session).`
 2. Agent runs fuse run `--` ls.
 3. fuse sees ls, classifies `SAFE`.
 4. fuse executes `/bin/sh` -c ls with the poisoned `LD_PRELOAD`.
 5. `evil.so` loads and executes arbitrary code inside the approved process.

Fix: fuse run must use a whitelist of safe environment variables or aggressively scrub dangerous variables (`LD_*`, `PYTHONPATH`, `PERL5LIB`, etc.) from the inherited environment before execution.

`2.2`. Interpreter "Suspicious" Bypass of Self-Protection
Severity: High
Section: Technical §6.2, §5.4

The "Self-protection" rules (§6.2) rely on regex matching of shell commands (`e.g`., rm `~/.fuse/`). However, python -c "..." is classified merely as "Suspicious" (`APPROVAL`) by the Fallback Heuristics (§5.4).
Exploit:
 1. Agent runs python -c "import shutil, os; `shutil.rmtree(os.path.expanduser('~/.fuse`'))".
 2. fuse sees python -c. It does not match hardcoded `BLOCKED` rules (which are regexes on rm, mv, etc.).
 3. It matches "Suspicious interpreter launch" -> `APPROVAL`.
 4. User sees generic "Suspicious: python -c" prompt and approves.
 5. fuse configuration/database is destroyed.

Fix: "Suspicious" patterns must be analyzed more deeply, or explicitly restricted from touching `~/.fuse`. Simply prompting "Suspicious" for arbitrary code execution negates the hardcoded `BLOCKED` protections.

`---`

3. Medium Severity Findings

`3.1`. `MCP` Proxy Resource Leak
Severity: Medium
Section: Technical §11.2

The `MCP` proxy passes resources/* messages through transparently.
If the downstream `MCP` server supports reading resources (`e.g`., a filesystem server), a compromised agent can read sensitive files (`/etc/shadow`, `~/.ssh/id_rsa`, `~/.fuse/state/secret.key`) via resources/read. The spec focuses entirely on tools/call.

Fix: Inspect resources/read and resources/subscribe requests. Block access to sensitive file paths (`~/.fuse`, `~/.ssh`, etc.) similar to how shell commands are blocked.

`3.2`. Regex Fragility on Shell Flags
Severity: Medium
Section: Technical §6.2

The hardcoded `BLOCKED` rules use regexes like \brm\s+....
Shell argument parsing is complex.
Exploit: rm \ -rf /. The backslash escapes the space. The shell sees rm and -rf. The regex \brm\s+ might fail to match depending on the Go regex engine's handling of \s vs literal backslash-space.
Similarly: rm$EMPTY -rf /.

Fix: Perform normalization after tokenization for rule matching, or use the tokenized argument list for the "destructive verb" checks instead of raw string regexes.

`---`

4. Open Questions & Ambiguities

1. Exit Code 2 Propagation: Technical §3.1 says "fuse must always use exit 2 to deny." Does Claude Code guarantee that exit code 2 halts the loop? If Claude Code treats it as a "Tool Error" and retries or continues, the block is soft.
2. go-shellwords splitting: The spec relies heavily on this library for "Compound command splitting". Does this library actually support splitting on ; or &&? (Reviewer note: It typically does not; it splits args). If custom splitting logic is written, where is the spec for it?
3. `TUI` in Hook Mode: How does fuse hook evaluate render a `TUI` on `/dev/tty` when running inside a hook triggered by an agent? Does the agent pause? Does the agent mistakenly capture the `TUI` output? The spec says "Claude Code owns stdin/stdout; fuse never writes to stdout". Is `/dev/tty` guaranteed to be available and interactive in the user's context when Claude Code is running (possibly in a background worker or `IDE` terminal)?

`---`

5. Overall Risk Assessment

Risk Level: Critical

The specification describes a tool that provides security theater. The "Fail-Safe on Missing File" defect (`1.1`) and "`CWD` Desync" defect (`1.3`) render the file inspection mechanism easily bypassable by any agent that chains commands (`e.g`. echo ... > file && run file).

Since the primary threat model is "agent runs destructive command," and agents frequently write scripts to run commands, fuse v1 fails to meet its primary security goal against a competent (or purely unlucky) agent.
