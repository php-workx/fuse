package db

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestOpenDB_WALMode(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	d, err := OpenDB(dbPath)
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	defer d.Close()

	var mode string
	err = d.db.QueryRow("PRAGMA journal_mode").Scan(&mode)
	if err != nil {
		t.Fatalf("query journal_mode: %v", err)
	}
	if mode != "wal" {
		t.Errorf("journal_mode = %q, want %q", mode, "wal")
	}
}

func TestDBPermissions(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	d, err := OpenDB(dbPath)
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	defer d.Close()

	info, err := os.Stat(dbPath)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}

	perm := info.Mode().Perm()
	if perm != 0o600 {
		t.Errorf("db file permissions = %o, want %o", perm, 0o600)
	}
}

func TestCreateAndConsumeApproval(t *testing.T) {
	d := openTestDB(t)

	expires := time.Now().Add(1 * time.Hour)
	err := d.CreateApproval("a1", "key1", "APPROVAL", "once", "sess1", "hmac1", &expires)
	if err != nil {
		t.Fatalf("CreateApproval: %v", err)
	}

	// Consume the approval.
	a, err := d.ConsumeApproval("key1", "sess1")
	if err != nil {
		t.Fatalf("ConsumeApproval: %v", err)
	}
	if a == nil {
		t.Fatal("ConsumeApproval returned nil, want approval")
	}
	if a.ID != "a1" {
		t.Errorf("ID = %q, want %q", a.ID, "a1")
	}
	if a.Decision != "APPROVAL" {
		t.Errorf("Decision = %q, want %q", a.Decision, "APPROVAL")
	}
	if a.ConsumedAt == nil {
		t.Error("ConsumedAt is nil after consumption")
	}

	// Try to consume again — should return nil (already consumed).
	a2, err := d.ConsumeApproval("key1", "sess1")
	if err != nil {
		t.Fatalf("ConsumeApproval second call: %v", err)
	}
	if a2 != nil {
		t.Error("ConsumeApproval returned non-nil on second consumption")
	}
}

func TestConsumeApproval_Expired(t *testing.T) {
	d := openTestDB(t)

	// Create an approval that already expired.
	past := time.Now().Add(-1 * time.Hour)
	err := d.CreateApproval("a2", "key2", "APPROVAL", "once", "sess1", "hmac2", &past)
	if err != nil {
		t.Fatalf("CreateApproval: %v", err)
	}

	a, err := d.ConsumeApproval("key2", "sess1")
	if err != nil {
		t.Fatalf("ConsumeApproval: %v", err)
	}
	if a != nil {
		t.Error("ConsumeApproval returned non-nil for expired approval")
	}
}

func TestConsumeApproval_Scope(t *testing.T) {
	d := openTestDB(t)
	expires := time.Now().Add(1 * time.Hour)

	t.Run("once", func(t *testing.T) {
		err := d.CreateApproval("once1", "k-once", "APPROVAL", "once", "", "h1", &expires)
		if err != nil {
			t.Fatalf("CreateApproval: %v", err)
		}

		a, err := d.ConsumeApproval("k-once", "")
		if err != nil {
			t.Fatalf("ConsumeApproval: %v", err)
		}
		if a == nil {
			t.Fatal("expected approval, got nil")
		}
		if a.ConsumedAt == nil {
			t.Error("once-scope should be consumed")
		}

		// Second consume should fail.
		a2, err := d.ConsumeApproval("k-once", "")
		if err != nil {
			t.Fatalf("ConsumeApproval: %v", err)
		}
		if a2 != nil {
			t.Error("once-scope should not be consumable twice")
		}
	})

	t.Run("command", func(t *testing.T) {
		err := d.CreateApproval("cmd1", "k-cmd", "APPROVAL", "command", "", "h2", &expires)
		if err != nil {
			t.Fatalf("CreateApproval: %v", err)
		}

		// Should be reusable.
		for i := 0; i < 3; i++ {
			a, err := d.ConsumeApproval("k-cmd", "")
			if err != nil {
				t.Fatalf("ConsumeApproval iter %d: %v", i, err)
			}
			if a == nil {
				t.Fatalf("iter %d: expected approval, got nil", i)
			}
			if a.ConsumedAt != nil {
				t.Errorf("iter %d: command-scope should not be consumed", i)
			}
		}
	})

	t.Run("session", func(t *testing.T) {
		err := d.CreateApproval("sess1", "k-sess", "APPROVAL", "session", "my-session", "h3", &expires)
		if err != nil {
			t.Fatalf("CreateApproval: %v", err)
		}

		// Should match with correct session ID.
		a, err := d.ConsumeApproval("k-sess", "my-session")
		if err != nil {
			t.Fatalf("ConsumeApproval: %v", err)
		}
		if a == nil {
			t.Fatal("expected approval for matching session")
		}

		// Should NOT match with different session ID.
		a2, err := d.ConsumeApproval("k-sess", "other-session")
		if err != nil {
			t.Fatalf("ConsumeApproval: %v", err)
		}
		if a2 != nil {
			t.Error("session-scope should not match different session ID")
		}
	})

	t.Run("forever", func(t *testing.T) {
		err := d.CreateApproval("f1", "k-forever", "APPROVAL", "forever", "", "h4", nil)
		if err != nil {
			t.Fatalf("CreateApproval: %v", err)
		}

		// Should be reusable forever.
		for i := 0; i < 3; i++ {
			a, err := d.ConsumeApproval("k-forever", "")
			if err != nil {
				t.Fatalf("ConsumeApproval iter %d: %v", i, err)
			}
			if a == nil {
				t.Fatalf("iter %d: expected approval, got nil", i)
			}
			if a.ConsumedAt != nil {
				t.Errorf("iter %d: forever-scope should not be consumed", i)
			}
		}
	})
}

func TestLogEvent(t *testing.T) {
	d := openTestDB(t)

	err := d.LogEvent(&EventRecord{
		SessionID:  "sess1",
		Command:    "echo hello",
		Decision:   "SAFE",
		RuleID:     "rule1",
		Reason:     "builtin allow",
		DurationMs: 5,
		Metadata:   `{"foo":"bar"}`,
	})
	if err != nil {
		t.Fatalf("LogEvent: %v", err)
	}

	var count int
	err = d.db.QueryRow("SELECT COUNT(*) FROM events").Scan(&count)
	if err != nil {
		t.Fatalf("count events: %v", err)
	}
	if count != 1 {
		t.Errorf("event count = %d, want 1", count)
	}

	// Verify stored values.
	var sessionID, command, decision, ruleID, reason string
	var durationMs int64
	err = d.db.QueryRow(`
		SELECT session_id, command, decision, rule_id, reason, duration_ms
		FROM events WHERE id = 1
	`).Scan(&sessionID, &command, &decision, &ruleID, &reason, &durationMs)
	if err != nil {
		t.Fatalf("query event: %v", err)
	}
	if command != "echo hello" {
		t.Errorf("command = %q, want %q", command, "echo hello")
	}
	if decision != "SAFE" {
		t.Errorf("decision = %q, want %q", decision, "SAFE")
	}
}

func TestPruneEvents(t *testing.T) {
	d := openTestDB(t)

	// Insert 10 events.
	for i := 0; i < 10; i++ {
		err := d.LogEvent(&EventRecord{
			SessionID: "s",
			Command:   "cmd",
			Decision:  "SAFE",
		})
		if err != nil {
			t.Fatalf("LogEvent %d: %v", i, err)
		}
	}

	// Keep only the most recent 3.
	deleted, err := d.PruneEvents(3)
	if err != nil {
		t.Fatalf("PruneEvents: %v", err)
	}
	if deleted != 7 {
		t.Errorf("deleted = %d, want 7", deleted)
	}

	var count int
	err = d.db.QueryRow("SELECT COUNT(*) FROM events").Scan(&count)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 3 {
		t.Errorf("remaining = %d, want 3", count)
	}

	// Verify the remaining events are the most recent (highest IDs).
	var minID int
	err = d.db.QueryRow("SELECT MIN(id) FROM events").Scan(&minID)
	if err != nil {
		t.Fatalf("min id: %v", err)
	}
	if minID <= 7 {
		t.Errorf("min remaining id = %d, expected > 7", minID)
	}
}

func TestCredentialScrubbing(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "API key",
			input: "curl -H api_key=sk-1234abcd https://api.example.com",
			want:  "curl -H api_key=[REDACTED] https://api.example.com",
		},
		{
			name:  "token",
			input: "export TOKEN=mysecrettoken123",
			want:  "export TOKEN=[REDACTED]",
		},
		{
			name:  "password",
			input: "mysql -u root password=hunter2",
			want:  "mysql -u root password=[REDACTED]",
		},
		{
			name:  "AWS access key",
			input: "aws configure set aws_access_key_id AKIAIOSFODNN7EXAMPLE",
			want:  "aws configure set aws_access_key_id [REDACTED]",
		},
		{
			name:  "Bearer token",
			input: "curl -H Authorization: Bearer eyJhbGciOiJIUzI1NiJ9 https://api.example.com",
			want:  "curl -H Authorization: [REDACTED] [REDACTED] https://api.example.com",
		},
		{
			name:  "no credentials",
			input: "ls -la /tmp",
			want:  "ls -la /tmp",
		},
		{
			name:  "secret",
			input: "export secret=mysecret",
			want:  "export secret=[REDACTED]",
		},
		{
			name:  "credential with colon",
			input: "credential: superSecret123",
			want:  "credential=[REDACTED]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ScrubCredentials(tt.input)
			if got != tt.want {
				t.Errorf("ScrubCredentials(%q)\n  got  = %q\n  want = %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestEnsureSecret(t *testing.T) {
	dir := t.TempDir()
	secretPath := filepath.Join(dir, "secret.key")

	// First call should create the secret.
	secret1, err := EnsureSecret(secretPath)
	if err != nil {
		t.Fatalf("EnsureSecret (create): %v", err)
	}
	if len(secret1) != 32 {
		t.Errorf("secret length = %d, want 32", len(secret1))
	}

	// Verify file permissions.
	info, err := os.Stat(secretPath)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("secret permissions = %o, want %o", perm, 0o600)
	}

	// Second call should read the same secret.
	secret2, err := EnsureSecret(secretPath)
	if err != nil {
		t.Fatalf("EnsureSecret (read): %v", err)
	}
	if string(secret1) != string(secret2) {
		t.Error("second call returned different secret")
	}
}

func TestCleanupExpired(t *testing.T) {
	d := openTestDB(t)

	// Create an expired approval.
	past := time.Now().Add(-1 * time.Hour)
	err := d.CreateApproval("exp1", "k-exp", "APPROVAL", "once", "", "h1", &past)
	if err != nil {
		t.Fatalf("CreateApproval expired: %v", err)
	}

	// Create a consumed approval.
	future := time.Now().Add(1 * time.Hour)
	err = d.CreateApproval("cons1", "k-cons", "APPROVAL", "once", "", "h2", &future)
	if err != nil {
		t.Fatalf("CreateApproval consumed: %v", err)
	}
	_, err = d.ConsumeApproval("k-cons", "")
	if err != nil {
		t.Fatalf("ConsumeApproval: %v", err)
	}

	// Create a valid, unconsumed approval.
	err = d.CreateApproval("valid1", "k-valid", "APPROVAL", "once", "", "h3", &future)
	if err != nil {
		t.Fatalf("CreateApproval valid: %v", err)
	}

	// Cleanup should remove only the expired one; the recently consumed
	// approval is retained for 1 hour.
	deleted, err := d.CleanupExpired()
	if err != nil {
		t.Fatalf("CleanupExpired: %v", err)
	}
	if deleted != 1 {
		t.Errorf("deleted = %d, want 1", deleted)
	}

	// The valid one and the recently consumed one should still be there.
	var count int
	err = d.db.QueryRow("SELECT COUNT(*) FROM approvals").Scan(&count)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 2 {
		t.Errorf("remaining = %d, want 2", count)
	}
}

func TestCleanupExpired_RetainsRecentlyConsumed(t *testing.T) {
	d := openTestDB(t)

	// Create and consume an approval 30 minutes ago (within the 1-hour retention window).
	future := time.Now().Add(2 * time.Hour)
	err := d.CreateApproval("recent1", "k-recent", "APPROVAL", "once", "", "h1", &future)
	if err != nil {
		t.Fatalf("CreateApproval: %v", err)
	}
	_, err = d.ConsumeApproval("k-recent", "")
	if err != nil {
		t.Fatalf("ConsumeApproval: %v", err)
	}

	// Backdate consumed_at to 30 minutes ago (within 1-hour retention).
	thirtyMinAgo := time.Now().Add(-30 * time.Minute).UTC().Format("2006-01-02T15:04:05.000Z")
	_, err = d.db.Exec(`UPDATE approvals SET consumed_at = ? WHERE id = ?`, thirtyMinAgo, "recent1")
	if err != nil {
		t.Fatalf("backdate consumed_at: %v", err)
	}

	deleted, err := d.CleanupExpired()
	if err != nil {
		t.Fatalf("CleanupExpired: %v", err)
	}
	if deleted != 0 {
		t.Errorf("deleted = %d, want 0 (recently consumed should be retained)", deleted)
	}

	var count int
	err = d.db.QueryRow("SELECT COUNT(*) FROM approvals").Scan(&count)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Errorf("remaining = %d, want 1", count)
	}
}

func TestCleanupExpired_DeletesOldConsumed(t *testing.T) {
	d := openTestDB(t)

	// Create and consume an approval, then backdate consumed_at to 2 hours ago.
	future := time.Now().Add(2 * time.Hour)
	err := d.CreateApproval("old1", "k-old", "APPROVAL", "once", "", "h1", &future)
	if err != nil {
		t.Fatalf("CreateApproval: %v", err)
	}
	_, err = d.ConsumeApproval("k-old", "")
	if err != nil {
		t.Fatalf("ConsumeApproval: %v", err)
	}

	// Backdate consumed_at to 2 hours ago (beyond 1-hour retention).
	twoHoursAgo := time.Now().Add(-2 * time.Hour).UTC().Format("2006-01-02T15:04:05.000Z")
	_, err = d.db.Exec(`UPDATE approvals SET consumed_at = ? WHERE id = ?`, twoHoursAgo, "old1")
	if err != nil {
		t.Fatalf("backdate consumed_at: %v", err)
	}

	deleted, err := d.CleanupExpired()
	if err != nil {
		t.Fatalf("CleanupExpired: %v", err)
	}
	if deleted != 1 {
		t.Errorf("deleted = %d, want 1 (old consumed should be deleted)", deleted)
	}

	var count int
	err = d.db.QueryRow("SELECT COUNT(*) FROM approvals").Scan(&count)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 0 {
		t.Errorf("remaining = %d, want 0", count)
	}
}

// openTestDB creates a temporary database for testing.
func openTestDB(t *testing.T) *DB {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	d, err := OpenDB(dbPath)
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}
