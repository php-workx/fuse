package tui

import (
	"fmt"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/php-workx/fuse/internal/db"
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

// --- FilterInfo tests ---

func TestApprovalsFilterInfo_WithStatusMsg(t *testing.T) {
	m := NewApprovalsModel(nil, nil)
	m.statusMsg = "Approved (once)"
	got := m.FilterInfo()
	if got != " Approved (once)" {
		t.Errorf("FilterInfo with status: got %q, want %q", got, " Approved (once)")
	}
}

func TestApprovalsFilterInfo_FocusPending(t *testing.T) {
	m := NewApprovalsModel(nil, nil)
	m.focus = focusPending
	got := m.FilterInfo()
	if got != " [pending]" {
		t.Errorf("FilterInfo pending: got %q, want %q", got, " [pending]")
	}
}

func TestApprovalsFilterInfo_FocusHistory(t *testing.T) {
	m := NewApprovalsModel(nil, nil)
	m.focus = focusHistory
	got := m.FilterInfo()
	if got != " [history]" {
		t.Errorf("FilterInfo history: got %q, want %q", got, " [history]")
	}
}

// --- StatusMsg tests ---

func TestStatusMsg_GetAndClear(t *testing.T) {
	m := NewApprovalsModel(nil, nil)
	m.statusMsg = "Denied"
	got := m.StatusMsg()
	if got != "Denied" {
		t.Errorf("StatusMsg first call: got %q, want %q", got, "Denied")
	}
	got = m.StatusMsg()
	if got != "" {
		t.Errorf("StatusMsg second call: got %q, want empty", got)
	}
}

func TestStatusMsg_EmptyInitially(t *testing.T) {
	m := NewApprovalsModel(nil, nil)
	got := m.StatusMsg()
	if got != "" {
		t.Errorf("StatusMsg initial: got %q, want empty", got)
	}
}

// --- handleActionResult tests ---

func TestHandleActionResult_ApproveSuccess(t *testing.T) {
	m := NewApprovalsModel(nil, nil)
	m.handleActionResult(approveResultMsg{scope: "session", err: nil})
	if m.statusMsg != "Approved (session)" {
		t.Errorf("approve success: got %q, want %q", m.statusMsg, "Approved (session)")
	}
}

func TestHandleActionResult_ApproveError(t *testing.T) {
	m := NewApprovalsModel(nil, nil)
	m.handleActionResult(approveResultMsg{err: errTest})
	if !containsStr(m.statusMsg, "Error:") {
		t.Errorf("approve error: got %q, want Error prefix", m.statusMsg)
	}
}

func TestHandleActionResult_DenySuccess(t *testing.T) {
	m := NewApprovalsModel(nil, nil)
	m.handleActionResult(denyResultMsg{err: nil})
	if m.statusMsg != "Denied" {
		t.Errorf("deny success: got %q, want %q", m.statusMsg, "Denied")
	}
}

func TestHandleActionResult_DeleteSuccess(t *testing.T) {
	m := NewApprovalsModel(nil, nil)
	m.handleActionResult(deleteResultMsg{err: nil})
	if m.statusMsg != "Revoked" {
		t.Errorf("delete success: got %q, want %q", m.statusMsg, "Revoked")
	}
}

func TestHandleActionResult_PurgeSuccess(t *testing.T) {
	m := NewApprovalsModel(nil, nil)
	m.handleActionResult(purgeResultMsg{deleted: 5, err: nil})
	if m.statusMsg != "Purged 5" {
		t.Errorf("purge success: got %q, want %q", m.statusMsg, "Purged 5")
	}
}

func TestHandleActionResult_PurgeError(t *testing.T) {
	m := NewApprovalsModel(nil, nil)
	m.handleActionResult(purgeResultMsg{err: errTest})
	if !containsStr(m.statusMsg, "Error:") {
		t.Errorf("purge error: got %q, want Error prefix", m.statusMsg)
	}
}

var errTest = fmt.Errorf("test error")

// --- moveCursor history tests ---

func TestApprovalsMoveCursor_History(t *testing.T) {
	m := NewApprovalsModel(nil, nil)
	m.focus = focusHistory
	m.SetData([]db.Approval{
		{ID: "a1", DecisionKey: "k1", CreatedAt: "2026-03-20T12:00:00Z"},
		{ID: "a2", DecisionKey: "k2", CreatedAt: "2026-03-20T12:01:00Z"},
		{ID: "a3", DecisionKey: "k3", CreatedAt: "2026-03-20T12:02:00Z"},
	})

	m.moveCursor(1)
	if m.historyIdx != 1 {
		t.Errorf("historyIdx = %d, want 1", m.historyIdx)
	}
	m.moveCursor(1)
	if m.historyIdx != 2 {
		t.Errorf("historyIdx = %d, want 2", m.historyIdx)
	}
	m.moveCursor(1) // clamp at end
	if m.historyIdx != 2 {
		t.Errorf("historyIdx = %d, want 2 (clamped)", m.historyIdx)
	}
	m.moveCursor(-10) // clamp at start
	if m.historyIdx != 0 {
		t.Errorf("historyIdx = %d, want 0", m.historyIdx)
	}
}

func TestApprovalsMoveCursor_HistoryEmpty(t *testing.T) {
	m := NewApprovalsModel(nil, nil)
	m.focus = focusHistory
	// No approvals set.
	m.moveCursor(1) // should not panic
	if m.historyIdx != 0 {
		t.Errorf("historyIdx on empty: got %d, want 0", m.historyIdx)
	}
}

// --- parseTime tests ---

func TestParseTime_ValidWithMillis(t *testing.T) {
	got := parseTime("2026-03-20T14:30:00.123Z")
	if got.Year() != 2026 || got.Month() != 3 || got.Day() != 20 {
		t.Errorf("valid with millis: got %v", got)
	}
}

func TestParseTime_ValidWithoutMillis(t *testing.T) {
	got := parseTime("2026-03-20T14:30:00Z")
	if got.Year() != 2026 || got.Month() != 3 || got.Day() != 20 {
		t.Errorf("valid without millis: got %v", got)
	}
}

func TestParseTime_InvalidFormat(t *testing.T) {
	got := parseTime("not-a-date")
	// Should fall back to time.Now() — just verify it's recent.
	if time.Since(got) > time.Minute {
		t.Errorf("invalid format should return ~now, got %v", got)
	}
}

func TestParseTime_Empty(t *testing.T) {
	got := parseTime("")
	// Should fall back to time.Now().
	if time.Since(got) > time.Minute {
		t.Errorf("empty should return ~now, got %v", got)
	}
}
