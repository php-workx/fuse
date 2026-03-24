# Plan: Documentation, Positioning & Adoption Path

## Context

Fuse's README is internal documentation wearing a public face. It has no visual
assets, no tagline, no install path, no proof the tool works, no trust model, and
no uninstall instructions. The project's own audit (2026-03-11) rates the honest
release posture as "internal dogfooding only; not yet validated for a stable public
release" — but the README doesn't say that, which creates a credibility gap.

Meanwhile, every competitor in the space (dcg, slb, agentguard, TaskPilot,
rubberband) has the same visual gap — zero terminal demos. There's a first-mover
opportunity on presentation, but only if the foundation is honest first.

**The core tension:** Fuse needs both trust and attention. Trust without attention
means nobody finds it. Attention without trust means nobody stays. This plan
addresses both, but sequences trust before polish — you can't market credibility
you haven't built yet.

**Competitive README audit (for reference, not as primary driver):**

| Repo | Score | Key lesson |
|------|-------|------------|
| charmbracelet/gum | 9.4 | GIF per feature, witty tagline |
| charmbracelet/vhs | 9.3 | Dark/light hero, reproducible recordings |
| astral-sh/ruff | 8.9 | One benchmark chart, social proof, concise |
| BurntSushi/ripgrep | 7.1 | Credible benchmarks, thorough |
| All competitors | 4-7 | Zero visual demos — opportunity |
| **fuse (current)** | **~4** | No tagline, no visuals, no install path, no trust model |

**Review inputs incorporated:**
- Senior doc review (2026-03-23): 7 gaps, 4 validation issues — all verified
- Architecture review: release posture disconnect, module path inconsistency,
  "firewall" framing overpromise, uninstall imprecision, scroll-tax README
  structure, docs reorg as busywork, vanity metrics

## Goals

1. Honest positioning that matches actual project maturity
2. Complete adoption path: get binary, try it, integrate, verify, undo
3. Trust model transparency: what fuse touches, stores, doesn't do, how to revert
4. Clear communication of what fuse is and what it is not
5. Compelling presentation that attracts developer attention
6. Standard OSS hygiene (LICENSE, CONTRIBUTING, CHANGELOG)
7. User-facing docs where they're referenced (QUICKSTART.md)

## Files to Create / Modify

| File | Change |
|------|--------|
| `README.md` | **REWRITE** — honest positioning, install path, proof, trust model, limitations |
| `LICENSE` | **NEW** — MIT |
| `CONTRIBUTING.md` | **NEW** — contributor guide |
| `CHANGELOG.md` | **NEW** — keep-a-changelog, backfilled from git history |
| `CODE_OF_CONDUCT.md` | **NEW** — Contributor Covenant |
| `docs/QUICKSTART.md` | **NEW** — 30-second try path (fixes broken AGENTS.md reference) |
| `docs/TRUST_MODEL.md` | **NEW** — filesystem footprint, network behavior, uninstall, limitations |
| `.github/ISSUE_TEMPLATE/` | **NEW** — bug report + feature request templates |
| `.github/pull_request_template.md` | **NEW** — PR template |
| `assets/vhs/` | **NEW** — `.tape` files for reproducible terminal recordings (Wave 3) |
| `assets/*.gif` | **NEW** — terminal demo recordings (Wave 3) |

**Docs approach: add first, reorg later.** Internal docs (`plans/`, `audits/`,
`release/`, `research/`) stay where they are. Add the public-facing docs first
(QUICKSTART.md, TRUST_MODEL.md, then `docs/guides/` if needed). Add a `docs/`
index file so readers can tell which docs are user-facing and which are internal.
Only move internal material under `docs/internal/` later if the directory gets
confusing — and only after the user-facing docs exist.

## Implementation

### 1. Release Posture & Scope Alignment

Before any README rewrite, reconcile the public positioning with the audit evidence.

**Current audit posture** (docs/audits/2026-03-11-review-summary.md):
> release-ready for internal dogfooding only; not yet validated for a stable
> public release across Claude, Codex, and cloud workflows

**What has changed since the audit (2026-03-11 → 2026-03-23):**
- LLM judge feature landed with full test coverage
- Integration tests isolated from production state
- Multiple security hardening rounds (path traversal, symlink resolution,
  credential scrubbing, context propagation)
- Pre-existing funlen/lint violations resolved
- Test suite runs clean with `-race` on all 13 packages

**Updated posture: public beta.** The project is past internal-only use. The
README must state plainly who should use it, on which platforms, through which
integrations, and with what confidence level.

**Release posture statement for README:**

```
**Status: public beta**

| | Status |
|---|--------|
| Platforms | macOS, Linux |
| Claude Code | primary integration |
| Codex CLI | beta |
| Windows | planned, not supported in v1 |

Fuse is a guardrail, not a sandbox. See [Limitations](#limitations) and
[Trust Model](docs/TRUST_MODEL.md) for what fuse can and cannot do.
```

This goes near the top of the README, before the install command. Users must be
able to tell whether they are early testers, normal adopters, or unpaid QA.

### 2. Distribution

The README rewrite depends on knowing how users get the binary. Resolve this
before writing install instructions.

**Module path:** `github.com/php-workx/fuse` (from go.mod). Every example command
in every document must use this exact path. Mismatched install commands are
credibility poison for a security tool.

**Distribution pipeline (goreleaser as backbone):**

| Phase | Method | Audience | When |
|-------|--------|----------|------|
| 1 | `go install github.com/php-workx/fuse/cmd/fuse@latest` | Go developers | Now (works today) |
| 2 | goreleaser + GitHub Releases (tarballs + checksums) | Everyone | Wave 1 |
| 3 | Homebrew tap (`php-workx/tap/fuse`) | macOS (primary), Linux (convenience) | Wave 1 |
| 4 | `.deb` and `.rpm` via goreleaser nFPM | Linux native | Wave 2 or 3 |
| 5 | Windows builds | Windows users | Separate phase, after v1 |

**Homebrew note:** Homebrew works on Linux via Linuxbrew
(`/home/linuxbrew/.linuxbrew`) but is not the native Linux packaging path.
macOS users expect `brew install`; Linux users expect tarballs, `.deb`, or
`.rpm`. Provide both from the start via goreleaser.

**References:**
- [goreleaser nFPM integration](https://goreleaser.com/customization/nfpm/)
- [nFPM docs](https://nfpm.goreleaser.com/docs/) — supports .deb, .rpm, .apk,
  ipk, Arch packages
- [Homebrew on Linux](https://docs.brew.sh/Homebrew-on-Linux)

**README install section (after goreleaser is set up):**
```bash
# macOS
brew install php-workx/tap/fuse

# Linux (deb)
curl -sSfL https://github.com/php-workx/fuse/releases/latest/download/fuse_amd64.deb -o fuse.deb
sudo dpkg -i fuse.deb

# Linux (tarball)
curl -sSfL https://github.com/php-workx/fuse/releases/latest/download/fuse_linux_amd64.tar.gz | tar xz
sudo mv fuse /usr/local/bin/

# From source (requires Go 1.25+)
go install github.com/php-workx/fuse/cmd/fuse@latest
```

**Until goreleaser is set up**, document only `go install` — the one path that
works today. Do not document install paths that don't exist yet.

### 3. Tagline & Framing

**Decision: "A local firewall for AI agent commands"**

The firewall analogy is strong — fuse inspects commands, applies rules, allows or
blocks, and logs events. This maps directly to what network firewalls do with
packets. The analogy is ~85% accurate; the gap is that in hook mode, fuse
advises rather than controls execution (the TOCTOU window). In proxy and run
modes, fuse IS fully inline.

The qualification goes in the pitch, not the tagline:

> **fuse** — a local firewall for AI agent commands
>
> Fuse classifies and gates shell commands and MCP tool calls before they
> execute. In proxy and run modes, fuse controls execution directly. In hook
> mode, fuse advises the agent — a guardrail, not a hard block.

This gets the "firewall" hook for attention while being immediately honest about
the enforcement model. The "What fuse is not" section covers the TOCTOU detail
for security-conscious evaluators.

### 4. README Structure

Keep the top brutally short: problem, install, proof, trust boundary. Everything
else goes below the fold or in linked docs.

```
┌─────────────────────────────────────┐
│  Name + tagline                     │  ← identity (1 line)
│  Maturity statement                 │  ← honest positioning (2 lines)
│  Badges (CI, Go, License)           │  ← 3 badges max
├─────────────────────────────────────┤
│  What + why (3 sentences)           │  ← problem → solution → how
│  Install (1 command)                │  ← go install, copy-paste
│  Try it (3 commands, 10 seconds)    │  ← proof it works
├─────────────────────────────────────┤
│  Hero GIF (when available)          │  ← visual proof (Wave 3)
├─────────────────────────────────────┤
│  What fuse is / what it is not      │  ← trust boundary, limitations upfront
│  What it touches on disk            │  ← filesystem table (link to TRUST_MODEL)
│  Uninstall                          │  ← full reversibility
├─────────────────────────────────────┤
│  <details> Integration guides       │  ← Claude Code, Codex, MCP proxy
│  <details> Configuration            │  ← link to docs/
│  <details> Development              │  ← just setup / just dev
│  License                            │  ← footer
└─────────────────────────────────────┘
```

**Above the fold (first screen):** problem, install, proof. Nothing else.

**Second screen:** trust boundary and reversibility. This is where a security-
conscious evaluator decides to stay or leave.

**Below the fold (collapsible):** integration details, configuration, development.
These serve users who already decided to try it.

#### Pain-first opening

```
AI coding agents run shell commands on your machine. Without a safety layer,
one bad autocomplete away from `rm -rf ~/` and there's nothing between the
agent's intent and your filesystem.
```

Two sentences. Problem stated. No product name yet.

#### Pitch (immediately after)

```
Fuse classifies every shell command and MCP tool call into SAFE, CAUTION,
APPROVAL, or BLOCKED — and gates execution before it happens. Runs locally,
no cloud, no API keys. Works as a Claude Code hook, Codex MCP server, or
generic MCP proxy.
```

#### Install + proof

Until goreleaser is set up (Wave 1), the only documented install path is
`go install`. After releases exist, update with platform-specific commands.

```bash
# Install (requires Go 1.25+)
go install github.com/php-workx/fuse/cmd/fuse@latest

# Enable fuse (ships disabled by default)
fuse enable

# See it work: block a dangerous command
echo '{"tool_name":"Bash","tool_input":{"command":"rm -rf /"},"session_id":"demo","cwd":"/tmp"}' \
  | fuse hook evaluate 2>&1
# => fuse:POLICY_BLOCK STOP. Recursive force-remove of root...

# Safe commands pass silently (exit 0, no output)
echo '{"tool_name":"Bash","tool_input":{"command":"ls -la"},"session_id":"demo","cwd":"/tmp"}' \
  | fuse hook evaluate 2>&1

# Integrate with your agent
fuse install claude    # or: fuse install codex
fuse doctor            # verify the setup
```

> **Note:** Use the installed `fuse` binary, not `go run ./cmd/fuse`. The exit
> codes differ (`go run` wraps exit 2 as exit 1).

#### What fuse is / what it is not

This section sets the trust boundary explicitly. Positioned above the integration
guides, not buried in a Limitations footer.

```
### What fuse is

- A classification and gating layer for shell commands and MCP tool calls
- A local-only tool with zero network dependencies
- A guardrail that catches obvious mistakes before they execute

### What fuse is not

- Not a sandbox — hook mode has a TOCTOU window (the agent executes after fuse allows)
- Not a replacement for OS-level security (seccomp, AppArmor, containers)
- Not infallible — classification is heuristic and regex-based
- Not a monitoring daemon — it runs per-invocation, not as a background service
```

#### Uninstall

Precise about what it does and doesn't remove:

```bash
# Remove hook/proxy integrations from Claude Code and Codex
fuse uninstall

# Also remove all fuse state (~/.fuse/ directory)
fuse uninstall --purge

# Temporarily disable (zero processing, instant pass-through)
fuse disable

# Re-enable
fuse enable
```

> **Note:** `fuse uninstall` removes integrations and optionally `~/.fuse/`. It
> does not remove the `fuse` binary itself. To fully remove: `fuse uninstall
> --purge && rm $(which fuse)`.

#### Why fuse over alternatives?

Instead of a named-competitor comparison table (maintenance burden, invites
arguments), answer the real decision questions:

```
**Why not just use Claude Code's built-in approval prompts?**
Claude Code asks before running some commands, but the rules are opaque and
not configurable. Fuse gives you explicit YAML policy, per-tag overrides,
event logging, and a TUI dashboard.

**Why not a shell wrapper or alias?**
Shell wrappers don't intercept MCP tool calls. Fuse works at the hook and
MCP protocol level, covering both shell commands and tool invocations.

**Why not a container or VM?**
Containers are heavy and break agent workflows that need filesystem access.
Fuse is a lightweight guardrail that runs alongside the agent, not a sandbox.
```

### 5. Trust Model Document (`docs/TRUST_MODEL.md`)

Standalone document linked from the README. Covers:

- **Filesystem footprint** — full table of files created/modified with purpose
- **Network behavior** — "none" with precise caveat about LLM judge CLI tools
- **Uninstall completeness** — exactly what `uninstall --purge` removes and what
  it doesn't (the binary)
- **Security boundaries** — TOCTOU window, heuristic classification limits,
  approval HMAC scheme
- **Threat model** — what fuse protects against (accidental destructive commands)
  and what it doesn't (malicious agents deliberately evading classification)

### 6. Quickstart (`docs/QUICKSTART.md`)

Fixes the broken AGENTS.md reference. Expanded version of the README's "try it"
section:

- Prerequisites (Go 1.25+)
- Install the binary
- Run the blocked-command demo (copy-paste, verify output)
- Integrate with Claude Code or Codex (one command each)
- Verify with `fuse doctor`
- Optional: enable dry-run mode first (`fuse enable --dry-run`)
- Where to go next (configuration, policy, trust model)

### 7. OSS Hygiene Files

**LICENSE:** MIT.

**CONTRIBUTING.md:**
- How to report bugs (issue template)
- How to suggest features (issue template)
- Development setup (`just setup`)
- Quality gates (`just dev`)
- PR conventions (conventional commits, one feature per PR)
- Code of conduct reference

**CHANGELOG.md:** Keep-a-changelog format. Backfill from git history.

**Issue templates:**
- Bug report: version, OS, steps to reproduce, expected vs actual, `fuse doctor`
- Feature request: problem statement, proposed solution, alternatives considered

**PR template:**
```markdown
## Summary
<!-- What does this PR do? -->

## Test plan
<!-- How was this tested? -->

## Checklist
- [ ] `just dev` passes
- [ ] Tests added for new functionality
- [ ] CHANGELOG.md updated (if user-facing)
```

### 8. Consistency Sweep

Before publishing the rewritten README, sweep existing docs for claims that
conflict with the new positioning:

- AGENTS.md references `docs/QUICKSTART.md` — must exist after Wave 1
- CLAUDE.md project description — align with new tagline
- `docs/audits/2026-03-11-review-summary.md` — release posture must be updated
  or the README maturity statement must reference it honestly
- Any doc claiming capabilities that the limitations section now qualifies
- README.md:129 "guardrail, not a sandbox" — tagline and all copy must be
  consistent with this

### 9. Visual Assets (Wave 3)

Terminal recordings using `vhs` for reproducible GIFs. Only after Waves 1 and 2
are complete — polish on top of substance, not instead of it.

**Hero GIF:** blocked command + safe command contrast (~5 seconds)
**Monitor GIF:** TUI with live events + approval
**Proxy GIF:** MCP tool call interception

All recordings have `.tape` source files in `assets/vhs/` for reproducibility.
Dark and light variants via GitHub `<picture>` element.

**Badges:** 3 max — CI, Go version, License. No coverage badge until publicly
reported. No version badge until releases exist.

**GitHub settings:** repo description, topics (`ai-safety`, `claude-code`, `codex`,
`mcp`, `guardrails`, `security`, `golang`). Social preview image.

## Execution Order

**Wave 1 — Evidence, scope, and distribution:**
- Write the release posture statement (public beta table)
- Set up goreleaser + GitHub Releases (tarballs + checksums for macOS and Linux)
- Set up Homebrew tap (`php-workx/tap/fuse`) for macOS
- Create `docs/TRUST_MODEL.md`
- Create `docs/QUICKSTART.md` (fixes AGENTS.md broken reference)
- LICENSE, CONTRIBUTING.md, CODE_OF_CONDUCT.md, CHANGELOG.md
- Issue templates + PR template
- Consistency sweep across existing docs (CLAUDE.md, AGENTS.md, audit posture)
- Add `docs/README.md` index (which docs are user-facing, which are internal)

**Wave 2 — Adoption path (README rewrite):**
- README.md rewrite:
  - Release posture table near the top
  - Pain-first opening
  - Platform-specific install commands (brew, tarball, go install)
  - "Try it" transcript
  - "What fuse is / what it is not" trust boundary
  - Filesystem table + link to TRUST_MODEL.md
  - Uninstall section (precise: removes integrations and ~/.fuse, not the binary)
  - "Why fuse over alternatives?" (decision-focused, not competitor-focused)
  - Integration guides in collapsible `<details>` sections
  - Limitations section (promoted, not buried)
- Link to QUICKSTART, TRUST_MODEL, CONTRIBUTING from README
- 3 badges (CI, Go, License)
- `.deb` and `.rpm` via goreleaser nFPM (Linux native packages)

**Wave 3 — Polish (optional, after credibility is established):**
- `vhs` terminal recordings (hero, monitor, proxy)
- Dark/light `<picture>` elements in README
- Logo (text-only is fine initially)
- GitHub repo description + topics + social preview
- Enable GitHub Discussions

## Boundaries

**Always:**
- Maturity statement is visible before the install command
- Every claim is verifiable against the codebase
- Limitations are stated before feature highlights
- Uninstall instructions are precise about scope (doesn't remove binary)
- Trust model is a first-class document, not an afterthought
- Module path in all examples matches go.mod (`github.com/php-workx/fuse`)

**Never:**
- Document install paths that don't work yet (no brew until tap exists)
- Hide limitations in a footer — state them prominently
- Prioritize star count over adoption quality
- Let marketing copy outrun evidence

## Verification

1. Maturity statement is visible in the first screen of the README
2. `go install github.com/php-workx/fuse/cmd/fuse@latest` works from a clean GOPATH
3. "Try it" transcript is copy-pasteable and produces documented output
4. "What fuse is not" section exists and is accurate
5. Uninstall section notes the binary is not removed
6. Filesystem table matches actual paths (`fuse doctor` output)
7. `docs/QUICKSTART.md` exists (fixes AGENTS.md reference)
8. `docs/TRUST_MODEL.md` exists and covers network behavior
9. No README example uses a module path other than `github.com/php-workx/fuse`
10. LICENSE, CONTRIBUTING, CHANGELOG exist and are linked from README
11. Consistency sweep complete — no stale claims in CLAUDE.md or AGENTS.md
12. Release posture in `docs/audits/` is reconciled with README maturity statement

