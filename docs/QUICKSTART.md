# Quickstart

Get fuse running in under a minute.

## Prerequisites

- Go 1.25+ (for `go install`)
- macOS or Linux (Windows is not supported in v1)

## Install

```bash
go install github.com/php-workx/fuse/cmd/fuse@latest
```

Verify:

```bash
fuse --help
```

## Try It

Enable fuse (it ships disabled by default):

```bash
fuse enable
```

Block a dangerous command:

```bash
echo '{"tool_name":"Bash","tool_input":{"command":"rm -rf /"}}' | fuse hook evaluate 2>&1
```

Expected output:

```
fuse:POLICY_BLOCK STOP. Recursive force-remove of root, home, or variable path Do not retry this exact command. Ask the user for guidance.
```

Allow a safe command (no output = allowed):

```bash
echo '{"tool_name":"Bash","tool_input":{"command":"ls -la"}}' | fuse hook evaluate 2>&1
```

Expected: no output, exit code 0.

## Integrate with Your Agent

### Claude Code

```bash
fuse install claude
```

This adds `PreToolUse` hooks to `~/.claude/settings.json` for Bash and MCP tools.

For stricter protection (file tools, secret paths):

```bash
fuse install claude --secure
```

### Codex CLI

```bash
fuse install codex
```

This adds a fuse-shell MCP server to `~/.codex/config.toml`.

## Verify

```bash
fuse doctor
```

Check that all items show `[ PASS ]`. If `fuse doctor --live` is available, it
also tests classification and TTY capabilities.

## Optional: Start in Dry-Run Mode

If you want to observe classifications without blocking anything:

```bash
fuse disable
fuse dryrun
```

Dry-run mode classifies and logs but never blocks or prompts. Switch to full
enforcement later with `fuse enable`.

## Next Steps

- [Trust Model](TRUST_MODEL.md) — what fuse touches on disk, network behavior, uninstall
- [README](../README.md) — full feature overview, limitations, configuration
- [CONTRIBUTING](../CONTRIBUTING.md) — how to contribute
