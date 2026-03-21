package tui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/runger/fuse/internal/db"
)

type viewMode int

const (
	viewEvents viewMode = iota
	viewStats
	viewApprovals
)

// Model is the top-level bubbletea model for the fuse TUI.
type Model struct {
	db         *db.DB
	secret     []byte // HMAC secret for creating approvals from the TUI
	mode       string
	activeView viewMode
	width      int
	height     int
	fetchGen   uint64
	fetching   bool
	lastErr    error
	events     EventsModel
	stats      StatsModel
	approvals  ApprovalsModel
	quitting   bool
}

// NewModel creates an initialized Model.
func NewModel(database *db.DB, mode string, secret []byte) Model {
	return Model{
		db:        database,
		secret:    secret,
		mode:      mode,
		events:    NewEventsModel(),
		stats:     NewStatsModel(),
		approvals: NewApprovalsModel(database, secret),
	}
}

// Init returns the initial command (first tick).
func (m Model) Init() tea.Cmd {
	// Clean up stale pending requests (>30 min) from crashed hooks.
	_, _ = m.db.CleanupStalePendingRequests(30 * time.Minute)
	return tickCmd(m.activeView)
}

// Update processes messages.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		contentH := msg.Height - 3 // header + footer + separator
		m.events.SetSize(msg.Width, contentH)
		m.stats.SetSize(msg.Width, contentH)
		m.approvals.SetSize(msg.Width, contentH)
		return m, nil

	case tickMsg:
		if m.fetching {
			return m, nil // skip — previous fetch still in-flight
		}
		// Pause polling while the detail panel is open to avoid
		// disrupting the user's reading (spec §6 cursor anchoring).
		if m.activeView == viewEvents && m.events.showDetail {
			return m, tickCmd(m.activeView) // reschedule without fetching
		}
		m.fetching = true
		return m, m.fetchData()

	case dataMsg:
		m.fetching = false
		if msg.reqGen < m.fetchGen {
			// Stale result — discard and schedule next tick.
			return m, tickCmd(m.activeView)
		}
		if msg.err != nil {
			m.lastErr = msg.err
		} else {
			m.lastErr = nil
			switch msg.view {
			case viewEvents:
				m.events.SetData(msg.events)
			case viewStats:
				m.stats.SetData(msg.summary)
			case viewApprovals:
				m.approvals.SetData(msg.approvals)
				m.approvals.SetPending(msg.pending)
			}
		}
		return m, tickCmd(m.activeView)

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	// Delegate non-key messages to active view.
	var cmd tea.Cmd
	switch m.activeView {
	case viewEvents:
		m.events, cmd = m.events.Update(msg)
	case viewStats:
		m.stats, cmd = m.stats.Update(msg)
	case viewApprovals:
		m.approvals, cmd = m.approvals.Update(msg)
	}
	return m, cmd
}

// handleKey processes keyboard input for the top-level model.
func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	k := msg.Key()

	// Always handle quit.
	if key.Matches(k, keys.Quit) {
		// Don't quit on 'q' while searching.
		if m.activeView != viewEvents || !m.events.searching || k.Code == tea.KeyLeftCtrl+'c' {
			m.quitting = true
			return m, tea.Quit
		}
	}

	// View switching (suppressed during search mode).
	if m.activeView != viewEvents || !m.events.searching {
		switch {
		case key.Matches(k, keys.Tab):
			m.activeView = (m.activeView + 1) % 3
			m.fetchGen++
			return m, m.fetchData()
		case key.Matches(k, keys.ViewEvents):
			if m.activeView != viewEvents {
				m.activeView = viewEvents
				m.fetchGen++
				return m, m.fetchData()
			}
		case key.Matches(k, keys.ViewStats):
			if m.activeView != viewStats {
				m.activeView = viewStats
				m.fetchGen++
				return m, m.fetchData()
			}
		case key.Matches(k, keys.ViewAppr):
			if m.activeView != viewApprovals {
				m.activeView = viewApprovals
				m.fetchGen++
				return m, m.fetchData()
			}
		}
	}

	// Delegate to active view.
	var cmd tea.Cmd
	switch m.activeView {
	case viewEvents:
		m.events, cmd = m.events.Update(msg)
	case viewStats:
		m.stats, cmd = m.stats.Update(msg)
	case viewApprovals:
		m.approvals, cmd = m.approvals.Update(msg)
	}
	return m, cmd
}

// View renders the TUI.
func (m Model) View() tea.View {
	if m.quitting {
		return tea.NewView("")
	}

	var b strings.Builder

	// Header bar.
	tabs := []string{"Events", "Stats", "Approvals"}
	var tabParts []string
	for i, name := range tabs {
		if viewMode(i) == m.activeView {
			tabParts = append(tabParts, styleActiveTab.Render(fmt.Sprintf("❯%s", name)))
		} else {
			tabParts = append(tabParts, styleTab.Render(fmt.Sprintf(" %s", name)))
		}
	}
	header := fmt.Sprintf("  fuse monitor  Mode: %s  │ %s", strings.ToUpper(m.mode), strings.Join(tabParts, "  "))
	b.WriteString(styleHeader.Render(header) + "\n")
	b.WriteString(strings.Repeat("─", m.width) + "\n")

	// Active view.
	switch m.activeView {
	case viewEvents:
		b.WriteString(m.events.View())
	case viewStats:
		b.WriteString(m.stats.View())
	case viewApprovals:
		b.WriteString(m.approvals.View())
	}

	// Footer.
	footer := "  " + footerHelp()
	if m.activeView == viewEvents {
		footer += m.events.FilterInfo()
	}
	if m.lastErr != nil {
		footer += "  " + styleError.Render("DB: "+m.lastErr.Error())
	}
	// Pad to bottom.
	b.WriteString("\n" + strings.Repeat("─", m.width) + "\n")
	b.WriteString(styleFooter.Render(footer))

	v := tea.NewView(b.String())
	v.AltScreen = true
	return v
}

// Message types.
type tickMsg struct {
	view viewMode
}

func tickCmd(view viewMode) tea.Cmd {
	interval := 1500 * time.Millisecond
	if view == viewStats {
		interval = 5 * time.Second
	}
	return tea.Tick(interval, func(t time.Time) tea.Msg {
		return tickMsg{view: view}
	})
}

type dataMsg struct {
	reqGen    uint64
	view      viewMode
	events    []db.EventRecord
	summary   db.EventSummary
	approvals []db.Approval
	pending   []db.PendingRequest
	err       error
}

func (m Model) fetchData() tea.Cmd {
	gen := m.fetchGen
	view := m.activeView
	database := m.db
	return func() tea.Msg {
		msg := dataMsg{reqGen: gen, view: view}
		switch view {
		case viewEvents:
			msg.events, msg.err = database.ListEvents(&db.EventFilter{Limit: 200})
		case viewStats:
			msg.summary, msg.err = database.SummarizeEvents()
		case viewApprovals:
			msg.approvals, msg.err = database.ListApprovals(50)
			// Also fetch pending requests (fast, small table).
			msg.pending, _ = database.ListPendingRequests()
		}
		return msg
	}
}
