package approve

import (
	"path/filepath"
	"testing"

	"github.com/php-workx/fuse/internal/core"
	"github.com/php-workx/fuse/internal/db"
)

func setupTestManager(t *testing.T) *Manager {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	database, err := db.OpenDB(dbPath)
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	secret := []byte("test-secret-key-32-bytes-long!!!")
	mgr, err := NewManager(database, secret)
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}
	return mgr
}

func TestCreateAndConsume(t *testing.T) {
	m := setupTestManager(t)

	decisionKey := "test-decision-key-hash"
	decision := string(core.DecisionApproval)
	scope := "once"
	sessionID := "session-123"

	// Create an approval.
	err := m.CreateApproval(decisionKey, decision, scope, sessionID)
	if err != nil {
		t.Fatalf("CreateApproval failed: %v", err)
	}

	// Consume the approval.
	got, err := m.ConsumeApproval(decisionKey, sessionID)
	if err != nil {
		t.Fatalf("ConsumeApproval failed: %v", err)
	}
	if got != core.DecisionApproval {
		t.Fatalf("expected DecisionApproval, got %q", got)
	}

	// "once" scope: consuming again should return empty (already consumed).
	got2, err := m.ConsumeApproval(decisionKey, sessionID)
	if err != nil {
		t.Fatalf("second ConsumeApproval failed: %v", err)
	}
	if got2 != "" {
		t.Fatalf("expected empty decision after once-consumed, got %q", got2)
	}
}

func TestCreateAndConsume_CommandScope(t *testing.T) {
	m := setupTestManager(t)

	decisionKey := "cmd-decision-key"
	decision := string(core.DecisionApproval)
	scope := "command"
	sessionID := "session-456"

	err := m.CreateApproval(decisionKey, decision, scope, sessionID)
	if err != nil {
		t.Fatalf("CreateApproval failed: %v", err)
	}

	// Command scope: should be reusable.
	for i := 0; i < 3; i++ {
		got, err := m.ConsumeApproval(decisionKey, sessionID)
		if err != nil {
			t.Fatalf("ConsumeApproval attempt %d failed: %v", i+1, err)
		}
		if got != core.DecisionApproval {
			t.Fatalf("attempt %d: expected DecisionApproval, got %q", i+1, got)
		}
	}
}

func TestConsume_NoApproval(t *testing.T) {
	m := setupTestManager(t)

	got, err := m.ConsumeApproval("nonexistent-key", "session-789")
	if err != nil {
		t.Fatalf("ConsumeApproval failed: %v", err)
	}
	if got != "" {
		t.Fatalf("expected empty decision for nonexistent approval, got %q", got)
	}
}

func TestConsume_InvalidHMAC(t *testing.T) {
	m := setupTestManager(t)

	decisionKey := "tampered-key"
	sessionID := "session-abc"

	// Create an approval with the manager (valid HMAC).
	err := m.CreateApproval(decisionKey, string(core.DecisionApproval), "command", sessionID)
	if err != nil {
		t.Fatalf("CreateApproval failed: %v", err)
	}

	// Now create a new manager with a different secret to simulate tampering.
	differentSecret := []byte("different-secret-key-32-bytes!!!")
	tampered, tampErr := NewManager(m.db, differentSecret)
	if tampErr != nil {
		t.Fatalf("NewManager with different secret failed: %v", tampErr)
	}

	// Consuming with the wrong secret should fail HMAC verification.
	_, err = tampered.ConsumeApproval(decisionKey, sessionID)
	if err == nil {
		t.Fatal("expected HMAC verification error, got nil")
	}
}

// fus-vu5r: a single decision_key is shared by every adapter (tui, hook,
// run, codex-shell, mcp-proxy, claude-file). These tests exercise the
// approval-cache fast path under each scope to confirm cross-source reuse.

func TestCrossSource_OnceScope_HookConsumesTUIApproval(t *testing.T) {
	m := setupTestManager(t)

	// TUI grants once-scope approval. Hook then consumes it.
	key := core.ComputeDecisionKey("git add docs/spec.md", "")
	if err := m.CreateApproval(key, string(core.DecisionApproval), "once", "sess-1"); err != nil {
		t.Fatalf("CreateApproval: %v", err)
	}

	// Hook adapter consumes — same key derived independently.
	got, err := m.ConsumeApproval(key, "sess-1")
	if err != nil {
		t.Fatalf("ConsumeApproval: %v", err)
	}
	if got != core.DecisionApproval {
		t.Fatalf("hook did not see TUI approval: got %q", got)
	}

	// Once-scope is consumed; second call returns empty.
	got2, _ := m.ConsumeApproval(key, "sess-1")
	if got2 != "" {
		t.Fatalf("once-scope should consume after first use, got %q", got2)
	}
}

func TestCrossSource_CommandScope_ReusableAcrossAdapters(t *testing.T) {
	m := setupTestManager(t)

	key := core.ComputeDecisionKey("kubectl get pods", "")
	if err := m.CreateApproval(key, string(core.DecisionApproval), "command", "sess-2"); err != nil {
		t.Fatalf("CreateApproval: %v", err)
	}

	// Multiple adapter calls (hook, run, codex-shell) all hit the same cache.
	for i := 0; i < 3; i++ {
		got, err := m.ConsumeApproval(key, "sess-2")
		if err != nil {
			t.Fatalf("attempt %d: %v", i+1, err)
		}
		if got != core.DecisionApproval {
			t.Fatalf("attempt %d: command scope should be reusable, got %q", i+1, got)
		}
	}
}

func TestCrossSource_SessionScope_RequiresMatchingSession(t *testing.T) {
	m := setupTestManager(t)

	key := core.ComputeDecisionKey("aws s3 ls", "")
	if err := m.CreateApproval(key, string(core.DecisionApproval), "session", "sess-A"); err != nil {
		t.Fatalf("CreateApproval: %v", err)
	}

	// Same session, any source — satisfied.
	got, err := m.ConsumeApproval(key, "sess-A")
	if err != nil {
		t.Fatalf("ConsumeApproval same session: %v", err)
	}
	if got != core.DecisionApproval {
		t.Fatalf("same session should match: got %q", got)
	}

	// Different session — must NOT match (scope='session' filter).
	got2, err := m.ConsumeApproval(key, "sess-B")
	if err != nil {
		t.Fatalf("ConsumeApproval other session: %v", err)
	}
	if got2 != "" {
		t.Fatalf("session scope leaked across sessions: got %q", got2)
	}
}

func TestCrossSource_ForeverScope_SatisfiesIndefinitely(t *testing.T) {
	m := setupTestManager(t)

	key := core.ComputeDecisionKey("rm -rf node_modules", "")
	if err := m.CreateApproval(key, string(core.DecisionApproval), "forever", "sess-3"); err != nil {
		t.Fatalf("CreateApproval: %v", err)
	}

	for i := 0; i < 5; i++ {
		got, err := m.ConsumeApproval(key, "sess-3")
		if err != nil {
			t.Fatalf("attempt %d: %v", i+1, err)
		}
		if got != core.DecisionApproval {
			t.Fatalf("attempt %d: forever scope should always satisfy, got %q", i+1, got)
		}
	}
}

func TestNewManager(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	database, err := db.OpenDB(dbPath)
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	defer database.Close()

	// Valid 32-byte secret should succeed.
	secret := []byte("valid-secret-key-32-bytes-long!!")
	mgr, err := NewManager(database, secret)
	if err != nil {
		t.Fatalf("NewManager failed with valid secret: %v", err)
	}
	if mgr == nil {
		t.Fatal("NewManager returned nil")
	}
	if mgr.db != database {
		t.Fatal("NewManager did not store database reference")
	}

	// Invalid length secret should fail.
	_, err = NewManager(database, []byte("too-short"))
	if err == nil {
		t.Fatal("expected error for short secret, got nil")
	}
}
