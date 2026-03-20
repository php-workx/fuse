package tui

import (
	"fmt"
	"sort"
	"strings"

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
	left := renderSection("By Decision", m.summary.ByDecision, colWidth, maxRows, true)
	right := renderSection("By Agent", m.summary.ByAgent, colWidth, maxRows, false)
	b.WriteString(sideBySide(left, right, colWidth))
	b.WriteString("\n")

	// By Source + By Workspace side by side.
	left = renderSection("By Source", m.summary.BySource, colWidth, maxRows, false)
	right = renderSection("By Workspace", m.summary.ByWorkspace, colWidth, maxRows, false)
	b.WriteString(sideBySide(left, right, colWidth))

	return b.String()
}

// Layout: "  LABEL         COUNT  ████████"
// Fixed columns: 2 indent + 14 label + 2 gap + 6 count (right-aligned) + 2 gap + bar
const (
	labelWidth = 14
	countWidth = 6
	rowPrefix  = 2 + labelWidth + 2 + countWidth + 2 // = 26
)

func renderSection(title string, counts map[string]int, width, maxRows int, colorDecisions bool) []string {
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
	maxBarWidth := width - rowPrefix
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

		label := sanitize(shorten(kv.key, labelWidth))
		paddedLabel := fmt.Sprintf("%-*s", labelWidth, label)
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

// visibleLen estimates the visible length of a string (strips ANSI codes).
func visibleLen(s string) int {
	return len(reControlChars.ReplaceAllString(s, ""))
}
