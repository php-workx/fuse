# Plan: V2 Classification Pipeline — Inline Analysis, Network Awareness, Persistent Judge

## Context

Fuse v1's classification is structural — it detects patterns (`bash -c`, `<<EOF`, `$(...)`,
`curl | bash`) but never reads what's inside them. A `bash <<EOF\nrm -rf /\nEOF` triggers
APPROVAL because of the `<<EOF` pattern, not because of the `rm -rf /` inside. Meanwhile
`curl http://169.254.169.254/metadata` is SAFE because no rule matches the URL.

The LLM judge can evaluate semantic risk, but it only triggers on CAUTION/APPROVAL, has
1-3s latency per call (process spawn overhead), and doesn't receive inline script content.

This plan upgrades three layers in one cohesive pass:

1. **Inline script extraction** — extract heredoc bodies and `$()` contents, classify them
   through the existing pipeline, and pass them to the judge
2. **Network/URL awareness** — detect SSRF targets, cloud metadata endpoints, and untrusted
   destinations in curl/wget/httpie commands
3. **Persistent judge provider** — eliminate per-call process spawn overhead by keeping a
   long-lived `claude` process with stream-json interface
4. **Policy LKG (last-known-good)** — prevent silent rule loss when policy.yaml has errors

Source research: `docs/research/openshell-patterns-for-fuse.md` (patterns #1, #2, #4, #5)

## Baseline Audit

| Metric | Command | Result |
|--------|---------|--------|
| Classification pipeline LOC | `wc -l internal/core/{classify,normalize,safecmds}.go` | 1,846 lines |
| Judge package LOC | `wc -l internal/judge/*.go` (excl tests) | 506 lines |
| Inline pattern regexes | `grep -c reInline internal/core/classify.go` | 33 references |
| curl/wget rules | `grep -c "curl\|wget\|http" internal/policy/builtins_security.go` | 15 mentions |
| curl in safe verbs | `grep curl internal/core/safecmds.go` | NOT present |
| Schema version | `grep currentSchemaVersion internal/db/schema.go` | 6 |
| Heredoc test fixtures | `grep -c "heredoc" testdata/fixtures/commands.yaml` | 2 |
| Heredoc test cases | `grep -c "heredoc\|Heredoc" internal/core/*_test.go` | 2 |

## Files to Modify

| File | Change |
|------|--------|
| `internal/core/normalize.go` | Add `extractHeredocBody()`, `extractCommandSubstitution()` |
| `internal/core/classify.go` | Pass extracted inline bodies through classification + to judge |
| `internal/core/urlinspect.go` | **NEW** — URL parsing, SSRF detection, cloud metadata blocking |
| `internal/core/urlinspect_test.go` | **NEW** — URL inspection tests |
| `internal/core/normalize_test.go` | Add heredoc/cmd-substitution extraction tests |
| `internal/core/classify_test.go` | Add inline body classification tests |
| `internal/policy/builtins_security.go` | Add URL-aware rules (SSRF, insecure certs) |
| `internal/judge/persistent.go` | **NEW** — stream-json persistent provider |
| `internal/judge/persistent_test.go` | **NEW** — persistent provider tests |
| `internal/judge/provider.go` | Add `PersistentProvider` to detection logic |
| `internal/judge/prompt.go` | Add `InlineScriptBody` to PromptContext |
| `internal/config/config.go` | Add `URLTrustPolicy`, `PolicyLKG`, `PersistentProvider` config |
| `internal/adapters/hook.go` | Update `buildJudgeContext()` for inline bodies |
| `internal/policy/policy.go` | Add `LoadPolicyWithLKG()` |
| `internal/policy/policy_lkg_test.go` | **NEW** — LKG fallback tests |
| `internal/db/schema.go` | Migration v7: `policy_lkg` table |
| `testdata/fixtures/commands.yaml` | Add heredoc, URL, inline script fixtures |

## Boundaries

**Always:**
- Inline extraction is bounded (50KB max body, depth 3 max recursion)
- URL inspection is pattern-based (no DNS resolution, no network calls)
- Persistent provider falls back to spawn-per-call if stream dies
- LKG never overrides a successfully loaded policy
- BLOCKED decisions are never downgraded, regardless of extracted content
- All extracted content is scrubbed via `ScrubCredentials` before judge

**Ask First:**
- Default trusted domains list (github.com, npmjs.org, pypi.org?) or empty?
- Persistent provider: start on first judge call, or at fuse startup?
- LKG storage: SQLite table or filesystem (`policy.yaml.lkg`)?

**Never:**
- DNS resolution or network calls during classification
- Extract/classify beyond depth 3 (pathological nesting)
- Send raw inline script content to judge without scrubbing
- Replace the structural classifier — the judge augments, doesn't replace

## Implementation

### 1. Inline Script Extraction (`internal/core/normalize.go`)

**`extractHeredocBody(raw string, marker string) (string, bool)`**

Extracts the body between a heredoc marker and its closing delimiter. Called from
`classificationNormalizeRecursive` when a heredoc pattern is detected in the outer
command (not `$(cat <<...)` which is string quoting).

```go
// extractHeredocBody finds the content between <<MARKER and the closing MARKER line.
// Handles <<-, <<'MARKER', <<"MARKER". Returns (body, complete).
// Body is capped at MaxScriptBytes. If truncated, complete=false.
func extractHeredocBody(raw string, marker string) (string, bool)
```

Reuse: `classificationNormalizeRecursive` at `normalize.go:117` already handles
`bash -c` extraction. Heredoc extraction follows the same pattern — detect the
construct, extract the body, recursively classify it.

**`extractCommandSubstitution(cmd string) []string`**

Extracts `$(...)` contents using balanced-paren counting. Skips `$(cat <<...)`
which is already exempted as string quoting.

```go
// extractCommandSubstitution finds $(...) patterns and extracts the inner commands.
// Skips $(cat <<...) patterns (string quoting, not code execution).
// Returns slice of extracted commands. Max depth 2.
func extractCommandSubstitution(cmd string) []string
```

**Integration point:** `classificationNormalizeRecursive` at line 117. After
wrapper stripping and `bash -c` extraction, add heredoc body and `$()`
extraction as additional inner command sources.

### 2. Inline Body Classification (`internal/core/classify.go`)

Modify `classifySingleCommand` (line 303) to:
1. Call `extractHeredocBody` when heredoc is detected AND not `$(cat <<...)`
2. Call `extractCommandSubstitution` for non-cat `$(...)` patterns
3. Classify extracted bodies through the same pipeline (recursive)
4. Store extracted body in a new `ClassifyResult.InlineBody` field for the judge

**New field on `ClassifyResult`:**
```go
type ClassifyResult struct {
    // ... existing fields ...
    InlineBody string // extracted inline script content (heredoc body, $() content)
}
```

### 3. URL/Network Awareness (`internal/core/urlinspect.go`)

**NEW file.** Detects and classifies URLs in network commands.

```go
// BlockedNetworkTargets are always-blocked destinations (SSRF protection).
var BlockedNetworkTargets = []string{
    "169.254.169.254",     // AWS/GCP metadata
    "metadata.google.internal",
    "100.100.100.200",     // Alibaba metadata
    "169.254.170.2",       // ECS task metadata
}

// BlockedIPRanges are always-blocked IP ranges.
var BlockedIPRanges = []net.IPNet{
    // 127.0.0.0/8 (loopback)
    // 169.254.0.0/16 (link-local)
}

// InspectCommandURLs extracts and classifies URLs from curl/wget/httpie commands.
// Returns (decision, reason) if any URL is suspicious.
func InspectCommandURLs(cmd string) (Decision, string)
```

**Integration point:** `classifySingleCommand` at line 303, after inline script
detection (step 5) and before builtin evaluation (step 8). URL inspection runs
only for commands whose basename is `curl`, `wget`, `http`, or `httpie`.

**New builtin rules** in `builtins_security.go`:
- `builtin:net:ssrf-metadata` — cloud metadata endpoints → BLOCKED
- `builtin:net:ssrf-loopback` — loopback/link-local → APPROVAL
- `builtin:net:curl-insecure` — `-k`/`--insecure` → CAUTION
- `builtin:net:wget-insecure` — `--no-check-certificate` → CAUTION

### 4. Judge: Inline Content Delivery (`internal/judge/prompt.go`)

Extend `PromptContext` to carry extracted inline script bodies:

```go
type PromptContext struct {
    // ... existing fields ...
    InlineScriptBody string // extracted heredoc body or $() content (scrubbed)
}
```

`BuildUserPrompt` includes inline body with label:
```
Inline script body (extracted from heredoc):
<body content>
```

**Integration point:** `buildJudgeContext` in `hook.go` populates `InlineScriptBody`
from `ClassifyResult.InlineBody` after classification.

### 5. Persistent Judge Provider (`internal/judge/persistent.go`)

**NEW file.** Manages a long-lived `claude` process with stream-json I/O.

```go
type PersistentProvider struct {
    cmd     *exec.Cmd
    stdin   io.WriteCloser
    stdout  *bufio.Reader
    mu      sync.Mutex      // serializes writes
    model   string
}

func NewPersistentProvider(model string) (*PersistentProvider, error)
func (p *PersistentProvider) Query(ctx context.Context, systemPrompt, userPrompt string) (string, error)
func (p *PersistentProvider) Name() string
func (p *PersistentProvider) Close() error
```

Spawns: `claude -p --bare --input-format stream-json --output-format stream-json --model <model>`

Each `Query` writes a JSON message to stdin, reads the response from stdout.
The system prompt is sent once on first call (or per-call if it changes).

**Lifecycle:**
- Created lazily on first `MaybeJudge` call (not at fuse startup)
- Restarted automatically if process dies
- Closed when the adapter shuts down (codex-shell exit, hook process exit)
- Falls back to spawn-per-call `claudeProvider` on persistent failure

**Config:**
```yaml
llm_judge:
  mode: shadow
  provider: auto
  persistent: true    # NEW — use stream-json persistent provider
  model: claude-haiku-4-5-20251001
```

### 6. Policy LKG Fallback (`internal/policy/policy.go`)

```go
// LoadPolicyWithLKG tries to load policy.yaml. On success, saves as LKG.
// On failure, falls back to last known good policy if available and recent.
func LoadPolicyWithLKG(path string, db *db.DB) (*PolicyConfig, error)
```

**LKG storage:** SQLite table `policy_lkg` (schema v7):
```sql
CREATE TABLE IF NOT EXISTS policy_lkg (
    id              INTEGER PRIMARY KEY,
    created_at      TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    policy_hash     TEXT NOT NULL,
    policy_content  TEXT NOT NULL,
    is_valid        INTEGER NOT NULL DEFAULT 1
);
```

On successful `LoadPolicy`: upsert into `policy_lkg`.
On failed `LoadPolicy`: query `policy_lkg` for most recent valid entry, parse
and return it with a `slog.Warn` about fallback.

**Config:**
```yaml
policy_lkg:
  enabled: true           # default
  max_age_days: 7         # LKG must be recent
```

### 7. Config Extensions (`internal/config/config.go`)

```go
type Config struct {
    // ... existing fields ...
    URLTrustPolicy URLTrustPolicyConfig `yaml:"url_trust_policy"`
    PolicyLKG      PolicyLKGConfig      `yaml:"policy_lkg"`
}

type URLTrustPolicyConfig struct {
    TrustedDomains []string `yaml:"trusted_domains"` // empty = all untrusted
    BlockSchemes   []string `yaml:"block_schemes"`   // e.g., ["file", "ftp"]
}

type PolicyLKGConfig struct {
    Enabled    bool `yaml:"enabled"`      // default true
    MaxAgeDays int  `yaml:"max_age_days"` // default 7
}

// LLMJudgeConfig gets new field:
type LLMJudgeConfig struct {
    // ... existing fields ...
    Persistent bool `yaml:"persistent"` // use stream-json persistent provider
}
```

## Tests

**`internal/core/normalize_test.go`** — add:
- `TestExtractHeredocBody_Simple`: `bash <<EOF\necho hello\nEOF` → "echo hello"
- `TestExtractHeredocBody_Indented`: `bash <<-EOF\n\techo hello\nEOF` → "echo hello"
- `TestExtractHeredocBody_QuotedMarker`: `bash <<'EOF'` vs `bash <<"EOF"`
- `TestExtractHeredocBody_Truncated`: body > 50KB → truncated, complete=false
- `TestExtractHeredocBody_CatExempt`: `$(cat <<'EOF'...)` → not extracted (string quoting)
- `TestExtractCommandSubstitution_Simple`: `echo $(whoami)` → ["whoami"]
- `TestExtractCommandSubstitution_Nested`: `$(echo $(id))` → ["echo $(id)"] (depth 1)
- `TestExtractCommandSubstitution_CatExempt`: `$(cat <<'EOF'...)` → [] (skipped)

**`internal/core/urlinspect_test.go`** — add:
- `TestInspectCommandURLs_MetadataEndpoint`: `curl http://169.254.169.254/latest/meta-data/` → BLOCKED
- `TestInspectCommandURLs_Loopback`: `curl http://127.0.0.1:8080/admin` → APPROVAL
- `TestInspectCommandURLs_NormalURL`: `curl https://api.github.com/repos` → SAFE
- `TestInspectCommandURLs_InsecureFlag`: `curl -k https://example.com` → CAUTION
- `TestInspectCommandURLs_NonNetworkCommand`: `git commit` → no URL inspection

**`internal/judge/persistent_test.go`** — add:
- `TestPersistentProvider_QueryResponse`: mock process, send/receive JSON
- `TestPersistentProvider_Timeout`: slow response → context timeout
- `TestPersistentProvider_ProcessDeath`: process exits mid-query → error + restart
- `TestPersistentProvider_FallbackToSpawn`: persistent fails → spawn-per-call works

**`internal/policy/policy_lkg_test.go`** — add:
- `TestLoadPolicyWithLKG_Success`: valid policy → loads and stores LKG
- `TestLoadPolicyWithLKG_ParseError_FallsBackToLKG`: broken YAML → uses LKG
- `TestLoadPolicyWithLKG_NoLKG_Fails`: broken YAML, no LKG → returns error
- `TestLoadPolicyWithLKG_StaleLKG`: LKG older than max_age_days → not used

## Conformance Checks

| Issue | Check Type | Check |
|-------|-----------|-------|
| 1 (inline extraction) | content_check | `{file: "internal/core/normalize.go", pattern: "extractHeredocBody"}` |
| 2 (URL inspection) | files_exist | `["internal/core/urlinspect.go"]` |
| 3 (URL rules) | content_check | `{file: "internal/policy/builtins_security.go", pattern: "ssrf-metadata"}` |
| 4 (judge inline) | content_check | `{file: "internal/judge/prompt.go", pattern: "InlineScriptBody"}` |
| 5 (persistent provider) | files_exist | `["internal/judge/persistent.go"]` |
| 6 (policy LKG) | content_check | `{file: "internal/policy/policy.go", pattern: "LoadPolicyWithLKG"}` |
| 7 (schema v7) | content_check | `{file: "internal/db/schema.go", pattern: "applyV7"}` |
| 8 (config) | content_check | `{file: "internal/config/config.go", pattern: "URLTrustPolicy"}` |
| full suite | tests | `go test ./... -short -timeout 120s` |

## Verification

1. `fuse test classify 'bash <<EOF\nrm -rf /\nEOF'` → BLOCKED (inline body classified)
2. `fuse test classify 'bash <<EOF\necho hello\nEOF'` → CAUTION (heredoc, safe body)
3. `fuse test classify 'curl http://169.254.169.254/latest/meta-data/'` → BLOCKED (SSRF)
4. `fuse test classify 'curl -k https://example.com'` → CAUTION (insecure cert)
5. `fuse test classify 'curl https://api.github.com/repos'` → SAFE (normal URL)
6. Corrupt `policy.yaml` → fuse warns and uses LKG, classification still works
7. Persistent judge: first call ~1-2s (startup), subsequent calls ~500ms
8. `go test ./... -short -timeout 120s` → all pass

## Issues

### Issue 1: Inline Script Extraction
**Dependencies:** None
**Acceptance:** `extractHeredocBody` and `extractCommandSubstitution` implemented,
heredoc bodies classified through existing pipeline, 8 new tests pass
**Description:** See Implementation §1. Add extraction functions to normalize.go,
integrate into classificationNormalizeRecursive. Heredoc bodies flow through the
same classify pipeline as `bash -c` inner commands.

### Issue 2: URL/Network Awareness
**Dependencies:** None
**Acceptance:** `urlinspect.go` exists, SSRF metadata blocked, loopback detected,
insecure cert flagged, 5 new tests pass
**Description:** See Implementation §3. New file for URL parsing and SSRF
detection. New builtin rules in builtins_security.go. Integration in
classifySingleCommand between inline detection and builtin evaluation.

### Issue 3: Inline Body to Judge
**Dependencies:** Issue 1
**Acceptance:** `InlineScriptBody` field in PromptContext, buildJudgeContext
populates it from ClassifyResult.InlineBody, BuildUserPrompt includes it
**Description:** See Implementation §2 and §4. Extracted inline content flows
from classifier → ClassifyResult → buildJudgeContext → PromptContext → judge prompt.

### Issue 4: Persistent Judge Provider
**Dependencies:** None
**Acceptance:** `persistent.go` exists, stream-json lifecycle works, falls back
to spawn-per-call on failure, 4 new tests pass
**Description:** See Implementation §5. New PersistentProvider that keeps a
long-lived claude process. Config flag `persistent: true`. Lazy init on first
MaybeJudge call.

### Issue 5: Policy LKG Fallback
**Dependencies:** None
**Acceptance:** `LoadPolicyWithLKG` exists, schema v7 migration, LKG stored on
success, LKG used on parse failure, 4 new tests pass
**Description:** See Implementation §6. Schema v7 adds policy_lkg table.
LoadPolicyWithLKG wraps LoadPolicy with save/restore logic.

### Issue 6: Config Extensions
**Dependencies:** Issues 2, 4, 5 (needs all config consumers to exist)
**Acceptance:** URLTrustPolicy, PolicyLKG, Persistent fields in config,
DefaultConfig populated with sensible defaults
**Description:** See Implementation §7. Config struct extensions for all v2
features. Defaults: LKG enabled, persistent false (opt-in), empty trust list.

### Issue 7: Test Fixtures + Integration Tests
**Dependencies:** Issues 1-6
**Acceptance:** `testdata/fixtures/commands.yaml` has heredoc, URL, inline script
entries. All verification commands produce expected output.
**Description:** Add golden fixtures for new classification patterns. Verify
end-to-end via `fuse test classify`.

## Execution Order

**Wave 1** (parallel): Issue 1, Issue 2, Issue 4, Issue 5
**Wave 2** (after Wave 1): Issue 3, Issue 6
**Wave 3** (after Wave 2): Issue 7

## Cross-Wave Shared Files

| File | Wave 1 Issues | Wave 2+ Issues | Mitigation |
|------|---------------|----------------|------------|
| `internal/core/classify.go` | Issue 1 (inline body), Issue 2 (URL step) | Issue 7 (fixtures) | Different code sections, serialize if needed |
| `internal/config/config.go` | — | Issue 6 | No Wave 1 dependency |
| `internal/judge/prompt.go` | — | Issue 3 | No Wave 1 dependency |
| `internal/adapters/hook.go` | — | Issue 3 | No Wave 1 dependency |

**Issue 1 and Issue 2 both modify classify.go** but in different sections (inline
detection vs URL inspection step). Low conflict risk, but serialize if the changes
overlap during implementation.

## v2 Scope vs Deferred

**v2 (this plan):**
- Heredoc body extraction + classification
- Command substitution extraction
- SSRF/cloud metadata detection
- URL trust policy (domain list)
- Insecure cert detection (curl -k, wget --no-check-certificate)
- Persistent judge provider (stream-json)
- Policy LKG fallback
- Judge receives inline script content

**Deferred (v3+):**
- Progressive enforcement L4→L7 (HTTP method + path inspection for curl)
- Binary identity TOFU (hash verification for interpreters)
- Denial aggregation → policy recommendations
- Credential injection in MCP proxy
- Pipe chain content extraction (`curl URL | python3` → what does the python do?)
- Judge trigger on SAFE for network commands (needs persistent provider first for latency)
- URL glob/regex patterns (v2 uses simple domain list)
- Ollama provider for fully offline judge

## Next Steps
- Run `/pre-mortem` to validate plan
- Run `/crank` for autonomous execution
- Or `/implement <issue>` for single issue
