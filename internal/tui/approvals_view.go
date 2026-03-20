package tui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/runger/fuse/internal/db"
)

// ApprovalsModel renders a list of approvals with status.
type ApprovalsModel struct {
	approvals []db.Approval
	cursor    int
	offset    int
	clock     func() time.Time // injectable for tests; defaults to time.Now().UTC()
	width     int
	height    int
}

// NewApprovalsModel creates an initialized ApprovalsModel.
func NewApprovalsModel() ApprovalsModel {
	return ApprovalsModel{
		clock: func() time.Time { return time.Now().UTC() },
	}
}

// SetData updates the approvals list.
func (m *ApprovalsModel) SetData(approvals []db.Approval) {
	m.approvals = approvals
}

// SetSize updates dimensions.
func (m *ApprovalsModel) SetSize(w, h int) {
	m.width = w
	m.height = h
}

// Update handles key messages.
func (m ApprovalsModel) Update(msg tea.Msg) (ApprovalsModel, tea.Cmd) {
	if msg, ok := msg.(tea.KeyMsg); ok {
		k := msg.Key()
		switch {
		case key.Matches(k, keys.Up):
			if m.cursor > 0 {
				m.cursor--
			}
		case key.Matches(k, keys.Down):
			if m.cursor < len(m.approvals)-1 {
				m.cursor++
			}
		case key.Matches(k, keys.Home):
			m.cursor = 0
			m.offset = 0
		case key.Matches(k, keys.End):
			if len(m.approvals) > 0 {
				m.cursor = len(m.approvals) - 1
			}
		}
	}
	return m, nil
}

// View renders the approvals table.
func (m ApprovalsModel) View() string {
	if len(m.approvals) == 0 {
		return styleDim.Render("  No approvals yet")
	}

	var b strings.Builder

	header := fmt.Sprintf("  %-19s  %-8s  %-8s  %-9s  %s",
		"CREATED", "SCOPE", "DECISION", "STATUS", "KEY")
	b.WriteString(styleColHeader.Render(shorten(header, m.width)) + "\n")

	visibleRows := m.height - 2
	if visibleRows < 1 {
		visibleRows = 1
	}

	// Clamp offset.
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+visibleRows {
		m.offset = m.cursor - visibleRows + 1
	}

	end := m.offset + visibleRows
	if end > len(m.approvals) {
		end = len(m.approvals)
	}

	now := m.clock()
	for i := m.offset; i < end; i++ {
		a := &m.approvals[i]
		created := formatApprovalTime(a.CreatedAt)
		status, statusStyle := approvalStatus(a, now)
		keyTrunc := shorten(a.DecisionKey, m.width-55)

		row := fmt.Sprintf("  %-19s  %-8s  %-8s  %s  %s",
			created,
			sanitize(a.Scope),
			sanitize(a.Decision),
			statusStyle.Render(fmt.Sprintf("%-9s", status)),
			sanitize(keyTrunc))

		if i == m.cursor {
			b.WriteString(styleCursor.Render(shorten(row, m.width)) + "\n")
		} else {
			b.WriteString(shorten(row, m.width) + "\n")
		}
	}

	return b.String()
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
