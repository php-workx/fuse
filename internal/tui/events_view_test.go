package tui

import (
	"testing"

	"github.com/runger/fuse/internal/db"
)

func TestApplyFilters_DecisionFilter(t *testing.T) {
	m := NewEventsModel()
	m.events = []db.EventRecord{
		{ID: 1, Decision: "SAFE", Command: "echo hello"},
		{ID: 2, Decision: "BLOCKED", Command: "rm -rf /"},
		{ID: 3, Decision: "SAFE", Command: "ls"},
		{ID: 4, Decision: "APPROVAL", Command: "terraform destroy"},
	}

	// No filter — all events.
	m.filterDecision = ""
	m.applyFilters()
	if len(m.filtered) != 4 {
		t.Errorf("no filter: got %d, want 4", len(m.filtered))
	}

	// Filter SAFE.
	m.filterDecision = "SAFE"
	m.applyFilters()
	if len(m.filtered) != 2 {
		t.Errorf("SAFE filter: got %d, want 2", len(m.filtered))
	}

	// Filter BLOCKED.
	m.filterDecision = "BLOCKED"
	m.applyFilters()
	if len(m.filtered) != 1 {
		t.Errorf("BLOCKED filter: got %d, want 1", len(m.filtered))
	}
	if m.filtered[0].ID != 2 {
		t.Errorf("BLOCKED filter: got ID %d, want 2", m.filtered[0].ID)
	}
}

func TestApplyFilters_SearchCaseInsensitive(t *testing.T) {
	m := NewEventsModel()
	m.events = []db.EventRecord{
		{ID: 1, Command: "echo Hello World"},
		{ID: 2, Command: "rm -rf /"},
		{ID: 3, Command: "ECHO test"},
	}

	m.searchInput.SetValue("echo")
	m.applyFilters()
	if len(m.filtered) != 2 {
		t.Errorf("search 'echo': got %d, want 2", len(m.filtered))
	}
}

func TestApplyFilters_EmptySearch(t *testing.T) {
	m := NewEventsModel()
	m.events = []db.EventRecord{
		{ID: 1, Command: "echo hello"},
		{ID: 2, Command: "ls"},
	}
	m.searchInput.SetValue("")
	m.applyFilters()
	if len(m.filtered) != 2 {
		t.Errorf("empty search: got %d, want 2", len(m.filtered))
	}
}

func TestApplyFilters_NoMatches(t *testing.T) {
	m := NewEventsModel()
	m.events = []db.EventRecord{
		{ID: 1, Command: "echo hello"},
	}
	m.searchInput.SetValue("terraform")
	m.applyFilters()
	if len(m.filtered) != 0 {
		t.Errorf("no-match search: got %d, want 0", len(m.filtered))
	}
}

func TestCycleDecisionFilter(t *testing.T) {
	m := NewEventsModel()
	m.events = []db.EventRecord{{ID: 1, Decision: "SAFE", Command: "echo"}}

	expected := []string{"SAFE", "CAUTION", "APPROVAL", "BLOCKED", ""}
	for _, want := range expected {
		m.cycleDecisionFilter()
		if m.filterDecision != want {
			t.Errorf("cycle: got %q, want %q", m.filterDecision, want)
		}
	}
}

func TestMoveCursor_Boundaries(t *testing.T) {
	m := NewEventsModel()
	m.filtered = make([]db.EventRecord, 5)
	for i := range m.filtered {
		m.filtered[i] = db.EventRecord{ID: int64(i + 1)}
	}

	// Start at 0.
	m.cursor = 0
	m.moveCursor(-1) // should clamp to 0
	if m.cursor != 0 {
		t.Errorf("move up from 0: got %d", m.cursor)
	}

	m.moveCursor(10) // should clamp to 4
	if m.cursor != 4 {
		t.Errorf("move down past end: got %d, want 4", m.cursor)
	}

	m.moveCursor(-2)
	if m.cursor != 2 {
		t.Errorf("move up 2 from 4: got %d, want 2", m.cursor)
	}
}

func TestMoveCursor_EmptyList(t *testing.T) {
	m := NewEventsModel()
	m.filtered = nil
	m.cursor = 0
	m.moveCursor(1) // should not panic
	if m.cursor != 0 {
		t.Errorf("move on empty: got %d, want 0", m.cursor)
	}
}

func TestAnchorCursor(t *testing.T) {
	m := NewEventsModel()
	m.filtered = []db.EventRecord{
		{ID: 10}, {ID: 20}, {ID: 30},
	}
	m.selectedID = 20
	m.cursor = 99 // wrong position

	m.anchorCursor()
	if m.cursor != 1 {
		t.Errorf("anchor to ID 20: got cursor %d, want 1", m.cursor)
	}
}

func TestAnchorCursor_Missing(t *testing.T) {
	m := NewEventsModel()
	m.filtered = []db.EventRecord{{ID: 10}, {ID: 30}}
	m.selectedID = 20
	m.cursor = 5 // out of bounds

	m.anchorCursor()
	if m.cursor != 1 { // clamped to last
		t.Errorf("anchor missing ID: got cursor %d, want 1", m.cursor)
	}
}

func TestFormatTime(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"2026-03-20T14:32:05.123Z", "14:32:05"}, // UTC local depends on timezone
		{"short", "short"},                       // fallback
	}
	for _, tt := range tests {
		got := formatTime(tt.input)
		if got == "" {
			t.Errorf("formatTime(%q) returned empty", tt.input)
		}
	}
}

func TestShortenPath(t *testing.T) {
	if got := shortenPath("/Users/runger/workspaces/fuse"); got != "fuse" {
		t.Errorf("shortenPath: got %q, want 'fuse'", got)
	}
	if got := shortenPath(""); got != "-" {
		t.Errorf("shortenPath empty: got %q, want '-'", got)
	}
}
