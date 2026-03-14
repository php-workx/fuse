package db

import (
	"fmt"
	"strings"
)

// EventFilter defines filters for querying events.
type EventFilter struct {
	Source   string
	Agent    string
	Decision string
	Session  string
	Limit    int
}

// EventRecord represents a single event row.
type EventRecord struct {
	ID        int64
	Timestamp string
	SessionID string
	Command   string
	Decision  string
	RuleID    string
	Reason    string
	Duration  int64
	Metadata  string
	Source    string
	Agent     string
}

// EventSummary holds aggregated event statistics.
type EventSummary struct {
	Decision string
	Source   string
	Count    int64
}

// ListEvents returns events matching the filter, ordered by most recent first.
func (d *DB) ListEvents(f EventFilter) ([]EventRecord, error) {
	var clauses []string
	var args []interface{}

	if f.Source != "" {
		clauses = append(clauses, "source = ?")
		args = append(args, f.Source)
	}
	if f.Agent != "" {
		clauses = append(clauses, "agent = ?")
		args = append(args, f.Agent)
	}
	if f.Decision != "" {
		clauses = append(clauses, "decision = ?")
		args = append(args, f.Decision)
	}
	if f.Session != "" {
		clauses = append(clauses, "session_id = ?")
		args = append(args, f.Session)
	}

	var qb strings.Builder
	qb.WriteString("SELECT id, timestamp, session_id, command, decision, rule_id, reason, duration_ms, metadata, source, agent FROM events")
	if len(clauses) > 0 {
		qb.WriteString(" WHERE ")
		qb.WriteString(strings.Join(clauses, " AND "))
	}
	qb.WriteString(" ORDER BY id DESC")
	if f.Limit > 0 {
		qb.WriteString(" LIMIT ?")
		args = append(args, f.Limit)
	}

	rows, err := d.db.Query(qb.String(), args...)
	if err != nil {
		return nil, fmt.Errorf("list events: %w", err)
	}
	defer rows.Close()

	var events []EventRecord
	for rows.Next() {
		var e EventRecord
		var sessionID, command, ruleID, reason, metadata, source, agent *string
		if err := rows.Scan(&e.ID, &e.Timestamp, &sessionID, &command, &e.Decision, &ruleID, &reason, &e.Duration, &metadata, &source, &agent); err != nil {
			return nil, fmt.Errorf("scan event: %w", err)
		}
		if sessionID != nil {
			e.SessionID = *sessionID
		}
		if command != nil {
			e.Command = *command
		}
		if ruleID != nil {
			e.RuleID = *ruleID
		}
		if reason != nil {
			e.Reason = *reason
		}
		if metadata != nil {
			e.Metadata = *metadata
		}
		if source != nil {
			e.Source = *source
		}
		if agent != nil {
			e.Agent = *agent
		}
		events = append(events, e)
	}
	return events, rows.Err()
}

// SummarizeEvents returns aggregated counts grouped by decision and source,
// computed over the entire retained event table (no row limit applied).
func (d *DB) SummarizeEvents(f EventFilter) ([]EventSummary, int64, error) {
	var clauses []string
	var args []interface{}

	if f.Source != "" {
		clauses = append(clauses, "source = ?")
		args = append(args, f.Source)
	}
	if f.Agent != "" {
		clauses = append(clauses, "agent = ?")
		args = append(args, f.Agent)
	}
	if f.Decision != "" {
		clauses = append(clauses, "decision = ?")
		args = append(args, f.Decision)
	}
	if f.Session != "" {
		clauses = append(clauses, "session_id = ?")
		args = append(args, f.Session)
	}

	where := ""
	if len(clauses) > 0 {
		where = " WHERE " + strings.Join(clauses, " AND ")
	}

	// Get total count.
	var total int64
	countQuery := "SELECT COUNT(*) FROM events" + where
	if err := d.db.QueryRow(countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count events: %w", err)
	}

	// Get grouped summary.
	summaryQuery := "SELECT decision, COALESCE(source, 'shell'), COUNT(*) FROM events" +
		where + " GROUP BY decision, COALESCE(source, 'shell') ORDER BY COUNT(*) DESC"
	rows, err := d.db.Query(summaryQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("summarize events: %w", err)
	}
	defer rows.Close()

	var summaries []EventSummary
	for rows.Next() {
		var s EventSummary
		if err := rows.Scan(&s.Decision, &s.Source, &s.Count); err != nil {
			return nil, 0, fmt.Errorf("scan summary: %w", err)
		}
		summaries = append(summaries, s)
	}
	return summaries, total, rows.Err()
}
