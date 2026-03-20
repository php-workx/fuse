package db

import (
	"fmt"
	"time"
)

// PendingRequest represents a command awaiting approval from the TUI or TTY prompt.
// These are ephemeral — created when a hook needs approval, deleted when resolved or timed out.
type PendingRequest struct {
	ID            string
	DecisionKey   string
	Command       string
	Reason        string
	Source        string // "hook", "codex-shell", "run", "mcp-proxy"
	SessionID     string
	Cwd           string
	WorkspaceRoot string
	CreatedAt     string
}

// InsertPendingRequest writes a new pending approval request to the database.
func (d *DB) InsertPendingRequest(req PendingRequest) error {
	_, err := d.db.Exec(`
		INSERT INTO pending_requests (id, decision_key, command, reason, source, session_id, cwd, workspace_root)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, req.ID, req.DecisionKey, req.Command, req.Reason, req.Source, req.SessionID, req.Cwd, req.WorkspaceRoot)
	if err != nil {
		return fmt.Errorf("insert pending request: %w", err)
	}
	return nil
}

// ListPendingRequests returns all pending requests ordered by creation time (oldest first).
func (d *DB) ListPendingRequests() ([]PendingRequest, error) {
	rows, err := d.db.Query(`
		SELECT id, decision_key, command, reason, source, session_id, cwd, workspace_root, created_at
		FROM pending_requests
		ORDER BY created_at ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("list pending requests: %w", err)
	}
	defer rows.Close()

	var requests []PendingRequest
	for rows.Next() {
		var r PendingRequest
		if err := rows.Scan(&r.ID, &r.DecisionKey, &r.Command, &r.Reason, &r.Source,
			&r.SessionID, &r.Cwd, &r.WorkspaceRoot, &r.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan pending request: %w", err)
		}
		requests = append(requests, r)
	}
	return requests, rows.Err()
}

// DeletePendingRequest removes a pending request by ID.
func (d *DB) DeletePendingRequest(id string) error {
	_, err := d.db.Exec("DELETE FROM pending_requests WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete pending request: %w", err)
	}
	return nil
}

// CleanupStalePendingRequests removes requests older than maxAge.
// Returns the number of deleted rows.
func (d *DB) CleanupStalePendingRequests(maxAge time.Duration) (int64, error) {
	cutoff := time.Now().UTC().Add(-maxAge).Format("2006-01-02T15:04:05.000Z")
	result, err := d.db.Exec("DELETE FROM pending_requests WHERE created_at < ?", cutoff)
	if err != nil {
		return 0, fmt.Errorf("cleanup stale pending requests: %w", err)
	}
	return result.RowsAffected()
}
