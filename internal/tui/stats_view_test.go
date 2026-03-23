package tui

import (
	"strings"
	"testing"

	"github.com/runger/fuse/internal/db"
)

// --- shortenToLastN tests ---

func TestShortenToLastN_DeepPath(t *testing.T) {
	got := shortenToLastN("/Users/runger/workspaces/fuse", 2)
	if got != ".../workspaces/fuse" {
		t.Errorf("deep path: got %q, want %q", got, ".../workspaces/fuse")
	}
}

func TestShortenToLastN_ShallowPath(t *testing.T) {
	// Exactly 2 components (after splitting): "workspaces" and "fuse".
	got := shortenToLastN("workspaces/fuse", 2)
	// len(parts) == 2, which is <= n, so returns original.
	if got != "workspaces/fuse" {
		t.Errorf("shallow path: got %q, want %q", got, "workspaces/fuse")
	}
}

func TestShortenToLastN_RootPath(t *testing.T) {
	// "/" splits into ["", ""], trailing empty removed -> [""].
	// len(parts) == 1 <= 2, returns original.
	got := shortenToLastN("/", 2)
	if got != "/" {
		t.Errorf("root path: got %q, want %q", got, "/")
	}
}

func TestShortenToLastN_Empty(t *testing.T) {
	got := shortenToLastN("", 2)
	if got != "(unknown)" {
		t.Errorf("empty path: got %q, want %q", got, "(unknown)")
	}
}

func TestShortenToLastN_SingleComponent(t *testing.T) {
	got := shortenToLastN("fuse", 2)
	if got != "fuse" {
		t.Errorf("single component: got %q, want %q", got, "fuse")
	}
}

func TestShortenToLastN_TrailingSlash(t *testing.T) {
	got := shortenToLastN("/Users/runger/workspaces/fuse/", 2)
	if got != ".../workspaces/fuse" {
		t.Errorf("trailing slash: got %q, want %q", got, ".../workspaces/fuse")
	}
}

// --- visibleLen tests ---

func TestVisibleLen_ASCII(t *testing.T) {
	got := visibleLen("hello world")
	if got != 11 {
		t.Errorf("ASCII: got %d, want 11", got)
	}
}

func TestVisibleLen_WithANSI(t *testing.T) {
	// ANSI codes should be stripped before counting.
	got := visibleLen("\x1b[31mhello\x1b[0m")
	if got != 5 {
		t.Errorf("ANSI: got %d, want 5", got)
	}
}

func TestVisibleLen_WithBlockChar(t *testing.T) {
	// Block characters like \u2588 are multi-byte but 1 rune each.
	got := visibleLen(string([]rune{'\u2588', '\u2588', '\u2588'}))
	if got != 3 {
		t.Errorf("block chars: got %d, want 3", got)
	}
}

func TestVisibleLen_Empty(t *testing.T) {
	got := visibleLen("")
	if got != 0 {
		t.Errorf("empty: got %d, want 0", got)
	}
}

// --- sortedCounts tests ---

func TestSortedCounts_DescendingByCount(t *testing.T) {
	m := map[string]int{
		"alpha": 3,
		"beta":  10,
		"gamma": 1,
	}
	got := sortedCounts(m)
	if len(got) != 3 {
		t.Fatalf("sortedCounts: got %d pairs, want 3", len(got))
	}
	if got[0].key != "beta" || got[0].count != 10 {
		t.Errorf("first: got %v, want {beta, 10}", got[0])
	}
	if got[1].key != "alpha" || got[1].count != 3 {
		t.Errorf("second: got %v, want {alpha, 3}", got[1])
	}
	if got[2].key != "gamma" || got[2].count != 1 {
		t.Errorf("third: got %v, want {gamma, 1}", got[2])
	}
}

func TestSortedCounts_AlphabeticalTiebreak(t *testing.T) {
	m := map[string]int{
		"cherry": 5,
		"apple":  5,
		"banana": 5,
	}
	got := sortedCounts(m)
	if len(got) != 3 {
		t.Fatalf("sortedCounts: got %d pairs, want 3", len(got))
	}
	// Same count, so alphabetical order: apple, banana, cherry.
	if got[0].key != "apple" {
		t.Errorf("tiebreak first: got %q, want apple", got[0].key)
	}
	if got[1].key != "banana" {
		t.Errorf("tiebreak second: got %q, want banana", got[1].key)
	}
	if got[2].key != "cherry" {
		t.Errorf("tiebreak third: got %q, want cherry", got[2].key)
	}
}

func TestSortedCounts_EmptyMap(t *testing.T) {
	got := sortedCounts(map[string]int{})
	if len(got) != 0 {
		t.Errorf("empty map: got %d pairs, want 0", len(got))
	}
}

func TestSortedCounts_NilMap(t *testing.T) {
	got := sortedCounts(nil)
	if len(got) != 0 {
		t.Errorf("nil map: got %d pairs, want 0", len(got))
	}
}

// --- SetData / SetJudgeSummary / View tests ---

func TestStatsView_SetData(t *testing.T) {
	m := NewStatsModel()
	m.SetSize(80, 40)
	m.SetData(db.EventSummary{
		Total:       100,
		ByDecision:  map[string]int{"SAFE": 80, "CAUTION": 15, "APPROVAL": 5},
		ByAgent:     map[string]int{"claude": 100},
		BySource:    map[string]int{"hook": 90, "codex-shell": 10},
		ByWorkspace: map[string]int{"/Users/dev/project": 100},
	})

	got := m.View()
	if !strings.Contains(got, "100") {
		t.Error("View should show total count")
	}
	if !strings.Contains(got, "SAFE") {
		t.Error("View should show SAFE decision")
	}
}

func TestStatsView_WithJudgeSummary(t *testing.T) {
	m := NewStatsModel()
	m.SetSize(80, 40)
	m.SetData(db.EventSummary{
		Total:       50,
		ByDecision:  map[string]int{"CAUTION": 50},
		ByAgent:     map[string]int{"claude": 50},
		BySource:    map[string]int{"hook": 50},
		ByWorkspace: map[string]int{"/tmp": 50},
	})
	m.SetJudgeSummary(db.JudgeSummary{
		Evaluated:      42,
		Agreed:         35,
		WouldUpgrade:   3,
		WouldDowngrade: 4,
		Errors:         0,
		AvgLatencyMs:   487,
	})

	got := m.View()
	if !strings.Contains(got, "Judge Accuracy") {
		t.Error("View should show Judge Accuracy section when evaluated > 0")
	}
	if !strings.Contains(got, "42") {
		t.Error("View should show evaluated count")
	}
	if !strings.Contains(got, "35") {
		t.Error("View should show agreed count")
	}
	if !strings.Contains(got, "487") {
		t.Error("View should show avg latency")
	}
}

func TestStatsView_NoJudgeSummary(t *testing.T) {
	m := NewStatsModel()
	m.SetSize(80, 40)
	m.SetData(db.EventSummary{
		Total:       10,
		ByDecision:  map[string]int{"SAFE": 10},
		ByAgent:     map[string]int{"claude": 10},
		BySource:    map[string]int{"hook": 10},
		ByWorkspace: map[string]int{"/tmp": 10},
	})
	// No SetJudgeSummary — Evaluated stays 0.

	got := m.View()
	if strings.Contains(got, "Judge Accuracy") {
		t.Error("View should NOT show Judge Accuracy section when evaluated == 0")
	}
}

func TestStatsView_NoData(t *testing.T) {
	m := NewStatsModel()
	m.SetSize(80, 40)
	got := m.View()
	if !strings.Contains(got, "No data") {
		t.Error("View should show 'No data' when no data set")
	}
}

func TestStatsView_RenderSection(t *testing.T) {
	counts := map[string]int{"alpha": 10, "beta": 5}
	lines := renderSection("Test Section", counts, 40, 10, labelWidthShort, false)
	if len(lines) < 3 {
		t.Fatalf("renderSection: expected at least 3 lines (title + 2 entries), got %d", len(lines))
	}
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "Test Section") {
		t.Error("renderSection should contain title")
	}
	if !strings.Contains(joined, "alpha") {
		t.Error("renderSection should contain 'alpha'")
	}
}

func TestStatsView_SideBySide(t *testing.T) {
	left := []string{"left-1", "left-2"}
	right := []string{"right-1", "right-2", "right-3"}
	got := sideBySide(left, right, 30)
	lines := strings.Split(got, "\n")
	// Should have max(2,3) = 3 non-empty lines.
	nonEmpty := 0
	for _, l := range lines {
		if strings.TrimSpace(l) != "" {
			nonEmpty++
		}
	}
	if nonEmpty < 3 {
		t.Errorf("sideBySide: expected at least 3 non-empty lines, got %d", nonEmpty)
	}
}

func TestStatsView_ShortenWorkspacePaths(t *testing.T) {
	input := map[string]int{
		"/Users/runger/workspaces/fuse":    50,
		"/Users/runger/workspaces/project": 30,
	}
	got := shortenWorkspacePaths(input)
	// Should shorten to last 2 components.
	if _, ok := got[".../workspaces/fuse"]; !ok {
		t.Errorf("shortenWorkspacePaths: expected '.../workspaces/fuse', got keys %v", got)
	}
}
