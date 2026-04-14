# Agent Instructions

This project uses **tk** for task and ticket tracking.

## Quick Reference

```bash
tk ready              # Find available work
tk show <id>          # View ticket details
tk start <id>         # Mark work in progress
tk close <id>         # Complete work
tk create "Title"     # Create a new ticket
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

## Issue Tracking with tk

**IMPORTANT**: This project uses **tk** for ALL task and ticket tracking. Do NOT use markdown TODOs, task lists, or other tracking methods.

### Why tk?

- Dependency-aware: Track blockers and relationships between tickets
- Plain markdown: Tickets are readable directly from `.tickets/`
- Agent-optimized: Ready work detection, partial ID matching, and timestamped notes
- Prevents duplicate tracking systems and confusion

### Quick Start

**Check for ready work:**

```bash
tk ready
```

**Create new tickets:**

```bash
tk create "Ticket title" -d "Detailed context" -t bug|feature|task|epic|chore -p 0-4
tk create "Follow-up task" -d "What this issue is about"
```

**Start and update:**

```bash
tk start <id>
tk add-note <id> "Implementation note"
```

**Complete work:**

```bash
tk close <id>
```

### Ticket Types

- `bug` - Something broken
- `feature` - New functionality
- `task` - Work item (tests, docs, refactoring)
- `epic` - Large feature with subtasks
- `chore` - Maintenance (dependencies, tooling)

### Priorities

- `0` - Critical (security, data loss, broken builds)
- `1` - High (major features, important bugs)
- `2` - Medium (default, nice-to-have)
- `3` - Low (polish, optimization)
- `4` - Backlog (future ideas)

### Workflow for AI Agents

1. **Check ready work**: `tk ready` shows open tickets with dependencies resolved
2. **Start your task**: `tk start <id>`
3. **Work on it**: Implement, test, document
4. **Discover new work?** Create a ticket with detailed context:
   - `tk create "Found bug" -d "Details about what was found" -p 1`
5. **Complete**: `tk close <id>`

### Important Rules

- ✅ Use `tk` for ALL task and ticket tracking
- ✅ Read live ticket state with `tk show`, `tk ready`, and `tk ls`
- ✅ Add notes with `tk add-note` when useful context should survive sessions
- ✅ Check `tk ready` before asking "what should I work on?"
- ❌ Do NOT create markdown TODO lists
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
