# Plan: V2 Classification Pipeline — Inline Analysis, Network Awareness, Judge Daemon

## Context

Fuse v1's classification is structural — it detects patterns (`bash -c`, `<<EOF`, `$(...)`,
`curl | bash`) but never reads what's inside them. A `bash <<EOF\nrm -rf /\nEOF` triggers
APPROVAL because of the `<<EOF` pattern, not because of the `rm -rf /` inside. Meanwhile
`curl http://169.254.169.254/metadata` is SAFE because no rule matches the URL.

The LLM judge can evaluate semantic risk, but it only triggers on CAUTION/APPROVAL, has
1-3s latency per call (process spawn overhead), and doesn't receive inline script content.

This plan upgrades three layers in one cohesive pass:

1. **Inline script extraction** — extract heredoc bodies and `$()` contents using the
   `mvdan.cc/sh` shell parser (already a dependency), classify them through the existing
   pipeline, and pass them to the judge
2. **Network/URL awareness** — detect SSRF targets (cloud metadata, loopback, private
   networks), untrusted destinations, and insecure flags in curl/wget/httpie commands
3. **Judge daemon** — Unix socket daemon with a long-lived `claude` process using
   stream-json, eliminating per-call spawn overhead. Based on the proven clai daemon
   pattern (`~/workspaces/clai/internal/claude/daemon.go`)
4. **Policy LKG (last-known-good)** — filesystem-based fallback to prevent silent rule
   loss when policy.yaml has errors

Source research: `docs/research/openshell-patterns-for-fuse.md` (patterns #1, #2, #4, #5)

Pre-mortem applied: `.agents/council/2026-03-24-pre-mortem-v2-classification.md`
— 12 findings addressed (pm-101 through pm-112)

## Baseline Audit

| Metric | Command | Result |
|--------|---------|--------|
| Classification pipeline LOC | `wc -l internal/core/{classify,normalize,safecmds}.go` | 1,846 lines |
| Judge package LOC | `wc -l internal/judge/*.go` (excl tests) | 506 lines |
| Inline pattern regexes | `grep -c reInline internal/core/classify.go` | 33 references |
| curl/wget rules | `grep -c "curl\|wget\|http" internal/policy/builtins_security.go` | 15 mentions |
| curl in safe verbs | `grep curl internal/core/safecmds.go` | NOT present |
| Shell parser in use | `grep "mvdan.cc/sh" internal/core/compound.go` | Yes — used for compound splitting |
| Heredoc test fixtures | `grep -c "heredoc" testdata/fixtures/commands.yaml` | 2 |
| clai daemon reference | `wc -l ~/workspaces/clai/internal/claude/daemon.go` | 503 lines |

## Files to Modify

| File | Change |
|------|--------|
| `internal/core/normalize.go` | Add `extractHeredocBody()`, `extractCommandSubstitution()` using mvdan.cc/sh parser |
| `internal/core/classify.go` | New `classifyInlineBodies()` and `classifyURLs()` functions called from `classifySingleCommand` |
| `internal/core/urlinspect.go` | **NEW** — URL parsing, SSRF detection, cloud metadata + loopback + RFC1918 blocking |
| `internal/core/urlinspect_test.go` | **NEW** — URL inspection tests including encoded IPs, localhost, IPv6 |
| `internal/core/normalize_test.go` | Add heredoc/cmd-substitution extraction tests |
| `internal/core/classify_test.go` | Add inline body classification tests |
| `internal/policy/builtins_security.go` | Add URL-aware rules (SSRF, insecure certs) |
| `internal/judge/daemon.go` | **NEW** — Unix socket daemon with long-lived claude process (stream-json) |
| `internal/judge/daemon_test.go` | **NEW** — daemon tests |
| `internal/judge/provider.go` | Add `DaemonProvider` to detection logic, add `Close()` to Provider interface |
| `internal/judge/prompt.go` | Add `InlineScriptBody` to PromptContext |
| `internal/config/config.go` | Add `URLTrustPolicy`, `PolicyLKG` config |
| `internal/adapters/hook.go` | Update `buildJudgeContext()` for inline bodies |
| `internal/policy/policy.go` | Add `LoadPolicyWithLKG()` using filesystem |
| `internal/policy/policy_lkg_test.go` | **NEW** — LKG fallback tests |
| `testdata/fixtures/commands.yaml` | Add heredoc, URL, inline script fixtures |

## Boundaries

**Always:**
- Inline extraction uses `mvdan.cc/sh` parser (not manual tokenization) for correctness
- Extraction is bounded (50KB max body, depth 3 max recursion — shared counter)
- URL inspection is pattern-based with `net/url.Parse` + `net.ParseIP` (no DNS resolution)
- Daemon falls back to spawn-per-call if socket unavailable
- LKG is filesystem-based (`policy.yaml.lkg`), no schema migration needed
- BLOCKED decisions are never downgraded, regardless of extracted content
- When heredoc body is truncated, structural APPROVAL stands (never downgrade on partial)
- URLs containing unresolved shell variables (`$HOST`) are treated as opaque (skip inspection)
- All extracted content is scrubbed via `ScrubCredentials` before judge
- Daemon auto-disables in hook mode (short-lived process, no amortization benefit)

**Never:**
- DNS resolution or network calls during classification
- Manual heredoc/`$()` parsing — use the shell parser
- Send raw inline script content to judge without scrubbing
- Replace the structural classifier — the judge augments, doesn't replace
- Store LKG in SQLite (policy loads before DB is available)
- Change system prompt mid-daemon-session (it's a CLI startup flag)

## Implementation

### 1. Inline Script Extraction (`internal/core/normalize.go`)

Use `mvdan.cc/sh/v3/syntax` (already imported in `compound.go`) to extract heredoc
bodies and command substitution contents. The parser handles all edge cases:
`<<-EOF` tab stripping, quoted markers (`<<'EOF'`), indented delimiters, nested
quotes inside `$(...)`.

**Pre-mortem fixes applied:**
- pm-104: Use shell parser instead of manual balanced-paren counting (handles `$("echo ')'")`)
- pm-107: Shell parser handles `<<-EOF` tab stripping and indented delimiter matching
- pm-108: When extraction truncates (>50KB), keep structural APPROVAL — never downgrade on partial body

```go
// extractHeredocBody uses the mvdan.cc/sh parser to extract the body of a heredoc.
// Returns (body, complete). When body > MaxScriptBytes, truncates and returns complete=false.
func extractHeredocBody(cmd string) (string, bool)

// extractCommandSubstitutions uses the mvdan.cc/sh parser to extract $() contents.
// Skips $(cat <<...) patterns (string quoting, not code execution).
// Returns slice of extracted commands.
func extractCommandSubstitutions(cmd string) []string
```

**Integration point:** New function `classifyInlineBodies(cmd string, evaluator PolicyEvaluator, cwd string)` in `classify.go`. Called from `classifySingleCommand` after `detectInlineScript` (step 5). Extracts bodies, classifies them recursively through the same pipeline. Stores bodies in `ClassifyResult.InlineBody` for the judge.

**Depth limiting (pm-108 fix):** The extraction functions share the recursion counter
with `classificationNormalizeRecursive` (`maxRecursionDepth = 3`). Heredoc extraction
counts as 1 depth level. `$()` extraction counts as 1 depth level. Combined nesting
beyond depth 3 stops extraction (structural classification stands).

### 2. URL/Network Awareness (`internal/core/urlinspect.go`)

**NEW file.** Detects and classifies URLs in network commands.

**Pre-mortem fixes applied:**
- pm-103: Added `localhost`, `::1`, `0.0.0.0` to blocked hostnames. Added RFC1918 ranges as CAUTION.
- pm-103: Use `net/url.Parse` for URL decoding, `net.ParseIP` for IP normalization after decoding.
- pm-105: Also scan MCP tool arguments for URLs (add `InspectURLsInArgs` for ClassifyMCPTool).
- pm-110: Skip URL inspection when URL contains unresolved shell variables (`$`, `{`).

```go
// BlockedHostnames are always-blocked destination names.
var BlockedHostnames = []string{
    "169.254.169.254",          // AWS/GCP metadata
    "metadata.google.internal", // GCP metadata
    "100.100.100.200",          // Alibaba metadata
    "169.254.170.2",            // ECS task metadata
    "localhost",                // loopback
    "0.0.0.0",                  // all interfaces
}

// BlockedIPRanges are always-blocked IP ranges → BLOCKED.
var BlockedIPRanges = []net.IPNet{
    parseCIDR("127.0.0.0/8"),     // loopback (IPv4)
    parseCIDR("169.254.0.0/16"),  // link-local
}

// CautionIPRanges are private network ranges → CAUTION.
var CautionIPRanges = []net.IPNet{
    parseCIDR("10.0.0.0/8"),      // RFC1918
    parseCIDR("172.16.0.0/12"),   // RFC1918
    parseCIDR("192.168.0.0/16"),  // RFC1918
}

// IPv6 blocked ranges.
var BlockedIPv6Ranges = []net.IPNet{
    parseCIDR("::1/128"),         // loopback (IPv6)
    parseCIDR("fe80::/10"),       // link-local (IPv6)
    parseCIDR("::ffff:169.254.169.254/128"), // IPv4-mapped metadata
}

// InspectCommandURLs extracts URLs from curl/wget/httpie commands and classifies them.
func InspectCommandURLs(cmd string) (Decision, string)

// InspectURLsInArgs scans MCP tool argument strings for suspicious URLs.
func InspectURLsInArgs(args map[string]interface{}) (Decision, string)
```

**Integration points:**
- `classifySingleCommand` in classify.go: new `classifyURLs(cmd)` function called
  between inline detection (step 5) and builtin evaluation (step 8). Runs only when
  command basename is `curl`, `wget`, `http`, or `httpie`.
- `ClassifyMCPTool` in mcpclassify.go: call `InspectURLsInArgs(args)` on the flattened
  argument map.

**Known v2 limitations (documented, not fixed):**
- Exotic IP encodings (hex `0xA9FEA9FE`, decimal `2852039166`, octal `0251.0376...`)
  are not normalized. Go's `net.ParseIP` doesn't handle these. Full normalization deferred to v3.
- DNS-based bypasses (`attacker.com` resolving to `169.254.169.254`) not detectable without
  DNS resolution, which is out of scope.

### 3. Judge Daemon (`internal/judge/daemon.go`)

**NEW file.** Unix socket daemon managing a long-lived `claude` process with
stream-json protocol. Based on the proven clai pattern
(`~/workspaces/clai/internal/claude/daemon.go`).

**Pre-mortem fixes applied:**
- pm-101: Uses `--print --verbose --input-format stream-json --output-format stream-json`
  (not `--bare` — `--verbose` is required for stream-json). Multi-turn is confirmed working
  in clai production.
- pm-106: Daemon auto-detects adapter mode. In hook mode (short-lived), connects to an
  already-running daemon (instant) or falls back to spawn-per-call. In codex-shell mode
  (long-lived), starts the daemon if not running.
- pm-101: `--system-prompt` is a CLI startup flag. The judge system prompt is static, so
  this is fine — set once at daemon start.
- pm-101: Stream-json message format uses `StreamMessage{Type:"user", Message:{Role:"user", Content:"..."}}`
  and reads `StreamResponse` lines until `Type:"result"`.

**Architecture:**

```
fuse daemon start (background process)
  └─→ listens on ~/.fuse/state/judge-daemon.sock
  └─→ spawns: claude --print --verbose --system-prompt "..." \
       --input-format stream-json --output-format stream-json \
       --model <model>
  └─→ init handshake (send "Ready", wait for system init + result)
  └─→ idle timeout: shuts down after 5 min inactivity

fuse hook evaluate (short-lived)
  └─→ MaybeJudge → DaemonProvider.Query
       └─→ connect to ~/.fuse/state/judge-daemon.sock
       └─→ send DaemonRequest{Prompt}
       └─→ read DaemonResponse{Result}
       └─→ ~500ms (inference only, no startup)
       └─→ if socket unavailable: fall back to claudeProvider (spawn-per-call)
```

```go
// --- Daemon server (runs as background process) ---

type claudeProcess struct {
    cmd     *exec.Cmd
    stdin   io.WriteCloser
    scanner *bufio.Scanner
    mu      sync.Mutex
}

type StreamMessage struct {
    Type    string `json:"type"`
    Message struct {
        Role    string `json:"role"`
        Content string `json:"content"`
    } `json:"message"`
}

type StreamResponse struct {
    Type    string `json:"type"`
    Subtype string `json:"subtype,omitempty"`
    Result  string `json:"result,omitempty"`
    Message struct {
        Content []struct {
            Type string `json:"type"`
            Text string `json:"text"`
        } `json:"content"`
    } `json:"message,omitempty"`
}

type DaemonRequest struct {
    Prompt       string `json:"prompt"`
    SystemPrompt string `json:"system_prompt,omitempty"` // ignored after first query
}

type DaemonResponse struct {
    Result string `json:"result,omitempty"`
    Error  string `json:"error,omitempty"`
}

func RunDaemon(ctx context.Context, model, systemPrompt string) error
func startClaudeProcess(ctx context.Context, model, systemPrompt string) (*claudeProcess, error)
func (c *claudeProcess) query(prompt string) (string, error)

// --- Daemon client (used by DaemonProvider) ---

type DaemonProvider struct {
    model string
}

func (p *DaemonProvider) Query(ctx context.Context, systemPrompt, userPrompt string) (string, error)
func (p *DaemonProvider) Name() string
func (p *DaemonProvider) Close() error // no-op for client, daemon manages process

// --- Daemon lifecycle ---

func StartDaemonProcess(model, systemPrompt string) error  // starts background process
func IsDaemonRunning() bool                                 // checks socket
func StopDaemon() error                                     // sends shutdown
```

**Provider interface change:** Add `Close() error` to `Provider` interface.
`claudeProvider.Close()` and `codexProvider.Close()` are no-ops. This lets
`MaybeJudge` clean up the old provider on config hot-reload (pm-106 from
pre-mortem — process leak prevention).

**Daemon start integration:**
- `fuse enable` starts the daemon if `llm_judge.daemon: true` in config
- `fuse disable` stops the daemon
- `fuse install claude` adds daemon auto-start hint
- CLI command: `fuse daemon start`, `fuse daemon stop`, `fuse daemon status`

**Config:**
```yaml
llm_judge:
  mode: shadow
  provider: auto
  daemon: true       # NEW — use Unix socket daemon for low-latency judge
  model: claude-haiku-4-5-20251001
```

### 4. Policy LKG Fallback (`internal/policy/policy.go`)

**Pre-mortem fix applied (pm-102):** Filesystem-based, not SQLite. Policy loads
before DB is available in all adapters. No schema migration needed.

```go
// LoadPolicyWithLKG tries to load policy.yaml. On success, saves as LKG.
// On failure, falls back to last known good policy if available and recent.
func LoadPolicyWithLKG(path string) (*PolicyConfig, error)
```

**LKG storage:** `~/.fuse/config/policy.yaml.lkg` — a copy of the last
successfully loaded policy.yaml, with a timestamp comment on line 1:

```yaml
# LKG saved: 2026-03-24T10:30:00Z
# Original: ~/.fuse/config/policy.yaml (sha256: abc123...)
rules:
  - ...
```

On successful `LoadPolicy`: copy `policy.yaml` to `policy.yaml.lkg` with timestamp.
On failed `LoadPolicy`: try `policy.yaml.lkg`, check timestamp freshness
(default 7 days), parse and return with `slog.Warn`.

**Config:**
```yaml
policy_lkg:
  enabled: true           # default
  max_age_days: 7         # LKG must be recent
```

### 5. Judge Inline Content (`internal/judge/prompt.go`)

Extend `PromptContext` to carry extracted inline script bodies:

```go
type PromptContext struct {
    // ... existing fields ...
    InlineScriptBody string // extracted heredoc body or $() content (scrubbed)
}
```

`BuildUserPrompt` includes inline body when present:
```
Inline script body (extracted from command):
<body content, scrubbed>
```

**Integration:** `buildJudgeContext` in `hook.go` populates `InlineScriptBody`
from `ClassifyResult.InlineBody` after classification. The field is a string
(immutable in Go), so `WithDecision` shallow copy is safe.

### 6. Config Extensions (`internal/config/config.go`)

```go
type Config struct {
    // ... existing fields ...
    URLTrustPolicy URLTrustPolicyConfig `yaml:"url_trust_policy"`
    PolicyLKG      PolicyLKGConfig      `yaml:"policy_lkg"`
}

type URLTrustPolicyConfig struct {
    TrustedDomains []string `yaml:"trusted_domains"` // empty = no domain trust enforcement
    BlockSchemes   []string `yaml:"block_schemes"`   // e.g., ["file", "ftp"]
}

type PolicyLKGConfig struct {
    Enabled    bool `yaml:"enabled"`      // default true
    MaxAgeDays int  `yaml:"max_age_days"` // default 7
}

// LLMJudgeConfig gets new field:
type LLMJudgeConfig struct {
    // ... existing fields ...
    Daemon bool `yaml:"daemon"` // use Unix socket daemon for low-latency judge
}
```

## Tests

**`internal/core/normalize_test.go`** — add:
- `TestExtractHeredocBody_Simple`: `bash <<EOF\necho hello\nEOF` → "echo hello"
- `TestExtractHeredocBody_TabStripped`: `bash <<-EOF\n\techo hello\n\tEOF` → "echo hello" (delimiter indented)
- `TestExtractHeredocBody_QuotedMarker`: `bash <<'EOF'` literal content (no expansion)
- `TestExtractHeredocBody_Truncated`: body > 50KB → truncated, complete=false
- `TestExtractHeredocBody_Empty`: `bash <<EOF\nEOF` → ("", true)
- `TestExtractHeredocBody_DelimiterInBody`: `bash <<EOF\necho "not EOF"\nEOF` → full body
- `TestExtractHeredocBody_CatExempt`: `$(cat <<'EOF'...)` → not extracted (string quoting)
- `TestExtractCommandSubstitution_Simple`: `echo $(whoami)` → ["whoami"]
- `TestExtractCommandSubstitution_QuotedParen`: `$(echo ")")` → ["echo \")\""] (not truncated)
- `TestExtractCommandSubstitution_Nested`: `$(echo $(id))` → ["echo $(id)"] (depth 1)
- `TestExtractCommandSubstitution_CatExempt`: `$(cat <<'EOF'...)` → [] (skipped)

**`internal/core/urlinspect_test.go`** — add:
- `TestInspectURLs_MetadataEndpoint`: `curl http://169.254.169.254/latest/meta-data/` → BLOCKED
- `TestInspectURLs_Localhost`: `curl http://localhost:8080/admin` → BLOCKED
- `TestInspectURLs_IPv6Loopback`: `curl http://[::1]:8080/` → BLOCKED
- `TestInspectURLs_RFC1918`: `curl http://10.0.0.1:8500/secrets` → CAUTION
- `TestInspectURLs_NormalURL`: `curl https://api.github.com/repos` → SAFE
- `TestInspectURLs_InsecureFlag`: `curl -k https://example.com` → CAUTION
- `TestInspectURLs_URLWithCredentials`: `curl http://admin:pass@169.254.169.254/` → BLOCKED
- `TestInspectURLs_ShellVariable`: `curl https://$HOST/api` → SAFE (opaque, skip)
- `TestInspectURLs_NonNetworkCommand`: `git commit` → no inspection
- `TestInspectURLs_MCPArguments`: MCP tool with metadata URL arg → BLOCKED

**`internal/judge/daemon_test.go`** — add:
- `TestDaemonProvider_QueryViaMock`: mock socket server, send/receive JSON
- `TestDaemonProvider_Timeout`: slow response → context timeout
- `TestDaemonProvider_SocketUnavailable`: no daemon → fallback to spawn-per-call
- `TestDaemonProvider_IsDaemonRunning`: check socket existence

**`internal/policy/policy_lkg_test.go`** — add:
- `TestLoadPolicyWithLKG_Success`: valid policy → loads and writes .lkg file
- `TestLoadPolicyWithLKG_ParseError_FallsBack`: broken YAML → uses .lkg
- `TestLoadPolicyWithLKG_NoLKG_Fails`: broken YAML, no .lkg → returns error
- `TestLoadPolicyWithLKG_StaleLKG`: .lkg older than max_age_days → not used

## Conformance Checks

| Issue | Check Type | Check |
|-------|-----------|-------|
| 1 (inline extraction) | content_check | `{file: "internal/core/normalize.go", pattern: "extractHeredocBody"}` + `{pattern: "extractCommandSubstitution"}` |
| 1 (inline classify) | content_check | `{file: "internal/core/classify.go", pattern: "classifyInlineBodies"}` |
| 2 (URL inspection) | files_exist + content_check | `["internal/core/urlinspect.go"]` + `{pattern: "BlockedHostnames"}` |
| 2 (MCP URL) | content_check | `{file: "internal/core/mcpclassify.go", pattern: "InspectURLsInArgs"}` |
| 3 (judge inline) | content_check | `{file: "internal/judge/prompt.go", pattern: "InlineScriptBody"}` + `{file: "internal/core/classify.go", pattern: "InlineBody"}` |
| 4 (daemon) | files_exist | `["internal/judge/daemon.go"]` |
| 4 (provider close) | content_check | `{file: "internal/judge/provider.go", pattern: "Close"}` |
| 5 (policy LKG) | content_check | `{file: "internal/policy/policy.go", pattern: "LoadPolicyWithLKG"}` |
| 6 (config) | content_check | `{file: "internal/config/config.go", pattern: "URLTrustPolicy"}` |
| full suite | tests | `go test ./... -short -timeout 120s` |

## Verification

1. `fuse test classify 'bash <<EOF\nrm -rf /\nEOF'` → BLOCKED (inline body classified)
2. `fuse test classify 'bash <<EOF\necho hello\nEOF'` → CAUTION (heredoc, safe body)
3. `fuse test classify 'curl http://169.254.169.254/latest/meta-data/'` → BLOCKED (SSRF)
4. `fuse test classify 'curl http://localhost:8080/admin'` → BLOCKED (loopback)
5. `fuse test classify 'curl http://10.0.0.1:8500/'` → CAUTION (RFC1918)
6. `fuse test classify 'curl -k https://example.com'` → CAUTION (insecure cert)
7. `fuse test classify 'curl https://api.github.com/repos'` → SAFE (normal URL)
8. Corrupt `policy.yaml` → fuse warns and uses `.lkg`, classification still works
9. `fuse daemon start && fuse daemon status` → daemon running
10. `go test ./... -short -timeout 120s` → all pass

## Issues

### Issue 1: Inline Script Extraction (mvdan.cc/sh parser)
**Dependencies:** None
**Acceptance:** `extractHeredocBody` and `extractCommandSubstitutions` use shell parser,
`classifyInlineBodies` function in classify.go, heredoc bodies classified recursively,
truncated bodies keep structural APPROVAL, 11 new tests pass
**Description:** See Implementation §1

### Issue 2: URL/Network Awareness
**Dependencies:** None
**Acceptance:** `urlinspect.go` exists with `BlockedHostnames` (incl localhost, ::1),
`BlockedIPRanges` (loopback, link-local), `CautionIPRanges` (RFC1918, IPv6),
`InspectURLsInArgs` for MCP arguments, 10 new tests pass
**Description:** See Implementation §2

### Issue 3: Inline Body to Judge
**Dependencies:** Issue 1
**Acceptance:** `InlineScriptBody` in PromptContext, `InlineBody` in ClassifyResult,
`buildJudgeContext` populates from ClassifyResult, `BuildUserPrompt` includes it
**Description:** See Implementation §5

### Issue 4: Judge Daemon
**Dependencies:** None
**Acceptance:** `daemon.go` exists with Unix socket server + claude stream-json process,
`DaemonProvider` implements `Provider` interface with `Close()`, `Provider` interface has
`Close()`, daemon auto-disabled in hook mode, fallback to spawn-per-call, 4 new tests pass
**Description:** See Implementation §3

### Issue 5: Policy LKG Fallback (filesystem)
**Dependencies:** None
**Acceptance:** `LoadPolicyWithLKG` uses `policy.yaml.lkg` file (no DB), saves on success,
falls back on parse error, respects max_age_days, 4 new tests pass
**Description:** See Implementation §4

### Issue 6: Config Extensions + Integration
**Dependencies:** Issues 1, 2, 4, 5
**Acceptance:** URLTrustPolicy, PolicyLKG, Daemon fields in config, DefaultConfig populated,
all adapters use new config fields
**Description:** See Implementation §6

### Issue 7: Test Fixtures + Integration Tests
**Dependencies:** Issues 1-6
**Acceptance:** `testdata/fixtures/commands.yaml` has heredoc, URL, inline script entries.
All verification commands produce expected output.
**Description:** Add golden fixtures for new classification patterns

## Execution Order

**Wave 1** (parallel): Issue 1, Issue 2, Issue 4, Issue 5
- Issue 1 and Issue 2 both touch classify.go — extract changes into named functions
  (`classifyInlineBodies`, `classifyURLs`) called from `classifySingleCommand` to
  minimize merge conflict. **Serialize if any overlap in classifySingleCommand body.**

**Wave 2** (after Wave 1): Issue 3, Issue 6
**Wave 3** (after Wave 2): Issue 7

## Cross-Wave Shared Files

| File | Wave 1 Issues | Wave 2+ Issues | Mitigation |
|------|---------------|----------------|------------|
| `internal/core/classify.go` | Issue 1 (`classifyInlineBodies`), Issue 2 (`classifyURLs`) | Issue 7 (fixtures) | Named functions minimize conflict; serialize if needed |
| `internal/config/config.go` | — | Issue 6 | No Wave 1 dependency |
| `internal/judge/prompt.go` | — | Issue 3 | No Wave 1 dependency |
| `internal/judge/provider.go` | Issue 4 (`Close()` on interface) | — | Only Issue 4 touches this |

## v2 Scope vs Deferred

**v2 (this plan):**
- Heredoc body extraction + classification (via shell parser)
- Command substitution extraction (via shell parser)
- SSRF/cloud metadata + loopback + localhost + RFC1918 detection
- URL trust policy (domain list)
- Insecure cert detection (curl -k, wget --no-check-certificate)
- MCP tool argument URL inspection
- Judge daemon (Unix socket, stream-json, idle timeout)
- Policy LKG fallback (filesystem)
- Judge receives inline script content

**Deferred (v3+):**
- Exotic IP encoding normalization (hex, octal, decimal bypass)
- DNS-based SSRF bypass detection
- Progressive enforcement L4→L7 (HTTP method + path inspection for curl)
- Binary identity TOFU (hash verification for interpreters)
- Denial aggregation → policy recommendations
- Credential injection in MCP proxy
- Judge trigger on SAFE for network commands
- URL glob/regex patterns (v2 uses simple domain list)
- Ollama provider for fully offline judge

## Next Steps
- Run `/crank` for autonomous execution
- Or `/implement <issue>` for single issue
