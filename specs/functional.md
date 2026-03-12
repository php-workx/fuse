# fuse — Functional Specification

Version: 3.0
Status: Final (post-review hardening)
Audience: Product, engineering, design, founders
Document type: Functional specification
Companion document: [specs/technical.md](technical.md)

---

## 1. Overview

### 1.1 Product summary

fuse is a **local-first developer tool** that sits between coding agents and risky execution paths.

In v1, fuse mediates:
- shell commands routed through its wrapper
- script-backed shell commands (with file inspection)
- MCP tool calls routed through its local proxy

fuse classifies requested actions, inspects referenced files when needed, and either:
- allows execution immediately
- pauses and asks the user for approval
- blocks execution

The product is designed for individual developers using agents like Claude Code locally, without requiring cloud admin support, org-level IAM setup, or centralized security infrastructure.

### 1.2 Product thesis

Agents are useful because they can act for a long time without interruption. The same property makes them dangerous: after 15-45 minutes of normal debugging or iteration, an agent may pivot into a destructive action like deleting a stack, destroying resources, or modifying credentials.

fuse exists to make that pivot visible and controllable.

### 1.3 v1 positioning

fuse is **not** an enterprise governance product.

fuse is:
- a local execution guardrail for agent workflows
- a unified shell + MCP mediation layer
- a lightweight approval gate for risky actions
- a runtime file inspection tool for referenced scripts

fuse is not:
- a cloud IAM broker
- a full sandbox product
- a complete egress proxy
- a compliance platform
- a team approval workflow system

---

## 2. Goals

### 2.1 Primary goals

1. Prevent destructive shell or MCP actions from executing silently.
2. Keep safe workflows nearly frictionless.
3. Inspect referenced files, not just inline commands.
4. Support Claude Code well in v1. Codex CLI is a fast-follow.
5. Keep setup simple enough for a single developer to complete in under 10 minutes.
6. Keep architecture small enough to build and maintain.

### 2.2 Secondary goals

1. Detect Terraform and CDK destructive commands at the command level (not plan-level artifact parsing).
2. Use exact-approval semantics rather than broad session approval.
3. Provide a small local event trail for debugging and support.
4. Allow future expansion into stronger containment or richer approval workflows.

---

## 3. Non-goals

v1 will **not**:
- intercept arbitrary SDK calls outside routed shell and MCP paths
- proxy all outbound HTTP or all cloud API traffic
- guarantee containment against a determined local user intentionally bypassing the tool
- create or manage cloud IAM roles, PAM grants, or service accounts
- provide centralized audit, search, dashboards, or compliance reporting
- provide multi-user approval workflows
- perform deep static analysis over full dependency or import graphs
- parse IaC plan/diff artifacts (Terraform plan JSON, CDK synth output)
- fully support every agent, editor, shell, and operating system on day one
- promise universal coverage of every destructive pattern

---

## 4. Threat model

### 4.1 Primary threat: accidental agent destruction

An agent, after an extended autonomous session, decides to run a destructive command (e.g., `terraform destroy`, `cdk destroy`, `rm -rf /`, `kubectl delete namespace production`). The agent is not malicious; it has reasoned itself into believing this is the right next step. fuse catches this at the shell or MCP boundary and requires explicit human approval.

**v1 catches this.** This is the core value proposition.

### 4.2 Secondary threat: script-mediated destruction

An agent writes or invokes a script (`python cleanup.py`) that contains destructive cloud SDK calls or shell subprocesses. The command itself (`python cleanup.py`) looks benign. fuse inspects the referenced file and detects the destructive patterns.

**v1 catches this for directly referenced files in supported languages.** v1 does not follow imports or dependency graphs.

### 4.3 Known bypass: two-step write-then-execute (TOCTOU)

An agent first writes a destructive script using a file-write tool, then executes it. If the file-write tool is not mediated by fuse (e.g., it uses a direct file-write MCP tool or the agent's built-in file editor), fuse only sees the execution step. File inspection at execution time mitigates this partially -- fuse will inspect the file content when the script is invoked. However, if the agent writes a launcher script that imports a destructive module, v1 will not follow the import chain.

**Additional TOCTOU gap in hook mode:** In hook mode (classify-only), fuse inspects a referenced file and exits. The agent could then modify the file between classification and Claude Code's execution. The file content hash binding (see technical.md §8.1) exists for approval matching but cannot prevent execution of modified files in hook mode because fuse is not the executor. For stronger guarantees, use `fuse run` mode where fuse controls both classification and execution atomically.

**v1 partially catches this.** File inspection at execution time provides significant mitigation. Symlinks are resolved to canonical paths before inspection to prevent symlink-based TOCTOU attacks.

### 4.4 Known bypass: encoding and obfuscation

An agent constructs a command using base64 encoding, environment variable expansion, or other obfuscation (`bash -c "$(echo dGVycmFmb3JtIGRlc3Ryb3k= | base64 -d)"`). v1 does not attempt to decode or deobfuscate arbitrary shell expressions.

**v1 does not catch this.** The fallback heuristic is: commands containing suspicious shell patterns (eval, base64 piping, nested bash -c) should be classified as APPROVAL by default. This limits damage without requiring full deobfuscation.

### 4.5 Known bypass: non-mediated execution paths

An agent uses a direct SDK call, a non-mediated MCP tool, or a separate terminal session. fuse has no visibility into these paths.

**v1 does not catch this.** This is explicitly out of scope and documented in the enforcement boundary.

### 4.6 Design principle: defense in depth, not perfection

fuse v1 is a **tripwire, not a jail**. It catches the most common and most dangerous accidental destruction patterns. It does not attempt to be a complete containment system. The spec is honest about bypass vectors so that users have accurate expectations.

---

## 5. Personas

### 5.1 Primary persona: individual developer using Claude Code

- Uses Claude Code locally for debugging, refactoring, DevOps tasks, and environment fixes
- Wants the agent to act autonomously for long periods
- Does not want destructive operations to run without explicit consent
- Does not want to involve cloud admins or security teams

### 5.2 Secondary persona: cloud/devops engineer

- More likely to work with Terraform, CDK, kubectl, cloud CLIs
- Needs protection against "agent escalated into destructive cleanup" scenarios
- Wants better context than simple "blocked command" messages

### 5.3 Future persona: individual developer using Codex CLI

- Uses Codex for coding and shell-heavy workflows
- Wants roughly the same protection model as Claude Code
- Codex integration is planned for post-v1 (see Milestone 3 in delivery plan)

---

## 6. Supported environments in v1

### 6.1 Agents

v1 ships with:
- Claude Code (primary, fully supported)

Fast-follow (Milestone 3):
- Codex CLI

Future / exploratory:
- Gemini CLI
- Cursor
- Windsurf

### 6.2 Platforms

v1 target:
- macOS
- Linux

Future:
- Windows

### 6.3 Cloud ecosystem detection presets

v1 ships detection and classification presets for:
- AWS
- GCP

Fast-follow presets:
- Azure
- Oracle Cloud Infrastructure (OCI)

Note: v1 does **not** depend on native cloud IAM integrations. Presets are pattern-matching rules for CLI commands and MCP tool names, not API integrations.

---

## 7. Enforcement boundary

### 7.1 Guaranteed mediation scope

In v1, fuse guarantees mediation only for:
- shell commands executed through its wrapper or agent hook integration path
- referenced script files invoked through those mediated shell commands
- MCP tool calls routed through its local MCP proxy

### 7.2 Out-of-scope execution paths

In v1, fuse does **not** guarantee mediation for:
- direct SDK calls made outside the mediated shell/MCP path
- arbitrary outbound HTTP outside a mediated path
- commands run in a separate terminal not routed through fuse
- direct tool integrations that bypass fuse (e.g., agent built-in file editors)

### 7.3 Failure model

fuse supports two enforcement modes:

#### Default mode: fail-closed on supported paths

If fuse cannot classify or inspect a supported shell or MCP action cleanly, the default behavior is to **require approval** rather than silently allow. This is a fail-closed (default-deny) posture for unclassifiable actions.

#### Degraded mode: fail-hard when unavailable

If fuse is unavailable entirely (process crashed, binary missing), the mediated execution path will fail and return an actionable error to the caller. The agent's shell commands will not silently bypass fuse.

---

## 8. Core user workflows

### 8.1 Workflow A: Claude tries to destroy a CDK stack

1. Claude Code works for 40 minutes debugging an issue.
2. Claude decides to run `cdk destroy PaymentsStack`.
3. Claude Code's Bash tool hook routes the command through fuse.
4. fuse classifies the command as destructive based on `cdk destroy` pattern.
5. fuse pauses execution and presents an approval prompt via its TUI.
6. User chooses Approve Once or Deny.
7. If approved, only the exact reviewed command executes.
8. fuse returns stdout/stderr/exit code to Claude Code. Session continues.

### 8.2 Workflow B: agent runs a Python script with cloud mutations

1. Agent attempts `python cleanup.py`.
2. Bash tool hook routes the command through fuse.
3. fuse detects `cleanup.py` as a referenced script file.
4. fuse inspects `cleanup.py` and finds `boto3` imports with `delete_stack` calls.
5. fuse classifies the command as APPROVAL based on file inspection signals.
6. fuse presents an approval prompt showing the command and detected file signals.
7. If approved, fuse binds approval to command hash + file content hash.
8. If the file changes before execution, the approval becomes invalid.

### 8.3 Workflow C: MCP cloud delete operation

1. Agent calls MCP tool `aws.delete_stack` through fuse's MCP proxy.
2. fuse classifies tool name and arguments as destructive.
3. fuse prompts the user for approval.
4. If approved, fuse forwards the exact tool call to the downstream MCP server.
5. If denied, fuse returns a structured error. The downstream server is never called.

### 8.4 Workflow D: safe commands remain fast

1. Agent runs `git status`, `ls`, `pytest`, `npm test`, `terraform plan`, `aws logs tail`.
2. fuse classifies these as SAFE.
3. Execution proceeds immediately with no prompt.
4. Developer experiences no friction.

---

## 9. Functional requirements

### 9.1 Shell mediation

The product shall:
1. intercept supported shell commands before execution via the agent hook integration path
2. normalize command strings before classification
3. detect inline scripts and heredocs
4. detect referenced executable files when present
5. classify commands into one of four decision classes
6. allow, prompt, or block accordingly

### 9.2 Referenced file inspection

The product shall:
1. inspect directly referenced script files for supported languages (.sh, .bash, .py, .js, .ts)
2. compute a stable file content hash (SHA-256) for approval binding
3. classify file-backed commands using both command context and file content
4. degrade safely on parse errors or oversized files (default to APPROVAL)

### 9.3 MCP proxying

The product shall:
1. expose a local MCP proxy that speaks the MCP stdio transport protocol
2. accept MCP tool calls from supported agent configurations
3. classify tool calls using tool name, arguments, and optional referenced-file inspection
4. forward allowed or approved calls to the configured downstream MCP server (via stdio subprocess)
5. block or deny disallowed calls and return a structured error

### 9.4 Approval flow

The product shall:
1. pause execution when approval is required
2. show a clear TUI prompt summarizing what was requested and why it was flagged
3. allow the user to Approve Once or Deny
4. bind approval to the exact reviewed artifact (command hash, file hash, tool call hash)
5. consume approval on use (single-use)
6. expire unconsumed approvals after a configurable timeout (default: 15 minutes)

### 9.5 Minimal local persistence

The product shall:
1. persist recent approvals and event records locally using embedded SQLite
2. require no external database, backend service, or network access

---

## 10. Decision model

### 10.1 Decision classes

fuse classifies actions into exactly four classes:

#### SAFE

- automatically executed
- no approval required
- logged to local event history

#### CAUTION

- automatically executed
- warning emitted to stderr (visible in event log, may be shown to user)
- no approval required in v1

#### APPROVAL

- execution paused
- explicit user approval required via TUI prompt
- approval bound to exact reviewed artifact

#### BLOCKED

- execution denied immediately
- no approval path
- user must modify their policy.yaml to reclassify a BLOCKED rule to APPROVAL if they want an override

The distinction between BLOCKED and APPROVAL is that BLOCKED rules represent actions considered too dangerous for one-click approval (e.g., `rm -rf /`). Users can override by editing policy, which is a deliberate, auditable act -- not a reflex click.

### 10.2 Decision inputs

A decision may be based on:
- raw command string
- normalized command
- inline code / heredoc content
- directly referenced file content and signals
- tool name and arguments (for MCP)
- detected provider/service/operation keywords

### 10.3 Default classification examples

These are concrete examples, not exhaustive:

| Command / tool | Default class |
|---|---|
| `ls`, `cat`, `pwd`, `echo`, `git status`, `git log`, `git diff` | SAFE |
| `pytest`, `npm test`, `cargo test`, `go test` | SAFE |
| `terraform plan`, `cdk diff`, `kubectl get pods`, `aws logs tail` | SAFE |
| `git push`, `git push --force` | CAUTION |
| `terraform apply` | APPROVAL |
| `terraform destroy`, `cdk destroy` | APPROVAL |
| `aws cloudformation delete-stack`, `gcloud projects delete` | APPROVAL |
| `kubectl delete namespace`, `kubectl delete -f` | APPROVAL |
| `python script.py` (with destructive file signals) | APPROVAL |
| `rm -rf /`, `rm -rf ~`, `mkfs`, `dd if=/dev/zero of=/dev/sda` | BLOCKED |
| `chmod -R 777 /`, `:(){ :\|: & };:` | BLOCKED |
| MCP `list_*`, `get_*`, `describe_*`, `read_*` | SAFE |
| MCP `update_*`, `apply_*`, `deploy_*` | CAUTION or APPROVAL |
| MCP `delete_*`, `destroy_*`, `terminate_*`, `drop_*` | APPROVAL |

### 10.4 Suspicious shell patterns

The following patterns should default to APPROVAL regardless of the inner command, because they may obscure intent:
- `eval ...`
- `bash -c "..."` / `sh -c "..."` with non-trivial content
- base64 decoding piped to shell (`| base64 -d | bash`)
- `curl ... | bash` / `wget ... | sh`
- nested command substitution with side effects

---

## 11. Approval model

### 11.1 Approval types

v1 supports exactly two user responses:
- **Approve Once**: approve this exact action for immediate execution
- **Deny**: reject execution; return an error to the caller

No persistent allowlists, session-scoped approvals, or "approve all similar" in v1.

### 11.2 Approval binding

Approvals bind to a **decision key** computed from the exact reviewed artifacts:

#### Shell-only command

Decision key hash inputs:
- literal string `"shell"`
- normalized command string

#### Script-backed command

Decision key hash inputs:
- literal string `"shell"`
- normalized command string
- SHA-256 of referenced file content

#### MCP tool call

Decision key hash inputs:
- literal string `"mcp"`
- server name
- tool name
- canonicalized argument JSON (keys sorted, whitespace normalized)

### 11.3 Approval lifecycle

1. User approves via TUI prompt.
2. Approval record is written to SQLite with `consumed: false`.
3. When fuse executes the approved action, it sets `consumed: true`.
4. A consumed approval cannot be reused.
5. Unconsumed approvals expire after 15 minutes (configurable via `approval_ttl_seconds` in config).
6. Expired or consumed approvals are cleaned up on next fuse invocation.

### 11.4 Approval invalidation

An approval is invalid and will not match if:
- the normalized command has changed
- the referenced file content hash has changed
- the MCP argument payload has changed
- the approval has been consumed
- the approval has expired

### 11.5 Approval prompt display

The TUI prompt must display:
- the exact command or MCP tool call
- current working directory (cwd)
- source context (shell or MCP, agent name if known)
- decision class (APPROVAL)
- reason(s) for flagging (which rules matched)
- relevant environment variables (AWS_PROFILE, TF_WORKSPACE, KUBECONFIG, etc.) to provide context for informed decisions
- referenced file path and detected signals, if applicable
- two options: `[A]pprove once` / `[D]eny`

---

## 12. File inspection requirements

### 12.1 Supported file types in v1

Directly referenced script files:
- `.sh`, `.bash`
- `.py`
- `.js`, `.ts`

### 12.2 Inspection boundaries

v1 file inspection is limited to:
- the single directly referenced file
- the exact file path resolved from the command arguments

v1 does **not**:
- recursively traverse import graphs or dependency trees
- inspect files referenced by `make`, `docker`, `gradle`, or other build tools
- inspect files passed via stdin or environment variables

### 12.3 Inspection objectives

File inspection identifies high-signal execution-relevant patterns:
- destructive filesystem operations (`rm -rf`, `shutil.rmtree`, `fs.rmSync`)
- subprocess execution (`subprocess.run`, `os.system`, `child_process.exec`)
- cloud CLI invocation (`aws`, `gcloud`, `az`, `oci` commands in subprocesses)
- cloud SDK imports combined with mutating calls (`boto3` + `delete_*`, `@google-cloud/*` + `delete`)
- HTTP requests to cloud control planes
- delete/destroy/terminate/drop verb patterns
- IAM/RBAC mutation patterns (`iam`, `add-iam-policy-binding`, `attach-role-policy`)

### 12.4 Parse failure handling

If a supported file cannot be parsed (syntax error, binary content, encoding issue):
- fuse falls back to heuristic text scanning (grep for dangerous patterns)
- if risk remains ambiguous, classify as APPROVAL
- log the parse failure in the event record

### 12.5 Oversized files

Default maximum file size for inspection: 1 MB (configurable via `max_inspect_file_bytes` in config).

If a referenced file exceeds the threshold:
- fuse inspects the first `max_inspect_file_bytes` bytes using heuristic text scanning
- the decision is based on whatever signals are found in the inspected portion
- if no signals are found but the file is truncated, classify as APPROVAL
- log that inspection was truncated

---

## 13. MCP proxy requirements

### 13.1 Proxy architecture

fuse MCP proxy is a **stdio-to-stdio proxy**:
- fuse exposes itself as an MCP server to the agent (via stdio transport)
- fuse spawns the downstream MCP server as a child process (via stdio transport)
- fuse intercepts `tools/call` requests, classifies them, and either forwards or denies
- fuse passes through all other MCP protocol messages (`initialize`, `tools/list`, `resources/*`, etc.) transparently

### 13.2 v1 risk mapping for MCP tools

Default classification is based on tool name pattern matching:

| Pattern | Default class |
|---|---|
| `list_*`, `get_*`, `describe_*`, `read_*`, `search_*` | SAFE |
| `create_*`, `update_*`, `put_*`, `apply_*`, `deploy_*` | CAUTION |
| `delete_*`, `destroy_*`, `terminate_*`, `drop_*`, `remove_*` | APPROVAL |

User policy rules can override these defaults.

### 13.3 Proxy denial response

When a tool call is denied, fuse returns a standard MCP error response:

```json
{
  "jsonrpc": "2.0",
  "id": "<request_id>",
  "error": {
    "code": -32600,
    "message": "Action denied by fuse safety runtime",
    "data": {
      "tool": "<tool_name>",
      "reason": "<denial_reason>",
      "decision": "BLOCKED"
    }
  }
}
```

The downstream MCP server is never invoked.

---

## 14. Agent integration requirements

### 14.1 Claude Code integration (v1 primary)

fuse integrates with Claude Code via the **hooks system** defined in `settings.json` (either `.claude/settings.json` for project-level or `~/.claude/settings.json` for user-level). Claude Code invokes fuse as a `PreToolUse` hook before every Bash tool call. fuse classifies the command, optionally prompts for approval via a TUI on `/dev/tty`, and returns an exit code (0 = allow, 2 = block). Claude Code handles execution natively.

See [technical.md §3](technical.md) for the full hook protocol specification.

### 14.2 Codex CLI integration (Milestone 3)

Codex integration is deferred to Milestone 3. The integration path is a **guarded shell MCP tool**: disable Codex's built-in `shell_tool` and register fuse as the sole shell MCP server, exposing a single `run_command` tool. Unlike the Claude Code hook (classify-only), the Codex integration executes commands directly because fuse *is* the shell.

See [technical.md §3](technical.md) for the full shell MCP tool specification.

### 14.3 Manual CLI usage

Manual CLI usage exists, but it is not the primary product experience. In normal use, users install fuse into Claude Code or Codex and the agent invokes it as hook/proxy infrastructure. Direct CLI usage is mainly for testing, debugging, and controlled non-agent workflows:

```bash
fuse run -- terraform destroy my-stack
fuse run -- python cleanup.py
```

This invokes the full classification, approval, and execution pipeline, but should be treated as a secondary control-plane surface rather than the main daily user interface.

---

## 15. Persistence requirements

### 15.1 Storage model

v1 uses local-only embedded SQLite storage. No external database, backend service, or network access required.

### 15.2 Data retained

- approval records (with expiration timestamps, consumption status)
- artifact hashes (for approval binding)
- event records (timestamp, action, decision, source, result)
- configuration and policy

### 15.3 Data not retained

- user accounts or identity data
- environment variable values (only names are logged, never values)
- file contents (only hashes are stored)
- remote audit logs or analytics

### 15.4 Secret handling

Event records and approval summaries must **never** include:
- environment variable values (log variable names only if relevant)
- file contents (log file path and hash only)
- MCP argument values that may contain secrets (log argument keys only, not values, unless explicitly safe)

---

## 16. Configuration reference

### 16.1 Top-level config: `config.yaml`

```yaml
# ~/.fuse/config/config.yaml
approval_ttl_seconds: 900           # default: 15 minutes
max_inspect_file_bytes: 1048576     # default: 1 MB
max_event_log_rows: 10000           # default: 10,000 rows
env_policy: "inherit_all"           # "inherit_all" or "inherit_minimal"
mcp_proxies: []                     # downstream MCP server definitions
log_level: "warn"                   # "error", "warn", "info", "debug"
```

### 16.2 Policy format: `policy.yaml`

```yaml
# ~/.fuse/config/policy.yaml
version: 1

rules:
  - id: "custom-rule-id"
    enabled: true
    match:
      command_contains: ["dangerous-tool"]
      command_regex: ["^sudo\\s+rm"]
      executable: ["my-dangerous-script"]
      file_extensions: [".py"]
      mcp_tool_regex: ["^myserver\\.delete_"]
      provider: ["aws"]
      operation: ["delete", "destroy"]
    action: approval  # one of: safe, caution, approval, blocked
    reason: "Human-readable reason shown in the approval prompt"

disabled_builtins:
  - "builtin:terraform:destroy"     # disable a specific built-in rule
```

### 16.3 Policy rule evaluation order

1. **Hardcoded critical safety checks** (e.g., `rm -rf /` -> BLOCKED). Cannot be overridden.
2. **User policy rules** from `policy.yaml`, evaluated in order.
3. **Built-in preset rules** shipped with fuse.
4. **Fallback heuristics** (suspicious shell patterns -> APPROVAL, unrecognized commands -> SAFE).

Most restrictive action across all matching rules wins. Users can disable specific built-in rules by ID to override them.

---

## 17. Performance requirements

### 17.1 Classification latency

- simple shell command classification (warm path, no DB access needed): under 50 ms
- simple shell command classification (cold start with SQLite open): under 150 ms
- referenced file inspection on files under 100 KB: under 250 ms
- MCP tool-call classification without file inspection: under 50 ms (warm), 150 ms (cold)

Note: Cold-start latency includes SQLite connection setup via pure-Go `modernc.org/sqlite`. Prototype benchmarking should validate these targets early. If cold start consistently exceeds 150 ms, consider lazy database access (skip DB for SAFE/BLOCKED decisions) or daemon architecture for hot-path queries.

### 17.2 Startup

- install target: under 10 minutes for a motivated developer
- first-run configuration target: under 5 minutes after install

### 17.3 Prompt responsiveness

Approval prompt must appear within 500 ms of the command being intercepted.

---

## 18. Disable and uninstall

### 18.1 Temporary disable

- **`fuse disable`**: exits immediately with code 0 (allow all) while preserving hook configuration.
- **`fuse enable`**: re-enables classification.

### 18.2 Uninstall

`fuse uninstall` removes fuse hook entries from `settings.json`, MCP proxy config, and optionally `~/.fuse/` (with `--purge` flag).

**Self-protection:** `fuse disable`, `fuse enable`, and `fuse uninstall` are hardcoded BLOCKED when run through the mediated path (agent hook or MCP). They can only be run directly by the user in an unmediated terminal. This prevents an agent from disabling its own guardrails.

### 18.3 Crash resilience

If fuse crashes or hangs, Claude Code's hook timeout (30s) kills the process and aborts the tool call. fuse does **not** fail-open on crash.

---

## 19. Minimum rule coverage requirements

v1 must ship with built-in rules covering all categories below. This minimum set is derived from analysis of DCG (27 pack categories, 50+ sub-packs), Codex CLI (25 unconditionally safe + conditional safety heuristics), TaskPilot (9 threat categories), and RubberBand (28 rules, 14 categories). See `specs/technical.md` §6.2–§6.5 for the complete rule definitions.

### 19.1 Hardcoded BLOCKED (cannot be overridden)

| Category | Coverage requirement |
|---|---|
| Catastrophic filesystem destruction | `rm -rf /`, `rm -rf ~`, `rm -rf /*`, `rm -rf $HOME` with all flag variants (combined, split, long, mixed) |
| Filesystem formatting | `mkfs`, `mkswap` on devices |
| Raw disk overwrite | `dd of=/dev/...`, redirect to raw device |
| Fork bomb | Classic `:(){:|:&};:` pattern |
| Catastrophic permission changes | `chmod 777 /`, `chown -R / ` |
| Self-protection | `fuse disable/enable/uninstall`, config file writes, `sqlite3 fuse.db` |

### 19.2 Cloud provider coverage (built-in presets)

| Provider | Minimum services covered |
|---|---|
| **AWS** | EC2 (terminate, delete-snapshot, delete-volume, delete-vpc, delete-subnet, delete-security-group), ECS, EKS, S3 (rb, rm --recursive), ECR, RDS, DynamoDB, Lambda, API Gateway (v1 + v2), SQS (delete, purge), SNS, CloudFormation, CloudFront, ELB/ALB, Route53, IAM (delete, attach/detach policy, create-access-key), Secrets Manager, KMS, Cognito, ElastiCache, Kinesis, Step Functions, EventBridge, SES, CloudWatch |
| **GCP** | Compute (instances, disks, snapshots, images), GKE, Cloud Run, Cloud Functions, App Engine, GCS (buckets, objects), Artifact Registry, Cloud SQL, BigQuery, Firestore, Spanner, Pub/Sub, Memorystore, VPC/firewall/routers, Cloud DNS, IAM (bindings, service accounts, SA keys), KMS |
| **Azure** | Resource groups, VMs, VMSS, AKS, App Service, Azure Functions, ACR, Storage (accounts, containers, blobs), CosmosDB, Azure SQL, Redis, Service Bus, Event Hubs, VNets/NSGs/public IPs/LBs/AppGW, DNS zones, Key Vault (vault + secrets), Azure AD (apps, SPs, groups), role assignments, monitoring |

### 19.3 Infrastructure as Code coverage

| Tool | Minimum operations covered |
|---|---|
| **Terraform / OpenTofu** | `destroy`, `apply`, `taint`, `state rm`, `state mv`, `force-unlock`, `workspace delete`, `plan -destroy`, `import` |
| **AWS CDK** | `destroy`, `deploy`, `deploy --force` |
| **Pulumi** | `destroy`, `up`, `up -y`, `refresh -y`, `cancel`, `stack rm`, `state delete` |
| **Ansible** | `ansible-playbook`, `ansible-galaxy remove` |

### 19.4 Platform & runtime coverage

| Category | Minimum coverage |
|---|---|
| **Git** | `reset --hard`, `clean -f`, `push --force`, `stash clear/drop`, `branch -D`, `checkout -- .`, `restore` |
| **Kubernetes** | `delete`, `drain`, `cordon`, `replace --force`, `rollout undo`; Helm `uninstall/delete` |
| **Containers** | Docker `system prune`, `volume rm`, `rm -f`, `rmi`, `run --privileged`, `--pid=host`, `--cap-add ALL/SYS_ADMIN`, mount docker.sock/root-fs |
| **Databases** | SQL `DROP DATABASE/TABLE`, `TRUNCATE TABLE`, `DELETE FROM ... ;` (no WHERE), `ALTER TABLE DROP`; MongoDB `dropDatabase/dropCollection`; CLI patterns: `psql -c`, `mysql -e`, `mongo --eval`, `redis-cli FLUSHALL/FLUSHDB/DEL` |
| **Remote execution** | SSH with remote command, SCP, rsync --delete |
| **System services** | systemctl stop/disable/mask, launchctl unload/bootout, kill PID 1, pkill -9, killall, iptables -F, truncate -s 0 |
| **PaaS CLIs** | Heroku, Fly.io, Vercel, Netlify, Railway destroy/delete |
| **Local filesystem** | `rm -rf` (all flag variants), `find -delete`, `find -exec rm`, `shred` |

### 19.5 Security threat coverage

| Category | Source | Minimum coverage |
|---|---|---|
| **Credential access** | RubberBand, TaskPilot | Reading cloud credential files, SSH keys, shell history, Docker config, npm tokens; copying/encoding key files |
| **Data exfiltration** | RubberBand, TaskPilot | curl POST/upload, wget POST, tar/zip staging, netcat connections, SCP outbound, DNS exfiltration, /dev/tcp redirect |
| **Reverse shells** | RubberBand, TaskPilot | bash /dev/tcp, python socket.connect, nc -e, mkfifo+nc, socat TCP |
| **Persistence** | RubberBand | Crontab modification, cron directory writes, systemd enable, launchd load, system profile writes, sudoers modification, authorized_keys writes |
| **Container escape** | RubberBand, TaskPilot | --privileged, --pid=host, --cap-add dangerous, Docker socket mount, host root mount, nsenter, unshare |
| **Obfuscation** | RubberBand | base64/xxd/printf/rev decode piped to shell, curl/wget piped to shell, find -exec shell |
| **Package managers** | TaskPilot | npm global install, pip install (especially from URL), gem/cargo/go install, brew/apt uninstall/remove |
| **Reconnaissance** | RubberBand | nmap, masscan, nikto, gobuster/dirb/ffuf |

### 19.6 Safe command baseline

v1 must ship with an explicit safe command set (at least 80 unconditionally safe commands + conditional safety for 12+ parameterized tools) to ensure zero-friction developer workflows. See `specs/technical.md` §6.5 for the complete list.

### 19.7 Minimum rule count

The v1 built-in rule corpus must contain:
- At least **15** hardcoded BLOCKED patterns
- At least **150** built-in preset rules across all categories
- At least **120** golden test fixtures validating rule behavior
- At least **80** unconditionally safe commands
- At least **12** conditionally safe command patterns

These counts are derived from the union of DCG, Codex, TaskPilot, and RubberBand coverage and represent the minimum viable safety surface for a tool mediating agent execution.

---

## 20. Delivery plan

### 20.1 Milestone 1: shell-only core (v1.0)

Ship: CLI, normalization, classification, hardcoded + built-in rules, policy.yaml, approval prompt, approval persistence, event logging, Claude Code hook integration, tests + golden fixtures.

This is the minimum viable product.

### 20.2 Milestone 2: referenced file inspection

Ship: Python/shell/JS/TS file inspection (regex heuristics), file-hash approval binding, file size limits.

### 20.3 Milestone 3: Codex + MCP

Ship: MCP stdio proxy, MCP classification, Codex CLI integration via guarded shell MCP, `fuse install codex`.

### 20.4 Milestone 4: IaC artifact support (future)

Ship: Terraform plan JSON inspection, CDK diff/synth inspection, artifact-hash binding. **Out of v1 scope.**

---

## 21. v1 success criteria

v1 is successful if:
1. a developer can install and configure fuse for Claude Code in under 10 minutes
2. destructive shell commands are paused before execution
3. safe commands execute with no perceptible delay
4. the approval prompt is clear, showing what was requested and why
5. referenced scripts with destructive patterns are flagged when invoked
6. MCP tool calls can be mediated through the same approval model (Milestone 3)
