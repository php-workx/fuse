package tui

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/runger/fuse/internal/db"
)

func ptr(s string) *string { return &s }

func TestApprovalStatus_Consumed(t *testing.T) {
	now := time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC)
	a := &db.Approval{ConsumedAt: ptr("2026-03-20T10:00:00Z"), ExpiresAt: ptr("2026-03-19T10:00:00Z")}
	status, _ := approvalStatus(a, now)
	if status != "CONSUMED" {
		t.Errorf("consumed+expired: got %q, want CONSUMED (precedence)", status)
	}
}

func TestApprovalStatus_Expired(t *testing.T) {
	now := time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC)
	a := &db.Approval{ExpiresAt: ptr("2026-03-20T10:00:00Z")}
	status, _ := approvalStatus(a, now)
	if status != "EXPIRED" {
		t.Errorf("expired: got %q, want EXPIRED", status)
	}
}

func TestApprovalStatus_Pending(t *testing.T) {
	now := time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC)
	a := &db.Approval{ExpiresAt: ptr("2026-03-21T10:00:00Z")}
	status, _ := approvalStatus(a, now)
	if status != "PENDING" {
		t.Errorf("pending: got %q, want PENDING", status)
	}
}

func TestApprovalStatus_NilExpires(t *testing.T) {
	now := time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC)
	a := &db.Approval{}
	status, _ := approvalStatus(a, now)
	if status != "PENDING" {
		t.Errorf("nil expires: got %q, want PENDING", status)
	}
}

func TestApprovalsView_PendingRequestDisplayed(t *testing.T) {
	m := NewApprovalsModel(nil, nil)
	m.width = 80
	m.height = 40
	m.SetPending([]db.PendingRequest{
		{
			ID:          "req-1",
			DecisionKey: "key-1",
			Command:     "terraform destroy",
			Reason:      "IaC rule",
			Source:      "hook",
			SessionID:   "sess-1",
			CreatedAt:   time.Now().UTC().Format("2006-01-02T15:04:05.000Z"),
		},
	})

	view := m.View()
	stripped := sanitize(view)

	if !containsStr(stripped, "terraform destroy") {
		t.Errorf("view should show pending command, got:\n%s", stripped)
	}
	if !containsStr(stripped, "hook") {
		t.Errorf("view should show source 'hook', got:\n%s", stripped)
	}
}

func TestApprovalsView_EmptyState(t *testing.T) {
	m := NewApprovalsModel(nil, nil)
	m.width = 80
	m.height = 40

	view := m.View()
	stripped := sanitize(view)

	if !containsStr(stripped, "No pending requests") {
		t.Errorf("expected 'No pending requests', got:\n%s", stripped)
	}
	if !containsStr(stripped, "No approvals yet") {
		t.Errorf("expected 'No approvals yet', got:\n%s", stripped)
	}
}

func TestApprovalsView_MoveCursor(t *testing.T) {
	m := NewApprovalsModel(nil, nil)
	m.SetPending([]db.PendingRequest{
		{ID: "r1", Command: "cmd1", Source: "hook", CreatedAt: "2026-03-20T12:00:00Z"},
		{ID: "r2", Command: "cmd2", Source: "hook", CreatedAt: "2026-03-20T12:01:00Z"},
	})

	m.focus = focusPending
	m.moveCursor(1)
	if m.pendingIdx != 1 {
		t.Errorf("pendingIdx = %d, want 1", m.pendingIdx)
	}
	m.moveCursor(1) // clamp
	if m.pendingIdx != 1 {
		t.Errorf("pendingIdx = %d, want 1 (clamped)", m.pendingIdx)
	}
	m.moveCursor(-5) // clamp to 0
	if m.pendingIdx != 0 {
		t.Errorf("pendingIdx = %d, want 0", m.pendingIdx)
	}
}

func TestApprovalsView_DetailPanelToggle(t *testing.T) {
	m := NewApprovalsModel(nil, nil)
	m.width = 80
	m.height = 40
	m.focus = focusHistory
	m.SetData([]db.Approval{
		{
			ID: "a1", DecisionKey: "key-1", Decision: "APPROVAL",
			Scope: "session", SessionID: "sess-1",
			CreatedAt: "2026-03-20T12:00:00Z",
			HMAC:      "hmac-test-value",
		},
	})

	// Press Enter — should open detail panel.
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !m.showDetail {
		t.Fatal("expected showDetail=true after Enter")
	}

	// Detail panel should contain approval fields.
	view := m.View()
	if !containsStr(view, "Approval Detail") {
		t.Error("detail panel should show 'Approval Detail' header")
	}
	if !containsStr(view, "key-1") {
		t.Error("detail panel should show decision key")
	}
	if !containsStr(view, "session") {
		t.Error("detail panel should show scope")
	}

	// Press Escape — should close detail panel.
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.showDetail {
		t.Fatal("expected showDetail=false after Escape")
	}
}

func TestApprovalsView_DetailPanelEmptyHistory(t *testing.T) {
	m := NewApprovalsModel(nil, nil)
	m.width = 80
	m.height = 40
	m.focus = focusHistory
	// Empty history.

	// Press Enter with no history items — should not open detail.
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.showDetail {
		t.Error("should not open detail panel with empty history")
	}
}

func makeKeyMsg(code rune) tea.KeyMsg {
	return tea.KeyPressMsg{Code: code}
}

func TestApprovalsView_ApproveScopeSelection(t *testing.T) {
	m := NewApprovalsModel(nil, nil)
	m.width = 80
	m.height = 40
	m.focus = focusPending
	m.SetPending([]db.PendingRequest{
		{ID: "r1", DecisionKey: "k1", Command: "cmd1", Source: "hook", CreatedAt: "2026-03-20T12:00:00Z"},
	})

	// Press 'a' — should enter scope selection mode.
	m, _ = m.Update(makeKeyMsg('a'))
	if !m.scopeSelect {
		t.Fatal("expected scopeSelect=true after pressing 'a'")
	}

	// Press Escape — should cancel scope selection.
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.scopeSelect {
		t.Fatal("expected scopeSelect=false after Escape")
	}

	// Press 'a' again, then 'o' for once scope.
	m, _ = m.Update(makeKeyMsg('a'))
	if !m.scopeSelect {
		t.Fatal("expected scopeSelect=true after second 'a'")
	}
	m, _ = m.Update(makeKeyMsg('o'))
	if m.scopeSelect {
		t.Fatal("expected scopeSelect=false after scope key")
	}
}

func TestApprovalsView_DenyPendingRequest(t *testing.T) {
	m := NewApprovalsModel(nil, nil)
	m.width = 80
	m.height = 40
	m.focus = focusPending
	m.SetPending([]db.PendingRequest{
		{ID: "r1", DecisionKey: "k1", Command: "cmd1", Source: "hook", CreatedAt: "2026-03-20T12:00:00Z"},
	})

	// Press 'd' — should produce a command (denyCmd).
	_, cmd := m.Update(makeKeyMsg('d'))
	if cmd == nil {
		t.Error("expected non-nil cmd after pressing 'd' on pending request")
	}
}

func TestApprovalsView_RevokeConfirmation(t *testing.T) {
	m := NewApprovalsModel(nil, nil)
	m.width = 80
	m.height = 40
	m.focus = focusHistory
	m.SetData([]db.Approval{
		{ID: "a1", DecisionKey: "k1", Decision: "APPROVAL", Scope: "once", CreatedAt: "2026-03-20T12:00:00Z"},
	})

	// Press 'x' (revoke key) — should enter confirmation mode.
	m, _ = m.Update(makeKeyMsg('x'))
	if m.confirming != "delete" {
		t.Errorf("expected confirming='delete', got %q", m.confirming)
	}

	// Press 'n' — should cancel.
	m, _ = m.Update(makeKeyMsg('n'))
	if m.confirming != "" {
		t.Errorf("expected confirming='', got %q", m.confirming)
	}
}

func TestApprovalsView_PurgeConfirmation(t *testing.T) {
	m := NewApprovalsModel(nil, nil)
	m.width = 80
	m.height = 40

	// Press 'X' — should enter purge confirmation.
	m, _ = m.Update(makeKeyMsg('X'))
	if m.confirming != "purge" {
		t.Errorf("expected confirming='purge', got %q", m.confirming)
	}

	// Press 'y' — should clear confirming and execute purge.
	m, cmd := m.Update(makeKeyMsg('y'))
	if m.confirming != "" {
		t.Errorf("expected confirming cleared after 'y', got %q", m.confirming)
	}
	// cmd may be nil since database is nil, but confirming should be cleared.
	_ = cmd
}

func TestApprovalsView_ApproveEmptyListNoOp(t *testing.T) {
	m := NewApprovalsModel(nil, nil)
	m.width = 80
	m.height = 40
	m.focus = focusPending
	// Empty pending list.

	// Press 'a' — should NOT enter scope selection (no pending items).
	m, _ = m.Update(makeKeyMsg('a'))
	if m.scopeSelect {
		t.Error("expected scopeSelect=false with empty pending list")
	}
}

func TestApprovalsView_DenyEmptyListNoOp(t *testing.T) {
	m := NewApprovalsModel(nil, nil)
	m.width = 80
	m.height = 40
	m.focus = focusPending

	// Press 'd' with empty list — should return nil cmd.
	_, cmd := m.Update(makeKeyMsg('d'))
	if cmd != nil {
		t.Error("expected nil cmd when denying with empty pending list")
	}
}

func containsStr(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
