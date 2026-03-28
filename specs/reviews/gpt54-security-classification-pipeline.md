ID: IMPL-001
  Severity: critical
  Where: internal/core/normalize.go:137, internal/core/classify.go:397, internal/core/
  normalize.go:363
  Why this is a problem: ClassificationNormalize blindly runs filepath.Base on the first token if
  it contains /. That is fine for /usr/bin/rm, but disastrous for leading env assignments like
  LD_PRELOAD=/tmp/x.so or PATH=/tmp. The assignment token gets mangled into x.so or tmp, so
  sensitive-env detection and inner-command extraction never see the real command shape. Wrapping
  the same assignment in env is even worse: env is stripped, then the sensitive assignment is
  gone.
  Exploit or failure scenario: I verified these: LD_PRELOAD=/tmp/x.so ls => SAFE; env LD_PRELOAD=/
  tmp/x.so ls => SAFE; PATH=/tmp curl $URL => SAFE. That is not a corner case. It is a direct
  bypass of the “security-sensitive environment variable assignment” rule.
  Recommendation: Parse leading POSIX assignments explicitly before basename stripping. Never run
  filepath.Base on VAR=value tokens. Evaluate sensitive assignments both as bare prefixes and
  through env ... VAR=value .... Add tests for assignment values containing /.

  ID: IMPL-002
  Severity: critical
  Where: internal/core/urlinspect.go:110, internal/core/urlinspect.go:141, internal/core/
  classify.go:337
  Why this is a problem: URL inspection only sees literal, contiguous scheme://... strings. If the
  destination is variable-driven or reconstructed from fragments, the URL layer sees nothing. The
  “scan inline bodies too” logic is still line-by-line regex scanning, so split strings are invis
  ible there as well.
  Exploit or failure scenario: I verified curl $URL and curl "$URL" both classify as SAFE. I also
  verified a Python heredoc building http://169.254. + 169.254/latest/ only got APPROVAL, not BLO
  CKED. That is a straight downgrade of metadata SSRF from a hard block to “maybe ask later” or no
  gate at all.
  Recommendation: Treat nonliteral destinations in network commands as at least APPROVAL. Detect
  $VAR, ${...}, and common string-concatenation patterns in inline code. If the destination cannot
  be resolved statically, do not auto-approve.

  ID: IMPL-003
  Severity: significant
  Where: internal/core/urlinspect.go:210, internal/core/urlinspect.go:260
  Why this is a problem: Host classification depends on canonical host strings that net.ParseIP
  understands. Short IPv4, hex-like IPv4, and IPv6 zone-ID forms miss the blocked-range logic and
  fall through to hostname handling. DNS-resolved aliases to private or link-local addresses are
  not resolved at all.
  Exploit or failure scenario: I verified curl http://127.1/ => CAUTION, curl http://0x7f.0.0.1/
  => CAUTION, and curl http://[fe80::1%25en0]/ => CAUTION. On this machine, 127.1 and 0x7f.0.0.1
  resolve to loopback. 169.254.169.254.nip.io is only CAUTION; inference: on resolvers honoring
  nip.io/sslip.io, that reaches metadata behind a hostname that this code treats as ordinary.
  Recommendation: Canonicalize odd IPv4 forms, strip zone IDs before IP checks, and classify hosts
  by resolved IP where possible. Loopback/link-local aliases should never be auto-approved.

  ID: IMPL-004
  Severity: significant
  Where: internal/core/inspect.go:186, internal/core/classify.go:383
  Why this is a problem: DetectReferencedFile uses strings.Fields, not shell parsing. Quoted
  interpreter targets with spaces get split into fake arguments, so file inspection never runs.
  Exploit or failure scenario: I verified python "dangerous boto3.py" => SAFE and bash "dangerous
  script.sh" => SAFE. The same dangerous script without a space in the filename classified as
  APPROVAL. Renaming a script is enough to bypass interpreter-backed inspection.
  Recommendation: Extract interpreter targets from the shell AST, not strings.Fields. Preserve
  quoted tokens and spaces. Add tests for quoted paths and paths containing spaces.

  ID: IMPL-005
  Severity: significant
  Where: internal/adapters/mcpproxy.go:245, internal/adapters/hook.go:151, internal/core/
  mcpclassify.go:36, internal/core/mcpclassify.go:117
  Why this is a problem: MCP tool names are normalized in one adapter and not in another. The
  proxy path feeds full mcp__server__action names into prefix matching, so destructive actions
  miss `delete_`/`remove_`/`destroy_` and `drop` to fallback CAUTION. Separately, nested args deeper than
  32 levels are silently ignored.
  Exploit or failure scenario: I verified ClassifyMCPTool("mcp__server__delete_items", {"id":"1"})
  => CAUTION, while delete_items => APPROVAL. I also verified a nested arg blob with rm -rf / at
  depth 40 classified SAFE.
  Recommendation: Normalize tool names identically everywhere before classification. Depth
  exhaustion must not silently become safe; return at least APPROVAL or CAUTION on incomplete
  extraction. Stop treating recursive string flattening as a serious security parser.

  ID: IMPL-006
  Severity: significant
  Where: internal/judge/judge.go:248
  Why this is a problem: The LLM downgrade guard only checks ExtractionIncomplete. Every other
  fail-closed or analysis-lost state is still downgradeable if the model is “confident”: oversize
  raw command, compound parse failure, bash -c extraction failure, omitted script contents, and
  similar cases.
  Exploit or failure scenario: This is an inference from the code path, not a live-model repro: an
  attacker can force a generic APPROVAL by malformed shell or oversized input, then let the judge
  relax it because the structural pipeline already discarded the hard reason. The code has no
  guard for those states.
  Recommendation: Never let the judge downgrade any fail-closed or incomplete-analysis result.
  Better: judge may upgrade only, or run in shadow mode for everything structurally uncertain.

  ID: IMPL-007
  Severity: significant
  Where: internal/judge/prompt.go:73, internal/adapters/hook.go:480, internal/db/events.go:103
  Why this is a problem: Untrusted command text, inline bodies, and script contents are pasted
  directly into the judge prompt as plain text. Returned JudgeReasoning is then stored without
  scrubbing. That is a prompt-injection surface and a log-leak surface.
  Exploit or failure scenario: A heredoc can instruct the model to output SAFE with high
  confidence. I did not run a live-model exploit, but the injection path is explicit. If the model
  echoes secrets in reasoning, that text is written to the DB unsanitized.
  Recommendation: Do not allow LLM downgrades. Scrub JudgeReasoning before storage. Treat any
  model-visible command/body content as hostile input, because it is.

  ID: IMPL-008
  Severity: moderate
  Where: internal/db/events.go:56
  Why this is a problem: ScrubCredentials is regex cosplay. It misses common structured secrets
  and only partially redacts others, while over-redacting arbitrary base64.
  Exploit or failure scenario: I verified these misses: {"token":"abc123","password":"hunter2"}
  unchanged, Authorization: Basic dXNlcjpwYXNz only partially redacted, cookies mostly unchanged,
  JWT-like tokens unchanged. That means secrets can leak through event logs and judge prompts.
  Recommendation: Add real coverage for quoted JSON/YAML keys, cookies, Basic auth, JWTs, AWS temp
  creds, and common header variants. Scrub all persisted text fields, not just Command.

  False Sense Of Safety

  - “Fail-closed” often means “generic APPROVAL after losing the dangerous detail,” not
    preservation of the original security boundary.
  - The URL layer looks robust because it has metadata IPs, schemes, and CIDRs. It is not. curl
    $URL being SAFE kills the SSRF story.
  - Inline extraction looks sophisticated because it uses mvdan.cc/sh, but the surrounding normal
    ization and regex scanning still lose context and miss reconstructed destinations.
  - MCP tests validate stripped action names, while one shipped adapter classifies full tool names
    and weakens destructive calls to CAUTION.
  - ScrubCredentials gives the appearance of privacy protection while missing basic JSON, cookie,
    and JWT cases.

  Missing Tests

  - LD_PRELOAD=/tmp/x.so ls, env LD_PRELOAD=/tmp/x.so ls, PATH=/tmp curl $URL
  - python "dangerous boto3.py", bash "./dangerous script.sh"
  - curl $URL, curl "${HOST}/api", inline requests.get(url)
  - Split URLs across lines or concatenated strings in heredocs and python -c
  - 127.1, 0x7f.0.0.1, IPv6 zone IDs, DNS alias hosts like nip.io/sslip.io
  - Full MCP names through the proxy path, not just stripped names
  - MCP args deeper than 32 levels
  - Judge downgrade attempts on parse-failure and oversize-command cases
  - JSON/YAML secret scrubbing, cookies, Basic auth, JWTs, and judge reasoning

  Bottom Line
  This is not ready to trust as a firewall. It has concrete bypasses to SAFE or CAUTION under
  hostile input: env-assignment normalization bugs, variable URLs, quoted script paths, MCP name
  inconsistency, and depth-truncated MCP args. At best it is a heuristic approval assistant. It is
  not a trustworthy security boundary.
