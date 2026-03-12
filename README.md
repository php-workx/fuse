# fuse

`fuse` is a local safety runtime for AI coding agents. In normal use, users install it into Claude Code or Codex and the agent invokes it as a hook or MCP proxy behind the scenes. `fuse` classifies shell commands and MCP tool calls as `SAFE`, `CAUTION`, `APPROVAL`, or `BLOCKED`, then prompts the human user when approval is required.

## Platform support

- macOS: `darwin/arm64`, `darwin/amd64`
- Linux: `linux/amd64`, `linux/arm64`
- Windows is not supported in v1

## Primary user path

Most users should think of `fuse` as integration infrastructure, not as a daily typed CLI:

- `fuse install claude`: install the Claude hook integration
- `fuse install codex`: install the Codex shell MCP integration
- `fuse doctor [--live]`: verify that the local setup, terminal, and hook/proxy wiring are healthy
- `fuse events` / `fuse stats`: inspect recent local activity from the shared `~/.fuse` event database

The lower-level runtime surfaces still exist:

- `fuse hook evaluate`: Claude Code pre-tool hook entrypoint for Bash, mediated MCP tools, and narrow secure-install file-path checks for `Read`/`Write`/`Edit`/`MultiEdit`
- `fuse proxy mcp --downstream-name <name>`: stdio MCP proxy with tool-call interception
- `fuse proxy codex-shell`: Codex shell MCP server exposing `run_command`
- `fuse run -- <command>`: manual debug/admin wrapper for classify/prompt/execute

## Install

### Claude Code

```bash
fuse install claude
```

This merges `fuse hook evaluate` into `~/.claude/settings.json` for both `Bash` and `mcp__.*` `PreToolUse` matchers.

```bash
fuse install claude --secure
```

`--secure` keeps the Bash and MCP hooks, merges the recommended Claude secure settings, and adds explicit `Read`, `Write`, `Edit`, and `MultiEdit` hook matchers. Those native file-tool checks are intentionally narrow and path-only: fuse blocks self-protection paths such as `~/.fuse`, `.claude/settings.json`, `.codex/config.toml`, `fuse.db`, `secret.key`, and `.git/hooks/**`, and requires approval for obvious secret-bearing locations such as `.env`, `./secrets/**`, cloud credential directories, kubeconfig paths, and common certificate/key file extensions.

### Codex

```bash
fuse install codex
```

This writes a `fuse-shell` MCP server entry into `~/.codex/config.toml`, disables Codex’s built-in shell tool, and points Codex at `fuse proxy codex-shell`.

## First-run verification

```bash
fuse doctor
fuse doctor --live
```

`--live` checks command classification plus terminal/TTY capabilities needed for approval prompts and foreground execution.

## Local observability

`fuse` stores local approvals and events in a shared per-user state directory under `~/.fuse`. This means multiple Claude and Codex instances across different workspace folders contribute to the same local audit store by default.

Use these commands to inspect that activity:

```bash
fuse events --limit 20
fuse events --source codex-shell --workspace /path/to/repo
fuse stats
```

`fuse events` shows recent activity with agent/source/workspace attribution. `fuse stats` summarizes decisions, agents, sources, and workspace roots across the local event store.

## Advanced usage

### MCP proxy mode

Configure a downstream MCP server in `~/.fuse/config/config.yaml`:

```yaml
mcp_proxies:
  - name: aws-mcp
    command: npx
    args: ["-y", "@aws/mcp-server"]
    env: {}
```

Then run:

```bash
fuse proxy mcp --downstream-name aws-mcp
```

The proxy enforces a 1 MiB frame limit, classifies `tools/call`, and blocks obvious reads of sensitive local paths such as `~/.fuse`, `~/.ssh`, `.claude`, `secret.key`, and `fuse.db`.

### Codex shell MCP mode

`fuse proxy codex-shell` serves a single `run_command` MCP tool. `SAFE` and `CAUTION` commands run immediately; `APPROVAL` commands prompt on `/dev/tty`; `BLOCKED` commands return MCP errors.

### Manual debug wrapper

```bash
fuse run --timeout 5m -- "terraform destroy prod"
```

`fuse run` is mainly for debugging, validation, and controlled manual execution. Most end users will not call it directly during normal Claude/Codex usage. It requires exactly one shell-command string after `--`, sanitizes inherited environment variables, reclassifies immediately before execution, and aborts if an inspected script changed after approval.

## Approval behavior

- Hook-mode approvals time out after 25 seconds.
- Run-mode and Codex-shell approvals time out after 5 minutes.
- If `/dev/tty` is unavailable, approval-required actions are denied with `fuse:NON_INTERACTIVE_MODE`.
- Approval records are stored in `~/.fuse/state/fuse.db` and signed with an HMAC backed by `~/.fuse/state/secret.key`.

## Limitations

- Hook mode still has a TOCTOU window because Claude Code executes natively after `fuse` allows the call.
- Classification is heuristic and regex-based; it is a guardrail, not a sandbox.
- Secure Claude native file-tool protection is path-based only; it does not inspect file contents or attempt full semantic mediation of every Claude tool.
- `fuse run` is a secondary debug/admin surface and a foreground wrapper, not a full job-control shell.
