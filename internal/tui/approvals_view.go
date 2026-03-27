package tui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/php-workx/fuse/internal/approve"
	"github.com/php-workx/fuse/internal/db"
)

// errMsgPrefix is the prefix for error status messages.
const errMsgPrefix = "Error: "

type approvalFocus int

const (
	focusPending approvalFocus = iota
	focusHistory
)

// timestampFormat is the standard timestamp format used for parsing approval timestamps.
// Delegates to the canonical constant defined in the db package.
const timestampFormat = db.TimestampMillisFormat

// timestampFormatNoMillis is the timestamp format without millisecond precision,
// used as a fallback when parsing timestamps that lack the .000 suffix.
const timestampFormatNoMillis = "2006-01-02T15:04:05Z"

// ApprovalsModel renders pending approval requests and approval history.
type ApprovalsModel struct {
	// Pending requests from hook processes.
	pending    []db.PendingRequest
	pendingIdx int

	// Approval history.
	approvals  []db.Approval
	historyIdx int
	histOffset int

	// UI state.
	focus       approvalFocus
	scopeSelect bool   // waiting for scope keypress after 'a'
	confirming  string // "delete", "purge", or ""
	confirmID   string
	statusMsg   string         // transient footer message
	showDetail  bool           // detail panel open for history item
	detailView  viewport.Model // scrollable detail panel

	// Policy recommendations from approval history (cached with TTL).
	recommendations   []db.PolicyRecommendation
	recommendationsAt time.Time // when recommendations were last queried

	// Dependencies.
	database *db.DB
	secret   []byte
	clock    func() time.Time
	width    int
	height   int
}

// NewApprovalsModel creates an initialized ApprovalsModel.
func NewApprovalsModel(database *db.DB, secret []byte) ApprovalsModel {
	return ApprovalsModel{
		database:   database,
		secret:     secret,
		clock:      func() time.Time { return time.Now().UTC() },
		detailView: viewport.New(),
	}
}

// SetData updates the approval history.
func (m *ApprovalsModel) SetData(approvals []db.Approval) {
	m.approvals = approvals
	// Load policy recommendations (best-effort, cached 30s to avoid per-tick queries).
	if m.database != nil && time.Since(m.recommendationsAt) > 30*time.Second {
		recs, err := m.database.FrequentApprovals(3)
		if err == nil {
			m.recommendations = recs
			m.recommendationsAt = time.Now()
		}
	}
}

// SetPending updates the pending requests list.
func (m *ApprovalsModel) SetPending(pending []db.PendingRequest) {
	m.pending = pending
	if m.pendingIdx >= len(m.pending) && len(m.pending) > 0 {
		m.pendingIdx = len(m.pending) - 1
	}
}

// SetSize updates dimensions.
func (m *ApprovalsModel) SetSize(w, h int) {
	m.width = w
	m.height = h
	m.detailView.SetWidth(max(w-4, 0))
}

// StatusMsg returns and clears the transient status message.
func (m *ApprovalsModel) StatusMsg() string {
	msg := m.statusMsg
	m.statusMsg = ""
	return msg
}

// Update handles key messages.
func (m ApprovalsModel) Update(msg tea.Msg) (ApprovalsModel, tea.Cmd) {
	// When the detail panel is open, delegate to the detail handler.
	if m.showDetail {
		return m.updateDetail(msg)
	}

	if msg, ok := msg.(tea.KeyMsg); ok {
		return m.handleKeyMsg(msg)
	}

	// Handle action results.
	m.handleActionResult(msg)
	return m, nil
}

// handleKeyMsg dispatches key messages to the appropriate handler.
func (m ApprovalsModel) handleKeyMsg(msg tea.KeyMsg) (ApprovalsModel, tea.Cmd) {
	k := msg.Key()

	// Confirmation dialog.
	if m.confirming != "" {
		return m.handleConfirm(k)
	}

	// Scope selection after pressing 'a'.
	if m.scopeSelect {
		return m.handleScopeSelect(k)
	}

	return m.handleNormalKey(k)
}

// handleNormalKey handles key presses in normal (non-modal) state.
func (m ApprovalsModel) handleNormalKey(k tea.Key) (ApprovalsModel, tea.Cmd) {
	switch {
	// Toggle focus between pending and history (left/right arrows).
	case key.Matches(k, keys.Left), key.Matches(k, keys.Right):
		if m.focus == focusPending {
			m.focus = focusHistory
		} else {
			m.focus = focusPending
		}
		return m, nil

	// Navigation.
	case key.Matches(k, keys.Up):
		m.moveCursor(-1)
	case key.Matches(k, keys.Down):
		m.moveCursor(1)

	// Approve pending request.
	case k.Code == 'a':
		if m.focus == focusPending && len(m.pending) > 0 {
			m.scopeSelect = true
			return m, nil
		}

	// Deny pending request.
	case k.Code == 'd' || k.Code == 'n':
		if m.focus == focusPending && len(m.pending) > 0 {
			return m, m.denyCmd()
		}

	// Toggle detail panel for history item.
	case key.Matches(k, keys.Enter):
		if m.focus == focusHistory && len(m.approvals) > 0 {
			m.toggleDetail()
			return m, nil
		}

	// Revoke approval from history.
	case key.Matches(k, keys.Delete):
		if m.focus == focusHistory && len(m.approvals) > 0 {
			m.confirming = "delete"
			m.confirmID = m.approvals[m.historyIdx].ID
			return m, nil
		}

	// Purge expired/consumed.
	case key.Matches(k, keys.BulkPurge):
		m.confirming = "purge"
		return m, nil
	default:
	}
	return m, nil
}

func (m *ApprovalsModel) handleActionResult(msg tea.Msg) {
	switch msg := msg.(type) {
	case approveResultMsg:
		if msg.err != nil {
			m.statusMsg = errMsgPrefix + msg.err.Error()
		} else {
			m.statusMsg = "Approved (" + msg.scope + ")"
		}
	case denyResultMsg:
		if msg.err != nil {
			m.statusMsg = errMsgPrefix + msg.err.Error()
		} else {
			m.statusMsg = "Denied"
		}
	case deleteResultMsg:
		if msg.err != nil {
			m.statusMsg = errMsgPrefix + msg.err.Error()
		} else {
			m.statusMsg = "Revoked"
		}
	case purgeResultMsg:
		if msg.err != nil {
			m.statusMsg = errMsgPrefix + msg.err.Error()
		} else {
			m.statusMsg = fmt.Sprintf("Purged %d", msg.deleted)
		}
	default:
	}
}

func (m ApprovalsModel) handleConfirm(k tea.Key) (ApprovalsModel, tea.Cmd) {
	switch k.Code {
	case 'y', 'Y':
		action := m.confirming
		m.confirming = ""
		switch action {
		case "delete":
			return m, m.deleteCmd(m.confirmID)
		case "purge":
			return m, m.purgeCmd()
		default:
		}
	case 'n', 'N', tea.KeyEscape:
		m.confirming = ""
	default:
	}
	return m, nil
}

func (m ApprovalsModel) handleScopeSelect(k tea.Key) (ApprovalsModel, tea.Cmd) {
	var scope string
	switch k.Code {
	case 'o', 'O':
		scope = "once"
	case 'c', 'C':
		scope = "command"
	case 's', 'S':
		scope = "session"
	case 'f', 'F':
		scope = "forever"
	case tea.KeyEscape:
		m.scopeSelect = false
		return m, nil
	default:
		return m, nil
	}

	m.scopeSelect = false
	return m, m.approveCmd(scope)
}

// updateDetail handles messages when the detail panel is focused.
func (m ApprovalsModel) updateDetail(msg tea.Msg) (ApprovalsModel, tea.Cmd) {
	if msg, ok := msg.(tea.KeyMsg); ok {
		k := msg.Key()
		switch {
		case key.Matches(k, keys.Enter), key.Matches(k, keys.Escape):
			m.showDetail = false
			return m, nil
		default:
		}
	}
	// Delegate scroll keys (j/k/PgUp/PgDn) to the viewport.
	var cmd tea.Cmd
	m.detailView, cmd = m.detailView.Update(msg)
	return m, cmd
}

func (m *ApprovalsModel) toggleDetail() {
	m.showDetail = !m.showDetail
	if m.showDetail && m.historyIdx >= 0 && m.historyIdx < len(m.approvals) {
		a := &m.approvals[m.historyIdx]
		content := m.renderDetail(a)
		m.detailView.SetContent(content)
		detailH := m.height * 40 / 100
		if detailH < 5 {
			detailH = 5
		}
		m.detailView.SetHeight(detailH)
		m.detailView.SetWidth(max(m.width-4, 0))
		m.detailView.GotoTop()
	}
}

func (m ApprovalsModel) renderDetail(a *db.Approval) string {
	var b strings.Builder
	now := m.clock()

	b.WriteString("  Approval Detail\n")
	b.WriteString("  " + strings.Repeat("─", max(m.width-6, 0)) + "\n")
	fmt.Fprintf(&b, "  ID:           %s\n", sanitize(a.ID))
	fmt.Fprintf(&b, "  Decision Key: %s\n", sanitize(a.DecisionKey))
	fmt.Fprintf(&b, "  Decision:     %s\n", sanitize(a.Decision))
	fmt.Fprintf(&b, "  Scope:        %s\n", sanitize(a.Scope))
	fmt.Fprintf(&b, "  Session ID:   %s\n", sanitize(fallbackValue(a.SessionID)))
	fmt.Fprintf(&b, "  Created At:   %s\n", formatApprovalTime(a.CreatedAt))

	status, _ := approvalStatus(a, now)
	fmt.Fprintf(&b, "  Status:       %s\n", status)

	if a.ConsumedAt != nil {
		fmt.Fprintf(&b, "  Consumed At:  %s\n", formatApprovalTime(*a.ConsumedAt))
	}
	if a.ExpiresAt != nil {
		fmt.Fprintf(&b, "  Expires At:   %s\n", formatApprovalTime(*a.ExpiresAt))
	}
	fmt.Fprintf(&b, "  HMAC:         %s\n", sanitize(shorten(a.HMAC, 20)))

	b.WriteString("\n  Press Enter or Esc to close\n")
	return b.String()
}

// View renders the approvals view.
func (m ApprovalsModel) View() string {
	var b strings.Builder

	b.WriteString(m.renderPendingSection())

	// Separator.
	b.WriteString("  " + strings.Repeat("─", max(m.width-4, 0)) + "\n")

	b.WriteString(m.renderHistorySection())
	b.WriteString(m.renderRecommendationsSection())

	// Detail panel (rendered via viewport for scrolling).
	if m.showDetail {
		b.WriteString(m.detailView.View())
	}

	b.WriteString(m.renderConfirmation())

	return b.String()
}

// renderPendingSection renders the pending approval requests section.
func (m ApprovalsModel) renderPendingSection() string {
	var b strings.Builder

	pendingCount := len(m.pending)
	if pendingCount == 0 {
		b.WriteString(styleDim.Render("  No pending requests") + "\n\n")
		return b.String()
	}

	fmt.Fprintf(&b, "  %s (%d)\n\n",
		styleError.Render("⚠ Pending Approval Requests"), pendingCount)

	for i := range m.pending {
		req := &m.pending[i]
		prefix := "  "
		if m.focus == focusPending && i == m.pendingIdx {
			prefix = styleCursor.Render("▶ ")
		}
		b.WriteString(prefix + sanitize(shorten(req.Command, max(m.width-4, 0))) + "\n")
		fmt.Fprintf(&b, "    Source: %s  Session: %s\n", sanitize(req.Source), sanitize(shorten(req.SessionID, 12)))
		if req.Reason != "" {
			b.WriteString("    Reason: " + sanitize(req.Reason) + "\n")
		}
		elapsed := time.Since(parseTime(req.CreatedAt)).Truncate(time.Second)
		fmt.Fprintf(&b, "    Waiting: %s\n", elapsed)
		b.WriteByte('\n')
	}

	if m.scopeSelect {
		b.WriteString("    Scope: [o]nce  [c]ommand  [s]ession  [f]orever\n\n")
	} else if m.focus == focusPending {
		b.WriteString("    [a] Approve  [d] Deny\n\n")
	}

	return b.String()
}

// renderHistorySection renders the approval history table.
func (m ApprovalsModel) renderHistorySection() string {
	var b strings.Builder

	fmt.Fprintf(&b, "  Approval History (%d)\n\n", len(m.approvals))

	if len(m.approvals) == 0 {
		b.WriteString(styleDim.Render("  No approvals yet") + "\n")
		return b.String()
	}

	header := fmt.Sprintf("  %-19s  %-8s  %-8s  %-9s  %s",
		"CREATED", "SCOPE", "DECISION", "STATUS", "KEY")
	b.WriteString(styleColHeader.Render(shorten(header, m.width)) + "\n")

	visibleRows := m.historyHeight()
	if m.historyIdx < m.histOffset {
		m.histOffset = m.historyIdx
	}
	if m.historyIdx >= m.histOffset+visibleRows {
		m.histOffset = m.historyIdx - visibleRows + 1
	}
	end := m.histOffset + visibleRows
	if end > len(m.approvals) {
		end = len(m.approvals)
	}

	now := m.clock()
	for i := m.histOffset; i < end; i++ {
		a := &m.approvals[i]
		created := formatApprovalTime(a.CreatedAt)
		status, sStyle := approvalStatus(a, now)
		keyTrunc := shorten(a.DecisionKey, m.width-55)

		row := fmt.Sprintf("  %-19s  %-8s  %-8s  %s  %s",
			created, sanitize(a.Scope), sanitize(a.Decision),
			sStyle.Render(fmt.Sprintf("%-9s", status)), sanitize(keyTrunc))

		if m.focus == focusHistory && i == m.historyIdx {
			b.WriteString(styleCursor.Render(shorten(row, m.width)) + "\n")
		} else {
			b.WriteString(shorten(row, m.width) + "\n")
		}
	}

	return b.String()
}

// renderRecommendationsSection renders the policy recommendations section.
func (m ApprovalsModel) renderRecommendationsSection() string {
	if len(m.recommendations) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("\n  " + strings.Repeat("─", max(m.width-4, 0)) + "\n")
	fmt.Fprintf(&b, "  Policy Recommendations (%d)\n\n", len(m.recommendations))
	for _, r := range m.recommendations {
		cmd := shorten(sanitize(r.Command), m.width-20)
		fmt.Fprintf(&b, "  %s  (%dx)\n", styleDim.Render(cmd), r.Count)
	}
	b.WriteString(styleDim.Render("\n  Run 'fuse doctor' for suggested policy.yaml rules") + "\n")
	return b.String()
}

// renderConfirmation renders the confirmation dialog when active.
func (m ApprovalsModel) renderConfirmation() string {
	if m.confirming == "" {
		return ""
	}

	var b strings.Builder
	b.WriteByte('\n')
	switch m.confirming {
	case "delete":
		b.WriteString(styleError.Render(fmt.Sprintf("  Revoke approval %s? [y/n]", shorten(m.confirmID, 8))))
	case "purge":
		b.WriteString(styleError.Render("  Purge all expired/consumed? [y/n]"))
	default:
	}
	b.WriteByte('\n')
	return b.String()
}

// FilterInfo returns approval-specific footer info.
func (m ApprovalsModel) FilterInfo() string {
	if m.statusMsg != "" {
		return " " + m.statusMsg
	}
	if m.focus == focusPending {
		return " [pending]"
	}
	return " [history]"
}

func (m *ApprovalsModel) moveCursor(delta int) {
	if m.focus == focusPending {
		m.pendingIdx += delta
		if m.pendingIdx < 0 {
			m.pendingIdx = 0
		}
		if m.pendingIdx >= len(m.pending) {
			m.pendingIdx = len(m.pending) - 1
		}
		if m.pendingIdx < 0 {
			m.pendingIdx = 0
		}
	} else {
		m.historyIdx += delta
		if m.historyIdx < 0 {
			m.historyIdx = 0
		}
		if m.historyIdx >= len(m.approvals) {
			m.historyIdx = len(m.approvals) - 1
		}
		if m.historyIdx < 0 {
			m.historyIdx = 0
		}
	}
}

func (m ApprovalsModel) historyHeight() int {
	h := m.height - len(m.pending)*4 - 8 // pending section + headers + separator
	if m.showDetail {
		h = h * 60 / 100 // table gets 60%, detail gets 40%
	}
	if h < 3 {
		h = 3
	}
	return h
}

// Action commands — executed as tea.Cmd goroutines.

type approveResultMsg struct {
	scope string
	err   error
}

type denyResultMsg struct {
	err error
}

type deleteResultMsg struct {
	err error
}

type purgeResultMsg struct {
	deleted int64
	err     error
}

func (m ApprovalsModel) approveCmd(scope string) tea.Cmd {
	if m.pendingIdx >= len(m.pending) {
		return nil
	}
	req := m.pending[m.pendingIdx]
	database := m.database
	secret := m.secret
	return func() tea.Msg {
		mgr, err := approve.NewManager(database, secret)
		if err != nil {
			return approveResultMsg{err: err}
		}
		err = mgr.CreateApproval(req.DecisionKey, "APPROVAL", scope, req.SessionID)
		if err == nil {
			_ = database.DeletePendingRequest(req.ID)
			_ = database.LogEvent(&db.EventRecord{
				Command:      req.Command,
				Decision:     "APPROVAL",
				Reason:       req.Reason,
				Source:       "tui",
				SessionID:    req.SessionID,
				UserResponse: "approved_via_tui (scope: " + scope + ")",
			})
		}
		return approveResultMsg{scope: scope, err: err}
	}
}

func (m ApprovalsModel) denyCmd() tea.Cmd {
	if m.pendingIdx >= len(m.pending) {
		return nil
	}
	req := m.pending[m.pendingIdx]
	database := m.database
	secret := m.secret
	return func() tea.Msg {
		mgr, err := approve.NewManager(database, secret)
		if err != nil {
			return denyResultMsg{err: err}
		}
		// Deny with "once" scope — blocks THIS request, not future ones.
		err = mgr.CreateApproval(req.DecisionKey, "BLOCKED", "once", req.SessionID)
		if err == nil {
			_ = database.DeletePendingRequest(req.ID)
			_ = database.LogEvent(&db.EventRecord{
				Command:      req.Command,
				Decision:     "BLOCKED",
				Reason:       req.Reason,
				Source:       "tui",
				SessionID:    req.SessionID,
				UserResponse: "denied_via_tui",
			})
		}
		return denyResultMsg{err: err}
	}
}

func (m ApprovalsModel) deleteCmd(id string) tea.Cmd {
	database := m.database
	return func() tea.Msg {
		return deleteResultMsg{err: database.DeleteApproval(id)}
	}
}

func (m ApprovalsModel) purgeCmd() tea.Cmd {
	database := m.database
	return func() tea.Msg {
		deleted, err := database.CleanupExpired()
		return purgeResultMsg{deleted: deleted, err: err}
	}
}

// approvalStatus computes the display status with precedence: CONSUMED > EXPIRED > PENDING.
func approvalStatus(a *db.Approval, now time.Time) (string, lipgloss.Style) {
	if a.ConsumedAt != nil {
		return "CONSUMED", styleDim
	}
	if a.ExpiresAt != nil {
		exp, err := time.Parse(timestampFormat, *a.ExpiresAt)
		if err != nil {
			exp, err = time.Parse(timestampFormatNoMillis, *a.ExpiresAt)
		}
		if err == nil && !now.Before(exp) {
			return "EXPIRED", styleBlocked
		}
	}
	return "PENDING", styleSafe
}

func formatApprovalTime(ts string) string {
	t, err := time.Parse(timestampFormat, ts)
	if err != nil {
		t, err = time.Parse(timestampFormatNoMillis, ts)
		if err != nil {
			return shorten(ts, 19)
		}
	}
	return t.Local().Format("2006-01-02 15:04:05")
}

func parseTime(ts string) time.Time {
	t, err := time.Parse(timestampFormat, ts)
	if err != nil {
		t, err = time.Parse(timestampFormatNoMillis, ts)
		if err != nil {
			return time.Now()
		}
	}
	return t
}
