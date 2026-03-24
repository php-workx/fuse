# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

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
