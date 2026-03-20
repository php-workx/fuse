package db

import (
	"testing"
	"time"
)

func TestInsertAndListPendingRequests(t *testing.T) {
	d := openTestDB(t)

	req := PendingRequest{
		ID:            "test-pending-1",
		DecisionKey:   "key-abc",
		Command:       "terraform destroy",
		Reason:        "IaC rule",
		Source:        "hook",
		SessionID:     "sess-1",
		Cwd:           "/tmp",
		WorkspaceRoot: "/workspace",
	}
	if err := d.InsertPendingRequest(req); err != nil {
		t.Fatalf("InsertPendingRequest: %v", err)
	}

	// Insert a second one.
	req2 := req
	req2.ID = "test-pending-2"
	req2.Command = "cdk destroy"
	req2.Source = "codex-shell"
	if err := d.InsertPendingRequest(req2); err != nil {
		t.Fatalf("InsertPendingRequest 2: %v", err)
	}

	requests, err := d.ListPendingRequests()
	if err != nil {
		t.Fatalf("ListPendingRequests: %v", err)
	}
	if len(requests) != 2 {
		t.Fatalf("expected 2 pending requests, got %d", len(requests))
	}

	// Ordered by created_at ASC (oldest first).
	if requests[0].ID != "test-pending-1" {
		t.Errorf("first request ID = %q, want test-pending-1", requests[0].ID)
	}
	if requests[1].Source != "codex-shell" {
		t.Errorf("second request source = %q, want codex-shell", requests[1].Source)
	}
}

func TestDeletePendingRequest(t *testing.T) {
	d := openTestDB(t)

	req := PendingRequest{
		ID:          "test-del-1",
		DecisionKey: "key-del",
		Command:     "rm -rf /",
		Source:      "hook",
	}
	if err := d.InsertPendingRequest(req); err != nil {
		t.Fatalf("insert: %v", err)
	}

	if err := d.DeletePendingRequest("test-del-1"); err != nil {
		t.Fatalf("delete: %v", err)
	}

	requests, err := d.ListPendingRequests()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(requests) != 0 {
		t.Errorf("expected 0 after delete, got %d", len(requests))
	}
}

func TestCleanupStalePendingRequests(t *testing.T) {
	d := openTestDB(t)

	// Insert with a manually-set old timestamp.
	_, err := d.db.Exec(`
		INSERT INTO pending_requests (id, decision_key, command, source, created_at)
		VALUES ('stale-1', 'key-s', 'cmd', 'hook', datetime('now', '-60 seconds'))
	`)
	if err != nil {
		t.Fatalf("insert stale: %v", err)
	}

	// Insert a fresh one.
	fresh := PendingRequest{ID: "fresh-1", DecisionKey: "key-f", Command: "cmd", Source: "hook"}
	if err := d.InsertPendingRequest(fresh); err != nil {
		t.Fatalf("insert fresh: %v", err)
	}

	deleted, err := d.CleanupStalePendingRequests(30 * time.Second)
	if err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	if deleted != 1 {
		t.Errorf("deleted = %d, want 1", deleted)
	}

	remaining, _ := d.ListPendingRequests()
	if len(remaining) != 1 {
		t.Errorf("remaining = %d, want 1", len(remaining))
	}
	if remaining[0].ID != "fresh-1" {
		t.Errorf("remaining ID = %q, want fresh-1", remaining[0].ID)
	}
}

func TestDeleteApproval(t *testing.T) {
	d := openTestDB(t)

	// Insert an approval.
	if err := d.CreateApproval("del-1", "key-d", "APPROVAL", "once", "sess", "hmac-test", nil); err != nil {
		t.Fatalf("create: %v", err)
	}

	if err := d.DeleteApproval("del-1"); err != nil {
		t.Fatalf("delete: %v", err)
	}

	// Verify gone.
	approvals, err := d.ListApprovals(10)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	for _, a := range approvals {
		if a.ID == "del-1" {
			t.Error("approval del-1 still exists after delete")
		}
	}

	// Delete non-existent should error.
	if err := d.DeleteApproval("nonexistent"); err == nil {
		t.Error("expected error for non-existent approval")
	}
}
