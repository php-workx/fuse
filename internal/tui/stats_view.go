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

	colWidth := m.width/2 - 4
	if colWidth < 30 {
		colWidth = 30
	}

	// By Decision + By Agent side by side.
	left := renderSection("By Decision", m.summary.ByDecision, colWidth, true)
	right := renderSection("By Agent", m.summary.ByAgent, colWidth, false)
	b.WriteString(sideBySide(left, right, colWidth))
	b.WriteString("\n")

	// By Source + By Workspace side by side.
	left = renderSection("By Source", m.summary.BySource, colWidth, false)
	right = renderSection("By Workspace", m.summary.ByWorkspace, colWidth, false)
	b.WriteString(sideBySide(left, right, colWidth))

	return b.String()
}

func renderSection(title string, counts map[string]int, width int, colorDecisions bool) []string {
	lines := []string{
		"  " + styleColHeader.Render(title),
		"  " + strings.Repeat("━", min(width-2, 30)),
	}

	sorted := sortedCounts(counts)
	if len(sorted) == 0 {
		lines = append(lines, styleDim.Render("  (none)"))
		return lines
	}

	maxVal := sorted[0].count
	maxBarWidth := width - 24 // space for label + count
	if maxBarWidth < 5 {
		maxBarWidth = 5
	}

	for _, kv := range sorted {
		barLen := 1
		if maxVal > 0 {
			barLen = kv.count * maxBarWidth / maxVal
			if barLen < 1 {
				barLen = 1
			}
		}
		bar := strings.Repeat("█", barLen)
		label := fmt.Sprintf("%-14s", sanitize(shorten(kv.key, 14)))

		if colorDecisions {
			label = decisionStyle(kv.key).Render(label)
		}

		lines = append(lines, fmt.Sprintf("  %s %s  %d", label, bar, kv.count))
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
		// Pad left column.
		padded := l + strings.Repeat(" ", max(0, colWidth-visibleLen(l)))
		b.WriteString(padded + "  " + r + "\n")
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
