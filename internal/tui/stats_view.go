package tui

import (
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"

	"github.com/runger/fuse/internal/db"
)

// StatsModel renders an aggregate summary dashboard.
type StatsModel struct {
	summary db.EventSummary
	hasData bool
	width   int
	height  int
}

// NewStatsModel creates an initialized StatsModel.
func NewStatsModel() StatsModel {
	return StatsModel{}
}

// SetData updates the summary data.
func (m *StatsModel) SetData(summary db.EventSummary) {
	m.summary = summary
	m.hasData = summary.Total > 0
}

// SetSize updates dimensions.
func (m *StatsModel) SetSize(w, h int) {
	m.width = w
	m.height = h
}

// Update handles messages (stats view is read-only, no key handling needed).
func (m StatsModel) Update(_ tea.Msg) (StatsModel, tea.Cmd) {
	return m, nil
}

// View renders the stats dashboard.
func (m StatsModel) View() string {
	if !m.hasData {
		return styleDim.Render("  No data yet")
	}

	var b strings.Builder

	fmt.Fprintf(&b, "\n  Total Events: %d\n\n", m.summary.Total)

	colWidth := m.width/2 - 2
	if colWidth < 35 {
		colWidth = 35
	}

	// Cap rows per section to half the available height minus headers.
	maxRows := (m.height - 6) / 2
	if maxRows < 3 {
		maxRows = 3
	}

	// By Decision + By Agent side by side.
	left := renderSection("By Decision", m.summary.ByDecision, colWidth, maxRows, labelWidthShort, true)
	right := renderSection("By Agent", m.summary.ByAgent, colWidth, maxRows, labelWidthShort, false)
	b.WriteString(sideBySide(left, right, colWidth))
	b.WriteString("\n")

	// By Source + By Workspace side by side.
	// Shorten workspace paths and cap to top 8 — workspaces are long-tail.
	shortWorkspaces := shortenWorkspacePaths(m.summary.ByWorkspace)
	wsMaxRows := maxRows
	if wsMaxRows > 8 {
		wsMaxRows = 8
	}
	left = renderSection("By Source", m.summary.BySource, colWidth, maxRows, labelWidthShort, false)
	right = renderSection("By Workspace", shortWorkspaces, colWidth, wsMaxRows, labelWidthWide, false)
	b.WriteString(sideBySide(left, right, colWidth))

	return b.String()
}

// Layout: "  LABEL         COUNT  ████████"
// Fixed columns: 2 indent + label + 2 gap + 6 count (right-aligned) + 2 gap + bar
const (
	labelWidthShort = 14 // for decision, agent, source
	labelWidthWide  = 24 // for workspace paths
	countWidth      = 6
)

func renderSection(title string, counts map[string]int, width, maxRows, lblWidth int, colorDecisions bool) []string {
	lines := []string{
		"  " + styleColHeader.Render(title),
	}

	sorted := sortedCounts(counts)
	if len(sorted) == 0 {
		lines = append(lines, styleDim.Render("  (none)"))
		return lines
	}

	// Cap entries to maxRows.
	if len(sorted) > maxRows {
		sorted = sorted[:maxRows]
	}

	maxVal := sorted[0].count
	prefix := 2 + lblWidth + 2 + countWidth + 2
	maxBarWidth := width - prefix
	if maxBarWidth < 3 {
		maxBarWidth = 3
	}

	for _, kv := range sorted {
		barLen := 0
		if maxVal > 0 {
			barLen = kv.count * maxBarWidth / maxVal
		}
		if barLen < 1 && kv.count > 0 {
			barLen = 1
		}
		bar := strings.Repeat("█", barLen)

		label := sanitize(shorten(kv.key, lblWidth))
		paddedLabel := fmt.Sprintf("%-*s", lblWidth, label)
		if colorDecisions {
			paddedLabel = decisionStyle(kv.key).Render(paddedLabel)
		}

		lines = append(lines, fmt.Sprintf("  %s  %*d  %s", paddedLabel, countWidth, kv.count, bar))
	}
	return lines
}

func sideBySide(left, right []string, colWidth int) string {
	maxLines := len(left)
	if len(right) > maxLines {
		maxLines = len(right)
	}

	var b strings.Builder
	for i := 0; i < maxLines; i++ {
		l := ""
		if i < len(left) {
			l = left[i]
		}
		r := ""
		if i < len(right) {
			r = right[i]
		}
		// Pad left column to fixed width.
		vl := visibleLen(l)
		pad := colWidth - vl
		if pad < 0 {
			pad = 0
		}
		b.WriteString(l + strings.Repeat(" ", pad) + r + "\n")
	}
	return b.String()
}

type countPair struct {
	key   string
	count int
}

func sortedCounts(m map[string]int) []countPair {
	pairs := make([]countPair, 0, len(m))
	for k, v := range m {
		pairs = append(pairs, countPair{k, v})
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].count != pairs[j].count {
			return pairs[i].count > pairs[j].count
		}
		return pairs[i].key < pairs[j].key
	})
	return pairs
}

// shortenWorkspacePaths replaces full paths with their last 2 components
// (e.g., "/Users/runger/workspaces/fuse" → "workspaces/fuse").
// Merges counts for paths that collide after shortening.
func shortenWorkspacePaths(counts map[string]int) map[string]int {
	short := make(map[string]int, len(counts))
	for path, count := range counts {
		label := shortenToLastN(path, 2)
		short[label] += count
	}
	return short
}

// shortenToLastN returns the last n path components, prefixed with ".../" if truncated.
func shortenToLastN(path string, n int) string {
	if path == "" {
		return "(unknown)"
	}
	parts := strings.Split(path, "/")
	// Remove trailing empty string from paths ending in /
	for len(parts) > 0 && parts[len(parts)-1] == "" {
		parts = parts[:len(parts)-1]
	}
	if len(parts) <= n {
		return path
	}
	return ".../" + strings.Join(parts[len(parts)-n:], "/")
}

// visibleLen returns the display column width of a string, stripping ANSI
// codes and counting runes (not bytes) so multi-byte chars like █ are 1 column.
func visibleLen(s string) int {
	stripped := reControlChars.ReplaceAllString(s, "")
	return utf8.RuneCountInString(stripped)
}
