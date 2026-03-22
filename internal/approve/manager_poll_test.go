package approve

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/runger/fuse/internal/db"
)

func openTestDB(t *testing.T) *db.DB {
	t.Helper()
	dir := t.TempDir()
	d, err := db.OpenDB(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	return d
}

func testSecret() []byte {
	return []byte("01234567890123456789012345678901") // 32 bytes
}

func testReq(decisionKey, command, reason, sessionID string) ApprovalRequest {
	return ApprovalRequest{
		DecisionKey:    decisionKey,
		Command:        command,
		Reason:         reason,
		SessionID:      sessionID,
		Source:         "hook",
		NonInteractive: true,
	}
}

func TestRequestApproval_PollResolvesFromDB(t *testing.T) {
	database := openTestDB(t)
	secret := testSecret()
	mgr, err := NewManager(database, secret)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	decisionKey := "test-key-poll"
	sessionID := "test-session"

	go func() {
		time.Sleep(300 * time.Millisecond)
		_ = mgr.CreateApproval(decisionKey, "APPROVAL", "once", sessionID)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	decision, err := mgr.RequestApproval(ctx, testReq(decisionKey, "echo test", "test reason", sessionID))
	if err != nil {
		t.Fatalf("RequestApproval: %v", err)
	}
	if decision != "APPROVAL" {
		t.Errorf("decision = %q, want APPROVAL", decision)
	}
}

func TestRequestApproval_TimeoutReturnsBlocked(t *testing.T) {
	database := openTestDB(t)
	secret := testSecret()
	mgr, err := NewManager(database, secret)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	decision, err := mgr.RequestApproval(ctx, testReq("no-approval-key", "echo test", "reason", "sess"))
	if decision != "BLOCKED" {
		t.Errorf("decision = %q, want BLOCKED (no approval, timed out)", decision)
	}
	_ = err
}

func TestRequestApproval_PendingRequestCleanedUp(t *testing.T) {
	database := openTestDB(t)
	secret := testSecret()
	mgr, err := NewManager(database, secret)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	_ = mgr.CreateApproval("cleanup-key", "APPROVAL", "once", "sess")

	ctx := context.Background()
	_, _ = mgr.RequestApproval(ctx, testReq("cleanup-key", "echo test", "reason", "sess"))

	pending, err := database.ListPendingRequests()
	if err != nil {
		t.Fatalf("ListPendingRequests: %v", err)
	}
	if len(pending) != 0 {
		t.Errorf("expected 0 pending requests after resolution, got %d", len(pending))
	}
}

func TestRequestApproval_TUIApproveResolvesHook(t *testing.T) {
	database := openTestDB(t)
	secret := testSecret()

	hookMgr, err := NewManager(database, secret)
	if err != nil {
		t.Fatalf("NewManager (hook): %v", err)
	}

	tuiMgr, err := NewManager(database, secret)
	if err != nil {
		t.Fatalf("NewManager (tui): %v", err)
	}

	decisionKey := "tui-e2e-approve"
	sessionID := "tui-sess"

	go func() {
		time.Sleep(300 * time.Millisecond)
		_ = tuiMgr.CreateApproval(decisionKey, "APPROVAL", "session", sessionID)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	decision, err := hookMgr.RequestApproval(ctx, testReq(decisionKey, "kubectl delete pod", "k8s rule", sessionID))
	if err != nil {
		t.Fatalf("RequestApproval: %v", err)
	}
	if decision != "APPROVAL" {
		t.Errorf("decision = %q, want APPROVAL", decision)
	}
}

func TestRequestApproval_TUIApproveSessionScopeReusable(t *testing.T) {
	database := openTestDB(t)
	secret := testSecret()

	mgr, err := NewManager(database, secret)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	decisionKey := "session-reuse-key"
	sessionID := "reuse-sess"

	if err := mgr.CreateApproval(decisionKey, "APPROVAL", "session", sessionID); err != nil {
		t.Fatalf("CreateApproval: %v", err)
	}

	ctx := context.Background()
	d1, err := mgr.RequestApproval(ctx, testReq(decisionKey, "echo 1", "test", sessionID))
	if err != nil {
		t.Fatalf("first RequestApproval: %v", err)
	}
	if d1 != "APPROVAL" {
		t.Errorf("first decision = %q, want APPROVAL", d1)
	}

	d2, err := mgr.RequestApproval(ctx, testReq(decisionKey, "echo 2", "test", sessionID))
	if err != nil {
		t.Fatalf("second RequestApproval: %v", err)
	}
	if d2 != "APPROVAL" {
		t.Errorf("second decision = %q, want APPROVAL (session scope reusable)", d2)
	}
}

func TestRequestApproval_DenyViaDB(t *testing.T) {
	database := openTestDB(t)
	secret := testSecret()
	mgr, err := NewManager(database, secret)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	decisionKey := "deny-key"
	sessionID := "sess"

	go func() {
		time.Sleep(300 * time.Millisecond)
		_ = mgr.CreateApproval(decisionKey, "BLOCKED", "once", sessionID)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	decision, err := mgr.RequestApproval(ctx, testReq(decisionKey, "terraform destroy", "IaC rule", sessionID))
	if err != nil {
		t.Fatalf("RequestApproval: %v", err)
	}
	if decision != "BLOCKED" {
		t.Errorf("decision = %q, want BLOCKED (denied via DB)", decision)
	}
}
