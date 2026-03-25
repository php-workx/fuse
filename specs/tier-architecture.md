# Tier Architecture Redesign

**Status:** Draft
**Date:** 2026-03-25
**Scope:** Decision tier semantics, profile system, judge integration, rule migration

## 1. Problem Statement

Fuse has four decision tiers (SAFE, CAUTION, APPROVAL, BLOCKED) but only two enforcement behaviors: allow (exit 0) and deny (exit 2). CAUTION and SAFE both exit 0 — they are operationally identical. APPROVAL and BLOCKED both exit 2 — one prompts for approval, the other hard-denies.

This creates three problems:

1. **CAUTION is meaningless to users.** It logs but doesn't protect. Users don't monitor logs in real-time, so CAUTION provides no security benefit beyond post-incident forensics.

2. **APPROVAL is over-assigned.** 90+ builtin rules statically produce APPROVAL. This means the user is interrupted constantly. Many of these commands are safe in context (e.g., `terraform destroy` in a dev workspace, `curl -X DELETE` on a test API). Static rules cannot distinguish context.

3. **The LLM judge has no clear role.** The judge can triage CAUTION and APPROVAL, but CAUTION is already auto-approved and APPROVAL already interrupts. The judge doesn't change the user experience in either case unless in active mode, which few users enable.

## 2. Design Decisions

The following decisions were made during the adversarial security review on 2026-03-25:

**D1: CAUTION becomes the judge triage tier.** Most commands that need review go to CAUTION. The judge reviews them and either confirms they're safe (auto-approve) or escalates to APPROVAL (ask user). Without the judge, CAUTION behavior is configurable.

**D2: APPROVAL shrinks to critical-but-not-blocker.** APPROVAL means "this probably needs human eyes." With the judge enabled, the judge reviews APPROVAL first and can downgrade if confident. Without the judge, APPROVAL always asks the user.

**D3: BLOCKED stays thin and non-negotiable.** True non-negotiables only. No context makes these OK. Expanded slightly to include patterns currently at APPROVAL that are genuinely never acceptable (reverse shells, obfuscation-to-exec, system credential modification).

**D4: Three user profiles provide sensible defaults.** Relaxed, Balanced, Strict. Selected during install, configurable afterward. Each profile sets the judge mode and fallback behavior for CAUTION.

**D5: The judge can downgrade APPROVAL to CAUTION but never to SAFE.** This prevents prompt injection from completely removing protection. APPROVAL -> CAUTION means auto-approved with logging. APPROVAL -> SAFE is prohibited.

## 3. New Tier Model

### 3.1 Tier Definitions

| Tier | Meaning | Enforcement |
|------|---------|-------------|
| **SAFE** | Routine command, no risk | Auto-approve, minimal logging |
| **CAUTION** | Suspicious but probably fine | Judge triage tier (see profile matrix) |
| **APPROVAL** | Critical, probably needs human eyes | Judge can review first (see profile matrix) |
| **BLOCKED** | Non-negotiable hard deny | Always denied, no override, no judge |

### 3.2 Profile Behavior Matrix

```
                 Relaxed              Balanced             Strict
--------------------------------------------------------------------------
BLOCKED      deny                  deny                  deny

APPROVAL     always ask user       judge reviews first:  always ask user
                                   - confirms -> ask
                                   - downgrades -> CAUTION behavior

CAUTION      log only              judge reviews:        judge reviews:
             (auto-approve)        - escalates -> ask    - escalates -> ask
                                   - confirms -> allow   - confirms -> allow

SAFE         allow                 allow                 allow

Judge        off                   on (both tiers)       on (CAUTION only)
```

### 3.3 Fallback Behavior (Judge Unavailable)

When the judge is enabled but unavailable (timeout, rate-limited, error), the system must decide how to handle CAUTION and APPROVAL commands. This is the **judge fallback** behavior:

| Tier | Judge fails in Balanced | Judge fails in Strict |
|------|------------------------|----------------------|
| APPROVAL | Ask user (safe fallback) | Ask user (always — judge doesn't review APPROVAL in strict) |
| CAUTION | Auto-approve with log | Auto-approve with log |

Rationale: judge failure on CAUTION should not block the user (fail-open for triage tier). Judge failure on APPROVAL in balanced mode falls back to asking the user (fail-closed for critical tier).

## 4. Profile System

### 4.1 Configuration Schema

Add to `config.yaml`:

```yaml
# Profile: relaxed | balanced | strict | custom
# Sets defaults for judge and caution behavior.
# Individual settings below override the profile defaults.
profile: balanced

# Judge configuration (defaults set by profile)
llm_judge:
  mode: active           # off | shadow | active
  provider: auto         # claude | codex | auto
  model: ""              # provider-specific model (empty = provider default)
  timeout: 10s
  upgrade_threshold: 0.7
  downgrade_threshold: 0.9
  max_calls_per_minute: 30
  trigger_decisions:     # which tiers the judge reviews
    - caution
    - approval

# Caution tier behavior when judge is off or unavailable
# log:     auto-approve, log only (default for relaxed)
# approve: treat as APPROVAL, ask user (manual override for paranoid users)
caution_fallback: log
```

### 4.2 Profile Defaults

| Setting | Relaxed | Balanced | Strict |
|---------|---------|----------|--------|
| `llm_judge.mode` | `off` | `active` | `active` |
| `llm_judge.trigger_decisions` | `[]` | `[caution, approval]` | `[caution]` |
| `llm_judge.downgrade_threshold` | n/a | `0.9` | n/a |
| `caution_fallback` | `log` | `log` | `log` |

Note on Strict: the judge does NOT review APPROVAL in strict mode. APPROVAL always goes to the user. The judge only reviews CAUTION to decide whether to escalate.

### 4.3 Profile Resolution

1. Load `profile` from config (default: `balanced`)
2. Apply profile defaults for all settings
3. Override with any explicitly set values in config
4. Explicit settings always win over profile defaults

This means a user can start with `profile: balanced` and override just `caution_fallback: approve` for belt-and-suspenders behavior.

### 4.4 Config Struct Changes

```go
type Config struct {
    Profile         string               `yaml:"profile"`          // relaxed | balanced | strict | custom
    LogLevel        string               `yaml:"log_level"`
    MaxEventLogRows int                  `yaml:"max_event_log_rows"`
    MCPProxies      []MCPProxy           `yaml:"mcp_proxies"`
    LLMJudge        LLMJudgeConfig       `yaml:"llm_judge"`
    URLTrustPolicy  URLTrustPolicyConfig `yaml:"url_trust_policy"`
    PolicyLKG       PolicyLKGConfig      `yaml:"policy_lkg"`
    CautionFallback string               `yaml:"caution_fallback"` // log | approve
}
```

## 5. Judge Integration Changes

### 5.1 Downgrade Cap

The judge may never downgrade APPROVAL to SAFE. The maximum downgrade from APPROVAL is to CAUTION. This is a one-line change in `judge.go`:

```go
// In MaybeJudge, after verdict.Applied is determined:
if verdict.Applied &&
    result.Decision == core.DecisionApproval &&
    verdict.JudgeDecision == core.DecisionSafe {
    // Cap downgrade: APPROVAL -> CAUTION max, never -> SAFE
    verdict.JudgeDecision = core.DecisionCaution
}
```

### 5.2 Profile-Aware Trigger Decisions

The judge's `trigger_decisions` list is set by the profile:

- **Balanced:** `[caution, approval]` — judge reviews both tiers
- **Strict:** `[caution]` — judge reviews CAUTION only, APPROVAL always asks user
- **Relaxed:** `[]` — judge disabled, no triggers

### 5.3 Judge System Prompt Update

The system prompt must reflect the new tier semantics. Key changes:

- Remove instruction about SAFE being the only auto-approve tier
- Clarify that CAUTION means "auto-approved unless you escalate"
- Clarify that downgrading APPROVAL to SAFE is not permitted
- Add instruction: "When you downgrade APPROVAL, use CAUTION (not SAFE)"

### 5.4 Judge Verdict Flow

```
Command classified as CAUTION:
  Judge enabled + triggers CAUTION?
    YES -> Query judge
      Judge says SAFE     -> SAFE (auto-approve, no log)
      Judge says CAUTION  -> CAUTION (auto-approve, log)
      Judge says APPROVAL -> APPROVAL (ask user)
      Judge fails         -> CAUTION (auto-approve, log) [fail-open for triage tier]
    NO -> Apply caution_fallback:
      "log"     -> auto-approve, log
      "approve" -> ask user

Command classified as APPROVAL:
  Judge enabled + triggers APPROVAL?
    YES -> Query judge
      Judge says SAFE     -> CAUTION (capped, auto-approve, log)
      Judge says CAUTION  -> CAUTION (auto-approve, log)
      Judge says APPROVAL -> APPROVAL (ask user)
      Judge fails         -> APPROVAL (ask user) [fail-closed for critical tier]
    NO -> Ask user (APPROVAL always asks when judge doesn't review it)
```

## 6. Rule Migration Plan

### 6.1 Migrate to BLOCKED (Currently APPROVAL)

These patterns are never acceptable regardless of context. Move from APPROVAL to BLOCKED:

| Rule ID | Pattern | Rationale |
|---------|---------|-----------|
| `builtin:revshell:bash-tcp` | `bash -i .*/dev/tcp/` | Active attack execution |
| `builtin:revshell:python` | `python socket.*connect` | Active attack execution |
| `builtin:revshell:nc-exec` | `nc -e` | Active attack execution |
| `builtin:revshell:mkfifo` | `mkfifo.*nc` | Active attack execution |
| `builtin:obfusc:base64-exec` | `base64 -d \| bash` | Obfuscated code execution |
| `builtin:obfusc:xxd-exec` | `xxd -r \| bash` | Obfuscated code execution |
| `builtin:obfusc:printf-exec` | `printf \\x \| bash` | Obfuscated code execution |
| `builtin:obfusc:rev-exec` | `rev \| bash` | Obfuscated code execution |
| `builtin:obfusc:curl-exec` | `curl \| bash` | Remote code execution |
| `builtin:obfusc:wget-exec` | `wget -O - \| bash` | Remote code execution |
| `builtin:persist:sudoers-write` | `>> /etc/sudoers` | Privilege escalation |
| `builtin:persist:authorized-keys` | `>> .ssh/authorized_keys` | Persistence mechanism |
| `builtin:persist:profile-write` | `>> /etc/profile` etc. | System-wide persistence |
| `builtin:exfil:dns-exfil` | `dig/nslookup $()` | Data exfiltration via DNS |
| `builtin:exfil:redirect-tcp` | `> /dev/tcp/` | Network exfiltration |
| `builtin:container:mount-root` | `docker -v /:/` | Full host filesystem access |
| `builtin:container:mount-sock` | `docker -v /var/run/docker.sock` | Container escape |

**Count:** ~17 rules move to BLOCKED.

### 6.2 Migrate to CAUTION (Currently APPROVAL)

These patterns are context-dependent. The judge should triage them. Move from APPROVAL to CAUTION:

**Cloud Provider Resource Deletion (all providers):**

All AWS `builtin:aws:delete-*`, `builtin:aws:terminate-*`, `builtin:aws:purge-*` rules (~27 rules).
All GCP `builtin:gcp:delete-*`, `builtin:gcp:sql-delete`, `builtin:gcp:bq-rm` rules (~17 rules).
All Azure `builtin:az:*-delete`, `builtin:az:group-delete` rules (~16 rules).

Rationale: `aws s3 rm s3://my-dev-bucket` in a dev workspace is routine. `aws s3 rm s3://prod-data` is dangerous. The judge can distinguish via workspace context, bucket name patterns, and cwd.

**Infrastructure as Code:**

| Rule ID | Pattern |
|---------|---------|
| `builtin:terraform:destroy` | `terraform destroy` |
| `builtin:terraform:apply` | `terraform apply` |
| `builtin:terraform:state-rm` | `terraform state rm` |
| `builtin:terraform:force-unlock` | `terraform force-unlock` |
| `builtin:terraform:workspace-delete` | `terraform workspace delete` |
| `builtin:cdk:destroy` | `cdk destroy` |
| `builtin:pulumi:destroy` | `pulumi destroy` |
| `builtin:pulumi:up` | `pulumi up` |
| `builtin:pulumi:up-yes` | `pulumi up --yes` |
| `builtin:pulumi:stack-rm` | `pulumi stack rm` |
| `builtin:pulumi:state-delete` | `pulumi state delete` |

Rationale: `terraform apply` on a dev stack is routine work. The judge can evaluate based on workspace, state file, and plan output.

**Kubernetes:**

| Rule ID | Pattern |
|---------|---------|
| `builtin:k8s:delete` | `kubectl delete` |
| `builtin:k8s:drain` | `kubectl drain` |
| `builtin:k8s:replace-force` | `kubectl replace --force` |
| `builtin:helm:uninstall` | `helm uninstall` |

**Database Operations:**

| Rule ID | Pattern |
|---------|---------|
| `builtin:db:drop-database` | `DROP DATABASE` |
| `builtin:db:drop-table` | `DROP TABLE` |
| `builtin:db:truncate` | `TRUNCATE` |
| `builtin:db:delete-no-where` | `DELETE` without WHERE |
| `builtin:db:mongo-drop` | `.dropDatabase()` |
| `builtin:db:redis-flush` | `FLUSHALL/FLUSHDB` |

Rationale: `DROP TABLE test_fixtures` in a test database is routine. The judge can evaluate context.

**Local Filesystem:**

| Rule ID | Pattern |
|---------|---------|
| `builtin:fs:rm-rf` | `rm -rf` |
| `builtin:fs:rm-split-rf` | `rm -r -f` |
| `builtin:fs:rm-long-rf` | `rm --recursive --force` |
| `builtin:fs:find-delete` | `find -delete` |
| `builtin:fs:find-exec-rm` | `find -exec rm` |

Note: `rm -rf /` remains BLOCKED via hardcoded rules. These patterns cover `rm -rf <specific-path>` which is context-dependent. The existing `IsSafeBuildCleanup` exemption for `node_modules`, `dist`, etc. continues to work (checked before builtin rules).

**Git Operations:**

| Rule ID | Pattern |
|---------|---------|
| `builtin:git:reset-hard` | `git reset --hard` |
| `builtin:git:clean` | `git clean -f` |
| `builtin:git:checkout-dot` | `git checkout -- .` |
| `builtin:git:stash-clear` | `git stash clear` |

Rationale: `git reset --hard HEAD~1` in a feature branch is routine. The judge can evaluate target ref and branch.

**PaaS CLIs:**

All `builtin:paas:*-destroy` and `builtin:paas:*-delete` rules (5 rules).

**Inline Script Patterns:**

All 12 inline script patterns that currently trigger APPROVAL (bash -c, python -c, node -e, eval, heredocs, pipe-to-shell, etc.) move to CAUTION.

Rationale: `bash -c 'echo hello'` is routine. The inline body extraction and classification pipeline already analyzes the content. The judge can make a contextual decision on the extracted body.

**URL Inspection:**

| Current APPROVAL trigger | New tier |
|-------------------------|----------|
| Destructive HTTP method (DELETE/PUT/PATCH) | CAUTION |
| File upload flag (`-d @file`, `-T`) | CAUTION |
| Shell variable in URL | CAUTION |

**MCP Tool Prefixes:**

`delete_*`, `remove_*`, `destroy_*`, etc. move from APPROVAL to CAUTION. The judge evaluates the tool name + arguments in context.

**File Inspection Signals:**

Dangerous signal categories (subprocess, cloud_cli, dynamic_exec, etc.) move from APPROVAL to CAUTION. The judge sees the script contents and can evaluate.

**Miscellaneous:**

| Rule ID | Pattern |
|---------|---------|
| `builtin:cred:cat-cloud-creds` | cat cloud credential files |
| `builtin:cred:base64-key` | base64-encode credential files |
| `builtin:exfil:nc-connect` | netcat to IP (not scan mode) |
| `builtin:indirect:find-exec` | `find -exec sh -c` |
| `builtin:container:privileged` | `docker run --privileged` |
| `builtin:container:host-pid` | `docker --pid=host` |
| `builtin:container:nsenter` | `nsenter` |
| `builtin:privesc:cap-add` | `docker --cap-add ALL` |
| `builtin:pkg:pip-install-url` | `pip install` from URL |
| `builtin:recon:masscan` | Aggressive scanning |
| `builtin:recon:nikto` | Web vuln scanning |
| `builtin:sys:kill-pid` | Killing PID 1 |
| `builtin:sys:iptables-flush` | Flushing firewall |
| `builtin:rsync:delete` | rsync --delete |
| `builtin:aws:iam-delete` | IAM user/role/policy deletion |
| `builtin:aws:iam-attach` | IAM policy attachment |
| `builtin:gcp:iam-binding` | GCP IAM binding changes |
| `builtin:gcp:kms-destroy` | KMS key version destroy |
| `builtin:az:role-assignment` | Azure RBAC changes |
| `builtin:az:ad-delete` | Azure AD object deletion |

**Count:** ~90 rules move from APPROVAL to CAUTION.

### 6.3 Stay as APPROVAL

These are fail-closed safety mechanisms or security-critical patterns that should always ask the user when the judge is not available to review them:

| Source | Pattern | Rationale |
|--------|---------|-----------|
| classify.go | Oversized commands (>64KB) | Fail-closed, unparseable |
| classify.go | Compound split parse failure | Fail-closed, unparseable |
| classify.go | bash -c extraction failure | Fail-closed, unparseable |
| classify.go | CWD change before file operation | Context manipulation |
| classify.go | Sensitive env var assignment | LD_PRELOAD, DYLD_*, PATH tampering |
| inspect.go | File not found | Script referenced but missing |
| inspect.go | Truncated file, no signals found | Cannot fully assess |
| binary_verify.go | Binary stat/hash failure | TOFU verification error |
| urlinspect.go | Unparseable URL | Fail-closed, cannot assess |

**Count:** ~9 patterns stay as APPROVAL.

Note: these are all fail-closed safety returns. They represent cases where the pipeline cannot fully analyze the input. Asking the user is the correct behavior because the system genuinely does not know whether the command is safe.

### 6.4 Migration Summary

| Tier | Before | After | Delta |
|------|--------|-------|-------|
| BLOCKED | ~10 hardcoded | ~27 | +17 from APPROVAL |
| APPROVAL | ~99 (90 builtin + 9 pipeline) | ~9 (pipeline fail-closed only) | -90 to CAUTION/BLOCKED |
| CAUTION | ~25 builtin | ~115 | +90 from APPROVAL |
| SAFE | unchanged | unchanged | 0 |

## 7. Adapter Changes

### 7.1 Hook Adapter (`hook.go`)

Current exit code contract stays the same:
- Exit 0 = allow (SAFE, CAUTION)
- Exit 2 = deny (APPROVAL without approval, BLOCKED)

Changes:
1. After classification + judge, check `caution_fallback` config for CAUTION decisions when judge is off/failed
2. If `caution_fallback: approve` and judge did not review: treat CAUTION as APPROVAL (exit 2, prompt)
3. If `caution_fallback: log` (default): CAUTION remains exit 0 with stderr log

### 7.2 Runner Adapter (`runner.go`)

Same logic as hook adapter. CAUTION with `caution_fallback: approve` triggers TUI approval prompt instead of auto-executing.

### 7.3 Codex-Shell Adapter (`codexshell.go`)

Same logic. CAUTION with `caution_fallback: approve` returns an error response requiring approval.

### 7.4 Common Pattern

Extract decision enforcement into a shared function:

```go
// EffectiveDecision returns the enforcement decision after applying
// profile settings and judge results.
func EffectiveDecision(
    result *core.ClassifyResult,
    verdict *judge.Verdict,
    cfg *config.Config,
) core.Decision {
    d := result.Decision

    // Judge already applied in MaybeJudge. Check fallback behavior.
    if d == core.DecisionCaution {
        judgeReviewed := verdict != nil && verdict.Error == ""
        if !judgeReviewed && cfg.CautionFallback == "approve" {
            return core.DecisionApproval
        }
    }

    return d
}
```

## 8. Install Flow Changes

### 8.1 Profile Selection

Add profile selection to `fuse install`:

```
$ fuse install claude

How should fuse handle suspicious commands?

  1. Relaxed   Block dangerous commands, log suspicious ones, never interrupt.
              Best for: experienced developers who want minimal friction.

  2. Balanced  Use an LLM judge to review suspicious commands.
              You are only asked when the judge thinks it is necessary.
              Best for: most users. Requires a Claude or Codex API.

  3. Strict    LLM judge reviews suspicious commands.
              Critical commands always require your confirmation.
              Best for: production environments or security-sensitive work.

Pick a profile [1-3] (default: 1):
```

Default is Relaxed (1) because Balanced requires an LLM provider, and we cannot assume one is available. If the user picks Balanced or Strict, validate that a provider is available (`claude` or `codex` CLI on PATH) and warn if not found.

### 8.2 Generated Config

After profile selection, write `~/.fuse/config/config.yaml`:

```yaml
# Fuse configuration
# Profile sets defaults. Override individual settings below.
# See: https://github.com/php-workx/fuse/docs/profiles.md
profile: balanced

# LLM Judge settings (set by profile, customize as needed)
# llm_judge:
#   mode: active
#   provider: auto
#   timeout: 10s

# Caution fallback when judge is unavailable
# log:     auto-approve and log (default)
# approve: ask for confirmation
# caution_fallback: log
```

### 8.3 Codex Install

Same profile selection for `fuse install codex`.

## 9. Event Log Changes

### 9.1 Profile Field

Add `profile` to EventRecord so events can be correlated with the active profile:

```go
type EventRecord struct {
    // ... existing fields ...
    Profile string `json:"profile,omitempty"` // relaxed | balanced | strict | custom
}
```

### 9.2 Effective Decision Logging

Log both the structural decision and the effective decision (after judge + profile):

```go
type EventRecord struct {
    // ... existing fields ...
    StructuralDecision string `json:"structural_decision,omitempty"` // pre-judge classification
    Decision           string `json:"decision,omitempty"`            // effective (enforced) decision
}
```

This enables analytics: "how often does the judge change the outcome?" and "what would have happened under a different profile?"

## 10. CLI Changes

### 10.1 Profile Commands

```bash
fuse profile              # show current profile and effective settings
fuse profile set <name>   # switch profile (relaxed|balanced|strict)
```

### 10.2 Doctor Integration

`fuse doctor` should report:
- Current profile
- Judge availability (provider detected? API key set?)
- Warning if balanced/strict profile but no judge provider available

## 11. Migration Path

### 11.1 Existing Users

Users upgrading from pre-profile fuse:

1. If no `profile` key in config: default to `relaxed` (preserves current behavior where CAUTION = auto-approve)
2. If `llm_judge.mode: active` already set: default to `balanced` (they've already opted into judge)
3. Print one-time migration notice on first run: "Fuse now supports profiles. Run `fuse profile` to see your settings."

### 11.2 Tag Overrides

User tag overrides in `policy.yaml` continue to work. A user who has `tag_overrides: { aws: { action: approval } }` overrides the new CAUTION default for AWS rules. Tag overrides always take precedence over profile defaults.

### 11.3 User Policy Rules

User-defined rules in `policy.yaml` are unaffected. They are evaluated before builtins and can produce any decision level.

## 12. Implementation Order

### Phase 1: Foundation (no behavior change)
1. Add `Profile` and `CautionFallback` fields to Config struct
2. Implement profile defaults resolution
3. Add `EffectiveDecision` helper function
4. Add `profile` command to CLI
5. Update `fuse doctor` with profile reporting

### Phase 2: Judge Changes
6. Implement downgrade cap (APPROVAL -> SAFE prohibited, capped at CAUTION)
7. Update judge system prompt for new tier semantics
8. Wire profile-aware `trigger_decisions` into judge initialization

### Phase 3: Rule Migration
9. Move 17 rules from APPROVAL to BLOCKED (builtins_security.go)
10. Move 90 rules from APPROVAL to CAUTION (builtins_core.go, builtins_security.go)
11. Move inline script patterns from APPROVAL to CAUTION (classify.go)
12. Move URL inspection APPROVAL triggers to CAUTION (urlinspect.go)
13. Move MCP tool APPROVAL prefixes to CAUTION (mcpclassify.go)
14. Move file inspection APPROVAL signals to CAUTION (inspect.go)
15. Update all affected tests

### Phase 4: Adapter Integration
16. Wire `EffectiveDecision` into hook adapter
17. Wire `EffectiveDecision` into runner adapter
18. Wire `EffectiveDecision` into codex-shell adapter
19. Add `caution_fallback: approve` enforcement path to all adapters

### Phase 5: Install Flow
20. Add profile selection to `fuse install claude`
21. Add profile selection to `fuse install codex`
22. Generate config with profile defaults
23. Implement migration detection for existing users

### Phase 6: Validation
24. Integration tests for each profile (relaxed, balanced, strict)
25. Integration tests for judge fallback behavior
26. Integration tests for rule migration (spot-check moved rules)
27. Integration test for downgrade cap
28. Update event log schema (add profile, structural_decision fields)

## 13. Risk Assessment

| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|------------|
| Rule migration changes default behavior | Certain | Medium | Default profile is Relaxed, preserving current auto-approve behavior |
| Judge dependency for security | Medium | High | Fail-closed for APPROVAL (ask user), fail-open for CAUTION (log only) |
| Existing tag overrides conflict | Low | Low | Tag overrides always take precedence, no change needed |
| Judge prompt injection downgrades APPROVAL | Low | Medium | Downgrade cap prevents APPROVAL -> SAFE |
| Users pick Relaxed and get no protection | Medium | Medium | Install flow explains trade-offs; BLOCKED tier always active |

## 14. Non-Goals

- **Per-directory profiles.** One profile per fuse installation. Workspace-specific overrides use `policy.yaml` tag overrides, not profile switching.
- **Custom profile definition.** Users can set `profile: custom` and configure every setting manually, but we don't support named custom profiles.
- **Judge as a hard requirement.** Balanced and Strict recommend the judge but work without it (fallback behavior applies).
- **Changing the SAFE tier.** Default-SAFE for unknown commands is unchanged. This is a UX decision, not a security gap for the stated threat model.
