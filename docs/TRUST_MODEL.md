# Fuse Trust Model

This document describes what fuse does and doesn't do on your system. Read this
before deciding whether to adopt fuse.

## What Fuse Touches

Fuse creates and modifies the following files:

| What | Path | Purpose | Created by |
|------|------|---------|------------|
| Base directory | `~/.fuse/` | Fuse home (override with `FUSE_HOME`) | First run |
| Config | `~/.fuse/config/config.yaml` | User settings (log level, LLM judge, MCP proxies) | `fuse doctor` or manual |
| Policy | `~/.fuse/config/policy.yaml` | Custom classification rules | Manual |
| State database | `~/.fuse/state/fuse.db` | Event log, approval records (SQLite, WAL mode) | First classification |
| HMAC secret | `~/.fuse/state/secret.key` | Signs approval records (tamper detection) | First approval |
| Enabled marker | `~/.fuse/state/enabled` | Whether fuse is active | `fuse enable` |
| Dry-run marker | `~/.fuse/state/dryrun` | Whether fuse is in dry-run mode | `fuse dryrun` |
| Claude Code hook | `~/.claude/settings.json` | Adds `PreToolUse` hook entries for Bash, MCP, Read, Write, Edit, MultiEdit | `fuse install claude` |
| Codex config | `~/.codex/config.toml` | Adds fuse-shell MCP server entry | `fuse install codex` |

## Network Behavior

**Fuse makes zero network calls.** Everything runs locally. There is no
telemetry, no phoning home, no update checks.

**Exception:** The optional LLM judge feature (`llm_judge` in config.yaml)
invokes locally-installed CLI tools (`claude` or `codex`) which may make their
own network calls to their respective APIs. Fuse itself does not make these
calls — it invokes the CLI binary via `exec` and reads stdout. The judge is
off by default and must be explicitly enabled.

## Uninstall

```bash
# Remove hook integrations from Claude Code and Codex
fuse uninstall

# Also remove all fuse state (~/.fuse/ directory)
fuse uninstall --purge

# Temporarily disable without uninstalling
fuse disable
```

**What `uninstall --purge` removes:**
- `~/.fuse/` (entire directory: config, state, database, secret)
- Fuse hook entries from `~/.claude/settings.json`
- Fuse MCP entry from `~/.codex/config.toml`

**What `uninstall --purge` does NOT remove:**
- The `fuse` binary itself (installed via `go install` or package manager)
- To fully remove: `fuse uninstall --purge && rm $(which fuse)`

## Security Boundaries

### What fuse protects against

- **Accidental destructive commands** — an AI agent running `rm -rf /` or
  `DROP DATABASE` when it meant something safer
- **Unreviewed risky operations** — `git push --force origin main`, `terraform
  destroy`, `kubectl delete` without human confirmation
- **Script execution without inspection** — `bash deploy.sh` where the script
  contents haven't been reviewed

### What fuse does NOT protect against

- **Malicious agents deliberately evading classification** — an agent that
  obfuscates commands (base64 encoding, variable expansion, indirect execution)
  can bypass heuristic pattern matching
- **TOCTOU in hook mode** — Claude Code asks fuse for approval, then executes
  the command itself. Fuse cannot guarantee the command that executes is the
  one that was classified. In proxy and run modes, fuse controls execution
  directly (no TOCTOU gap)
- **OS-level containment** — fuse does not sandbox the agent. It does not use
  seccomp, AppArmor, namespaces, or containers. It is a classification and
  gating layer, not an isolation boundary
- **File content inspection for all tools** — native Claude Code file tools
  (Read, Write, Edit) are checked for path-based rules only, not content

### Classification Limits

- Classification is heuristic and regex-based. It is a guardrail, not a guarantee.
- Built-in rules cover common patterns (git, AWS, GCP, Kubernetes, security tools).
  Novel or obscure commands may not match any rule.
- The LLM judge (optional) adds semantic understanding but is also fallible.

### Approval Integrity

- Approval records are HMAC-signed with a per-user secret (`~/.fuse/state/secret.key`)
- Signatures prevent tampering with stored approvals
- Approval scopes: `once` (single use), `command` (same command), `session`
  (same agent session), `forever` (permanent)
- The secret is generated on first use and stored locally. It is not derived
  from any external credential.

## Enforcement Model by Mode

| Mode | Enforcement | TOCTOU gap? |
|------|------------|-------------|
| **Hook** (Claude Code) | Advisory — fuse classifies, Claude Code executes | Yes — agent controls execution |
| **Proxy** (MCP) | Inline — fuse intercepts and gates tool calls | No — fuse controls the pipe |
| **Run** (manual) | Inline — fuse classifies, then executes | No — fuse controls execution |
| **Codex shell** (MCP) | Inline — fuse is the shell server | No — fuse controls execution |

In hook mode, Claude Code respects fuse's classification (exit 0 = allow, exit 2
= block). But fuse cannot enforce this — a modified or misconfigured agent could
ignore the exit code. This is a fundamental property of the hook architecture,
not a bug.

## Data Retention

- **Event log** (`fuse.db`): stores classification events with command, decision,
  timestamp, agent, workspace. Pruned to 10,000 rows by default (configurable
  via `max_event_log_rows` in config.yaml).
- **Approval records**: stored in the same database, scoped by lifetime.
- **No PII collection**: fuse stores command and decision history locally for
  visibility and approvals. Use `fuse events` to inspect what was recorded.
- **Credential scrubbing**: a broad set of credential patterns is redacted
  before storage and before sending content to the LLM judge, including common
  API keys, auth headers, cookies, PEM blocks, URL userinfo, and similar secret
  material across command, reason, metadata, and judge fields.
