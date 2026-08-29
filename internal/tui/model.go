package tui

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/php-workx/fuse/internal/attention"
	"github.com/php-workx/fuse/internal/db"
)

// NotificationOptions controls approval notification behavior for one TUI run.
// Its values cannot be changed after construction.
type NotificationOptions struct {
	enabled bool
	sound   bool
}

// NewNotificationOptions creates immutable notification options.
func NewNotificationOptions(enabled, sound bool) NotificationOptions {
	return NotificationOptions{enabled: enabled, sound: sound}
}

// Enabled reports whether approval notifications are enabled.
func (o NotificationOptions) Enabled() bool {
	return o.enabled
}

// Sound reports whether notification sounds are enabled.
func (o NotificationOptions) Sound() bool {
	return o.sound
}

type viewMode int

const (
	viewEvents viewMode = iota
	viewStats
	viewApprovals
)

// Model is the top-level bubbletea model for the fuse TUI.
type Model struct {
	db            *db.DB
	secret        []byte // HMAC secret for creating approvals from the TUI
	mode          string
	notifications NotificationOptions
	attention     attention.Service

	activeView    viewMode
	width         int
	height        int
	fetchGen      uint64
	fetching      bool
	lastErr       error
	pending       []db.PendingRequest
	pendingGen    uint64
	pendingFetching bool
	pendingErr    error

	notificationStatus       attention.Status
	notificationErr          error
	attentionEventsClosed    bool
	events        EventsModel
	stats         StatsModel
	approvals     ApprovalsModel
	quitting      bool
}

// NewModel creates an initialized Model.
func NewModel(database *db.DB, mode string, secret []byte, notifications NotificationOptions, notifier attention.Service) Model {
	if !notifications.Enabled() {
		notifier = nil
	}
	model := Model{
		db:            database,
		secret:        secret,
		mode:          mode,
		notifications: notifications,
		attention:     notifier,
		events:        NewEventsModel(),
		stats:         NewStatsModel(),
		approvals:     NewApprovalsModel(database, secret),
	}
	if notifier != nil {
		model.notificationStatus = notifier.Status()
	} else if notifications.Enabled() {
		model.notificationStatus = attention.Status{
			Ownership:      attention.OwnershipDisabled,
			DegradedReason: "native notifier unavailable",
		}
	}
	return model
}

// cleanupMsg is sent after the startup cleanup completes.
type cleanupMsg struct {
	err error
}

// Init cleans stale rows before starting the global pending poll.
func (m Model) Init() tea.Cmd {
	database := m.db
	return func() tea.Msg {
		_, err := database.CleanupStalePendingRequests(30 * time.Minute)
		return cleanupMsg{err: err}
	}
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

	case cleanupMsg:
		return m.handleCleanup(msg)

	case pendingPollMsg:
		return m.handlePendingPoll()

	case pendingFetchMsg:
		return m.handlePendingFetch(msg)

	case attentionEventMsg:
		return m.handleAttentionEvent(msg)

	case activationLookupMsg:
		return m.handleActivationLookup(msg)

	case attentionResultMsg:
		m.notificationStatus = msg.status
		m.notificationErr = msg.err
		return m, nil

	case attentionCloseMsg:
		if msg.err != nil {
			m.notificationErr = msg.err
		}
		return m, nil

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

// handleCleanup starts polling only after stale request cleanup is complete.
func (m Model) handleCleanup(msg cleanupMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.pendingErr = msg.err
	}
	m.pendingFetching = true
	m.pendingGen++
	cmds := []tea.Cmd{m.fetchPending()}
	if m.attention != nil && !m.attentionEventsClosed {
		cmds = append(cmds, m.waitAttentionEvent())
	}
	return m, tea.Batch(cmds...)
}

// handlePendingPoll starts the next independent global pending fetch.
func (m Model) handlePendingPoll() (tea.Model, tea.Cmd) {
	if m.pendingFetching {
		return m, pendingPollCmd()
	}
	m.pendingFetching = true
	m.pendingGen++
	return m, m.fetchPending()
}

// handlePendingFetch updates the global snapshot and schedules notification work.
func (m Model) handlePendingFetch(msg pendingFetchMsg) (tea.Model, tea.Cmd) {
	if msg.reqGen != m.pendingGen {
		return m, pendingPollCmd()
	}
	m.pendingFetching = false
	if msg.err != nil {
		m.pendingErr = msg.err
		return m, pendingPollCmd()
	}

	previous := m.pending
	m.pending = msg.pending
	m.pendingErr = nil
	m.approvals.SetPending(msg.pending)
	added, removed, currentIDs := pendingDiff(previous, msg.pending)
	return m, tea.Batch(m.syncAttention(added, removed, currentIDs), pendingPollCmd())
}

// handleAttentionEvent routes one helper event and re-arms its wait when open.
func (m Model) handleAttentionEvent(msg attentionEventMsg) (tea.Model, tea.Cmd) {
	if !msg.ok {
		m.attentionEventsClosed = true
		return m, nil
	}

	var cmds []tea.Cmd
	if m.attention != nil {
		cmds = append(cmds, m.waitAttentionEvent())
	}
	if msg.event.Status != (attention.Status{}) {
		m.notificationStatus = msg.event.Status
	}
	if msg.event.Err != nil {
		m.notificationErr = msg.event.Err
	}
	if msg.event.Kind == attention.EventActivated {
		cmds = append(cmds, m.activationLookup(msg.event.PendingID))
	}
	return m, tea.Batch(cmds...)
}

// handleActivationLookup applies authoritative pending state before focusing the terminal.
func (m Model) handleActivationLookup(msg activationLookupMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.pendingErr = msg.err
		return m, nil
	}

	m.activeView = viewApprovals
	if !msg.found {
		m.approvals.scopeSelect = false
		m.approvals.statusMsg = "Request already resolved."
	} else {
		if !m.approvals.SelectPending(msg.request.ID) {
			m.pending = append(m.pending, msg.request)
			m.approvals.SetPending(m.pending)
			m.approvals.SelectPending(msg.request.ID)
		}
	}
	return m, m.focusAttention()
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
		default:
		}
	}
	return m, tickCmd(m.activeView)
}

// handleKey processes keyboard input for the top-level model.
func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	k := msg.Key()

	if m.shouldQuit(k) {
		m.quitting = true
		if m.attention == nil {
			return m, tea.Quit
		}
		return m, tea.Sequence(m.closeAttention(), tea.Quit)
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
	tabs := []string{"Events", "Stats", fmt.Sprintf("Approvals (%d)", len(m.pending))}
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
	if m.pendingErr != nil {
		footer += "  " + styleError.Render("Pending DB: "+m.pendingErr.Error())
	}
	if m.notifications.Enabled() {
		if m.notificationErr != nil {
			footer += "  " + styleError.Render("Notifications: "+m.notificationErr.Error())
		} else if reason := m.notificationStatus.DegradedReason; reason != "" {
			footer += "  " + styleError.Render("Notifications: "+reason)
		}
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
		}
		return msg
	}
}

// pendingPollMsg triggers the next global pending-request poll.
type pendingPollMsg struct{}

// pendingFetchMsg carries a global pending-request query result.
type pendingFetchMsg struct {
	reqGen  uint64
	pending []db.PendingRequest
	err     error
}

// attentionEventMsg carries one event from the notification service.
type attentionEventMsg struct {
	event attention.Event
	ok    bool
}

// activationLookupMsg carries the authoritative result for a notification click.
type activationLookupMsg struct {
	request db.PendingRequest
	found   bool
	err     error
}

// attentionResultMsg carries the outcome of asynchronous notification work.
type attentionResultMsg struct {
	status attention.Status
	err    error
}

// attentionCloseMsg reports orderly notification service shutdown.
type attentionCloseMsg struct {
	err error
}

func pendingPollCmd() tea.Cmd {
	return tea.Tick(500*time.Millisecond, func(time.Time) tea.Msg {
		return pendingPollMsg{}
	})
}

func (m Model) fetchPending() tea.Cmd {
	gen := m.pendingGen
	database := m.db
	return func() tea.Msg {
		pending, err := database.ListPendingRequests()
		return pendingFetchMsg{reqGen: gen, pending: pending, err: err}
	}
}

func (m Model) waitAttentionEvent() tea.Cmd {
	events := m.attention.Events()
	return func() tea.Msg {
		event, ok := <-events
		return attentionEventMsg{event: event, ok: ok}
	}
}

func (m Model) activationLookup(id string) tea.Cmd {
	database := m.db
	return func() tea.Msg {
		request, found, err := database.GetPendingRequest(id)
		return activationLookupMsg{request: request, found: found, err: err}
	}
}

func pendingDiff(previous, current []db.PendingRequest) (added []db.PendingRequest, removed, currentIDs []string) {
	previousIDs := make(map[string]struct{}, len(previous))
	for _, request := range previous {
		previousIDs[request.ID] = struct{}{}
	}
	currentIDs = make([]string, 0, len(current))
	for _, request := range current {
		currentIDs = append(currentIDs, request.ID)
		if _, existed := previousIDs[request.ID]; !existed {
			added = append(added, request)
		}
	}
	currentSet := make(map[string]struct{}, len(current))
	for _, id := range currentIDs {
		currentSet[id] = struct{}{}
	}
	for _, request := range previous {
		if _, exists := currentSet[request.ID]; !exists {
			removed = append(removed, request.ID)
		}
	}
	return added, removed, currentIDs
}

func (m Model) syncAttention(added []db.PendingRequest, removed, currentIDs []string) tea.Cmd {
	if m.attention == nil {
		return nil
	}

	notifier := m.attention
	sound := m.notifications.Sound()
	return func() tea.Msg {
		ctx := context.Background()
		var firstErr error
		for _, request := range added {
			if err := notifier.Post(ctx, noticeForPending(request, sound)); err != nil && firstErr == nil {
				firstErr = err
			}
		}
		for _, id := range removed {
			if err := notifier.Remove(ctx, id); err != nil && firstErr == nil {
				firstErr = err
			}
		}
		if _, err := notifier.Reconcile(ctx, currentIDs); err != nil && firstErr == nil {
			firstErr = err
		}
		return attentionResultMsg{status: notifier.Status(), err: firstErr}
	}
}

func noticeForPending(request db.PendingRequest, sound bool) attention.Notice {
	workspace := filepath.Base(strings.TrimRight(request.WorkspaceRoot, string(filepath.Separator)))
	if workspace == "." || workspace == string(filepath.Separator) || workspace == "" {
		workspace = "Workspace"
	}
	source := request.Source
	if source == "" {
		source = "unknown source"
	}
	return attention.Notice{
		ID:       request.ID,
		Title:    "Fuse approval required",
		Subtitle: shorten(workspace, 64),
		Body:     "Source: " + shorten(source, 64),
		Sound:    sound,
	}
}

func (m Model) focusAttention() tea.Cmd {
	if m.attention == nil {
		return nil
	}
	notifier := m.attention
	return func() tea.Msg {
		_, err := notifier.Focus(context.Background())
		return attentionResultMsg{status: notifier.Status(), err: err}
	}
}

func (m Model) closeAttention() tea.Cmd {
	notifier := m.attention
	return func() tea.Msg {
		return attentionCloseMsg{err: notifier.Close(context.Background())}
	}
}
