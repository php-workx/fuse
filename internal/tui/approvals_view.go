package tui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/runger/fuse/internal/approve"
	"github.com/runger/fuse/internal/db"
)

type approvalFocus int

const (
	focusPending approvalFocus = iota
	focusHistory
)

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
	statusMsg   string // transient footer message

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
		database: database,
		secret:   secret,
		clock:    func() time.Time { return time.Now().UTC() },
	}
}

// SetData updates the approval history.
func (m *ApprovalsModel) SetData(approvals []db.Approval) {
	m.approvals = approvals
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
}

// StatusMsg returns and clears the transient status message.
func (m *ApprovalsModel) StatusMsg() string {
	msg := m.statusMsg
	m.statusMsg = ""
	return msg
}

// Update handles key messages.
func (m ApprovalsModel) Update(msg tea.Msg) (ApprovalsModel, tea.Cmd) {
	if msg, ok := msg.(tea.KeyMsg); ok {
		k := msg.Key()

		// Confirmation dialog.
		if m.confirming != "" {
			return m.handleConfirm(k)
		}

		// Scope selection after pressing 'a'.
		if m.scopeSelect {
			return m.handleScopeSelect(k)
		}

		switch {
		// Toggle focus between pending and history.
		case key.Matches(k, keys.Tab):
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
		}
	}

	// Handle action results.
	switch msg := msg.(type) {
	case approveResultMsg:
		if msg.err != nil {
			m.statusMsg = "Error: " + msg.err.Error()
		} else {
			m.statusMsg = "Approved (" + msg.scope + ")"
		}
	case denyResultMsg:
		if msg.err != nil {
			m.statusMsg = "Error: " + msg.err.Error()
		} else {
			m.statusMsg = "Denied"
		}
	case deleteResultMsg:
		if msg.err != nil {
			m.statusMsg = "Error: " + msg.err.Error()
		} else {
			m.statusMsg = "Revoked"
		}
	case purgeResultMsg:
		if msg.err != nil {
			m.statusMsg = "Error: " + msg.err.Error()
		} else {
			m.statusMsg = fmt.Sprintf("Purged %d", msg.deleted)
		}
	}

	return m, nil
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
		}
	case 'n', 'N', tea.KeyEscape:
		m.confirming = ""
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

// View renders the approvals view.
func (m ApprovalsModel) View() string {
	var b strings.Builder

	// Pending section.
	pendingCount := len(m.pending)
	if pendingCount > 0 {
		fmt.Fprintf(&b, "  %s (%d)\n\n",
			styleError.Render("⚠ Pending Approval Requests"), pendingCount)

		for i := range m.pending {
			req := &m.pending[i]
			prefix := "  "
			if m.focus == focusPending && i == m.pendingIdx {
				prefix = styleCursor.Render("▶ ")
			}
			b.WriteString(prefix + sanitize(shorten(req.Command, m.width-4)) + "\n")
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
	} else {
		b.WriteString(styleDim.Render("  No pending requests") + "\n\n")
	}

	// Separator.
	b.WriteString("  " + strings.Repeat("─", m.width-4) + "\n")

	// History section.
	fmt.Fprintf(&b, "  Approval History (%d)\n\n", len(m.approvals))

	if len(m.approvals) == 0 {
		b.WriteString(styleDim.Render("  No approvals yet") + "\n")
	} else {
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
	}

	// Confirmation dialog.
	if m.confirming != "" {
		b.WriteByte('\n')
		switch m.confirming {
		case "delete":
			b.WriteString(styleError.Render(fmt.Sprintf("  Revoke approval %s? [y/n]", shorten(m.confirmID, 8))))
		case "purge":
			b.WriteString(styleError.Render("  Purge all expired/consumed? [y/n]"))
		}
		b.WriteByte('\n')
	}

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
		exp, err := time.Parse("2006-01-02T15:04:05.000Z", *a.ExpiresAt)
		if err != nil {
			exp, err = time.Parse("2006-01-02T15:04:05Z", *a.ExpiresAt)
		}
		if err == nil && !now.Before(exp) {
			return "EXPIRED", styleBlocked
		}
	}
	return "PENDING", styleSafe
}

func formatApprovalTime(ts string) string {
	t, err := time.Parse("2006-01-02T15:04:05.000Z", ts)
	if err != nil {
		t, err = time.Parse("2006-01-02T15:04:05Z", ts)
		if err != nil {
			return shorten(ts, 19)
		}
	}
	return t.Local().Format("2006-01-02 15:04:05")
}

func parseTime(ts string) time.Time {
	t, err := time.Parse("2006-01-02T15:04:05.000Z", ts)
	if err != nil {
		t, err = time.Parse("2006-01-02T15:04:05Z", ts)
		if err != nil {
			return time.Now()
		}
	}
	return t
}
