# Fuse Security Posture

**Last updated:** 2026-03-27

## What Fuse Protects

Fuse is a local command firewall for AI coding agents. It classifies shell commands and MCP tool calls before execution and gates them as SAFE, CAUTION, APPROVAL, or BLOCKED.

### Hard-blocked (BLOCKED) — never execute

- **Catastrophic filesystem destruction:** `rm -rf /`, `mkfs`, `dd of=/dev/*`, fork bombs, `chmod 777 /`
- **Reverse shells:** `bash -i >/dev/tcp/`, `nc -e`, `mkfifo` + `nc` pipe constructions
- **Obfuscation-to-exec:** `base64 -d | bash`, `xxd -r | bash`, `printf \x | bash`, `rev | bash`
- **Persistence mechanisms:** writes to `/etc/sudoers`, `.ssh/authorized_keys`, system shell profiles
- **Container escape:** mounting host root (`-v /:/`) or Docker socket (`-v /var/run/docker.sock`)
- **Network exfiltration:** redirects to `/dev/tcp/`
- **Self-protection:** modifications to fuse config, Claude hooks, fuse database, or fuse secret key

### Requires approval (APPROVAL) — user must confirm

- `curl | bash` and `wget | bash` install patterns
- Python socket reverse shell patterns
- DNS exfiltration via command substitution in DNS queries
- Fail-closed states: oversized commands, unparseable shell syntax, missing referenced files
- Sensitive environment variable injection (`LD_PRELOAD`, `PATH`, `DYLD_*`)
- Sensitive credential reads such as GPG key material and `.pypirc`

### Logged and monitored (CAUTION) — auto-approved with audit trail

- Inline script execution (`bash -c`, `python -c`, heredocs, pipe-to-interpreter)
- Network commands with non-allowlisted hostnames
- Many credential-related file reads (`.pem`, `.key`, cloud configs, SSH keys, `.env` files)
- Data upload flags on network commands
- Git hook modifications, shell history reads
- Package installations (`pip`, `npm -g`, `cargo install`)
- Commands with shell variable destinations that cannot be resolved safely
- Non-canonical IP encodings in URLs

### Native file tool protections (Claude Read/Write/Edit/MultiEdit)

When installed with `--secure`, fuse hooks Claude's native file tools and blocks or requires approval for:

- **Blocked:** fuse config/state files, Claude settings, Codex config, `.git/hooks/*`
- **Approval required:** `.env` files, `.ssh/`, `~/.aws/`, `~/.config/gcloud/`, `~/.azure/`, `kubeconfig`, `.gnupg/`, `.docker/config.json`, `.npmrc`, `.pypirc`, certificate/key files (`.pem`, `.key`, `.crt`, `.p12`)
- **Symlink bypass prevention:** resolves symlinks before path checks

### SSRF prevention

- Cloud metadata endpoints blocked across all IP encodings: canonical, hex, octal, dotted-octal, short-form, mixed, decimal integer, IPv4-mapped IPv6
- Blocked schemes include `file://`, `gopher://`, `dict://`, `ftp://`, `ftps://`, `scp://`, `ldap://`, and `smb://`
- Private/internal IP ranges flagged (RFC1918, link-local, carrier-grade NAT)

### MCP tool classification

- Tool name prefix matching with `mcp__<server>__` normalization
- Destructive pattern scanning in arguments
- URL inspection in argument values
- Depth exhaustion detection for deeply nested arguments

### LLM judge integration

- Optional second-opinion judge for CAUTION and APPROVAL commands
- Three-layer downgrade guard: fail-closed results never downgradeable, extraction-incomplete never downgradeable, APPROVAL never downgradeable to SAFE
- Rate-limited, timeout-protected, fail-open on error

## What Fuse Does NOT Protect

Fuse is a guardrail, not a sandbox. These limitations are by design:

1. **No filesystem sandbox.** Fuse mediates commands, not syscalls. An agent with code execution can bypass fuse entirely via compiled binaries, shared libraries, or direct syscalls.

2. **No network firewall.** Fuse inspects URLs in commands but does not intercept actual network traffic. DNS rebinding, redirect-based SSRF after the initial request, and traffic from spawned processes are not caught.

3. **No defense against hostile agents.** Fuse assumes the agent is not intentionally adversarial. An agent that deliberately obfuscates commands or uses variable indirection can evade pattern-based detection.

4. **No content-level DLP.** Fuse does not scan file contents being read or written for secrets. It classifies based on file paths and command patterns.

5. **No cross-command state tracking.** Each command is classified independently. Multi-step attacks spread across separate commands (e.g., `export URL=http://metadata; curl $URL`) are not correlated.

6. **No protection for non-mediated tools.** Tools not routed through fuse (direct MCP servers, non-hooked CLI tools) are invisible.

## Recommended Setup

### Claude Code

```bash
fuse install claude --secure
fuse doctor --security
```

The `--secure` flag configures the hook plus recommended Claude safety settings. The `--security` flag on doctor validates the posture and reports drift.

### Codex CLI

```bash
fuse install codex
fuse doctor --security
```

### Verify posture

```bash
fuse doctor --security --live
```

Reports on hook installation, secure settings, MCP mediation, and Codex shell configuration.

## Credential Scrubbing

Event logs and LLM judge prompts are scrubbed for:
- API keys, tokens, secrets, passwords (generic patterns)
- AWS access keys (AKIA, ASIA)
- Bearer and Basic auth headers
- PEM blocks
- Vendor tokens (GitHub PAT, Slack, Stripe, Google)
- URL userinfo (user:pass@host)
- JSON key-value secrets
- Cookie headers
- JWT tokens
- High-entropy base64 blobs (64+ characters)

Scrubbing is applied to all persisted text fields (Command, Reason, Metadata, JudgeReasoning).
Judge errors are scrubbed too before storage.
