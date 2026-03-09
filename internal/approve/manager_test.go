package approve

import (
	"path/filepath"
	"testing"

	"github.com/runger/fuse/internal/core"
	"github.com/runger/fuse/internal/db"
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
	return NewManager(database, secret)
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
	tampered := NewManager(m.db, differentSecret)

	// Consuming with the wrong secret should fail HMAC verification.
	_, err = tampered.ConsumeApproval(decisionKey, sessionID)
	if err == nil {
		t.Fatal("expected HMAC verification error, got nil")
	}
}

func TestNewManager(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	database, err := db.OpenDB(dbPath)
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	defer database.Close()

	secret := []byte("my-secret")
	mgr := NewManager(database, secret)
	if mgr == nil {
		t.Fatal("NewManager returned nil")
	}
	if mgr.db != database {
		t.Fatal("NewManager did not store database reference")
	}
}
