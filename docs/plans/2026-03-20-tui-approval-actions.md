# Plan: TUI Approval Command Center

## Context

The TUI's approvals tab is read-only. The user wants to approve/deny commands directly from `fuse monitor` — a central command center where incoming approval requests appear in real-time and can be resolved without switching terminals.

## How It Must Work

The hook process and TUI process share the SQLite database. The hook currently prompts on `/dev/tty`, but when the TUI is running it owns the terminal. The solution: **database-mediated approval** where the hook writes a pending request, and either the TTY prompt or the TUI can resolve it.

```
┌─ Agent (Claude/Codex) ──────────────────────────────────┐
│  Tool call: "terraform destroy"                          │
└──────────────┬───────────────────────────────────────────┘
               │ PreToolUse hook
               v
┌─ fuse hook evaluate ────────────────────────────────────┐
│  Classify → APPROVAL                                     │
│  ConsumeApproval(key) → not found                        │
│  INSERT pending_requests (key, command, reason, cwd)     │
│                                                          │
│  ┌──────────────┐    ┌──────────────┐                    │
│  │ TTY prompt    │ OR │ Poll DB      │  (parallel)       │
│  │ /dev/tty      │    │ every 200ms  │                    │
│  └──────┬───────┘    └──────┬───────┘                    │
│         │ first wins        │                             │
│         └───────┬───────────┘                             │
│  DELETE pending_requests                                  │
│  Return decision                                         │
└──────────────────────────────────────────────────────────┘

┌─ fuse monitor (TUI) ───────────────────────────────────┐
│  Poll pending_requests every 500ms                       │
│  Show notification bar: "⚠ APPROVAL: terraform destroy"  │
│  User presses [a]pprove → CreateApproval(key, scope)    │
│  User presses [d]eny → CreateApproval(key, BLOCKED)     │
│  Hook's DB poll picks it up within 200ms                 │
└──────────────────────────────────────────────────────────┘
```

**Backwards compatible**: Without the TUI, the `/dev/tty` prompt works exactly as before. The pending request goes unresolved by the TUI, and the TTY prompt handles it. With the TUI, either can resolve — first wins.

## Database Schema: `pending_requests` Table

```sql
CREATE TABLE IF NOT EXISTS pending_requests (
    id TEXT PRIMARY KEY,
    decision_key TEXT NOT NULL,
    command TEXT NOT NULL,
    reason TEXT,
    source TEXT NOT NULL DEFAULT 'hook',
    session_id TEXT,
    cwd TEXT,
    workspace_root TEXT,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%f', 'now') || 'Z')
);
CREATE INDEX IF NOT EXISTS idx_pending_created ON pending_requests(created_at);
```

The `source` field identifies where the request came from (hook, codex-shell, run, mcp-proxy) so the TUI can display the origin. This matters because the rewrite of `RequestApproval` makes ALL callers (not just hooks) write pending requests — the TUI becomes the central approval center for all sources.

Rows are short-lived (created when hook needs approval, deleted when resolved or timed out). No HMAC needed — these are ephemeral requests, not stored approvals.

## Timing Budget (300 seconds)

The hook timeout defaults to 300s (`FUSE_HOOK_TIMEOUT`), giving the user ample time to approve
commands via `fuse monitor`. Claude Code's default hook timeout is 600s; fuse uses 300s as a
conservative default. The TTY prompt times out at 25s (intentionally shorter), after which the
DB poll continues waiting for TUI approval until the hook timeout expires.

| Step | Time | Cumulative |
|------|------|------------|
| Classification + DB open | ~50ms | 50ms |
| Insert pending request | ~5ms | 55ms |
| Start TTY prompt + DB poll | ~5ms | 60ms |
| TTY prompt timeout (if no TTY) | 25s | ~25s |
| TUI picks up request (500ms poll) | ≤500ms | ~26s |
| User reads + decides | 0-5min | 0-5min |
| TUI writes approval | ~5ms | — |
| Hook poll detects approval | ≤200ms | — |
| Delete pending request + return | ~5ms | — |
| **Hook timeout** | | **300s** |

If the user doesn't approve within 300s, the hook returns `PENDING_APPROVAL WAIT` and the
pending request persists in the DB. The agent may retry; a subsequent retry finds the cached
approval instantly via `ConsumeApproval`.

## Files to Modify

| File | Change |
|------|--------|
| `internal/db/schema.go` | **Migration v5**: Add `pending_requests` table |
| `internal/db/pending.go` | **NEW**: `InsertPendingRequest`, `ListPendingRequests`, `DeletePendingRequest` |
| `internal/db/approvals.go` | **Add**: `DeleteApproval(id)` for revoking from TUI |
| `internal/approve/manager.go` | Rewrite `RequestApproval`: insert pending, poll + prompt in parallel |
| `internal/tui/approvals_view.go` | Rewrite: show pending requests prominently, approve/deny actions, detail panel, revoke |
| `internal/tui/model.go` | Add `pendingMsg` type, poll pending_requests, handle approve/deny/revoke commands |
| `internal/tui/keys.go` | Add approve (`a`), deny (`d` or `n`), revoke (`x`), purge (`X`) bindings |
| `internal/db/db_test.go` | Tests for pending request CRUD |
| `internal/db/pending_test.go` | **NEW**: Tests for pending request lifecycle |

## Implementation

### 1. Schema Migration v5 (`internal/db/schema.go`)

Add to the migration chain:

```go
{version: 5, sql: `
    CREATE TABLE IF NOT EXISTS pending_requests (
        id TEXT PRIMARY KEY,
        decision_key TEXT NOT NULL,
        command TEXT NOT NULL,
        reason TEXT,
        session_id TEXT,
        cwd TEXT,
        workspace_root TEXT,
        created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%f', 'now') || 'Z')
    );
    CREATE INDEX IF NOT EXISTS idx_pending_created ON pending_requests(created_at);
`},
```

### 2. Pending Request CRUD (`internal/db/pending.go`)

```go
type PendingRequest struct {
    ID            string
    DecisionKey   string
    Command       string
    Reason        string
    Source        string // "hook", "codex-shell", "run", "mcp-proxy"
    SessionID     string
    Cwd           string
    WorkspaceRoot string
    CreatedAt     string
}

func (d *DB) InsertPendingRequest(req PendingRequest) error
func (d *DB) ListPendingRequests() ([]PendingRequest, error)
func (d *DB) DeletePendingRequest(id string) error
func (d *DB) CleanupStalePendingRequests(maxAge time.Duration) (int64, error)
```

`CleanupStalePendingRequests` deletes requests older than `maxAge` (30s default) — catches cases where the hook crashed without cleanup.

### 3. Rewrite `RequestApproval` (`internal/approve/manager.go`)

Current:
```
ConsumeApproval → if not found → PromptUser → CreateApproval
```

New:
```
ConsumeApproval → if not found:
  1. InsertPendingRequest(decisionKey, command, reason, session, cwd)
  2. defer DeletePendingRequest
  3. Launch two goroutines:
     a. promptCh ← PromptUser(ctx, command, reason, hookMode, nonInteractive)
     b. pollCh  ← pollForApproval(ctx, db, decisionKey, sessionID, 200ms)
  4. select { case result := <-promptCh; case result := <-pollCh }
  5. First to return wins. Cancel the other via context.
  6. If approved via prompt: CreateApproval (existing behavior)
  7. If approved via poll: approval already exists in DB
  8. Return decision
```

The `pollForApproval` goroutine:
```go
func pollForApproval(ctx context.Context, db *db.DB, decisionKey, sessionID string, interval time.Duration) <-chan pollResult {
    ch := make(chan pollResult, 1)
    go func() {
        ticker := time.NewTicker(interval)
        defer ticker.Stop()
        for {
            select {
            case <-ctx.Done():
                return
            case <-ticker.C:
                decision, err := db.ConsumeApproval(decisionKey, sessionID)
                if err != nil {
                    continue
                }
                if decision != "" {
                    ch <- pollResult{decision: decision}
                    return
                }
            }
        }
    }()
    return ch
}
```

**Key constraint**: The hook's 300s timeout context (`hookTimeout - 2s`) cancels both goroutines. The TTY prompt has its own 25s timeout (intentionally shorter — the DB poll continues after). The DB poll respects `ctx.Done()` via select.

**Non-interactive mode** (no TTY): The TTY prompt fails immediately. The DB poll continues for the remaining hook timeout (up to ~298s), giving the user time to approve via `fuse monitor`. If no approval arrives, returns BLOCKED with `PENDING_APPROVAL WAIT` message. The pending request persists so the user can still approve; the agent's retry finds the cached approval.

### 4. Approvals View Rewrite (`internal/tui/approvals_view.go`)

The view splits into two sections:

```
┌─────────────────────────────────────────────────────────┐
│ ⚠ Pending Approval Requests (1)                         │
│                                                         │
│ ▶ terraform destroy                                     │
│   Source: hook (claude)                                  │
│   Reason: Terraform operations require approval         │
│   Workspace: /Users/runger/workspaces/infra             │
│   Waiting: 3s                                           │
│                                                         │
│   [a] Approve  [d] Deny                                │
├─────────────────────────────────────────────────────────┤
│ Approval History (24)                                   │
│                                                         │
│ CREATED            SCOPE     DECISION  STATUS    KEY    │
│ 2026-03-20 14:32   session   APPROVAL  PENDING   6d5e..│
│ 2026-03-20 14:30   once      APPROVAL  CONSUMED  ab3f..│
│ ...                                                     │
└─────────────────────────────────────────────────────────┘
```

**Pending section** (top):
- Polls `ListPendingRequests()` every 500ms (faster than events)
- Shows command, reason, workspace, elapsed time
- `a` → approve (prompts for scope: once/command/session/forever)
- `d` or `n` → deny (creates BLOCKED approval with "once" scope — denies THIS specific request but doesn't block future attempts with the same command pattern)
- If multiple pending: cursor selects which one

**History section** (bottom):
- Existing approval list (current behavior)
- `Enter` → detail panel
- `x` → revoke selected approval
- `X` → purge expired/consumed

**Scope selection** (after pressing `a`):
```
Scope: [o]nce  [c]ommand  [s]ession  [f]orever
```
Single keypress, same as the current TTY prompt.

### 5. Model Changes (`internal/tui/model.go`)

New message types:

```go
type pendingRequestsMsg struct {
    requests []db.PendingRequest
    err      error
}

type approveRequestMsg struct {
    decisionKey string
    scope       string
    sessionID   string
    err         error
}

type denyRequestMsg struct {
    decisionKey string
    sessionID   string
    err         error
    // Deny creates a BLOCKED approval with "once" scope.
    // This denies the specific request but does NOT permanently
    // block future commands with the same pattern.
}

type deleteApprovalMsg struct {
    id  string
    err error
}

type purgeApprovalsMsg struct {
    deleted int64
    err     error
}
```

The model passes `*db.DB` and the HMAC `secret` to the approval action commands so they can call `Manager.CreateApproval`.

**Secret handling**: The TUI needs the HMAC secret to create valid approvals. `runMonitor()` reads it via `db.EnsureSecret(config.SecretPath())` and passes to `NewModel`. The model stores it for approval creation.

### 6. Updated ApprovalsModel Struct

```go
type ApprovalsModel struct {
    // Pending requests (live, from hook processes)
    pending      []db.PendingRequest
    pendingIdx   int

    // Approval history
    approvals    []db.Approval
    historyIdx   int
    offset       int

    // UI state
    focus        approvalFocus   // pendingSection or historySection
    showDetail   bool
    detailView   viewport.Model
    scopeSelect  bool           // true when waiting for scope keypress after 'a'
    confirming   string         // "delete", "purge", or ""
    confirmID    string
    statusMsg    string         // transient footer message ("Approved", "Denied", etc.)

    // Dependencies
    db           *db.DB
    secret       []byte
    clock        func() time.Time
    width, height int
}
```

### 7. Key Bindings (Approvals Tab)

| Key | Context | Action |
|-----|---------|--------|
| `a` | Pending request selected | Approve → scope selection |
| `d`, `n` | Pending request selected | Deny (create BLOCKED record) |
| `o`/`c`/`s`/`f` | Scope selection active | Select scope, complete approval |
| `Enter` | History item selected | Toggle detail panel |
| `x` | History item selected | Revoke (y/n confirm) |
| `X` | Any | Purge expired/consumed (y/n confirm) |
| `Tab` | Any | Toggle focus between pending and history sections |
| `j`/`k` | Any | Navigate within focused section |
| `Esc` | Any | Close detail/cancel scope/cancel confirm |

### 8. Polling Strategy

| Data | Interval | Condition |
|------|----------|-----------|
| Pending requests | 500ms | Always when approvals tab active |
| Approval history | 1.5s | When approvals tab active |
| Events | 1.5s | When events tab active |
| Stats | 5s | When stats tab active |

Pending requests need faster polling because the hook is waiting (200ms poll on the hook side, 25s timeout).

## Edge Cases

- **Hook crashes before cleanup**: `CleanupStalePendingRequests(30min)` runs on TUI startup. Stale requests auto-expire.
- **TUI not running**: Hook falls back to TTY prompt. Pending request goes unresolved by TUI; TTY prompt handles it. Pending request cleaned up on next hook run.
- **Both TTY and TUI approve**: First writer wins. `ConsumeApproval` for "once" scope uses atomic UPDATE with `consumed_at IS NULL` guard. For other scopes, duplicate approval records are harmless (same decision key, same decision).
- **User denies from TUI**: TUI creates a BLOCKED approval record. Hook's `ConsumeApproval` finds it and returns BLOCKED. The TTY prompt goroutine is cancelled via context.
- **Multiple pending requests**: Displayed in order. User can switch between them with cursor. Each has its own decision key.
- **Approval after hook timeout (async flow)**: Hook returns `PENDING_APPROVAL WAIT` and the pending request persists in the DB. The user can approve via TUI at any time. When the agent retries the command, the hook's `ConsumeApproval` finds the cached approval instantly and returns exit 0 (allow). This supports the async approval workflow where users may take 10-15 minutes to respond.

## Security Considerations

- **HMAC on created approvals**: TUI creates approvals via `Manager.CreateApproval` which signs with the HMAC secret. Hook verifies HMAC before trusting. No bypass possible.
- **Secret access**: TUI reads `~/.fuse/state/secret.key` (same as hook). Both are local processes under the same user. No new trust boundary.
- **Pending requests are unsigned**: They're just notifications — the approval record itself (created by TUI) is HMAC-signed. A tampered pending request can't produce a valid approval without the secret.

## Verification

1. `go test ./internal/db/ -run TestPending -v` — pending request CRUD
2. `go test ./internal/approve/ -run TestRequestApproval -v` — parallel prompt + poll
3. `fuse monitor` → Approvals tab → run `fuse test classify "terraform destroy"` in another terminal with fuse in enabled mode → pending request appears in TUI
4. Press `a` then `s` (session scope) → approval created → hook proceeds
5. Run same command again → no prompt (session approval consumed)
6. Press `d` on a pending request → hook returns BLOCKED
7. `x` on history item → revoke works
8. `X` → purge works
9. Without TUI running → TTY prompt works as before (backwards compat)
10. `go test -race ./...` — full suite
11. `just check-local` — quality gate

## Implementation Sequence

1. Schema migration v5 (pending_requests table)
2. `internal/db/pending.go` + tests
3. `internal/db/approvals.go` — add `DeleteApproval`
4. `internal/approve/manager.go` — rewrite `RequestApproval` with parallel prompt + poll
5. `internal/tui/approvals_view.go` — rewrite with pending section + actions
6. `internal/tui/model.go` — pending polling, approve/deny/revoke commands
7. `internal/tui/keys.go` — new bindings
8. `internal/cli/monitor.go` — pass HMAC secret to model
9. Integration testing
