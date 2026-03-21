---
type: research
date: 2026-03-20
source: OpenShell (NVIDIA/OpenShell) codebase analysis
status: captured
next: /plan when ready for v2 policy improvements
---

# Research: OpenShell Patterns Applicable to Fuse

## Executive Summary

OpenShell is an enterprise container sandbox for AI agents (kernel isolation, OPA policies, Kubernetes). Fuse is a lightweight local approval gate (shell classification, user prompts, zero infrastructure). The overlap is in the **policy and decision layer**. Six patterns from OpenShell could strengthen fuse's classification and policy system without adding deployment complexity.

## Key Findings

### 1. SSRF-Aware URL Classification

**OpenShell pattern:** Always-blocks loopback (127.0.0.0/8), link-local (169.254.0.0/16), metadata endpoints (169.254.169.254). Blocks private IPs (RFC 1918) by default with explicit allowlist override.

**Fuse gap:** `curl`/`wget` classification is command-pattern only. `curl http://169.254.169.254/latest/meta-data/` (AWS metadata SSRF) is classified as SAFE because no builtin rule matches the URL.

**Proposed change:** Add IP/URL awareness to the classification pipeline. Hardcode always-blocked ranges (loopback, link-local, cloud metadata). Add CAUTION for private IP ranges. Could be a new classification layer between builtin rules and the default-SAFE fallback.

**Complexity:** Medium. Needs URL parsing in the classifier, IP range checking. ~200 LOC in `internal/core/`.

**Files:** `internal/core/classify.go` (new layer), `internal/core/url_classify.go` (new)

### 2. Progressive Enforcement (L4 → L7)

**OpenShell pattern:** L4 allows/blocks connections by host:port. If L4 allows, L7 inspects individual HTTP requests (method + path matching). Enforcement modes: audit (log but allow) vs enforce (deny).

**Fuse analogy:** Currently fuse has one decision per command. Progressive enforcement would mean: broad SAFE at the command level, then deeper inspection of arguments. Example: `curl` is SAFE, but `curl -X DELETE https://api.production.com/users` should be APPROVAL.

**Proposed change:** Add argument-aware classification for network commands. Inspect URL targets, HTTP methods in flags (-X, --request), and data payloads (-d, --data) for `curl`, `wget`, `httpie`.

**Complexity:** Medium-high. Needs flag parsing for curl/wget/httpie. ~300 LOC.

**Files:** `internal/core/classify.go` (new layer), `internal/core/network_classify.go` (new)

### 3. Binary Identity TOFU (Trust-on-First-Use)

**OpenShell pattern:** SHA256 hash of binary on first network request, cached. Subsequent requests verify the binary hasn't been tampered with. Prevents binary substitution attacks mid-session.

**Fuse analogy:** An agent could `mv /tmp/malicious /usr/local/bin/python` and then `python evil.py` would be classified based on the `python` command name, not the actual binary. Fuse trusts command names blindly.

**Proposed change:** Optional binary hash verification for interpreters (python, node, bash, ruby, perl). On first invocation, hash the resolved binary path. On subsequent invocations, verify the hash. BLOCKED if hash changes mid-session.

**Complexity:** Low-medium. ~150 LOC. Needs `exec.LookPath` + file hashing. Session-scoped cache.

**Files:** `internal/core/binary_verify.go` (new), `internal/adapters/hook.go` (integration point)

**Note:** This is defense-in-depth. The primary threat model assumes the agent is semi-trusted. Binary TOFU adds protection against supply-chain or environment tampering.

### 4. Policy Versioning with Last-Known-Good (LKG) Fallback

**OpenShell pattern:** Policy revisions stored with status: pending → loaded → failed → superseded. If a new policy fails validation, the system falls back to the last successfully loaded version. Hot-reload with version tracking.

**Fuse gap:** If `policy.yaml` has a syntax error, `LoadPolicy` returns an error, and fuse falls back to no-policy (builtin rules only). The user's custom rules silently disappear.

**Proposed change:** Store last-known-good policy alongside current. If `LoadPolicy` fails, warn and use LKG. Track policy version (hash of content) to detect changes.

**Complexity:** Low. ~100 LOC. Store LKG as `policy.yaml.lkg` or in SQLite.

**Files:** `internal/policy/policy.go` (LoadPolicy LKG logic), `internal/config/` (LKG path)

### 5. Denial Aggregation → Policy Recommendations

**OpenShell pattern:** `DenialAggregator` deduplicates repeated denials by (host, port, binary), accumulates counts, and generates policy recommendation YAML from observed denials.

**Fuse analogy:** Users repeatedly approve/deny the same commands. Fuse has no mechanism to suggest "you've approved `terraform plan` 12 times this session — add it as a permanent SAFE rule?"

**Proposed change:** Aggregate approval/denial patterns from the events table. Surface recommendations via `fuse doctor` or the TUI stats view. Optionally auto-generate `policy.yaml` additions.

**Complexity:** Medium. Aggregation query + recommendation format. ~200 LOC.

**Files:** `internal/db/recommendations.go` (new), `internal/cli/doctor.go` (integration), `internal/tui/stats_view.go` (display)

### 6. Credential Stripping in MCP Proxy

**OpenShell pattern:** Inference router intercepts requests to `inference.local`, rewrites Authorization headers with real API keys from the provider system. Agent never sees actual credentials.

**Fuse analogy:** The MCP proxy (`fuse proxy mcp`) forwards requests to downstream MCP servers. If those servers require API keys, the agent currently needs the keys in its environment. Fuse could inject them transparently.

**Proposed change:** Add credential injection to the MCP proxy config. `config.yaml` maps downstream server names to credential sources (env var, file, keychain). Fuse strips agent-provided auth and injects the configured credential.

**Complexity:** Medium. Config schema change + proxy header rewriting. ~200 LOC.

**Files:** `internal/config/config.go` (schema), `internal/adapters/mcpproxy.go` (injection logic)

## Priority Ranking

| # | Pattern | Impact | Complexity | Recommendation |
|---|---------|--------|------------|----------------|
| 1 | SSRF-aware URL classification | High (security) | Medium | v2 priority |
| 2 | Policy versioning + LKG | High (reliability) | Low | v2 priority |
| 3 | Denial aggregation → recommendations | Medium (UX) | Medium | v2 |
| 4 | Progressive enforcement (L4→L7) | Medium (security) | Medium-high | v2 or v3 |
| 5 | Binary identity TOFU | Low-medium (defense-in-depth) | Low-medium | v3 |
| 6 | Credential stripping in MCP proxy | Low (niche) | Medium | v3 or later |

## Relationship to Current Work

These patterns build on the existing classification pipeline and don't conflict with the TUI, concurrent MCP, or tag_override work already shipped. The SSRF and LKG patterns are the most impactful for the least effort.

## Next Steps

When ready to implement:
1. `/plan` with this research as input — decompose into issues
2. SSRF classification and LKG fallback can be Wave 1 (independent, different files)
3. Denial aggregation depends on having enough event history to be useful
4. Progressive enforcement and binary TOFU are larger efforts for a later wave
