package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// EventRecord represents one persisted fuse event.
type EventRecord struct {
	ID                int64  `json:"id"`
	Timestamp         string `json:"timestamp"`
	SessionID         string `json:"session_id,omitempty"`
	Command           string `json:"command,omitempty"`
	Decision          string `json:"decision,omitempty"`
	RuleID            string `json:"rule_id,omitempty"`
	Reason            string `json:"reason,omitempty"`
	DurationMs        int64  `json:"duration_ms,omitempty"`
	Metadata          string `json:"metadata,omitempty"`
	Source            string `json:"source,omitempty"`
	Agent             string `json:"agent,omitempty"`
	Cwd               string `json:"cwd,omitempty"`
	WorkspaceRoot     string `json:"workspace_root,omitempty"`
	FileInspected     bool   `json:"file_inspected,omitempty"`
	ApprovalID        string `json:"approval_id,omitempty"`
	UserResponse      string `json:"user_response,omitempty"`
	ExecutionExitCode *int64 `json:"execution_exit_code,omitempty"`
}

// EventFilter limits ListEvents results.
type EventFilter struct {
	Limit         int
	Source        string
	Agent         string
	Decision      string
	WorkspaceRoot string
}

// EventSummary aggregates local usage for debugging and dogfooding.
type EventSummary struct {
	Total       int            `json:"total"`
	ByDecision  map[string]int `json:"by_decision"`
	ByAgent     map[string]int `json:"by_agent"`
	BySource    map[string]int `json:"by_source"`
	ByWorkspace map[string]int `json:"by_workspace"`
}

// credentialPatterns defines patterns to scrub from command strings before storage.
var credentialPatterns = []struct {
	re          *regexp.Regexp
	replacement string
}{
	{
		re:          regexp.MustCompile(`(?i)(api[_-]?key|token|secret|password|credential)[=:]\s*\S+`),
		replacement: "${1}=[REDACTED]",
	},
	{
		re:          regexp.MustCompile(`AKIA[0-9A-Z]{16}`),
		replacement: "[REDACTED]",
	},
	{
		re:          regexp.MustCompile(`(?i)Bearer\s+\S+`),
		replacement: "Bearer [REDACTED]",
	},
	{
		re:          regexp.MustCompile(`(?i)(-p\s+|--password[= ]\s*)\S+`),
		replacement: "${1}[REDACTED]",
	},
	{
		re:          regexp.MustCompile(`(?i)Authorization:\s*\S+`),
		replacement: "Authorization: [REDACTED]",
	},
}

// ScrubCredentials removes potential credentials from a command string.
func ScrubCredentials(command string) string {
	for _, p := range credentialPatterns {
		command = p.re.ReplaceAllString(command, p.replacement)
	}
	return command
}

// LogEvent inserts an event record with credential scrubbing and normalized path metadata.
func (d *DB) LogEvent(record EventRecord) error {
	record.Command = ScrubCredentials(record.Command)
	record.Cwd = normalizeEventPath(record.Cwd)
	if record.WorkspaceRoot == "" {
		record.WorkspaceRoot = detectWorkspaceRoot(record.Cwd)
	} else {
		record.WorkspaceRoot = normalizeEventPath(record.WorkspaceRoot)
	}

	var executionExitCode any
	if record.ExecutionExitCode != nil {
		executionExitCode = *record.ExecutionExitCode
	}

	_, err := d.db.Exec(`
		INSERT INTO events (
			session_id, command, decision, rule_id, reason, duration_ms, metadata,
			source, agent, cwd, workspace_root, file_inspected, approval_id, user_response, execution_exit_code
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		record.SessionID,
		record.Command,
		record.Decision,
		record.RuleID,
		record.Reason,
		record.DurationMs,
		record.Metadata,
		record.Source,
		record.Agent,
		record.Cwd,
		record.WorkspaceRoot,
		boolToInt(record.FileInspected),
		record.ApprovalID,
		record.UserResponse,
		executionExitCode,
	)
	if err != nil {
		return fmt.Errorf("log event: %w", err)
	}
	return nil
}

// ListEvents returns recent events ordered newest-first.
func (d *DB) ListEvents(filter EventFilter) ([]EventRecord, error) {
	limit := filter.Limit
	if limit <= 0 {
		limit = 20
	}

	var clauses []string
	var args []any
	if filter.Source != "" {
		clauses = append(clauses, "source = ?")
		args = append(args, filter.Source)
	}
	if filter.Agent != "" {
		clauses = append(clauses, "agent = ?")
		args = append(args, filter.Agent)
	}
	if filter.Decision != "" {
		clauses = append(clauses, "decision = ?")
		args = append(args, filter.Decision)
	}
	if filter.WorkspaceRoot != "" {
		clauses = append(clauses, "workspace_root = ?")
		args = append(args, normalizeEventPath(filter.WorkspaceRoot))
	}

	query := `
		SELECT id, timestamp, session_id, command, decision, rule_id, reason, duration_ms, metadata,
		       source, agent, cwd, workspace_root, file_inspected, approval_id, user_response, execution_exit_code
		FROM events
	`
	if len(clauses) > 0 {
		query += " WHERE " + strings.Join(clauses, " AND ")
	}
	query += " ORDER BY id DESC LIMIT ?"
	args = append(args, limit)

	rows, err := d.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list events: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var events []EventRecord
	for rows.Next() {
		var event EventRecord
		var fileInspected int
		var executionExitCode sql.NullInt64
		if err := rows.Scan(
			&event.ID,
			&event.Timestamp,
			&event.SessionID,
			&event.Command,
			&event.Decision,
			&event.RuleID,
			&event.Reason,
			&event.DurationMs,
			&event.Metadata,
			&event.Source,
			&event.Agent,
			&event.Cwd,
			&event.WorkspaceRoot,
			&fileInspected,
			&event.ApprovalID,
			&event.UserResponse,
			&executionExitCode,
		); err != nil {
			return nil, fmt.Errorf("scan event: %w", err)
		}
		event.FileInspected = fileInspected != 0
		if executionExitCode.Valid {
			code := executionExitCode.Int64
			event.ExecutionExitCode = &code
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate events: %w", err)
	}
	return events, nil
}

// SummarizeEvents aggregates counts across the full local event table.
func (d *DB) SummarizeEvents() (EventSummary, error) {
	events, err := d.ListEvents(EventFilter{Limit: 10000})
	if err != nil {
		return EventSummary{}, err
	}

	summary := EventSummary{
		Total:       len(events),
		ByDecision:  map[string]int{},
		ByAgent:     map[string]int{},
		BySource:    map[string]int{},
		ByWorkspace: map[string]int{},
	}
	for _, event := range events {
		incrementCount(summary.ByDecision, event.Decision)
		incrementCount(summary.ByAgent, event.Agent)
		incrementCount(summary.BySource, event.Source)
		incrementCount(summary.ByWorkspace, event.WorkspaceRoot)
	}
	return summary, nil
}

// PruneEvents keeps the most recent maxRows events, deleting the oldest.
// Returns the number of rows deleted.
func (d *DB) PruneEvents(maxRows int) (int64, error) {
	result, err := d.db.Exec(`
		DELETE FROM events
		WHERE id NOT IN (
			SELECT id FROM events ORDER BY id DESC LIMIT ?
		)
	`, maxRows)
	if err != nil {
		return 0, fmt.Errorf("prune events: %w", err)
	}
	return result.RowsAffected()
}

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func normalizeEventPath(path string) string {
	if path == "" {
		return ""
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err == nil {
		return resolved
	}
	return abs
}

func detectWorkspaceRoot(cwd string) string {
	cwd = normalizeEventPath(cwd)
	if cwd == "" {
		return ""
	}

	info, err := os.Stat(cwd)
	if err != nil {
		return cwd
	}
	if !info.IsDir() {
		cwd = filepath.Dir(cwd)
	}

	dir := cwd
	for {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return cwd
		}
		dir = parent
	}
}

func incrementCount(m map[string]int, key string) {
	if key == "" {
		key = "(unknown)"
	}
	m[key]++
}

// SortedCounts returns map entries sorted by count descending, then key ascending.
func SortedCounts(m map[string]int) []struct {
	Key   string
	Count int
} {
	pairs := make([]struct {
		Key   string
		Count int
	}, 0, len(m))
	for key, count := range m {
		pairs = append(pairs, struct {
			Key   string
			Count int
		}{Key: key, Count: count})
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].Count == pairs[j].Count {
			return pairs[i].Key < pairs[j].Key
		}
		return pairs[i].Count > pairs[j].Count
	})
	return pairs
}
