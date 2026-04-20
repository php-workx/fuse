# Quickstart

Get fuse running in under a minute.

## Prerequisites

- Go 1.25+ (for `go install`)
- macOS, Linux, or Windows early-adopter testing

## Install

Windows PowerShell:

```powershell
irm https://raw.githubusercontent.com/php-workx/fuse/main/install.ps1 | iex
```

macOS, Linux, or source install:

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

This uses Codex native Bash hooks when the installed Codex version supports
them. Otherwise it falls back to a fuse-shell MCP server in
`~/.codex/config.toml`.

## Verify

```bash
fuse doctor
```

Check that all items show `[ PASS ]`. If `fuse doctor --live` is available, it
also tests classification and TTY capabilities.

The `fuse binary in PATH` check also reports the path and version of the
binary your agent hooks will actually invoke, for example:

```
  [ PASS ]  fuse binary in PATH
           path: /usr/local/bin/fuse; version: fuse 1.4.0 (abc1234) built 2026-04-18
```

### Detecting a stale hook binary

Agent hooks call `fuse` from `PATH`. If that binary is older than the `fuse`
you just ran `fuse doctor` with (for example, you rebuilt from source but did
not reinstall, or you still have an older release on `PATH` shadowing a new
install), your hooks will apply stale classification policy. `fuse doctor`
flags this with `[ WARN ]` and a concrete fix hint:

```
  [ WARN ]  fuse binary in PATH
           hook binary appears stale or mismatched: path: /usr/local/bin/fuse; version: fuse 1.3.0 (old-commit) built 2026-04-01; current build: fuse 1.4.0 (new-commit) built 2026-04-18
           fix: reinstall fuse (e.g. `go install ./cmd/fuse` or download the latest release) so the hook uses the current build
```

(Without `--verbose`, `fuse doctor` truncates detail lines longer than 120
characters; run `fuse doctor --verbose` to see the full path and version
strings.)

The same warning is printed at the end of `fuse install claude` and `fuse
install codex` when the installer detects drift.

To fix it, reinstall `fuse` so the binary on `PATH` matches the build you want
your agents to run, then rerun `fuse doctor`:

```bash
# From source
go install github.com/php-workx/fuse/cmd/fuse@latest

# Or from a release (macOS Homebrew example)
brew upgrade php-workx/tap/fuse

# Verify
fuse doctor
```

Notes:

- When the running binary has no build metadata (e.g. a `go install` without
  ldflags), `fuse doctor` cannot distinguish stale from current and reports
  PASS with a note that the hook binary could not be verified. Install a
  release build or rebuild with the project `justfile` targets to get
  meaningful drift detection.
- When the running process and the `PATH` binary resolve to the same file on
  disk (after symlink resolution), the check passes without running `fuse
  version` — there is no drift to report.

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
