package tui

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"

	"github.com/php-workx/fuse/internal/db"
)

// Decision filter cycle: ALL → SAFE → CAUTION → APPROVAL → BLOCKED → ALL
var decisionCycle = []string{"", "SAFE", "CAUTION", "APPROVAL", "BLOCKED"}

// EventsModel renders a live event table with filtering and a detail panel.
type EventsModel struct {
	events         []db.EventRecord
	filtered       []db.EventRecord // events after applying filters
	cursor         int
	selectedID     int64 // sticky cursor: tracks event ID across refreshes
	offset         int
	filterDecision string // "" = all
	searchInput    textinput.Model
	searching      bool
	showDetail     bool
	detailView     viewport.Model
	width, height  int
}

// NewEventsModel creates an initialized EventsModel.
func NewEventsModel() EventsModel {
	ti := textinput.New()
	ti.Placeholder = "search command..."
	ti.CharLimit = 120
	return EventsModel{
		searchInput: ti,
		detailView:  viewport.New(),
	}
}

// SetData updates the events and reapplies filters.
func (m *EventsModel) SetData(events []db.EventRecord) {
	m.events = events
	m.applyFilters()
	m.anchorCursor()
}

// SetSize updates dimensions.
func (m *EventsModel) SetSize(w, h int) {
	m.width = w
	m.height = h
	m.detailView.SetWidth(w - 4)
}

// Update handles key messages for the events view.
func (m EventsModel) Update(msg tea.Msg) (EventsModel, tea.Cmd) {
	if m.searching {
		return m.updateSearch(msg)
	}

	// When the detail panel is open, delegate scroll keys to the viewport
	// and only handle Enter/Esc to close it.
	if m.showDetail {
		return m.updateDetail(msg)
	}

	if msg, ok := msg.(tea.KeyMsg); ok {
		k := msg.Key()
		switch {
		case key.Matches(k, keys.Search):
			m.searching = true
			m.searchInput.Focus()
			return m, nil
		case key.Matches(k, keys.FilterDec):
			m.cycleDecisionFilter()
			return m, nil
		case key.Matches(k, keys.Up):
			m.moveCursor(-1)
		case key.Matches(k, keys.Down):
			m.moveCursor(1)
		case key.Matches(k, keys.PageUp):
			m.moveCursor(-pageSize(m.height))
		case key.Matches(k, keys.PageDown):
			m.moveCursor(pageSize(m.height))
		case key.Matches(k, keys.Home):
			m.cursor = 0
			m.offset = 0
			if len(m.filtered) > 0 {
				m.selectedID = m.filtered[0].ID
			}
		case key.Matches(k, keys.End):
			if len(m.filtered) > 0 {
				m.cursor = len(m.filtered) - 1
				m.selectedID = m.filtered[m.cursor].ID
			}
		case key.Matches(k, keys.Enter):
			m.toggleDetail()
		case key.Matches(k, keys.Escape):
			if m.searchInput.Value() != "" {
				m.searchInput.SetValue("")
				m.applyFilters()
			}
		default:
		}
	}
	return m, nil
}

// updateDetail handles messages when the detail panel is focused.
// j/k/PgUp/PgDn scroll the viewport; Enter/Esc close the panel.
func (m EventsModel) updateDetail(msg tea.Msg) (EventsModel, tea.Cmd) {
	if msg, ok := msg.(tea.KeyMsg); ok {
		k := msg.Key()
		switch {
		case key.Matches(k, keys.Enter), key.Matches(k, keys.Escape):
			m.showDetail = false
			return m, nil
		default:
		}
	}
	// Delegate all other messages (including scroll keys) to the viewport.
	var cmd tea.Cmd
	m.detailView, cmd = m.detailView.Update(msg)
	return m, cmd
}

func (m EventsModel) updateSearch(msg tea.Msg) (EventsModel, tea.Cmd) {
	if msg, ok := msg.(tea.KeyMsg); ok {
		k := msg.Key()
		switch {
		case key.Matches(k, keys.Enter):
			m.searching = false
			m.searchInput.Blur()
			m.applyFilters()
			return m, nil
		case key.Matches(k, keys.Escape):
			m.searching = false
			m.searchInput.Blur()
			m.searchInput.SetValue("")
			m.applyFilters()
			return m, nil
		default:
		}
	}
	var cmd tea.Cmd
	m.searchInput, cmd = m.searchInput.Update(msg)
	m.applyFilters()
	return m, cmd
}

// View renders the events table.
func (m EventsModel) View() string {
	if len(m.events) == 0 {
		return styleDim.Render("  Waiting for events...")
	}

	if len(m.filtered) == 0 {
		return styleDim.Render("  No matching events")
	}

	var b strings.Builder

	// Search bar.
	if m.searching {
		b.WriteString("  / " + m.searchInput.View() + "\n")
	} else if m.searchInput.Value() != "" {
		b.WriteString(styleDim.Render(fmt.Sprintf("  search: %s", m.searchInput.Value())) + "\n")
	}

	// Column headers.
	header := fmt.Sprintf("  %-8s  %-8s  %-11s  %-6s  %-12s  %-20s  %s",
		"TIME", "DECISION", "JUDGE", "AGENT", "SOURCE", "WORKSPACE", "COMMAND")
	b.WriteString(styleColHeader.Render(shorten(header, m.width)) + "\n")

	// Compute visible rows.
	tableHeight := m.tableHeight()
	m.clampOffset(tableHeight)

	end := m.offset + tableHeight
	if end > len(m.filtered) {
		end = len(m.filtered)
	}

	for i := m.offset; i < end; i++ {
		e := &m.filtered[i]
		ts := formatTime(e.Timestamp)
		ws := sanitize(shortenPath(e.WorkspaceRoot))
		decision := sanitize(fallbackValue(e.Decision))
		agent := sanitize(fallbackValue(e.Agent))
		source := sanitize(fallbackValue(e.Source))
		cmdWidth := m.width - 75
		if cmdWidth < 0 {
			cmdWidth = 0
		}
		command := sanitize(shorten(e.Command, cmdWidth))

		decCol := decisionStyle(e.Decision).Render(fmt.Sprintf("%-8s", decision))
		judgeCol := formatJudgeColumn(e)
		row := fmt.Sprintf("  %-8s  %s  %s  %-6s  %-12s  %-20s  %s",
			ts, decCol, judgeCol, agent, source, shorten(ws, 20), command)

		if i == m.cursor {
			b.WriteString(styleCursor.Render(shorten(row, m.width)) + "\n")
		} else {
			b.WriteString(shorten(row, m.width) + "\n")
		}
	}

	// Detail panel (rendered via viewport for scrolling).
	if m.showDetail {
		b.WriteString(m.detailView.View())
	}

	return b.String()
}

// FilterInfo returns the current filter state for the footer.
func (m EventsModel) FilterInfo() string {
	var parts []string
	if m.filterDecision != "" {
		parts = append(parts, "decision:"+m.filterDecision)
	}
	if m.searchInput.Value() != "" {
		parts = append(parts, "search:"+m.searchInput.Value())
	}
	if len(parts) == 0 {
		return ""
	}
	return " [" + strings.Join(parts, " ") + "]"
}

func (m EventsModel) renderDetail(e *db.EventRecord) string {
	// Available content width inside the detail border (border + padding = ~4 chars).
	contentWidth := m.width - 6
	if contentWidth < 40 {
		contentWidth = 40
	}

	const labelCol = 15 // "  Command:     " = 15 chars
	valueWidth := contentWidth - labelCol
	if valueWidth < 20 {
		valueWidth = 20
	}

	var b strings.Builder
	fmt.Fprintf(&b, "  ID:          %d\n", e.ID)
	fmt.Fprintf(&b, "  Time:        %s\n", e.Timestamp)
	fmt.Fprintf(&b, "  Decision:    %s\n", decisionStyle(e.Decision).Render(sanitize(fallbackValue(e.Decision))))
	writeWrapped(&b, "  Command:     ", sanitize(e.Command), valueWidth)
	fmt.Fprintf(&b, "  Rule ID:     %s\n", sanitize(fallbackValue(e.RuleID)))
	writeWrapped(&b, "  Reason:      ", sanitize(fallbackValue(e.Reason)), valueWidth)
	fmt.Fprintf(&b, "  Agent:       %s\n", sanitize(fallbackValue(e.Agent)))
	fmt.Fprintf(&b, "  Source:      %s\n", sanitize(fallbackValue(e.Source)))
	fmt.Fprintf(&b, "  Session:     %s\n", sanitize(fallbackValue(e.SessionID)))
	fmt.Fprintf(&b, "  Workspace:   %s\n", sanitize(fallbackValue(e.WorkspaceRoot)))
	if e.DurationMs > 0 {
		fmt.Fprintf(&b, "  Duration:    %dms\n", e.DurationMs)
	} else {
		b.WriteString("  Duration:    -\n")
	}
	if e.ExecutionExitCode != nil {
		fmt.Fprintf(&b, "  Exit Code:   %d\n", *e.ExecutionExitCode)
	} else {
		b.WriteString("  Exit Code:   -\n")
	}

	// Judge information.
	if e.JudgeError != "" {
		fmt.Fprintf(&b, "  Judge:       ERROR -- %s\n", sanitize(e.JudgeError))
	} else if e.JudgeDecision != "" {
		judgeLine := fmt.Sprintf("%s (%d%%) via %s",
			sanitize(e.JudgeDecision),
			int(e.JudgeConfidence*100),
			sanitize(fallbackValue(e.JudgeProvider)))
		if e.JudgeReasoning != "" {
			reasonWidth := valueWidth - 40
			if reasonWidth < 0 {
				reasonWidth = 0
			}
			judgeLine += fmt.Sprintf(" -- %q", sanitize(shorten(e.JudgeReasoning, reasonWidth)))
		}
		if e.JudgeApplied {
			judgeLine += " [APPLIED]"
		}
		writeWrapped(&b, "  Judge:       ", judgeLine, valueWidth)

		mode := "shadow (not applied)"
		if e.JudgeApplied {
			mode = "applied"
		}
		fmt.Fprintf(&b, "  Judge Mode:  %s\n", mode)
	}

	return styleDetail.Render(b.String())
}

// writeWrapped writes a labeled value, wrapping long values to fit within maxWidth.
// Continuation lines are indented to align with the value start.
// Uses rune-aware slicing so multi-byte characters are never split mid-rune.
func writeWrapped(b *strings.Builder, label, value string, maxWidth int) {
	runes := []rune(value)
	if len(runes) <= maxWidth {
		b.WriteString(label + value + "\n")
		return
	}

	indent := strings.Repeat(" ", len(label))
	first := true
	for len(runes) > 0 {
		chunk := runes
		if len(chunk) > maxWidth {
			// Try to break at a space.
			cut := maxWidth
			for cut > maxWidth/2 {
				if chunk[cut] == ' ' {
					break
				}
				cut--
			}
			if cut <= maxWidth/2 {
				cut = maxWidth // no good break point — hard wrap
			}
			chunk = runes[:cut]
			runes = runes[cut:]
			// Skip leading space on next line.
			if len(runes) > 0 && runes[0] == ' ' {
				runes = runes[1:]
			}
		} else {
			runes = nil
		}

		if first {
			b.WriteString(label + string(chunk) + "\n")
			first = false
		} else {
			b.WriteString(indent + string(chunk) + "\n")
		}
	}
}

// formatJudgeColumn returns an 11-char wide judge indicator for the table row.
// Format: "=SAFE  94%" (agree) or ">APPR  88%" (disagree). Empty if no judge.
func formatJudgeColumn(e *db.EventRecord) string {
	if e.JudgeDecision == "" {
		return fmt.Sprintf("%-11s", "")
	}

	prefix := "="
	if !strings.EqualFold(e.JudgeDecision, e.Decision) {
		prefix = ">"
	}

	abbrev := abbreviateDecision(e.JudgeDecision)
	conf := int(e.JudgeConfidence * 100)
	text := fmt.Sprintf("%s%-4s %3d%%", prefix, abbrev, conf)

	if e.JudgeApplied {
		return decisionStyle(e.JudgeDecision).Render(fmt.Sprintf("%-11s", text))
	}
	return styleDim.Render(fmt.Sprintf("%-11s", text))
}

// abbreviateDecision shortens decision names to max 4 chars for the judge column.
func abbreviateDecision(d string) string {
	upper := strings.ToUpper(d)
	switch upper {
	case "APPROVAL":
		return "APPR"
	case "CAUTION":
		return "CAUT"
	case "BLOCKED":
		return "BLKD"
	default:
		if len(upper) > 4 {
			return upper[:4]
		}
		return upper
	}
}

func (m *EventsModel) applyFilters() {
	m.filtered = m.filtered[:0]
	search := strings.ToLower(m.searchInput.Value())
	for i := range m.events {
		e := &m.events[i]
		if m.filterDecision != "" && !strings.EqualFold(e.Decision, m.filterDecision) {
			continue
		}
		if search != "" && !strings.Contains(strings.ToLower(e.Command), search) {
			continue
		}
		m.filtered = append(m.filtered, *e)
	}
}

func (m *EventsModel) anchorCursor() {
	if m.selectedID == 0 {
		return
	}
	for i := range m.filtered {
		if m.filtered[i].ID == m.selectedID {
			m.cursor = i
			return
		}
	}
	// Event no longer in list — clamp.
	if m.cursor >= len(m.filtered) && len(m.filtered) > 0 {
		m.cursor = len(m.filtered) - 1
	}
}

func (m *EventsModel) cycleDecisionFilter() {
	current := 0
	for i, d := range decisionCycle {
		if d == m.filterDecision {
			current = i
			break
		}
	}
	m.filterDecision = decisionCycle[(current+1)%len(decisionCycle)]
	m.cursor = 0
	m.offset = 0
	m.selectedID = 0 // let anchorCursor pick a new anchor after filter change
	m.applyFilters()
}

func (m *EventsModel) moveCursor(delta int) {
	m.cursor += delta
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor >= len(m.filtered) {
		m.cursor = len(m.filtered) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor >= 0 && m.cursor < len(m.filtered) {
		m.selectedID = m.filtered[m.cursor].ID
	}
}

func (m *EventsModel) toggleDetail() {
	m.showDetail = !m.showDetail
	if m.showDetail && m.cursor >= 0 && m.cursor < len(m.filtered) {
		// Set viewport content and size for scrolling.
		content := m.renderDetail(&m.filtered[m.cursor])
		m.detailView.SetContent(content)
		detailH := m.height * 40 / 100
		if detailH < 3 {
			detailH = 3
		}
		m.detailView.SetHeight(detailH)
		m.detailView.SetWidth(m.width - 4)
		m.detailView.GotoTop()
	}
}

func (m EventsModel) tableHeight() int {
	h := m.height - 2 // column header + potential search bar
	if m.searching || m.searchInput.Value() != "" {
		h--
	}
	if m.showDetail {
		h = h * 60 / 100 // table gets 60%, detail gets 40%
	}
	if h < 1 {
		h = 1
	}
	return h
}

func (m *EventsModel) clampOffset(visibleRows int) {
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+visibleRows {
		m.offset = m.cursor - visibleRows + 1
	}
}

func pageSize(height int) int {
	ps := height / 2
	if ps < 1 {
		ps = 1
	}
	return ps
}

func formatTime(ts string) string {
	t, err := time.Parse(db.TimestampMillisFormat, ts)
	if err != nil {
		// Try without milliseconds.
		t, err = time.Parse("2006-01-02T15:04:05Z", ts)
		if err != nil {
			return shorten(ts, 8) // fallback: first 8 chars
		}
	}
	return t.Local().Format("15:04:05")
}

func shortenPath(p string) string {
	if p == "" {
		return "-"
	}
	return filepath.Base(p)
}
