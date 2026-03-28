# Plan: V2 Classification Pipeline — Inline Analysis, Network Awareness, Policy LKG

## Context

Fuse v1's classification is structural — it detects patterns (`bash -c`, `<<EOF`, `$(...)`,
`curl | bash`) but never reads what's inside them. A `bash <<EOF\nrm -rf /\nEOF` triggers
APPROVAL because of the `<<EOF` pattern, not because of the `rm -rf /` inside. Meanwhile
`curl http://169.254.169.254/metadata` is SAFE because no rule matches the URL.

The LLM judge can evaluate semantic risk, but it only triggers on CAUTION/APPROVAL, has
1-3s latency per call (process spawn overhead), and doesn't receive inline script content.

This plan upgrades the classification pipeline in three areas:

1. **Inline script extraction** — extract heredoc bodies and `$()` contents using the
   `mvdan.cc/sh` shell parser (already a dependency), classify them through the existing
   pipeline, and pass them to the judge
2. **Network/URL awareness** — detect SSRF targets (cloud metadata, loopback, private
   networks), untrusted destinations, and insecure flags in curl/wget/httpie commands
3. **Policy LKG (last-known-good)** — filesystem-based fallback to prevent silent rule
   loss when policy.yaml has errors

**Judge daemon deferred to v2.1.** An ecosystem review (ECO-001 through ECO-011)
found critical issues with the persistent `claude` process approach:
- ECO-001: Multi-turn history contaminates classifications across workspaces
- ECO-002: Init handshake ordering differs from plan assumption on live CLI
- ECO-004: Judge process isn't sterile (loads hooks, plugins, skills)
- ECO-010: Memory growth (~34MB over 5 turns) makes global daemon risky on laptops
The v2.1 daemon will either use direct Anthropic API calls (truly stateless) or a
supervisor managing fresh per-query worker processes. See Deferred section.

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
| `internal/judge/prompt.go` | Add `InlineScriptBody` to PromptContext |
| `internal/config/config.go` | Add `URLTrustPolicy`, `PolicyLKG` config |
| `internal/adapters/hook.go` | Update `buildJudgeContext()` for inline bodies |
| `internal/policy/policy.go` | Add `LoadPolicyWithLKG()` using filesystem |
| `internal/policy/policy_lkg_test.go` | **NEW** — LKG fallback tests |
| `testdata/fixtures/commands.yaml` | Add heredoc, URL, inline script fixtures |

## Boundaries

**Always:**
- Hard-cap raw command length (64KB, matching existing `maxInputSize`) before parser (SEC-007)
- Inline extraction uses `mvdan.cc/sh` parser (not manual tokenization) for correctness
- Extraction is bounded (50KB max body, depth 3 max recursion — shared counter)
- Track `ExtractionIncomplete` flag when body truncated or depth exhausted (SEC-009)
- When `ExtractionIncomplete`, structural decision stands — never downgrade (SEC-009)
- Fail-closed on parse errors: unparseable syntax → APPROVAL with reason (SEC-008)
- URL inspection is pattern-based with `net/url.Parse` + `net.ParseIP` (no DNS resolution)
- URLs with shell expansion tokens (`$`, `${`, backticks, `$()`) → APPROVAL (SEC-001)
- Non-allowlisted hostnames in network commands → CAUTION minimum (SEC-004)
- Redirect-following flags (`curl -L`, `wget` default, `httpie --follow`) → CAUTION (SEC-003)
- URL scanning runs on extracted inline bodies too, not just top-level commands (SEC-006)
- Default-blocked URL schemes: `file`, `gopher`, `dict`, `ftp`, `ftps`, `scp`, `sftp`, `tftp`, `ldap`, `ldaps`, `smb` (SEC-011)
- Non-canonical numeric hosts (hex, octal, decimal IP) → CAUTION (SEC-002 partial fix)
- Scrub inline bodies with expanded patterns: PEM blocks, SSH keys, vendor tokens,
  URL userinfo, high-entropy blobs. Skip sending body to judge if still secret-heavy (SEC-010)
- LKG is filesystem-based (`policy.yaml.lkg`), no schema migration needed
- LKG fallback is loud: stderr warning + `fuse doctor` reports active policy hash (ECO-009)
- BLOCKED decisions are never downgraded, regardless of extracted content

**Never:**
- DNS resolution or network calls during classification
- Manual heredoc/`$()` parsing — use the shell parser
- Send raw inline script content to judge without scrubbing
- Replace the structural classifier — the judge augments, doesn't replace
- Store LKG in SQLite (policy loads before DB is available)
- Leave URLs with shell variables as SAFE — always escalate (SEC-001)
- Leave unknown hostnames in network commands as SAFE — always CAUTION (SEC-004)
- Gate URL scanning on command basename only — scan all extracted bodies (SEC-006)
- Skip pre-parse input size validation — cap before parser (SEC-007)
- Allow downgrade when extraction is incomplete (SEC-009)

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

**Pre-parse safety (SEC-007):** Before feeding any command to `mvdan.cc/sh`, enforce
the existing `maxInputSize` cap (64KB, `classify.go:11`). Commands exceeding this are
already rejected as BLOCKED. Additionally, recover from parser panics (the mvdan.cc/sh
parser is well-tested but untrusted input demands defense-in-depth). On parse error or
panic → fail-closed to APPROVAL with reason "unparseable inline script" (SEC-008).

**Integration point:** New function `classifyInlineBodies(cmd string, evaluator
PolicyEvaluator, cwd string)` in `classify.go`. Called from `classifySingleCommand`
after `detectInlineScript` (step 5). Extracts bodies, classifies them recursively
through the same pipeline. Stores bodies in `ClassifyResult.InlineBody` for the judge.
Also runs `InspectCommandURLs` on each extracted body (SEC-006).

**ExtractionIncomplete flag (SEC-009):** New field on `ClassifyResult`:
```go
type ClassifyResult struct {
    // ... existing fields ...
    InlineBody           string // extracted inline script content
    ExtractionIncomplete bool   // true when body truncated or depth exhausted
}
```
When `ExtractionIncomplete` is true:
- The structural decision (APPROVAL from heredoc/inline pattern) stands
- The judge may still evaluate the partial body but CANNOT downgrade
- `MaybeJudge` checks `result.ExtractionIncomplete` and skips downgrade even if the
  judge says SAFE with high confidence

**Depth limiting:** The extraction functions share the recursion counter with
`classificationNormalizeRecursive` (`maxRecursionDepth = 3`). Heredoc extraction
counts as 1 depth level. `$()` extraction counts as 1 depth level. Combined nesting
beyond depth 3 stops extraction, sets `ExtractionIncomplete`, structural classification
stands.

### 2. URL/Network Awareness (`internal/core/urlinspect.go`)

**NEW file.** Detects and classifies URLs in network commands.

**Pre-mortem fixes applied:**
- pm-103: Added `localhost`, `::1`, `0.0.0.0` to blocked hostnames. Added RFC1918 ranges as CAUTION.
- pm-103: Use `net/url.Parse` for URL decoding, `net.ParseIP` for IP normalization after decoding.
- pm-105: Also scan MCP tool arguments for URLs (add `InspectURLsInArgs` for ClassifyMCPTool).
- pm-110: Skip URL inspection when URL contains unresolved shell variables (`$`, `{`).

```go
// BlockedHostnames are always-blocked destination names → BLOCKED.
// Includes trailing-dot and case variants (matched after lowercasing + dot-trimming).
var BlockedHostnames = []string{
    "169.254.169.254",          // AWS/GCP metadata (IPv4)
    "metadata.google.internal", // GCP metadata (hostname)
    "100.100.100.200",          // Alibaba metadata
    "169.254.170.2",            // ECS task metadata
    "192.0.0.192",              // OCI metadata
    "168.63.129.16",            // Azure WireServer / IMDS
    "localhost",                // loopback
    "0.0.0.0",                  // all interfaces
}

// BlockedIPRanges → BLOCKED.
var BlockedIPRanges = []net.IPNet{
    parseCIDR("127.0.0.0/8"),          // loopback (IPv4)
    parseCIDR("169.254.0.0/16"),       // link-local (IPv4)
    parseCIDR("::1/128"),              // loopback (IPv6)
    parseCIDR("fe80::/10"),            // link-local (IPv6)
    parseCIDR("::ffff:169.254.0.0/112"), // IPv4-mapped link-local
    parseCIDR("fd00:ec2::254/128"),    // AWS IMDS IPv6
    parseCIDR("fd20:ce::254/128"),     // GCP metadata IPv6
}

// CautionIPRanges → CAUTION (private networks, carrier-grade NAT, benchmarking).
var CautionIPRanges = []net.IPNet{
    parseCIDR("10.0.0.0/8"),      // RFC1918
    parseCIDR("172.16.0.0/12"),   // RFC1918
    parseCIDR("192.168.0.0/16"),  // RFC1918
    parseCIDR("100.64.0.0/10"),   // carrier-grade NAT (RFC6598)
    parseCIDR("198.18.0.0/15"),   // benchmarking (RFC2544)
    parseCIDR("fc00::/7"),        // IPv6 unique-local (private)
}

// BlockedSchemes are always-blocked URL schemes → BLOCKED.
// These are not configurable — dangerous schemes are never safe.
var BlockedSchemes = []string{
    "file", "gopher", "dict", "ftp", "ftps",
    "scp", "sftp", "tftp", "ldap", "ldaps", "smb",
}

// InspectCommandURLs extracts URLs from a command string and classifies them.
// Runs on any command text, not gated by basename (SEC-006).
func InspectCommandURLs(cmd string) (Decision, string)

// InspectURLsInArgs scans MCP tool argument strings for suspicious URLs.
// Walks nested JSON to find string values containing URLs.
func InspectURLsInArgs(args map[string]interface{}) (Decision, string)

// isShellExpansion returns true if a URL host contains shell variable syntax.
// These URLs cannot be statically inspected → force APPROVAL (SEC-001).
func isShellExpansion(host string) bool

// isNonCanonicalNumericHost detects hex/octal/decimal IP encodings → CAUTION (SEC-002).
func isNonCanonicalNumericHost(host string) bool

// hasRedirectFlags returns true if the command enables HTTP redirects (SEC-003).
func hasRedirectFlags(cmd string) bool
```

**URL pre-processing pipeline:**
1. Parse URL with `net/url.Parse`
2. Lowercase host, trim trailing dots
3. Strip `userinfo@` (SEC-010: also scrub before logging)
4. Check scheme against `BlockedSchemes` → BLOCKED
5. If host contains shell expansion tokens (`$`, `` ` ``) → APPROVAL (SEC-001)
6. If host is non-canonical numeric (hex/octal/decimal/short-form), decode it to a canonical IP first
7. Resolve host against `BlockedHostnames` → BLOCKED
8. Parse IP with `net.ParseIP`, check `BlockedIPRanges` → BLOCKED
9. Check `CautionIPRanges` (RFC1918, carrier-grade NAT) → CAUTION
10. If redirect flags present and host not in `TrustedDomains` → CAUTION (SEC-003)
11. If host not in `TrustedDomains` and is a network command → CAUTION (SEC-004)

**Integration points:**
- `classifySingleCommand` in classify.go: new `classifyURLs(cmd)` function.
  For commands with basename `curl`, `wget`, `http`, `httpie`: full URL pipeline.
  For ALL commands and extracted inline bodies: scan for URLs matching blocked
  hostnames/IPs (SEC-006). This catches `python -c "urllib.request.urlopen(...)"`.
- `ClassifyMCPTool` in mcpclassify.go: call `InspectURLsInArgs(args)` which walks
  nested JSON to find URL strings.

**Known v2 limitations (documented, deferred to v3):**
- DNS-based bypasses (`attacker.com` → `169.254.169.254`). Unknown hostnames in network
  commands get CAUTION (not SAFE), which limits blast radius. Bounded DNS resolution in v3.
- HTTP redirect chain following. Redirect flags are flagged as CAUTION. Runtime redirect
  blocking is v3.
- Multi-language URL extraction (Python `urlopen`, Go `http.Get`). URLs in extracted
  inline bodies are scanned for blocked IPs/hostnames but language-specific API detection
  is v3.

### 3. Policy LKG Fallback (`internal/policy/policy.go`)

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
(default 7 days), parse and return with **loud warning** (ECO-009):
- `slog.Warn` on every hook invocation while LKG is active
- `fuse doctor` shows "WARNING: using fallback policy (policy.yaml has errors)"
- `fuse doctor` shows active policy hash so users can verify which rules are live
- stderr message on CAUTION/APPROVAL decisions: "[fuse] WARNING: using fallback policy"

**Config:**
```yaml
policy_lkg:
  enabled: true           # default
  max_age_days: 7         # LKG must be recent
```

### 4. Judge Inline Content (`internal/judge/prompt.go`)

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

### 5. Config Extensions (`internal/config/config.go`)

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
- `TestInspectURLs_AzureWireServer`: `curl http://168.63.129.16/metadata` → BLOCKED
- `TestInspectURLs_OracleMetadata`: `curl http://192.0.0.192/opc/v2/` → BLOCKED
- `TestInspectURLs_AWSIPv6Metadata`: `curl http://[fd00:ec2::254]/latest/` → BLOCKED
- `TestInspectURLs_RFC1918`: `curl http://10.0.0.1:8500/secrets` → CAUTION
- `TestInspectURLs_CarrierGradeNAT`: `curl http://100.64.0.1/` → CAUTION
- `TestInspectURLs_NormalURL`: `curl https://api.github.com/repos` → SAFE
- `TestInspectURLs_InsecureFlag`: `curl -k https://example.com` → CAUTION
- `TestInspectURLs_URLWithCredentials`: `curl http://admin:pass@169.254.169.254/` → BLOCKED
- `TestInspectURLs_ShellVariable`: `curl https://$HOST/api` → APPROVAL (SEC-001)
- `TestInspectURLs_ShellSubstitution`: `curl http://$(echo host)/` → APPROVAL (SEC-001)
- `TestInspectURLs_RedirectFlag`: `curl -L https://untrusted.com` → CAUTION (SEC-003)
- `TestInspectURLs_WgetFollowsRedirects`: `wget https://untrusted.com` → CAUTION (SEC-003)
- `TestInspectURLs_UnknownHostname`: `curl https://random-host.tld/` → CAUTION (SEC-004)
- `TestInspectURLs_NonCanonicalIP`: `curl http://0x7f000001/` → BLOCKED (loopback via decoded IP)
- `TestInspectURLs_BlockedScheme_File`: `curl file:///etc/passwd` → BLOCKED (SEC-011)
- `TestInspectURLs_BlockedScheme_Gopher`: `curl gopher://127.0.0.1:25/` → BLOCKED (SEC-011)
- `TestInspectURLs_TrailingDotHostname`: `curl http://metadata.google.internal./` → BLOCKED
- `TestInspectURLs_NonNetworkCommand`: `git commit` → no inspection
- `TestInspectURLs_MCPArguments`: MCP tool with metadata URL arg → BLOCKED
- `TestInspectURLs_InlineBodyURL`: Python heredoc with `urlopen("http://169.254.169.254")` → BLOCKED (SEC-006)

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
| 4 (policy LKG) | content_check | `{file: "internal/policy/policy.go", pattern: "LoadPolicyWithLKG"}` |
| 5 (config) | content_check | `{file: "internal/config/config.go", pattern: "URLTrustPolicy"}` |
| full suite | tests | `go test ./... -short -timeout 120s` |

## Verification

1. `fuse test classify 'bash <<EOF\nrm -rf /\nEOF'` → BLOCKED (inline body classified)
2. `fuse test classify 'bash <<EOF\necho hello\nEOF'` → CAUTION (heredoc, safe body)
3. `fuse test classify 'curl http://169.254.169.254/latest/meta-data/'` → BLOCKED (SSRF)
4. `fuse test classify 'curl http://localhost:8080/admin'` → BLOCKED (loopback)
5. `fuse test classify 'curl http://10.0.0.1:8500/'` → CAUTION (RFC1918)
6. `fuse test classify 'curl -k https://example.com'` → CAUTION (insecure cert)
7. `fuse test classify 'curl https://api.github.com/repos'` → SAFE (normal URL)
8. Corrupt `policy.yaml` → fuse warns LOUDLY (stderr + doctor) and uses `.lkg`
9. `go test ./... -short -timeout 120s` → all pass

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
**Acceptance:** `InlineScriptBody` in PromptContext, `InlineBody` and
`ExtractionIncomplete` in ClassifyResult, `buildJudgeContext` populates from
ClassifyResult, `BuildUserPrompt` includes it, `MaybeJudge` skips downgrade
when `ExtractionIncomplete` is true
**Description:** See Implementation §4

### Issue 4: Policy LKG Fallback (filesystem)
**Dependencies:** None
**Acceptance:** `LoadPolicyWithLKG` uses `policy.yaml.lkg` file (no DB), saves on success,
falls back on parse error with loud warning (stderr + doctor), respects max_age_days,
4 new tests pass
**Description:** See Implementation §3

### Issue 5: Config Extensions + Integration
**Dependencies:** Issues 1, 2, 4
**Acceptance:** URLTrustPolicy, PolicyLKG fields in config, DefaultConfig populated
with secure defaults (blocked schemes, LKG enabled), all adapters use new config fields
**Description:** See Implementation §5

### Issue 6: Test Fixtures + Integration Tests
**Dependencies:** Issues 1-5
**Acceptance:** `testdata/fixtures/commands.yaml` has heredoc, URL, inline script entries.
All verification commands produce expected output.
**Description:** Add golden fixtures for new classification patterns

## Execution Order

**Wave 1** (parallel): Issue 1, Issue 2, Issue 4
- Issue 1 and Issue 2 both touch classify.go — extract changes into named functions
  (`classifyInlineBodies`, `classifyURLs`) called from `classifySingleCommand` to
  minimize merge conflict. **Serialize if any overlap in classifySingleCommand body.**

**Wave 2** (after Wave 1): Issue 3, Issue 5
**Wave 3** (after Wave 2): Issue 6

## Cross-Wave Shared Files

| File | Wave 1 Issues | Wave 2+ Issues | Mitigation |
|------|---------------|----------------|------------|
| `internal/core/classify.go` | Issue 1 (`classifyInlineBodies`), Issue 2 (`classifyURLs`) | Issue 6 (fixtures) | Named functions minimize conflict; serialize if needed |
| `internal/config/config.go` | — | Issue 5 | No Wave 1 dependency |
| `internal/judge/prompt.go` | — | Issue 3 | No Wave 1 dependency |

## v2 Scope vs Deferred

**v2 (this plan):**
- Heredoc body extraction + classification (via shell parser)
- Command substitution extraction (via shell parser)
- SSRF/cloud metadata + loopback + localhost + RFC1918 detection
- Default-blocked URL schemes (file, gopher, dict, ftp, scp, ldap, smb)
- Shell variables in URLs → APPROVAL (not SAFE)
- Non-allowlisted hostnames → CAUTION (not SAFE)
- Redirect flags → CAUTION
- URL trust policy (domain list)
- Insecure cert detection (curl -k, wget --no-check-certificate)
- MCP tool argument URL inspection
- URL scanning on extracted inline bodies (not just top-level commands)
- Pre-parse input cap + parser panic recovery
- ExtractionIncomplete flag prevents judge downgrade on partial bodies
- Policy LKG fallback (filesystem, loud warnings)
- Judge receives inline script content
- Expanded credential scrubbing for inline bodies

**Deferred (v2.1 — judge latency optimization):**
- Judge daemon with stateless per-query workers (not shared conversation)
  - ECO-001: Multi-turn history contamination requires fresh process per query
  - ECO-004: Need sterile judge environment (no hooks, plugins, skills)
  - Options: direct Anthropic API from Go, or supervisor managing worker pool
- `Close()` on Provider interface (needed for daemon cleanup)

**Deferred (v3+):**
- Full IP canonicalization (hex/octal/decimal → canonical dotted-quad)
- DNS-based SSRF bypass detection (bounded A/AAAA resolution before execution)
- HTTP redirect chain following (runtime redirect blocking for curl -L)
- Multi-language URL extraction (Python urlopen, Go http.Get, Node fetch)
- Progressive enforcement L4→L7 (HTTP method + path inspection for curl)
- Binary identity TOFU (hash verification for interpreters)
- Denial aggregation → policy recommendations
- Credential injection in MCP proxy
- Server-specific MCP classifiers keyed by full tool name (ECO-008)
- Judge trigger on SAFE for network commands
- URL glob/regex patterns (v2 uses simple domain list)
- Differential testing: parser extraction vs bash -n for shell syntax edge cases
- Ollama provider for fully offline judge

## Next Steps
- Run `/crank` for autonomous execution
- Or `/implement <issue>` for single issue
