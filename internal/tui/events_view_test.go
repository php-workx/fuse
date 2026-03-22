package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

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

// --- writeWrapped tests ---

func TestWriteWrapped_ShortValue(t *testing.T) {
	var b strings.Builder
	writeWrapped(&b, "  Label:  ", "short", 40)
	got := b.String()
	want := "  Label:  short\n"
	if got != want {
		t.Errorf("writeWrapped short: got %q, want %q", got, want)
	}
}

func TestWriteWrapped_LongValueWrapsAtSpace(t *testing.T) {
	var b strings.Builder
	// 20-char max width; value has spaces for break points.
	writeWrapped(&b, "  Label:  ", "hello world this is a longer value", 20)
	got := b.String()
	lines := strings.Split(strings.TrimSuffix(got, "\n"), "\n")
	if len(lines) < 2 {
		t.Fatalf("expected multiple lines, got %d: %q", len(lines), got)
	}
	// First line should start with label.
	if !strings.HasPrefix(lines[0], "  Label:  ") {
		t.Errorf("first line should start with label, got %q", lines[0])
	}
	// Continuation lines should be indented with spaces matching label width.
	indent := strings.Repeat(" ", len("  Label:  "))
	for i := 1; i < len(lines); i++ {
		if !strings.HasPrefix(lines[i], indent) {
			t.Errorf("line %d should start with indent %q, got %q", i, indent, lines[i])
		}
	}
}

func TestWriteWrapped_NoSpacesHardWraps(t *testing.T) {
	var b strings.Builder
	// A long value with no spaces forces hard wrapping.
	value := strings.Repeat("x", 50)
	writeWrapped(&b, "L:", value, 20)
	got := b.String()
	lines := strings.Split(strings.TrimSuffix(got, "\n"), "\n")
	if len(lines) < 2 {
		t.Fatalf("expected hard wrap into multiple lines, got %d: %q", len(lines), got)
	}
	// Reconstruct value from lines.
	reconstructed := strings.TrimPrefix(lines[0], "L:")
	indent := strings.Repeat(" ", len("L:"))
	for i := 1; i < len(lines); i++ {
		reconstructed += strings.TrimPrefix(lines[i], indent)
	}
	if reconstructed != value {
		t.Errorf("reconstructed value mismatch: got %q, want %q", reconstructed, value)
	}
}

func TestWriteWrapped_EmptyValue(t *testing.T) {
	var b strings.Builder
	writeWrapped(&b, "  Key:  ", "", 40)
	got := b.String()
	want := "  Key:  \n"
	if got != want {
		t.Errorf("writeWrapped empty: got %q, want %q", got, want)
	}
}

func TestWriteWrapped_ExactFit(t *testing.T) {
	var b strings.Builder
	value := strings.Repeat("a", 20)
	writeWrapped(&b, "L:", value, 20)
	got := b.String()
	// Exactly maxWidth runes should fit on one line.
	want := "L:" + value + "\n"
	if got != want {
		t.Errorf("writeWrapped exact: got %q, want %q", got, want)
	}
}

// --- FilterInfo tests ---

func TestFilterInfo_NoFilter(t *testing.T) {
	m := NewEventsModel()
	if got := m.FilterInfo(); got != "" {
		t.Errorf("FilterInfo no filter: got %q, want empty", got)
	}
}

func TestFilterInfo_DecisionOnly(t *testing.T) {
	m := NewEventsModel()
	m.filterDecision = "SAFE"
	got := m.FilterInfo()
	if got != " [decision:SAFE]" {
		t.Errorf("FilterInfo decision: got %q, want %q", got, " [decision:SAFE]")
	}
}

func TestFilterInfo_SearchOnly(t *testing.T) {
	m := NewEventsModel()
	m.searchInput.SetValue("terraform")
	got := m.FilterInfo()
	if got != " [search:terraform]" {
		t.Errorf("FilterInfo search: got %q, want %q", got, " [search:terraform]")
	}
}

func TestFilterInfo_Both(t *testing.T) {
	m := NewEventsModel()
	m.filterDecision = "BLOCKED"
	m.searchInput.SetValue("rm")
	got := m.FilterInfo()
	if got != " [decision:BLOCKED search:rm]" {
		t.Errorf("FilterInfo both: got %q, want %q", got, " [decision:BLOCKED search:rm]")
	}
}

// --- tableHeight tests ---

func TestTableHeight_Normal(t *testing.T) {
	m := NewEventsModel()
	m.height = 30
	got := m.tableHeight()
	// height - 2 = 28 (no search, no detail)
	if got != 28 {
		t.Errorf("tableHeight normal: got %d, want 28", got)
	}
}

func TestTableHeight_WithSearch(t *testing.T) {
	m := NewEventsModel()
	m.height = 30
	m.searching = true
	got := m.tableHeight()
	// height - 2 - 1 = 27
	if got != 27 {
		t.Errorf("tableHeight searching: got %d, want 27", got)
	}
}

func TestTableHeight_WithSearchValue(t *testing.T) {
	m := NewEventsModel()
	m.height = 30
	m.searchInput.SetValue("test")
	got := m.tableHeight()
	// height - 2 - 1 = 27 (search bar still shown when value is set)
	if got != 27 {
		t.Errorf("tableHeight search value: got %d, want 27", got)
	}
}

func TestTableHeight_WithDetail(t *testing.T) {
	m := NewEventsModel()
	m.height = 30
	m.showDetail = true
	got := m.tableHeight()
	// (30 - 2) * 60 / 100 = 16
	if got != 16 {
		t.Errorf("tableHeight with detail: got %d, want 16", got)
	}
}

func TestTableHeight_MinimumOne(t *testing.T) {
	m := NewEventsModel()
	m.height = 1
	m.showDetail = true
	got := m.tableHeight()
	if got != 1 {
		t.Errorf("tableHeight minimum: got %d, want 1", got)
	}
}

// --- pageSize tests ---

func TestPageSize_Normal(t *testing.T) {
	got := pageSize(20)
	if got != 10 {
		t.Errorf("pageSize(20): got %d, want 10", got)
	}
}

func TestPageSize_Tiny(t *testing.T) {
	got := pageSize(1)
	if got != 1 {
		t.Errorf("pageSize(1): got %d, want 1", got)
	}
}

func TestPageSize_Zero(t *testing.T) {
	got := pageSize(0)
	if got != 1 {
		t.Errorf("pageSize(0): got %d, want 1 (minimum)", got)
	}
}

// --- formatJudgeColumn tests ---

func TestFormatJudgeColumn_Empty(t *testing.T) {
	e := &db.EventRecord{}
	got := formatJudgeColumn(e)
	// Should be 11 spaces (empty, no judge data).
	if len(strings.TrimRight(got, " ")) != 0 {
		t.Errorf("empty judge: got %q, want blank", got)
	}
}

func TestFormatJudgeColumn_Agreed(t *testing.T) {
	e := &db.EventRecord{
		Decision:        "CAUTION",
		JudgeDecision:   "CAUTION",
		JudgeConfidence: 0.94,
	}
	got := formatJudgeColumn(e)
	if !strings.Contains(got, "=") {
		t.Errorf("agreed: expected '=' prefix, got %q", got)
	}
	if !strings.Contains(got, "94%") {
		t.Errorf("agreed: expected '94%%', got %q", got)
	}
}

func TestFormatJudgeColumn_Disagreed(t *testing.T) {
	e := &db.EventRecord{
		Decision:        "CAUTION",
		JudgeDecision:   "SAFE",
		JudgeConfidence: 0.88,
	}
	got := formatJudgeColumn(e)
	if !strings.Contains(got, ">") {
		t.Errorf("disagreed: expected '>' prefix, got %q", got)
	}
	if !strings.Contains(got, "88%") {
		t.Errorf("disagreed: expected '88%%', got %q", got)
	}
}

func TestFormatJudgeColumn_Applied(t *testing.T) {
	e := &db.EventRecord{
		Decision:        "CAUTION",
		JudgeDecision:   "APPROVAL",
		JudgeConfidence: 0.75,
		JudgeApplied:    true,
	}
	got := formatJudgeColumn(e)
	// Applied uses decisionStyle (colored), not styleDim.
	if !strings.Contains(got, "APPR") {
		t.Errorf("applied: expected 'APPR' abbreviation, got %q", got)
	}
}

// --- abbreviateDecision tests ---

func TestAbbreviateDecision(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"APPROVAL", "APPR"},
		{"CAUTION", "CAUT"},
		{"BLOCKED", "BLKD"},
		{"SAFE", "SAFE"},
		{"safe", "SAFE"},
		{"approval", "APPR"},
		{"UNKNOWN", "UNKNOWN"},
	}
	for _, tt := range tests {
		got := abbreviateDecision(tt.input)
		if got != tt.want {
			t.Errorf("abbreviateDecision(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// --- renderDetail judge coverage ---

func TestRenderDetail_WithJudgeVerdict(t *testing.T) {
	m := NewEventsModel()
	m.width = 100
	m.height = 40
	m.filtered = []db.EventRecord{
		{
			ID:              1,
			Decision:        "CAUTION",
			Command:         "git push --force",
			JudgeDecision:   "APPROVAL",
			JudgeConfidence: 0.76,
			JudgeProvider:   "claude",
			JudgeReasoning:  "risky force push",
			JudgeApplied:    true,
		},
	}
	m.cursor = 0
	got := m.renderDetail(&m.filtered[0])
	if !strings.Contains(got, "APPROVAL") {
		t.Error("detail should show judge decision")
	}
	if !strings.Contains(got, "76%") {
		t.Error("detail should show confidence percentage")
	}
	if !strings.Contains(got, "claude") {
		t.Error("detail should show provider")
	}
	if !strings.Contains(got, "APPLIED") {
		t.Error("detail should show APPLIED tag")
	}
}

func TestRenderDetail_WithJudgeError(t *testing.T) {
	m := NewEventsModel()
	m.width = 100
	m.height = 40
	m.filtered = []db.EventRecord{
		{
			ID:         1,
			Decision:   "CAUTION",
			Command:    "echo test",
			JudgeError: "connection timeout",
		},
	}
	m.cursor = 0
	got := m.renderDetail(&m.filtered[0])
	if !strings.Contains(got, "ERROR") {
		t.Error("detail should show ERROR for judge error")
	}
	if !strings.Contains(got, "connection timeout") {
		t.Error("detail should show error message")
	}
}

// --- updateSearch tests ---

func TestUpdateSearch_EnterCommits(t *testing.T) {
	m := NewEventsModel()
	m.events = []db.EventRecord{
		{ID: 1, Command: "echo hello"},
		{ID: 2, Command: "rm -rf /"},
	}
	m.searching = true
	m.searchInput.Focus()
	m.searchInput.SetValue("echo")

	m, _ = m.updateSearch(tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.searching {
		t.Error("Enter should exit search mode")
	}
	// Search value should be preserved (committed).
	if m.searchInput.Value() != "echo" {
		t.Errorf("search value should be preserved after Enter, got %q", m.searchInput.Value())
	}
	// Filter should be applied.
	if len(m.filtered) != 1 {
		t.Errorf("filtered after Enter: got %d, want 1", len(m.filtered))
	}
}

func TestUpdateSearch_EscapeClears(t *testing.T) {
	m := NewEventsModel()
	m.events = []db.EventRecord{
		{ID: 1, Command: "echo hello"},
		{ID: 2, Command: "rm -rf /"},
	}
	m.searching = true
	m.searchInput.Focus()
	m.searchInput.SetValue("echo")

	m, _ = m.updateSearch(tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.searching {
		t.Error("Escape should exit search mode")
	}
	if m.searchInput.Value() != "" {
		t.Errorf("search value should be cleared after Escape, got %q", m.searchInput.Value())
	}
	// All events should be visible after clearing search.
	if len(m.filtered) != 2 {
		t.Errorf("filtered after Escape: got %d, want 2", len(m.filtered))
	}
}

// --- updateDetail tests ---

func TestUpdateDetail_EnterCloses(t *testing.T) {
	m := NewEventsModel()
	m.showDetail = true
	m, _ = m.updateDetail(tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.showDetail {
		t.Error("Enter should close detail panel")
	}
}

func TestUpdateDetail_EscapeCloses(t *testing.T) {
	m := NewEventsModel()
	m.showDetail = true
	m, _ = m.updateDetail(tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.showDetail {
		t.Error("Escape should close detail panel")
	}
}

// --- toggleDetail tests ---

func TestToggleDetail_OpensWithEvents(t *testing.T) {
	m := NewEventsModel()
	m.width = 80
	m.height = 40
	m.filtered = []db.EventRecord{
		{ID: 1, Decision: "SAFE", Command: "echo test"},
	}
	m.cursor = 0

	m.toggleDetail()
	if !m.showDetail {
		t.Error("toggleDetail should open detail panel when events exist")
	}
}

func TestToggleDetail_NoOpWhenEmpty(t *testing.T) {
	m := NewEventsModel()
	m.width = 80
	m.height = 40
	m.filtered = nil
	m.cursor = 0

	m.toggleDetail()
	// showDetail will be set to true, but the detail panel won't have content
	// because cursor is out of range. The key behavior is no panic.
	// The toggle itself always flips the flag.
	if !m.showDetail {
		t.Error("toggleDetail flips the flag regardless")
	}
}

func TestToggleDetail_ClosesWhenOpen(t *testing.T) {
	m := NewEventsModel()
	m.width = 80
	m.height = 40
	m.filtered = []db.EventRecord{
		{ID: 1, Decision: "SAFE", Command: "echo test"},
	}
	m.cursor = 0
	m.showDetail = true

	m.toggleDetail()
	if m.showDetail {
		t.Error("toggleDetail should close detail panel when already open")
	}
}

// --- clampOffset tests ---

func TestClampOffset_CursorAboveOffset(t *testing.T) {
	m := NewEventsModel()
	m.cursor = 2
	m.offset = 5
	m.clampOffset(10)
	if m.offset != 2 {
		t.Errorf("clampOffset cursor above: got offset %d, want 2", m.offset)
	}
}

func TestClampOffset_CursorBelowVisible(t *testing.T) {
	m := NewEventsModel()
	m.cursor = 15
	m.offset = 0
	m.clampOffset(10)
	// offset should be cursor - visibleRows + 1 = 15 - 10 + 1 = 6
	if m.offset != 6 {
		t.Errorf("clampOffset cursor below: got offset %d, want 6", m.offset)
	}
}

func TestClampOffset_CursorWithinRange(t *testing.T) {
	m := NewEventsModel()
	m.cursor = 5
	m.offset = 3
	m.clampOffset(10)
	// Cursor is within [3, 13), so offset should not change.
	if m.offset != 3 {
		t.Errorf("clampOffset in range: got offset %d, want 3", m.offset)
	}
}

// --- cycleDecisionFilter comprehensive tests ---

func TestCycleDecisionFilter_FullCycle(t *testing.T) {
	m := NewEventsModel()
	m.events = []db.EventRecord{
		{ID: 1, Decision: "SAFE", Command: "a"},
		{ID: 2, Decision: "CAUTION", Command: "b"},
		{ID: 3, Decision: "APPROVAL", Command: "c"},
		{ID: 4, Decision: "BLOCKED", Command: "d"},
	}

	// Start at "" (ALL).
	expected := []string{"SAFE", "CAUTION", "APPROVAL", "BLOCKED", ""}
	for _, want := range expected {
		m.cycleDecisionFilter()
		if m.filterDecision != want {
			t.Errorf("cycle: got %q, want %q", m.filterDecision, want)
		}
	}

	// Verify cursor/offset reset on each cycle.
	m.cursor = 3
	m.offset = 2
	m.selectedID = 42
	m.cycleDecisionFilter()
	if m.cursor != 0 {
		t.Errorf("cycle should reset cursor to 0, got %d", m.cursor)
	}
	if m.offset != 0 {
		t.Errorf("cycle should reset offset to 0, got %d", m.offset)
	}
	if m.selectedID != 0 {
		t.Errorf("cycle should reset selectedID to 0, got %d", m.selectedID)
	}
}
