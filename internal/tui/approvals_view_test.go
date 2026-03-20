package tui

import (
	"testing"
	"time"

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

func containsStr(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
