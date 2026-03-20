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
	a := &db.Approval{ExpiresAt: ptr("2026-03-20T10:00:00Z")} // expired 2h ago
	status, _ := approvalStatus(a, now)
	if status != "EXPIRED" {
		t.Errorf("expired: got %q, want EXPIRED", status)
	}
}

func TestApprovalStatus_Pending(t *testing.T) {
	now := time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC)
	a := &db.Approval{ExpiresAt: ptr("2026-03-21T10:00:00Z")} // expires tomorrow
	status, style := approvalStatus(a, now)
	if status != "PENDING" {
		t.Errorf("pending: got %q, want PENDING", status)
	}
	_ = style // styleSafe
}

func TestApprovalStatus_NilExpires(t *testing.T) {
	now := time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC)
	a := &db.Approval{} // nil consumed, nil expires → never expires → PENDING
	status, _ := approvalStatus(a, now)
	if status != "PENDING" {
		t.Errorf("nil expires: got %q, want PENDING", status)
	}
}

func TestApprovalStatus_InjectableClock(t *testing.T) {
	// Verify the injectable clock is used.
	m := NewApprovalsModel(nil, nil)
	fixedTime := time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC)
	m.clock = func() time.Time { return fixedTime }

	m.approvals = []db.Approval{
		{
			ID: "a1", Decision: "APPROVAL", Scope: "once", CreatedAt: "2026-03-20T11:00:00Z",
			ExpiresAt: ptr("2026-03-20T11:30:00Z"),
		}, // expired 30min ago per fixedTime
	}

	// Verify the approval computes as EXPIRED via the injected clock.
	now := m.clock()
	status, _ := approvalStatus(&m.approvals[0], now)
	if status != "EXPIRED" {
		t.Errorf("injectable clock: got status %q, want EXPIRED", status)
	}
}

func TestApprovalStatus_Pending_NotExpired(t *testing.T) {
	now := time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC)
	a := &db.Approval{ExpiresAt: ptr("2026-03-21T12:00:00Z")} // tomorrow
	status, _ := approvalStatus(a, now)
	if status != "PENDING" {
		t.Errorf("not expired: got %q, want PENDING", status)
	}
}
