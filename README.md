# fuse

`fuse` is a command-safety runtime for AI coding agents. It classifies shell commands and MCP tool calls as `SAFE`, `CAUTION`, `APPROVAL`, or `BLOCKED`, then prompts the human user when approval is required.

## Supported surfaces

- `fuse hook evaluate`: Claude Code pre-tool hook entrypoint for Bash, mediated MCP tools, and narrow secure-install file-path checks for `Read`/`Write`/`Edit`/`MultiEdit`
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

```bash
fuse install claude --secure
```

`--secure` keeps the Bash and MCP hooks, merges the recommended Claude secure settings, and adds explicit `Read`, `Write`, `Edit`, and `MultiEdit` hook matchers. Those native file-tool checks are intentionally narrow and path-only: fuse blocks self-protection paths such as `~/.fuse`, `.claude/settings.json`, `.codex/config.toml`, `fuse.db`, `secret.key`, and `.git/hooks/**`, and requires approval for obvious secret-bearing locations such as `.env`, `./secrets/**`, cloud credential directories, kubeconfig paths, and common certificate/key file extensions.

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

## Development

Install local tooling:

```bash
make install-dev
make install-hooks
```

This repo uses explicit fast and slow verification layers:

- `make check-fast`: formatting, workflow lint, unit tests, and `golangci-lint`
- `make check-pre-push`: `check-fast` plus race tests, `govulncheck`, release compatibility checks, and cross-builds

The repo-managed git hooks live under `.githooks/`:

- `pre-commit` runs the fast checks for early feedback
- `pre-push` runs the slower last-resort safety net before CI

If you already use the Python `pre-commit` tool, this repo also ships [.pre-commit-config.yaml](.pre-commit-config.yaml). Install it separately with `pipx install pre-commit` or your preferred Python toolchain, then the repo hooks will delegate to it automatically.

## Limitations

- Hook mode still has a TOCTOU window because Claude Code executes natively after `fuse` allows the call.
- Classification is heuristic and regex-based; it is a guardrail, not a sandbox.
- Secure Claude native file-tool protection is path-based only; it does not inspect file contents or attempt full semantic mediation of every Claude tool.
- `fuse run` is a foreground wrapper, not a full job-control shell.
