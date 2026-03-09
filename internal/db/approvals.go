package db

import (
	"database/sql"
	"fmt"
	"time"
)

// Approval represents a stored approval record.
type Approval struct {
	ID          string
	DecisionKey string
	Decision    string
	Scope       string
	SessionID   string
	HMAC        string
	CreatedAt   string
	ConsumedAt  *string
	ExpiresAt   *string
}

// CreateApproval inserts a new approval record.
func (d *DB) CreateApproval(id, decisionKey, decision, scope, sessionID, hmac string, expiresAt *time.Time) error {
	var expiresAtStr *string
	if expiresAt != nil {
		s := expiresAt.UTC().Format("2006-01-02T15:04:05.000Z")
		expiresAtStr = &s
	}

	_, err := d.db.Exec(`
		INSERT INTO approvals (id, decision_key, decision, scope, session_id, hmac, expires_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, id, decisionKey, decision, scope, sessionID, hmac, expiresAtStr)
	if err != nil {
		return fmt.Errorf("create approval: %w", err)
	}
	return nil
}

// ConsumeApproval atomically finds and (depending on scope) consumes an
// approval matching decisionKey. Returns the approval if found and valid,
// or nil if no matching approval exists.
//
// Scope behaviour:
//   - "once"    — consumed (consumed_at set), single use
//   - "command" — NOT consumed, reusable for same command pattern
//   - "session" — must match sessionID, NOT consumed
//   - "forever" — never consumed, always valid
func (d *DB) ConsumeApproval(decisionKey, sessionID string) (*Approval, error) {
	now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")

	// First try to find a matching unconsumed, non-expired approval.
	// Order by scope priority: forever > session > command > once.
	row := d.db.QueryRow(`
		SELECT id, decision_key, decision, scope, session_id, hmac, created_at, consumed_at, expires_at
		FROM approvals
		WHERE decision_key = ?
		  AND consumed_at IS NULL
		  AND (expires_at IS NULL OR expires_at > ?)
		ORDER BY
			CASE scope
				WHEN 'forever' THEN 0
				WHEN 'session' THEN 1
				WHEN 'command' THEN 2
				WHEN 'once' THEN 3
			END
		LIMIT 1
	`, decisionKey, now)

	var a Approval
	err := row.Scan(&a.ID, &a.DecisionKey, &a.Decision, &a.Scope,
		&a.SessionID, &a.HMAC, &a.CreatedAt, &a.ConsumedAt, &a.ExpiresAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("query approval: %w", err)
	}

	// Check session scope: must match session ID.
	if a.Scope == "session" && a.SessionID != sessionID {
		return nil, nil
	}

	// Consume "once" scope approvals atomically.
	if a.Scope == "once" {
		result, err := d.db.Exec(`
			UPDATE approvals
			SET consumed_at = ?
			WHERE id = ?
			  AND consumed_at IS NULL
		`, now, a.ID)
		if err != nil {
			return nil, fmt.Errorf("consume approval: %w", err)
		}
		rows, _ := result.RowsAffected()
		if rows == 0 {
			// Another goroutine consumed it first.
			return nil, nil
		}
		a.ConsumedAt = &now
	}

	// "command", "session", and "forever" scopes are NOT consumed.
	return &a, nil
}

// CleanupExpired deletes consumed approvals and expired approvals.
// Returns the number of rows deleted.
func (d *DB) CleanupExpired() (int64, error) {
	now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")

	result, err := d.db.Exec(`
		DELETE FROM approvals
		WHERE consumed_at IS NOT NULL
		   OR (expires_at IS NOT NULL AND expires_at <= ?)
	`, now)
	if err != nil {
		return 0, fmt.Errorf("cleanup expired: %w", err)
	}
	return result.RowsAffected()
}
