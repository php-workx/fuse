# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

### Security

- A user `allow` rule no longer clears a command fuse cannot parse, or the commands inside subshells, `{ }` groups, `if`/`while`/`for`/`case` bodies, functions, and process substitutions; hardcoded rules now run before user rules for unparseable input
- Built-in rules now see commands inside compound constructs, behind git global options (`git -C dir push --force`), and behind about 50 launchers (`mise exec`, `stdbuf`, `chrt`, `uv run`, `npx`, `doppler run`, `time -p`, `exec`, and others); `su -c`, `runuser`, and `pkexec` escalate like `sudo`
- Every recursive `rm` spelling (`-fr`, `-Rf`, `-rfv`, `-R -f`, `-r`, `--recursive`) aimed at a home or system directory is blocked, not only `-rf`
- ssh remote commands are analyzed like `sh -c`, including nested `bash -c`, `eval`, and further ssh hops
- Process substitution into a shell (`bash <(...)`, `source <(...)`, `> >(sh)`) is flagged like piping to a shell
- Text echoed back to agents in deny and caution messages (reasons, file paths, MCP tool names) is capped at 256 bytes

## [0.1.0] - 2026-03-24

Initial public beta release.

### Added

- Classification pipeline: SAFE, CAUTION, APPROVAL, BLOCKED for shell commands and MCP tool calls
- Three runtime modes: Claude Code hook, Codex shell MCP server, generic MCP proxy
- `fuse run` manual execution wrapper with TOCTOU checks and environment sanitization
- TUI monitor (`fuse monitor`) with live event stream, approval command center, and stats dashboard
- LLM judge for AI-assisted classification review (shadow and active modes)
- Per-tag enforcement mode with configurable rule overrides via `policy.yaml`
- Secure install mode (`fuse install claude --secure`) with native file-tool path checks
- HMAC-signed approval records with scoped lifetimes (once, command, session, forever)
- SQLite event log with local observability commands (`fuse events`, `fuse stats`)
- `fuse doctor` setup verification with optional `--live` TTY/classification checks
- Built-in classification rules for git, AWS, GCP, Kubernetes, security, and infrastructure commands
- Shell compatibility checks for bash, zsh, and fish (gated by `FUSE_RELEASE_CHECK=1`)

### Platforms

- macOS: darwin/arm64, darwin/amd64
- Linux: linux/amd64, linux/arm64
- Windows: not supported in v1

[Unreleased]: https://github.com/php-workx/fuse/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/php-workx/fuse/releases/tag/v0.1.0
