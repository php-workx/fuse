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

	"github.com/runger/fuse/internal/db"
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
		case key.Matches(k, keys.End):
			if len(m.filtered) > 0 {
				m.cursor = len(m.filtered) - 1
			}
		case key.Matches(k, keys.Enter):
			m.toggleDetail()
		case key.Matches(k, keys.Escape):
			if m.showDetail {
				m.showDetail = false
			} else if m.searchInput.Value() != "" {
				m.searchInput.SetValue("")
				m.applyFilters()
			}
		}
	}
	return m, nil
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
	header := fmt.Sprintf("  %-8s  %-8s  %-6s  %-12s  %-20s  %s",
		"TIME", "DECISION", "AGENT", "SOURCE", "WORKSPACE", "COMMAND")
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
		ws := shortenPath(e.WorkspaceRoot)
		decision := sanitize(fallbackValue(e.Decision))
		agent := sanitize(fallbackValue(e.Agent))
		source := sanitize(fallbackValue(e.Source))
		command := sanitize(shorten(e.Command, m.width-62))

		decCol := decisionStyle(e.Decision).Render(fmt.Sprintf("%-8s", decision))
		row := fmt.Sprintf("  %-8s  %s  %-6s  %-12s  %-20s  %s",
			ts, decCol, agent, source, shorten(ws, 20), command)

		if i == m.cursor {
			b.WriteString(styleCursor.Render(shorten(row, m.width)) + "\n")
		} else {
			b.WriteString(shorten(row, m.width) + "\n")
		}
	}

	// Detail panel.
	if m.showDetail && m.cursor >= 0 && m.cursor < len(m.filtered) {
		b.WriteString(m.renderDetail(&m.filtered[m.cursor]))
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
	var b strings.Builder
	fmt.Fprintf(&b, "  ID:          %d\n", e.ID)
	fmt.Fprintf(&b, "  Time:        %s\n", e.Timestamp)
	fmt.Fprintf(&b, "  Decision:    %s\n", decisionStyle(e.Decision).Render(e.Decision))
	fmt.Fprintf(&b, "  Command:     %s\n", sanitize(e.Command))
	fmt.Fprintf(&b, "  Rule ID:     %s\n", fallbackValue(e.RuleID))
	fmt.Fprintf(&b, "  Reason:      %s\n", sanitize(fallbackValue(e.Reason)))
	fmt.Fprintf(&b, "  Agent:       %s\n", fallbackValue(e.Agent))
	fmt.Fprintf(&b, "  Source:      %s\n", fallbackValue(e.Source))
	fmt.Fprintf(&b, "  Session:     %s\n", fallbackValue(e.SessionID))
	fmt.Fprintf(&b, "  Workspace:   %s\n", fallbackValue(e.WorkspaceRoot))
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
	return styleDetail.Render(b.String())
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
	t, err := time.Parse("2006-01-02T15:04:05.000Z", ts)
	if err != nil {
		// Try without milliseconds.
		t, err = time.Parse("2006-01-02T15:04:05Z", ts)
		if err != nil {
			return ts[:8] // fallback: first 8 chars
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
