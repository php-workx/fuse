package tui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/php-workx/fuse/internal/db"
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

// cleanupMsg is sent after the startup cleanup completes (fire-and-forget).
type cleanupMsg struct{}

// Init returns the initial commands (first tick + async cleanup).
func (m Model) Init() tea.Cmd {
	database := m.db
	cleanupCmd := func() tea.Msg {
		// Clean up stale pending requests (>30 min) from crashed hooks.
		_, _ = database.CleanupStalePendingRequests(30 * time.Minute)
		return cleanupMsg{}
	}
	return tea.Batch(tickCmd(m.activeView), cleanupCmd)
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
		return m.handleTick(msg)

	case dataMsg:
		return m.handleData(msg)

	case tea.KeyMsg:
		return m.handleKey(msg)
	default:
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
	default:
	}
	return m, cmd
}

// handleTick processes periodic tick messages for data polling.
func (m Model) handleTick(msg tickMsg) (tea.Model, tea.Cmd) {
	// Discard stale ticks from a previous view and reschedule.
	if msg.view != m.activeView {
		return m, tickCmd(m.activeView)
	}
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
}

// handleData processes data fetch results.
func (m Model) handleData(msg dataMsg) (tea.Model, tea.Cmd) {
	if msg.reqGen < m.fetchGen {
		// Stale result — discard and schedule next tick.
		return m, tickCmd(m.activeView)
	}
	m.fetching = false
	if msg.err != nil {
		m.lastErr = msg.err
	} else {
		m.lastErr = nil
		switch msg.view {
		case viewEvents:
			m.events.SetData(msg.events)
		case viewStats:
			m.stats.SetData(msg.summary)
			m.stats.SetJudgeSummary(msg.judgeSummary)
		case viewApprovals:
			m.approvals.SetData(msg.approvals)
			m.approvals.SetPending(msg.pending)
		default:
		}
	}
	return m, tickCmd(m.activeView)
}

// handleKey processes keyboard input for the top-level model.
func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	k := msg.Key()

	// Always handle quit.
	if m.shouldQuit(k) {
		m.quitting = true
		return m, tea.Quit
	}

	// View switching (suppressed during search mode).
	if m.activeView != viewEvents || !m.events.searching {
		if switched, cmd := m.handleViewSwitch(k); switched {
			return m, cmd
		}
	}

	// Delegate to active view.
	return m.delegateToActiveView(msg)
}

// shouldQuit returns true if the key should trigger a quit.
func (m Model) shouldQuit(k tea.Key) bool {
	if !key.Matches(k, keys.Quit) {
		return false
	}
	// Don't quit on 'q' while searching — only Ctrl+C quits.
	return m.activeView != viewEvents || !m.events.searching || k.String() == "ctrl+c"
}

// handleViewSwitch processes view-switching keys (Tab, 1/2/3).
// Returns true and a fetch command if a view switch occurred.
func (m *Model) handleViewSwitch(k tea.Key) (bool, tea.Cmd) {
	var target viewMode
	switch {
	case key.Matches(k, keys.Tab):
		target = (m.activeView + 1) % 3
	case key.Matches(k, keys.ViewEvents):
		target = viewEvents
	case key.Matches(k, keys.ViewStats):
		target = viewStats
	case key.Matches(k, keys.ViewAppr):
		target = viewApprovals
	default:
		return false, nil
	}
	if target == m.activeView && !key.Matches(k, keys.Tab) {
		return false, nil
	}
	m.activeView = target
	m.fetchGen++
	m.fetching = true
	return true, m.fetchData()
}

// delegateToActiveView forwards a message to the currently active view.
func (m Model) delegateToActiveView(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch m.activeView {
	case viewEvents:
		m.events, cmd = m.events.Update(msg)
	case viewStats:
		m.stats, cmd = m.stats.Update(msg)
	case viewApprovals:
		m.approvals, cmd = m.approvals.Update(msg)
	default:
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
	default:
	}

	// Footer.
	footer := "  " + footerHelp()
	switch m.activeView {
	case viewEvents:
		footer += m.events.FilterInfo()
	case viewApprovals:
		footer += m.approvals.FilterInfo()
	default:
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
	switch view {
	case viewApprovals:
		interval = 500 * time.Millisecond // faster polling for pending requests
	case viewStats:
		interval = 5 * time.Second
	default:
	}
	return tea.Tick(interval, func(t time.Time) tea.Msg {
		return tickMsg{view: view}
	})
}

type dataMsg struct {
	reqGen       uint64
	view         viewMode
	events       []db.EventRecord
	summary      db.EventSummary
	judgeSummary db.JudgeSummary
	approvals    []db.Approval
	pending      []db.PendingRequest
	err          error
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
			// Fetch judge accuracy summary independently — a transient error
			// should not block the main stats update.
			msg.judgeSummary, _ = database.SummarizeJudgeAccuracy()
		case viewApprovals:
			msg.approvals, msg.err = database.ListApprovals(50)
			// Fetch pending requests independently — a transient pending error
			// should not block the approval history update.
			msg.pending, _ = database.ListPendingRequests()
		default:
		}
		return msg
	}
}
