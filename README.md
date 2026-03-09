# fuse

`fuse` is a command-safety runtime for AI coding agents. It classifies shell commands and MCP tool calls as `SAFE`, `CAUTION`, `APPROVAL`, or `BLOCKED`, then prompts the human user when approval is required.

## Supported surfaces

- `fuse hook evaluate`: Claude Code pre-tool hook entrypoint for Bash and mediated MCP tools
- `fuse run -- <command>`: manual classify/prompt/execute wrapper
- `fuse proxy mcp --downstream-name <name>`: stdio MCP proxy with tool-call interception
- `fuse proxy codex-shell`: Codex shell MCP server exposing `run_command`
- `fuse doctor [--live]`: setup and terminal capability diagnostics

## Install

### Claude Code

```bash
fuse install claude
```

This merges `fuse hook evaluate` into `~/.claude/settings.json` for both `Bash` and `mcp__.*` `PreToolUse` matchers.

### Codex

```bash
fuse install codex
```

This writes a `fuse-shell` MCP server entry into `~/.codex/config.toml`, disables Codex’s built-in shell tool, and points Codex at `fuse proxy codex-shell`.

## Usage

### Manual run mode

```bash
fuse run --timeout 5m -- "terraform destroy prod"
```

`fuse run` requires exactly one shell-command string after `--`. It sanitizes inherited environment variables, reclassifies immediately before execution, and aborts if an inspected script changed after approval.

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

## Approval behavior

- Hook-mode approvals time out after 25 seconds.
- Run-mode and Codex-shell approvals time out after 5 minutes.
- If `/dev/tty` is unavailable, approval-required actions are denied with `fuse:NON_INTERACTIVE_MODE`.
- Approval records are stored in `~/.fuse/state/fuse.db` and signed with an HMAC backed by `~/.fuse/state/secret.key`.

## Diagnostics

```bash
fuse doctor
fuse doctor --live
```

`--live` checks command classification plus terminal/TTY capabilities needed for approval prompts and foreground execution.

## Limitations

- Hook mode still has a TOCTOU window because Claude Code executes natively after `fuse` allows the call.
- Classification is heuristic and regex-based; it is a guardrail, not a sandbox.
- `fuse run` is a foreground wrapper, not a full job-control shell.
