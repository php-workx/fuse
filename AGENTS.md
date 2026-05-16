# Agent Instructions

This project uses **epos** for task and ticket tracking.

## Quick Reference

```bash
epos ready              # Find available work
epos blocked            # Find blocked work
epos show <id>          # View ticket details
epos claim <id> -o "$AGENT_ID"    # Claim work
epos close <id> -r "Reason"       # Complete work
epos new "Title"        # Create a new ticket
```

## Non-Interactive Shell Commands

**ALWAYS use non-interactive flags** with file operations to avoid hanging on confirmation prompts.

Shell commands like `cp`, `mv`, and `rm` may be aliased to include `-i` (interactive) mode on some systems, causing the agent to hang indefinitely waiting for y/n input.

**Use these forms instead:**
```bash
# Force overwrite without prompting
cp -f source dest           # NOT: cp source dest
mv -f source dest           # NOT: mv source dest
rm -f file                  # NOT: rm file

# For recursive operations
rm -rf directory            # NOT: rm -r directory
cp -rf source dest          # NOT: cp -r source dest
```

**Other commands that may prompt:**
- `scp` - use `-o BatchMode=yes` for non-interactive
- `ssh` - use `-o BatchMode=yes` to fail instead of prompting
- `apt-get` - use `-y` flag
- `brew` - use `HOMEBREW_NO_AUTO_UPDATE=1` env var

## Issue Tracking with epos

**IMPORTANT**: This project uses **epos** for ALL task and ticket tracking. Do NOT use markdown TODOs, task lists, direct `.tickets/` edits, or other tracking methods.

### Why epos?

- Dependency-aware: Track blockers and relationships between tickets
- Plain markdown: Tickets are readable directly from `.tickets/`
- Agent-optimized: Ready work detection, claims/leases, validation, JSON export, and partial ID matching
- Prevents duplicate tracking systems and confusion

### Quick Start

**Check for ready work:**

```bash
epos ready
epos blocked
```

**Create new tickets:**

```bash
epos new "Ticket title" --type bug --priority 2 --body "Detailed context"
epos new "Follow-up task" --body "What this issue is about"
```

Use one of these `--type` values: `issue`, `bug`, `feature`, `task`, `epic`, `chore`, `spike`, or `doc`.

**Claim and update:**

```bash
export AGENT_ID="agent-<role>-<short-session-id>"
epos claim <id> -o "$AGENT_ID"
epos edit <id> --note "Implementation note"
epos validate <id>
```

**Complete work:**

```bash
epos close <id> -r "Implemented and verified"
```

### Ticket Types

- `issue` - General issue or defect report
- `bug` - Something broken
- `feature` - New functionality
- `task` - Work item (tests, docs, refactoring)
- `epic` - Large feature with subtasks
- `chore` - Maintenance (dependencies, tooling)
- `spike` - Research or investigation
- `doc` - Documentation work

### Priorities

Use numeric priorities. Higher numbers are more important. Prefer the existing project's priority scale when editing existing tickets, and set an explicit priority for newly created work.

### Workflow for AI Agents

1. **Check ready work**: `epos ready --json` shows open tickets with dependencies resolved
2. **Claim before working**: `epos claim <id> -o "$AGENT_ID"` coordinates multi-agent work
3. **Validate ticket shape**: `epos validate <id>` before handing work to execution
4. **Work on it**: Implement, test, document
5. **Discover new work?** Create a ticket with structured fields and detailed context:
   - `epos new "Found bug" --type bug --priority 2 --body "Details about what was found"`
6. **Complete**: `epos close <id> -r "Implemented and verified"`

### Important Rules

- ✅ Use `epos` for ALL task and ticket tracking
- ✅ Read live ticket state with `epos show`, `epos ready`, `epos blocked`, and `epos export`
- ✅ Mutate tickets only through `epos new`, `epos edit`, `epos close`, `epos reopen`, `epos claim`, and `epos release`
- ✅ Add notes with `epos edit <id> --note "..."` when useful context should survive sessions
- ✅ Check `epos ready` before asking "what should I work on?"
- ✅ Run `epos lint` or `epos validate <id>` before handing tickets to another agent or execution harness
- ❌ Do NOT create markdown TODO lists
- ❌ Do NOT edit `.tickets/` files directly
- ❌ Do NOT use external issue trackers
- ❌ Do NOT duplicate tracking systems

## Landing the Plane (Session Completion)

**When ending a work session**, you MUST complete ALL steps below. Work is NOT complete until `git push` succeeds.

**MANDATORY WORKFLOW:**

1. **File tickets for remaining work** - Create tickets for anything that needs follow-up
2. **Run quality gates** (if code changed) - Tests, linters, builds
3. **Update issue status** - Close finished work, update in-progress items
4. **PUSH TO REMOTE** - This is MANDATORY:
   ```bash
   git pull --rebase
   git push
   git status  # MUST show "up to date with origin"
   ```
5. **Clean up** - Clear stashes, prune remote branches
6. **Verify** - All changes committed AND pushed
7. **Hand off** - Provide context for next session

**CRITICAL RULES:**
- Work is NOT complete until `git push` succeeds
- NEVER stop before pushing - that leaves work stranded locally
- NEVER say "ready to push when you are" - YOU must push
- If push fails, resolve and retry until it succeeds
