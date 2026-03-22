package db

import "fmt"

// ListApprovals returns recent approvals ordered by creation time (newest first).
// The hmac field is intentionally excluded — it is validation material with no
// monitoring purpose and should not be loaded into the UI.
func (d *DB) ListApprovals(limit int) ([]Approval, error) {
	rows, err := d.db.Query(`
		SELECT id, decision_key, decision, scope, session_id,
		       created_at, consumed_at, expires_at
		FROM approvals
		ORDER BY created_at DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("list approvals: %w", err)
	}
	defer rows.Close()

	var approvals []Approval
	for rows.Next() {
		var a Approval
		if err := rows.Scan(&a.ID, &a.DecisionKey, &a.Decision, &a.Scope,
			&a.SessionID, &a.CreatedAt, &a.ConsumedAt, &a.ExpiresAt); err != nil {
			return nil, fmt.Errorf("scan approval: %w", err)
		}
		approvals = append(approvals, a)
	}
	return approvals, rows.Err()
}
