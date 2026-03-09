  Reviewer: Senior Principal Systems Engineer (Go Specialist)
  Date: March 8, 2026
  Target: specs/technical.md (v3.0)

  Executive Summary


  As a Go implementation plan, this specification contains three fatal technical contradictions that will
  prevent the codebase from functioning as described.


   1. Regex Incompatibility: The spec mandates Go's regexp package (RE2) but defines rules using Lookarounds
      ((?!...)), which RE2 explicitly does not support. The code will not compile.
   2. Library Mismatch: The spec relies on mattn/go-shellwords for "compound command splitting." This
      library parses arguments (argv), not shell operators (&&, ;, |). It cannot perform the requested task.
   3. Database Latency: The requirement for <50ms warm-path classification is incompatible with opening a
      new modernc.org/sqlite connection on every hook invocation, given the initialization overhead of the
      translation layer.

  The architecture is otherwise sound (clean separation of concerns, standard library usage), but the core
  logic for rule matching and parsing needs a rewrite before a single line of code is written.

  ---


  1. Critical Implementation Blockers

  1.1. Incompatible Regex Dialect (RE2 vs. PCRE)
  Severity: Blocker
  Section: Technical §1.1, §6.3.14, §6.3.15, §5.5


  The spec explicitly states: "Go's regexp package (RE2-based)... PCRE-compatible libraries must never be
  substituted."
  However, the rule definitions rely heavily on Negative Lookaheads ((?!...)), which are not supported in
  Go's regexp (RE2 is strictly linear-time).


  Specific failing patterns in the spec:
   * \bpython... (?!-[cmeWvV]) (§5.5)
   * \bgit\s+restore\s+(?!.*--staged) (§6.3.1)
   * \bnode\s+(?!-[ep]) (§5.5)

  Consequence: regexp.MustCompile will panic at init time. The application will crash immediately on
  startup.


  Engineering Fix:
   1. Rewrite Regexes: You must rewrite these patterns to be additive rather than subtractive, or check the
      negative condition in Go code (e.g., match_python AND !match_flag).
   2. Switch Libraries (Violates Spec): Use dlclark/regexp2 (PCRE), but this sacrifices the linear-time
      guarantee and opens the door to ReDoS (Regular Expression Denial of Service).
   3. Recommendation: Stick to regexp (RE2) for safety, but rewrite the rules in the spec to use positive
      matching only, or handle exclusions in the Go logic layer (if match && !excluded { ... }).


  1.2. Incorrect Shell Parser Selection
  Severity: Blocker
  Section: Technical §5.3 ("Compound command splitting")

  The spec says: "Use mattn/go-shellwords or equivalent for proper quote-aware splitting [on ; && || |]."


  mattn/go-shellwords is an implementation of the UNIX Bourne shell word splitting rules. It takes a string
  and returns []string (argv). It does not recognize ;, &&, or | as control operators; it treats them as
  arguments or fails.


  Example:
  Input: echo "hello"; rm -rf /
  go-shellwords: Returns ["echo", "hello; rm -rf /"] (depending on exact parsing rules, it likely won't
  split the semicolon as a command separator).


  Consequence: fuse will see one command with a weird argument, fail to match the destructive regex, and
  default to SAFE/APPROVAL.


  Engineering Fix:
  You need a real shell lexer. mvdan.cc/sh/syntax is the industry standard Go library for this. It generates
  an AST. Use mvdan.cc/sh to parse the input, then walk the AST to extract commands. Do not try to
  regex-split shell commands; you will fail.

  1.3. SQLite Initialization Overhead (Performance)
  Severity: High
  Section: Technical §1.2, §17.1


  The spec requires <50ms classification latency (warm path) and uses modernc.org/sqlite (pure Go).
  modernc.org/sqlite is a transpilation of the C source. Its init() time is significantly heavier than CGo
  variants because it initializes a virtual memory space and translation tables.
  Cold-starting this DB on every shell hook (which happens constantly in agent loops) will introduce
  perceptible lag (likely 100ms-300ms depending on hardware).

  Consequence: The tool will feel sluggish. Agents with short timeouts might flake.


  Engineering Fix:
   1. Lazy Loading (Mandatory): As suggested in §1.2, do not sql.Open until a decision other than SAFE is
      reached. SAFE commands (99% of traffic) must avoid DB init entirely.
   2. Daemonize (Milestone 3): For fuse run or heavy hook usage, a background daemon holding the DB lock is
      the only way to get sub-10ms response times with SQLite.

  ---

  2. Architecture & Platform Findings


  2.1. TTY Handling in Hooks
  Severity: Medium
  Section: Technical §8.3, §3.1


  The spec relies on opening /dev/tty for the TUI.
   * Linux/macOS: This works via os.OpenFile("/dev/tty", os.O_RDWR, 0).
   * Context: When Claude Code runs a hook, it might be running inside a non-interactive shell or a pipe
     wrapper. If the agent itself doesn't have a TTY attached (e.g., running in a headless CI/CD env or a
     background worker), opening /dev/tty will return ENXIO (no such device or address).


  Engineering Fix:
  The code must handle the err != nil on opening /dev/tty. The spec says "auto-deny". This is safe, but
  users might be confused if they run the agent in a way that detaches the TTY. Explicit error messaging
  ("fuse requires an interactive terminal for approval") is needed.

  2.2. Signal Forwarding & Process Groups
  Severity: Medium
  Section: Technical §10.1


  The spec prescribes: cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true} and then forwarding signals to
  -pid.
  This is correct for Linux.
  On macOS (Darwin), this usually works, but syscall.Kill behavior with negative PIDs can sometimes be
  finicky regarding permissions if the parent process isn't the session leader.


  Engineering Fix:
  Ensure the fuse binary itself handles TTY signals correctly (e.g., signal.Ignore(syscall.SIGTTOU) might be
  needed if fuse tries to manipulate the terminal while in the background).


  2.3. Null Byte Injection in Text Fields
  Severity: Low
  Section: Technical §8.1


  The decision key construction strips null bytes (\x00).
  Go strings can contain null bytes. If the normalizer doesn't strip them before the hashing function is
  called (it does in §5.3, but §8.1 repeats it), you might get inconsistent hashes if the order of
  operations varies.

  Engineering Fix:
  Ensure DisplayNormalize is the single source of truth. Do not re-sanitize in ComputeDecisionKey; accept
  the normalized string as verified.

  ---

  3. Go-Specific "Gotchas"


   1. path/filepath vs Symlinks: §7.1 mentions filepath.EvalSymlinks. Note that on macOS, /tmp often
      resolves to /private/tmp. The regex rules in §6.2 check for \brm\s+.... If the user types rm /tmp/foo
      and you canonicalize it to rm /private/tmp/foo, make sure your regexes are robust enough to match the
      canonical path if you are matching against that. (The spec implies matching against the command
      string, not the path, but file inspection uses the path).
   2. os/exec Argument Splitting: If fuse run -- <cmd> is used, os.Args gives the command as a single string
      (if quoted) or multiple strings. The spec says "Parse command from argv (everything after --)". You'll
      need to re-join them? Or expect a single string?
       * Usage: fuse run -- ls -la -> []string{"ls", "-la"}.
       * Usage: fuse run -- "ls -la" -> []string{"ls -la"}.
       * Fix: Decide if fuse behaves like sh -c (one string arg) or exec (list of args). The spec implies sh
         -c execution, so strings.Join(args, " ") is likely required, which destroys quote boundaries.
         Better to require a single argument for safety.

  ---

  4. Feasibility Verdict

  Can this be built in Go?
  Yes, but not strictly following the spec.


  Required Deviations:
   1. Regex: Must abandon Lookarounds. Rewrite rules or accept dlclark/regexp2 (and the performance/security
      tradeoff).
   2. Parsing: Must abandon mattn/go-shellwords. Adopt mvdan.cc/sh or write a custom lexer.
   3. Performance: Must implement lazy DB loading immediately, not as a "mitigation strategy."


  Overall complexity: Low-Medium. Go is an excellent choice for this tool (static binary, cross-platform,
  system interaction), provided the parsing logic is corrected.


  Recommendation:
  Approve the project architecture, but reject the parsing/matching technical definitions. Request a V3.1
  spec update to address the RE2 and Lexer issues before coding begins.
