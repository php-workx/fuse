# Plan: Fuse TUI — Live Dashboard for Monitoring Activity

## Context

The fuse CLI currently shows events as static tabwriter output (`fuse events`). There's no way to tail events live, filter interactively, or see pending approvals at a glance. The user wants a k9s/OpenShell-style TUI — a live-updating, keyboard-driven dashboard launched via `fuse monitor`.

## Dependencies

```text
github.com/charmbracelet/bubbletea/v2  v2.0.2
github.com/charmbracelet/lipgloss/v2   v2.0.2
github.com/charmbracelet/bubbles/v2    v2.0.0
```

All stable releases. No existing TUI libraries in go.mod.

## Files to Modify

| File | Change |
|------|--------|
| `go.mod` | Add bubbletea/v2, lipgloss/v2, bubbles/v2 |
| `internal/tui/model.go` | **NEW** — Top-level bubbletea Model, tick loop, view switching |
| `internal/tui/events_view.go` | **NEW** — Events table with color-coded decisions, cursor, detail panel |
| `internal/tui/stats_view.go` | **NEW** — Summary dashboard with bar charts |
| `internal/tui/approvals_view.go` | **NEW** — Pending/consumed approvals list |
| `internal/tui/styles.go` | **NEW** — Lipgloss styles, decision color map |
| `internal/tui/keys.go` | **NEW** — Key binding definitions |
| `internal/db/approvals_list.go` | **NEW** — `ListApprovals(limit int) ([]Approval, error)` query |
| `internal/cli/monitor.go` | **NEW** — Cobra command `fuse monitor` (alias: `fuse tui`) |
| `internal/cli/help.go` | Add `"monitor"` to `groupCommandOrder[groupObserve]` |

## Architecture

```text
┌─ fuse monitor ──────────────────────────────────────────────────┐
│  [fuse monitor]  Mode: DRYRUN  │ ❶Events  ❷Stats  ❸Approvals  │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  TIME      DECISION   AGENT   SOURCE       COMMAND              │
│  14:32:05  SAFE       claude  hook         go test ./...        │
│  14:32:03  BLOCKED    claude  hook         rm -rf /             │
│  14:31:58  APPROVAL   codex   codex-shell  terraform destroy    │
│  14:31:55  SAFE       claude  hook         git add -A           │
│  14:31:50  CAUTION    claude  hook         git push --force     │
│  ...                                                            │
│                                                                 │
├─────────────────────────────────────────────────────────────────┤
│  q quit  Tab view  j/k nav  / search  Enter detail  d filter   │
└─────────────────────────────────────────────────────────────────┘
```

Single bubbletea Program with three switchable views. Polls SQLite every 1.5s via `tea.Tick`. Shared `*db.DB` handle opened once by the cobra command.

## Implementation

### 1. Add Dependencies

```bash
go get github.com/charmbracelet/bubbletea/v2@v2.0.2
go get github.com/charmbracelet/lipgloss/v2@v2.0.2
go get github.com/charmbracelet/bubbles/v2@v2.0.0
```

### 2. `internal/db/approvals_list.go` — New Query

```go
func (d *DB) ListApprovals(limit int) ([]Approval, error)
```

```sql
SELECT id, decision_key, decision, scope, session_id,
       created_at, consumed_at, expires_at
FROM approvals
ORDER BY created_at DESC
LIMIT ?
```

Note: `hmac` is intentionally excluded — it is validation material with no monitoring purpose and should not be loaded into the UI.

Reuses existing `Approval` struct from `approvals.go`. Note: no index covers `created_at` for ordering (existing indexes are on `decision_key, consumed_at` and `expires_at`). The table is small enough for v1; add `CREATE INDEX idx_approvals_created ON approvals(created_at)` if approval volume grows.

### 3. `internal/tui/styles.go` — Decision Colors + Layout Styles

```go
func DecisionStyle(decision string) lipgloss.Style
```

| Decision | Color |
|----------|-------|
| SAFE | Green (10) |
| CAUTION | Yellow (11) |
| APPROVAL | Blue (12) |
| BLOCKED | Red (9) |

Plus: header bar (bold, dark bg), footer (faint), active tab (bold underline), cursor row (reverse), detail border (rounded).

### 4. `internal/tui/keys.go` — Keyboard Bindings

| Key | Action |
|-----|--------|
| `q`, `Ctrl+C` | Quit |
| `Tab`, `1`/`2`/`3` | Switch view |
| `j`/`k`, `↑`/`↓` | Navigate rows |
| `Enter` | Toggle detail panel |
| `Esc` | Close detail / clear filter |
| `/` | Enter search mode (free-text filter on command field) |
| `d` | Cycle decision filter: ALL → SAFE → CAUTION → APPROVAL → BLOCKED |
| `PgUp`/`PgDn` | Page scroll |
| `g`/`G` | Jump to top/bottom |

**Search mode:** When `searching` is true (after pressing `/`), all key messages route exclusively to the text input model. Global single-key bindings (`q`, `d`, `j`, `k`, `g`, `G`, `1`/`2`/`3`) are suppressed. Only `Enter` (apply filter and exit search) and `Esc` (clear and exit search) are handled outside the text input. `Ctrl+C` remains active for quit.

**Interactive behavior acceptance criteria:**

- Search (`/`) is case-insensitive substring matching on the command field. Matching applies on each keystroke. An empty search restores the unfiltered list. If no rows match, the table body shows `No matching events`.
- `Esc` first closes the detail panel if open; if no detail panel is open, it clears the search text and restores the current decision filter.
- `d` immediately reloads rows using the next decision filter in the cycle (ALL → SAFE → CAUTION → APPROVAL → BLOCKED → ALL) and resets cursor and offset to the first visible row. The current filter is displayed in the footer.
- `Enter` toggles the detail panel for the currently highlighted row. When the detail panel is visible, the cursor row's detail is shown; pressing `Enter` again or `Esc` closes it.

### 5. `internal/tui/model.go` — Top-Level Model

```go
type viewMode int

const (
    viewEvents viewMode = iota
    viewStats
    viewApprovals
)

type Model struct {
    db         *db.DB
    mode       string           // from config.Mode() (FuseMode), formatted: "enabled" / "dryrun" / "disabled"
    activeView viewMode         // events / stats / approvals
    width, height int
    fetchGen   uint64           // incremented on view switch or filter change
    fetching   bool             // true while fetchData() goroutine is in-flight
    lastErr    error            // last DB error, shown in footer bar
    events     EventsModel
    stats      StatsModel
    approvals  ApprovalsModel
    quitting   bool
}
```

**Sub-view contract:** All three sub-models (EventsModel, StatsModel, ApprovalsModel) must implement:
- `Update(tea.Msg) (Self, tea.Cmd)` — handle delegated messages
- `View() string` — render the view content
- `SetSize(width, height int)` — accept new terminal dimensions

Model.Update() handles `tea.WindowSizeMsg` and global `tea.KeyMsg` (quit, tab switch), then delegates remaining messages to the active sub-view. `dataMsg` is dispatched to the relevant sub-view's data setter based on `activeView`.

**Tick loop:** Fetch fresh data for the active view on a repeating interval. Events and approvals views poll every 1.5s; stats view polls every 5s (its `GROUP BY` aggregations are O(N) over the events table and degrade with history size). A `fetching` guard prevents goroutine accumulation when a query exceeds the tick interval — skip issuing a new `fetchData()` if the previous one hasn't returned. Set `m.fetching = true` before dispatching, clear it on `dataMsg` receipt. The next tick is scheduled only after the previous `dataMsg` is processed, preventing query stacking during slow I/O. When new events arrive in the events view, maintain a **sticky cursor** by tracking the selected event's ID across refreshes — after data update, recompute cursor index to point at the same event ID (or clamp to list bounds if it was deleted). This prevents cursor drift when new rows are prepended.

Fetch strategy:
- Events view active → `db.ListEvents` with current filters
- Stats view active → `db.SummarizeEvents`
- Approvals view active → `db.ListApprovals`

DB queries run in `tea.Cmd` goroutines (non-blocking UI thread). `tickCmd` is returned from the `dataMsg` handler (not `tickMsg`) to ensure sequential fetching:

```go
type tickMsg time.Time

func tickCmd() tea.Cmd {
    return tea.Tick(1500*time.Millisecond, func(t time.Time) tea.Msg {
        return tickMsg(t)
    })
}

// On each tick, fetch data in a goroutine.
// Exactly one of events/summary/approvals is populated per fetch (based on activeView).
type dataMsg struct {
    reqGen    uint64           // generation counter at time of dispatch
    view      viewMode         // which view initiated the fetch
    events    []db.EventRecord
    summary   db.EventSummary  // value type — matches db.SummarizeEvents() return
    approvals []db.Approval
    err       error            // non-nil if DB query failed; UI shows stale-data warning
}

func (m Model) fetchData() tea.Cmd {
    gen := m.fetchGen
    view := m.activeView
    return func() tea.Msg {
        msg := dataMsg{reqGen: gen, view: view}
        switch view {
        case viewEvents:
            msg.events, msg.err = m.db.ListEvents(&db.EventFilter{Limit: 200, ...})
        case viewStats:
            msg.summary, msg.err = m.db.SummarizeEvents()
        case viewApprovals:
            msg.approvals, msg.err = m.db.ListApprovals(50)
        }
        return msg
    }
}
```

**Stale result handling:** `Update` must compare `dataMsg.reqGen` against `m.fetchGen` and discard any result where `reqGen < m.fetchGen` (the user switched views or changed filters while the query was in flight). Increment `fetchGen` on every view switch, filter change, or search update to invalidate in-flight fetches. When `dataMsg.err != nil`, preserve the previous data snapshot and render an error indicator in the footer status line (e.g., `"DB read failed — showing last good data"`).

**Resize flow:** Model handles `tea.WindowSizeMsg`, stores dimensions, subtracts header/footer height, and calls `SetSize(w, contentHeight)` on each sub-view. Sub-views do not independently handle `tea.WindowSizeMsg`.

**Error display:** When `dataMsg.err != nil`, show a status-bar warning (e.g., "DB read failed — showing stale data") in the footer. The TUI continues running with the last successfully fetched data.

**View()** renders: header bar + active view + footer help bar.

### 6. `internal/tui/events_view.go` — Events View

- Manual table rendering (not bubbles/table) for full color control per row
- Each row: `TIME | DECISION | AGENT | SOURCE | WORKSPACE | COMMAND`
- WORKSPACE column maps to `EventRecord.WorkspaceRoot`, shortened to the last path component for display
- TIME formatted as `HH:MM:SS` for compactness
- DECISION column color-coded via `DecisionStyle`
- Cursor row highlighted with reverse video
- Command truncated to fit terminal width (local `shorten` helper in `internal/tui`, or extract shared text-formatting helpers into a package both `cli` and `tui` can import)
- `/` search filters command field via `strings.Contains`
- `d` cycles decision filter
- `Enter` opens detail panel (bottom 40% of view, using `bubbles/viewport` for internal scrolling) showing these fields in order: ID, Time, Decision, Command, Rule ID, Reason, Agent, Source, Session, Workspace, Duration, Exit Code. Any remaining EventRecord fields may be omitted in v1. When the detail panel is focused, `j`/`k`/`PgUp`/`PgDn` scroll the panel content; `Esc` or `Enter` closes it
- **Cursor anchoring:** On data refresh, anchor cursor by event ID — find the previously-selected event's ID in the new data and restore cursor to its new position. If the event is no longer present, clamp cursor to the nearest row. When the detail panel is open, pause polling to avoid disrupting the user's reading

```go
type EventsModel struct {
    events         []db.EventRecord
    cursor         int
    selectedID     int64           // sticky cursor: tracks event ID across refreshes
    offset         int
    filterDecision string          // "" = all
    searchInput    textinput.Model // bubbles/v2/textinput for robust text entry
    searching      bool            // true when / input is active
    showDetail     bool
    detailView     viewport.Model  // bubbles/v2/viewport for scrollable detail panel
    width, height  int
}
```

**Detail panel** (shown when Enter pressed on a row):

```text
╭─ Event Detail ──────────────────────────────────────────╮
│  ID:          1234                                       │
│  Time:        2026-03-20T14:32:05.123Z                   │
│  Decision:    BLOCKED                                    │
│  Command:     rm -rf /                                   │
│  Rule ID:     hardcoded:rm-rf-root                       │
│  Reason:      recursive deletion of filesystem root      │
│  Agent:       claude                                     │
│  Source:      hook                                        │
│  Session:     abc123                                     │
│  Workspace:   /Users/runger/workspaces/fuse              │
│  Duration:    5ms                                        │
│  Exit Code:   -                                          │
╰──────────────────────────────────────────────────────────╯
```

### 7. `internal/tui/stats_view.go` — Stats View

Reuses `db.SummarizeEvents()` directly (`events.go:118`). **Known gap:** `SummarizeEvents()` runs 5 separate auto-committed queries, so breakdowns can diverge from the total count by 1-2 events under concurrent writes. Wrapping in a read transaction (`BeginTx` with `ReadOnly: true`) would give a consistent snapshot at zero cost in WAL mode — file as a follow-up fix to `db/events.go`. Renders:
- Total event count (bold)
- Four sections: By Decision, By Agent, By Source, By Workspace
- Horizontal bars using `█` characters, proportional to count
- Decision labels color-coded

```text
  Total Events: 1,234

  By Decision                    By Agent
  ━━━━━━━━━━━━━━━                ━━━━━━━━━━━━━━━
  SAFE      ██████████████  892   claude  ████████████  756
  CAUTION   ████            201   codex   ██████        412
  APPROVAL  ███              98   manual  ██             66
  BLOCKED   █                43

  By Source                      By Workspace
  ━━━━━━━━━━━━━━━                ━━━━━━━━━━━━━━━
  hook        █████████████  812  .../fuse     ████████  634
  codex-shell ████████       356  .../app      ██████    412
  run         ██              66  .../lib      ████      188
```

### 8. `internal/tui/approvals_view.go` — Approvals View

Table: `CREATED | SCOPE | DECISION | STATUS | KEY (truncated)`

Status computed from fields with precedence CONSUMED > EXPIRED > PENDING:
- `consumed_at != nil` → "CONSUMED" (dim) — takes priority regardless of expiry
- `consumed_at == nil && expires_at != nil && now >= expires_at` → "EXPIRED" (red)
- `consumed_at == nil && (expires_at == nil || now < expires_at)` → "PENDING" (green)

`expires_at == nil` means the approval never expires. `now` comes from an injectable `clock func() time.Time` field (defaults to `time.Now().UTC()` in production, fixed value in tests).

### 9. `internal/cli/monitor.go` — Cobra Command

```go
var monitorCmd = &cobra.Command{
    Use:     "monitor",
    Aliases: []string{"tui"},
    Short:   "Live dashboard for monitoring fuse activity",
    GroupID: groupObserve,
    RunE:    func(cmd *cobra.Command, args []string) error { return runMonitor() },
}

func init() {
    rootCmd.AddCommand(monitorCmd)
}

func runMonitor() error {
    if !isTerminal(int(os.Stdin.Fd())) || !isTerminal(int(os.Stdout.Fd())) {
        return fmt.Errorf("fuse monitor requires an interactive terminal")
    }

    database, exists, err := openEventsDB()  // reuse from events.go
    if err != nil {
        return fmt.Errorf("open database: %w", err)
    }
    if !exists {
        return fmt.Errorf("no fuse database found (run some commands first)")
    }
    defer database.Close()

    mode := config.Mode()
    m := tui.NewModel(database, mode)
    p := tea.NewProgram(m, tea.WithAltScreen())
    _, err = p.Run()
    return err
}
```

- Uses `tea.WithAltScreen()` for full-terminal takeover
- Reuses `openEventsDB()` from `events.go:154` (opens read-write and runs migrations; a read-only opener with `?mode=ro` skipping migrations could be added later to reduce startup lock contention)
- Reuses `isTerminal()` from `help_width_unix.go`
- Fails gracefully if not a terminal or no database

### 10. `internal/cli/help.go` — Update Group Order

```go
groupObserve: {"events", "stats", "monitor", "doctor", "test"},
```

## Key Reuse Points

| Function | Location | Used For |
|----------|----------|----------|
| `openEventsDB()` | `cli/events.go:154` | Open DB in monitor command |
| `isTerminal()` | `cli/help_width_unix.go:18` | Check stdout is a terminal |
| `fallbackValue()` | `internal/tui` (local copy) | Display "-" for empty fields |
| `shorten()` | `internal/tui` (local copy) | Truncate command text |
| `db.ListEvents()` | `db/events.go` | Fetch events with filters |
| `db.SummarizeEvents()` | `db/events.go` | Aggregate stats |
| `db.EventRecord` | `db/events.go` | Event data model (17 fields) |
| `db.Approval` | `db/approvals.go` | Approval data model |
| `config.Mode()` | `config/paths.go` | Current fuse mode (returns `FuseMode` int; format for display via switch or `String()` method) |

## v1 Scope vs Deferred

**v1 (this implementation):**
- Events tail with color-coded decisions and cursor navigation
- Decision filter cycling (`d` key)
- Free-text command search (`/` key)
- Detail panel on Enter
- Stats summary view
- Approvals list view
- Live polling at 1.5s
- Header showing fuse mode
- `fuse monitor` / `fuse tui` command

**Deferred:**
- Source/agent/workspace filter text inputs
- Time-series sparklines
- Session grouping (collapsible sections)
- Filesystem watcher instead of polling
- Mouse support
- Export/yank to clipboard
- Live classification testing from TUI
- Configurable refresh rate

## Design Constraints

- **SQLite concurrent access**: TUI opens its own `*db.DB` handle with `_busy_timeout=5000` (5s). WAL mode allows concurrent readers (TUI) and writers (fuse hook/codex-shell) with minimal contention. The busy timeout handles transient lock waits (e.g., during WAL checkpoint or schema migration). Tick-loop queries must handle `SQLITE_BUSY` gracefully — show stale data with a footer status indicator rather than assuming reads always succeed. Note: the writer's periodic VACUUM (every 100th WAL checkpoint) acquires an exclusive lock that blocks TUI reads for its duration (typically <1s for a small DB, proportional to DB size). If this stall becomes noticeable, VACUUM cadence can be reduced or moved to an explicit maintenance command.
- **bubbletea v2 alt-screen**: Full terminal takeover; restores on exit. Clean even on panic (bubbletea handles signal restoration).
- **Empty database**: Show "Waiting for events..." in events view, "No data yet" in stats, "No approvals yet" in approvals view, rather than empty table.
- **Database errors**: Query errors MUST be propagated to the UI as an explicit error/stale-data indicator (e.g., footer warning). The monitor must not silently treat read failures as empty results.
- **Zero-result filtering/search**: Any filtered or search result with zero matching rows shows a dedicated empty-state message (e.g., `No matching events`) instead of a blank table. Verification must include empty DB, empty approvals, and zero-match search/filter cases.
- **No CGo**: All dependencies (bubbletea, lipgloss, modernc.org/sqlite) are pure Go.
- **Terminal safety**: All strings loaded from the database and rendered in the TUI MUST be sanitized before display. Strip ANSI/OSC escape sequences and non-printable control characters to prevent terminal injection/spoofing via crafted event data.

## Implementation Sequence

1. `go get` dependencies (updates go.mod/go.sum)
2. `internal/db/approvals_list.go` + test
3. `internal/tui/styles.go` + `internal/tui/keys.go` (parallel)
4. `internal/tui/events_view.go`
5. `internal/tui/stats_view.go` + `internal/tui/approvals_view.go` (parallel)
6. `internal/tui/model.go` (composes the above)
7. `internal/cli/monitor.go` + `internal/cli/help.go` update
8. Manual testing + snapshot tests. Snapshot tests must use fixed terminal sizes (120x40 and 80x24), seeded fixture data, descending count order with lexical tie-breaks on equal counts, and bar scaling where the maximum value in each section gets a full-width bar. Golden snapshots are required for Events, Stats, and Approvals views
9. `just check-local` quality gate

## Verification

1. `go build ./...` — compiles with new dependencies
2. `go test ./internal/db/ -run TestListApprovals -v` — new DB method works
3. `fuse monitor` — launches TUI, shows events, press Tab to switch views
4. Press `d` to cycle decision filter, `/` to search, `Enter` for detail
5. **Deterministic refresh test:** The model must accept injected tick/data messages in tests. A deterministic integration test shall seed a temp SQLite DB, deliver a synthetic `tickMsg`, and assert that the active view refreshes from the updated DB contents without waiting on wall-clock time.
6. **Manual smoke test:** Run `fuse test classify "echo hello"` in another terminal — event appears in TUI within 2s (smoke test only, not primary acceptance test)
7. Press `q` — clean exit, terminal restored
8. `go test -race ./...` — full suite green
9. `just check-local` — full quality gate

**Required test boundaries:**

- **Unit tests:** decision color mapping, search/filter logic (case-insensitivity, empty input, no matches), cursor/offset math (boundary conditions, page size), approval status computation (precedence, nil expires_at, expired vs consumed), empty-state rendering.
- **Integration tests:** model update cycle (Init → tick → dataMsg → View), view switching via tab/number keys, tick-driven refresh against a temp SQLite DB.
- **System smoke test:** `fuse monitor` startup in a pseudo-terminal, tab switching between all three views, and clean exit via `q`.
