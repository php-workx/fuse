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

func TestRequestApproval_PollResolvesFromDB(t *testing.T) {
	database := openTestDB(t)
	secret := testSecret()
	mgr, err := NewManager(database, secret)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	decisionKey := "test-key-poll"
	sessionID := "test-session"

	// Pre-create an approval in the DB (simulating TUI action).
	// Use a goroutine with a small delay to simulate async TUI approval.
	go func() {
		time.Sleep(300 * time.Millisecond)
		_ = mgr.CreateApproval(decisionKey, "APPROVAL", "once", sessionID)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// RequestApproval should find the externally-created approval via DB poll
	// without needing TTY input (nonInteractive=true skips TTY prompt).
	decision, err := mgr.RequestApproval(ctx, decisionKey, "echo test", "test reason", sessionID, "hook", false, true)
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

	// No approval created — should timeout and return BLOCKED.
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	decision, err := mgr.RequestApproval(ctx, "no-approval-key", "echo test", "reason", "sess", "hook", false, true)
	// In non-interactive mode, the prompt fails immediately with errNonInteractive.
	// The poll runs but finds nothing within the context timeout.
	// Expected: BLOCKED (either from prompt failure or context timeout).
	if decision != "BLOCKED" {
		t.Errorf("decision = %q, want BLOCKED (no approval, timed out)", decision)
	}
	_ = err // error is expected (prompt user: non-interactive)
}

func TestRequestApproval_PendingRequestCleanedUp(t *testing.T) {
	database := openTestDB(t)
	secret := testSecret()
	mgr, err := NewManager(database, secret)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	// Pre-create approval so it resolves quickly.
	_ = mgr.CreateApproval("cleanup-key", "APPROVAL", "once", "sess")

	ctx := context.Background()
	_, _ = mgr.RequestApproval(ctx, "cleanup-key", "echo test", "reason", "sess", "hook", false, true)

	// After RequestApproval returns, the pending request should be cleaned up.
	pending, err := database.ListPendingRequests()
	if err != nil {
		t.Fatalf("ListPendingRequests: %v", err)
	}
	if len(pending) != 0 {
		t.Errorf("expected 0 pending requests after resolution, got %d", len(pending))
	}
}

// TestRequestApproval_TUIApproveResolvesHook simulates the full TUI→hook flow:
// a separate Manager (simulating the TUI process) creates an approval that
// the hook's DB poll picks up.
func TestRequestApproval_TUIApproveResolvesHook(t *testing.T) {
	database := openTestDB(t)
	secret := testSecret()

	// Hook-side manager.
	hookMgr, err := NewManager(database, secret)
	if err != nil {
		t.Fatalf("NewManager (hook): %v", err)
	}

	// TUI-side manager (same DB and secret, different Manager instance).
	tuiMgr, err := NewManager(database, secret)
	if err != nil {
		t.Fatalf("NewManager (tui): %v", err)
	}

	decisionKey := "tui-e2e-approve"
	sessionID := "tui-sess"

	// Simulate TUI approving after 300ms.
	go func() {
		time.Sleep(300 * time.Millisecond)
		_ = tuiMgr.CreateApproval(decisionKey, "APPROVAL", "session", sessionID)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	decision, err := hookMgr.RequestApproval(ctx, decisionKey, "kubectl delete pod", "k8s rule", sessionID, "hook", false, true)
	if err != nil {
		t.Fatalf("RequestApproval: %v", err)
	}
	if decision != "APPROVAL" {
		t.Errorf("decision = %q, want APPROVAL", decision)
	}
}

// TestRequestApproval_TUIApproveSessionScopeReusable verifies that a session-scoped
// approval from the TUI is reusable for subsequent commands with the same key.
func TestRequestApproval_TUIApproveSessionScopeReusable(t *testing.T) {
	database := openTestDB(t)
	secret := testSecret()

	mgr, err := NewManager(database, secret)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	decisionKey := "session-reuse-key"
	sessionID := "reuse-sess"

	// Create session-scoped approval (simulating TUI).
	if err := mgr.CreateApproval(decisionKey, "APPROVAL", "session", sessionID); err != nil {
		t.Fatalf("CreateApproval: %v", err)
	}

	// First consumption — should succeed.
	ctx := context.Background()
	d1, err := mgr.RequestApproval(ctx, decisionKey, "echo 1", "test", sessionID, "hook", false, true)
	if err != nil {
		t.Fatalf("first RequestApproval: %v", err)
	}
	if d1 != "APPROVAL" {
		t.Errorf("first decision = %q, want APPROVAL", d1)
	}

	// Second consumption — session scope is reusable, should also succeed.
	d2, err := mgr.RequestApproval(ctx, decisionKey, "echo 2", "test", sessionID, "hook", false, true)
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

	// Pre-create a BLOCKED approval (simulating TUI deny).
	go func() {
		time.Sleep(300 * time.Millisecond)
		_ = mgr.CreateApproval(decisionKey, "BLOCKED", "once", sessionID)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	decision, err := mgr.RequestApproval(ctx, decisionKey, "terraform destroy", "IaC rule", sessionID, "hook", false, true)
	if err != nil {
		t.Fatalf("RequestApproval: %v", err)
	}
	if decision != "BLOCKED" {
		t.Errorf("decision = %q, want BLOCKED (denied via DB)", decision)
	}
}
