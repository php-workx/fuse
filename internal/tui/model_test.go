package tui

import (
	"os"
	"path/filepath"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/runger/fuse/internal/db"
)

func openTestDB(t *testing.T) *db.DB {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	d, err := db.OpenDB(dbPath)
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	return d
}

func seedEvents(t *testing.T, d *db.DB, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		_ = d.LogEvent(&db.EventRecord{
			Command:  "echo test",
			Decision: "SAFE",
			Source:   "hook",
			Agent:    "claude",
		})
	}
}

func TestModel_InitReturnsTick(t *testing.T) {
	d := openTestDB(t)
	m := NewModel(d, "dryrun", nil)
	cmd := m.Init()
	if cmd == nil {
		t.Fatal("Init() should return a tick command, got nil")
	}
}

func TestModel_TickFetchesData(t *testing.T) {
	d := openTestDB(t)
	seedEvents(t, d, 3)

	m := NewModel(d, "enabled", nil)
	m.width = 120
	m.height = 40
	m.events.SetSize(120, 37)

	// Simulate tick.
	updated, cmd := m.Update(tickMsg{view: viewEvents})
	m = updated.(Model)

	if !m.fetching {
		t.Error("after tick, fetching should be true")
	}
	if cmd == nil {
		t.Fatal("tick should return fetchData command")
	}

	// Execute the fetch command synchronously.
	msg := cmd()
	updated, cmd = m.Update(msg)
	m = updated.(Model)

	if m.fetching {
		t.Error("after dataMsg, fetching should be false")
	}
	if len(m.events.events) != 3 {
		t.Errorf("events: got %d, want 3", len(m.events.events))
	}
	if cmd == nil {
		t.Fatal("dataMsg should schedule next tick")
	}
}

func TestModel_StaleResultDiscarded(t *testing.T) {
	d := openTestDB(t)
	seedEvents(t, d, 1)

	m := NewModel(d, "enabled", nil)
	m.width = 120
	m.height = 40
	m.events.SetSize(120, 37)

	// Simulate: fetchGen=0, start fetch.
	updated, cmd := m.Update(tickMsg{view: viewEvents})
	m = updated.(Model)

	// User switches view → fetchGen increments.
	m.fetchGen++

	// Stale result arrives with reqGen=0.
	staleMsg := dataMsg{reqGen: 0, view: viewEvents, events: []db.EventRecord{{ID: 999}}}
	updated, _ = m.Update(staleMsg)
	m = updated.(Model)

	// Events should NOT be updated (stale).
	if len(m.events.events) != 0 {
		t.Errorf("stale result should be discarded, got %d events", len(m.events.events))
	}
	_ = cmd
}

func TestModel_ViewSwitching(t *testing.T) {
	d := openTestDB(t)
	m := NewModel(d, "dryrun", nil)
	m.width = 80
	m.height = 24

	if m.activeView != viewEvents {
		t.Fatal("initial view should be events")
	}

	// Press Tab.
	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	m = updated.(Model)
	if m.activeView != viewStats {
		t.Errorf("after Tab: got %d, want viewStats", m.activeView)
	}

	// Press Tab again.
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	m = updated.(Model)
	if m.activeView != viewApprovals {
		t.Errorf("after Tab: got %d, want viewApprovals", m.activeView)
	}

	// Tab on approvals view stays on approvals (delegates to focus toggle).
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	m = updated.(Model)
	if m.activeView != viewApprovals {
		t.Errorf("Tab on approvals should stay (focus toggle): got %d, want viewApprovals", m.activeView)
	}

	// Use number key '1' to switch back to events.
	updated, _ = m.Update(tea.KeyPressMsg{Code: '1'})
	m = updated.(Model)
	if m.activeView != viewEvents {
		t.Errorf("after key '1': got %d, want viewEvents", m.activeView)
	}
}

func TestModel_ErrorPreservesData(t *testing.T) {
	d := openTestDB(t)
	seedEvents(t, d, 2)

	m := NewModel(d, "enabled", nil)
	m.width = 120
	m.height = 40
	m.events.SetSize(120, 37)

	// First successful fetch.
	updated, cmd := m.Update(tickMsg{view: viewEvents})
	m = updated.(Model)
	msg := cmd()
	updated, _ = m.Update(msg)
	m = updated.(Model)

	if len(m.events.events) != 2 {
		t.Fatalf("initial fetch: got %d events, want 2", len(m.events.events))
	}

	// Simulate error fetch.
	errMsg := dataMsg{reqGen: m.fetchGen, view: viewEvents, err: os.ErrClosed}
	updated, _ = m.Update(errMsg)
	m = updated.(Model)

	// Data should be preserved.
	if len(m.events.events) != 2 {
		t.Errorf("after error, events should be preserved: got %d", len(m.events.events))
	}
	if m.lastErr == nil {
		t.Error("lastErr should be set after error fetch")
	}
}

func TestModel_DetailPausesPolling(t *testing.T) {
	d := openTestDB(t)
	m := NewModel(d, "dryrun", nil)
	m.width = 80
	m.height = 24
	m.events.SetSize(80, 21)
	m.events.events = []db.EventRecord{{ID: 1, Decision: "SAFE", Command: "test"}}
	m.events.applyFilters()

	// Open detail panel.
	m.events.showDetail = true

	// Tick should NOT trigger a fetch.
	updated, cmd := m.Update(tickMsg{view: viewEvents})
	m = updated.(Model)

	if m.fetching {
		t.Error("tick while detail open should not set fetching=true")
	}
	if cmd == nil {
		t.Fatal("tick while detail open should still reschedule")
	}
}

func TestModel_ResizePropagates(t *testing.T) {
	d := openTestDB(t)
	m := NewModel(d, "dryrun", nil)

	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 50})
	m = updated.(Model)

	if m.width != 100 || m.height != 50 {
		t.Errorf("resize: got %dx%d, want 100x50", m.width, m.height)
	}
	if m.events.width != 100 {
		t.Errorf("events width: got %d, want 100", m.events.width)
	}
}

// --- tickCmd tests ---

func TestTickCmd_EventsInterval(t *testing.T) {
	cmd := tickCmd(viewEvents)
	if cmd == nil {
		t.Fatal("tickCmd(viewEvents) should return a non-nil Cmd")
	}
}

func TestTickCmd_ApprovalsInterval(t *testing.T) {
	cmd := tickCmd(viewApprovals)
	if cmd == nil {
		t.Fatal("tickCmd(viewApprovals) should return a non-nil Cmd")
	}
}

func TestTickCmd_StatsInterval(t *testing.T) {
	cmd := tickCmd(viewStats)
	if cmd == nil {
		t.Fatal("tickCmd(viewStats) should return a non-nil Cmd")
	}
}

// --- Stale tick discard tests ---

func TestModel_StaleTickReschedules(t *testing.T) {
	d := openTestDB(t)
	m := NewModel(d, "dryrun", nil)
	m.width = 80
	m.height = 24
	m.events.SetSize(80, 21)

	// Switch to stats view.
	m.activeView = viewStats

	// Send a tick for events (stale since we're on stats).
	updated, cmd := m.Update(tickMsg{view: viewEvents})
	m = updated.(Model)

	// Should NOT have started a fetch (stale tick).
	if m.fetching {
		t.Error("stale tick should not trigger a fetch")
	}

	// Should reschedule a tick (non-nil cmd).
	if cmd == nil {
		t.Fatal("stale tick should reschedule, got nil cmd")
	}
}

func TestModel_StaleTickDoesNotFetch(t *testing.T) {
	d := openTestDB(t)
	seedEvents(t, d, 2)

	m := NewModel(d, "dryrun", nil)
	m.width = 80
	m.height = 24

	// Switch to approvals view.
	m.activeView = viewApprovals

	// Send tick for events view (wrong view).
	updated, _ := m.Update(tickMsg{view: viewEvents})
	m = updated.(Model)

	// Events should remain empty since we never fetched for events.
	if len(m.events.events) != 0 {
		t.Errorf("stale tick should not fetch events data, got %d events", len(m.events.events))
	}
}
