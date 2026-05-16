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
	ID                 int64  `json:"id"`
	Timestamp          string `json:"timestamp"`
	SessionID          string `json:"session_id,omitempty"`
	Command            string `json:"command,omitempty"`
	Decision           string `json:"decision,omitempty"`
	StructuralDecision string `json:"structural_decision,omitempty"`
	Profile            string `json:"profile,omitempty"`
	RuleID             string `json:"rule_id,omitempty"`
	Reason             string `json:"reason,omitempty"`
	DurationMs         int64  `json:"duration_ms,omitempty"`
	Metadata           string `json:"metadata,omitempty"`
	Source             string `json:"source,omitempty"`
	Agent              string `json:"agent,omitempty"`
	Cwd                string `json:"cwd,omitempty"`
	WorkspaceRoot      string `json:"workspace_root,omitempty"`
	FileInspected      bool   `json:"file_inspected,omitempty"`
	ApprovalID         string `json:"approval_id,omitempty"`
	UserResponse       string `json:"user_response,omitempty"`
	ExecutionExitCode  *int64 `json:"execution_exit_code,omitempty"`

	// LLM judge fields (empty when judge is off or not triggered).
	JudgeDecision   string  `json:"judge_decision,omitempty"`
	JudgeConfidence float64 `json:"judge_confidence,omitempty"`
	JudgeReasoning  string  `json:"judge_reasoning,omitempty"`
	JudgeApplied    bool    `json:"judge_applied,omitempty"`
	JudgeProvider   string  `json:"judge_provider,omitempty"`
	JudgeLatencyMs  int64   `json:"judge_latency_ms,omitempty"`
	JudgeError      string  `json:"judge_error,omitempty"`
}

// EventFilter limits ListEvents results.
type EventFilter struct {
	Limit         int
	Source        string
	Agent         string
	Decision      string
	Session       string
	WorkspaceRoot string
}

// EventSummary aggregates local usage for debugging and dogfooding.
type EventSummary struct {
	Total         int            `json:"total"`
	ByDecision    map[string]int `json:"by_decision"`
	ByAgent       map[string]int `json:"by_agent"`
	BySource      map[string]int `json:"by_source"`
	BySourceAgent map[string]int `json:"by_source_agent"`
	ByWorkspace   map[string]int `json:"by_workspace"`
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
		// Authorization header — captures scheme + credential value.
		re:          regexp.MustCompile(`(?i)Authorization:\s*\S+\s+\S+`),
		replacement: "Authorization: [REDACTED]",
	},
	// SEC-010: Expanded patterns for inline body scrubbing.
	{
		// PEM-encoded keys/certificates
		re:          regexp.MustCompile(`-----BEGIN [A-Z ]+-----[\s\S]*?-----END [A-Z ]+-----`),
		replacement: "[REDACTED PEM BLOCK]",
	},
	{
		// GitHub fine-grained PATs (ghp_), OAuth tokens (gho_), etc.
		re:          regexp.MustCompile(`\b(?:AIza[A-Za-z0-9_-]{16,}|(?:ghp|gho|ghu|ghs|ghr|glpat|xoxb|xoxp|xoxa|xoxr|sk-|rk-|whsec_)[A-Za-z0-9_-]{16,})\b`),
		replacement: "[REDACTED VENDOR TOKEN]",
	},
	{
		// URL userinfo (user:pass@host)
		re:          regexp.MustCompile(`://[^:/?#]+:[^@/?#]+@`),
		replacement: "://[REDACTED]@",
	},
	{
		// JSON key-value secrets (quoted keys with secret-related names)
		re:          regexp.MustCompile(`(?i)"(password|secret|token|key|credential|auth|apikey|api_key|access_key|private_key)"\s*:\s*"[^"]*"`),
		replacement: `"${1}":"[REDACTED]"`,
	},
	// Authorization: Basic/Digest now covered by the generic Authorization pattern above.
	{
		// Cookie header values
		re:          regexp.MustCompile(`(?im)(Cookie|Set-Cookie):\s*.*$`),
		replacement: "${1}: [REDACTED]",
	},
	{
		// JWT tokens (three base64url-encoded segments)
		re:          regexp.MustCompile(`\beyJ[A-Za-z0-9_-]+\.eyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\b`),
		replacement: "[REDACTED JWT]",
	},
	{
		// AWS temporary credentials (STS) — extends existing AKIA pattern
		re:          regexp.MustCompile(`ASIA[0-9A-Z]{16}`),
		replacement: "[REDACTED]",
	},
	{
		// High-entropy base64 blobs (64+ chars, likely keys/secrets).
		// Raised from 40 to 64 to avoid redacting SHA-256 hashes, long paths,
		// and Go module names. Vendor-specific patterns above catch shorter secrets.
		re:          regexp.MustCompile(`\b[A-Za-z0-9+/]{64,}={0,3}\b`),
		replacement: "[REDACTED BASE64]",
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
func (d *DB) LogEvent(record *EventRecord) error {
	record.Command = ScrubCredentials(record.Command)
	record.Reason = ScrubCredentials(record.Reason)
	record.Metadata = ScrubCredentials(record.Metadata)
	record.JudgeReasoning = ScrubCredentials(record.JudgeReasoning)
	record.JudgeError = ScrubCredentials(record.JudgeError)
	record.Cwd = normalizeEventPath(record.Cwd)
	if record.StructuralDecision == "" {
		record.StructuralDecision = record.Decision
	}
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
			session_id, command, decision, structural_decision, profile, rule_id, reason, duration_ms, metadata,
			source, agent, cwd, workspace_root, file_inspected, approval_id, user_response, execution_exit_code,
			judge_decision, judge_confidence, judge_reasoning, judge_applied, judge_provider, judge_latency_ms, judge_error
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		record.SessionID,
		record.Command,
		record.Decision,
		record.StructuralDecision,
		record.Profile,
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
		record.JudgeDecision,
		record.JudgeConfidence,
		record.JudgeReasoning,
		boolToInt(record.JudgeApplied),
		record.JudgeProvider,
		record.JudgeLatencyMs,
		record.JudgeError,
	)
	if err != nil {
		return fmt.Errorf("log event: %w", err)
	}
	return nil
}

// ListEvents returns recent events ordered newest-first.
func (d *DB) ListEvents(filter *EventFilter) ([]EventRecord, error) {
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
	if filter.Session != "" {
		clauses = append(clauses, "session_id = ?")
		args = append(args, filter.Session)
	}
	if filter.WorkspaceRoot != "" {
		clauses = append(clauses, "workspace_root = ?")
		args = append(args, normalizeEventPath(filter.WorkspaceRoot))
	}

	var qb strings.Builder
	qb.WriteString(`SELECT id, timestamp, session_id, command, decision, structural_decision, profile, rule_id, reason, duration_ms, metadata,
		source, agent, cwd, workspace_root, file_inspected, approval_id, user_response, execution_exit_code,
		judge_decision, judge_confidence, judge_reasoning, judge_applied, judge_provider, judge_latency_ms, judge_error
		FROM events`)
	if len(clauses) > 0 {
		qb.WriteString(" WHERE ")
		qb.WriteString(strings.Join(clauses, " AND "))
	}
	qb.WriteString(" ORDER BY id DESC LIMIT ?")
	args = append(args, limit)

	rows, err := d.db.Query(qb.String(), args...)
	if err != nil {
		return nil, fmt.Errorf("list events: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var events []EventRecord
	for rows.Next() {
		event, err := scanEventRow(rows)
		if err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate events: %w", err)
	}
	return events, nil
}

// ListEventsForReplay returns historical events oldest-first for classifier replay.
// A non-positive limit means all events. Replay callers must never execute commands.
func (d *DB) ListEventsForReplay(limit int) ([]EventRecord, error) {
	var qb strings.Builder
	qb.WriteString(`SELECT id, timestamp, session_id, command, decision, structural_decision, profile, rule_id, reason, duration_ms, metadata,
		source, agent, cwd, workspace_root, file_inspected, approval_id, user_response, execution_exit_code,
		judge_decision, judge_confidence, judge_reasoning, judge_applied, judge_provider, judge_latency_ms, judge_error
		FROM events ORDER BY id ASC`)
	var args []any
	if limit > 0 {
		qb.WriteString(" LIMIT ?")
		args = append(args, limit)
	}

	rows, err := d.db.Query(qb.String(), args...)
	if err != nil {
		return nil, fmt.Errorf("list replay events: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var events []EventRecord
	for rows.Next() {
		event, err := scanEventRow(rows)
		if err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate replay events: %w", err)
	}
	return events, nil
}

// scanEventRow scans a single row from an events query into an EventRecord.
func scanEventRow(rows *sql.Rows) (EventRecord, error) {
	var event EventRecord
	var sessionID, command, decision, structuralDecision, profile, ruleID, reason, metadata sql.NullString
	var source, agent, cwd, workspaceRoot, approvalID, userResponse sql.NullString
	var fileInspected sql.NullInt64
	var executionExitCode sql.NullInt64
	var judgeDecision, judgeReasoning, judgeProvider, judgeError sql.NullString
	var judgeConfidence sql.NullFloat64
	var judgeApplied, judgeLatencyMs sql.NullInt64
	if err := rows.Scan(
		&event.ID,
		&event.Timestamp,
		&sessionID,
		&command,
		&decision,
		&structuralDecision,
		&profile,
		&ruleID,
		&reason,
		&event.DurationMs,
		&metadata,
		&source,
		&agent,
		&cwd,
		&workspaceRoot,
		&fileInspected,
		&approvalID,
		&userResponse,
		&executionExitCode,
		&judgeDecision,
		&judgeConfidence,
		&judgeReasoning,
		&judgeApplied,
		&judgeProvider,
		&judgeLatencyMs,
		&judgeError,
	); err != nil {
		return EventRecord{}, fmt.Errorf("scan event: %w", err)
	}
	event.SessionID = sessionID.String
	event.Command = command.String
	event.Decision = decision.String
	event.StructuralDecision = structuralDecision.String
	event.Profile = profile.String
	event.RuleID = ruleID.String
	event.Reason = reason.String
	event.Metadata = metadata.String
	event.Source = source.String
	event.Agent = agent.String
	event.Cwd = cwd.String
	event.WorkspaceRoot = workspaceRoot.String
	event.ApprovalID = approvalID.String
	event.UserResponse = userResponse.String
	event.FileInspected = fileInspected.Valid && fileInspected.Int64 != 0
	if executionExitCode.Valid {
		code := executionExitCode.Int64
		event.ExecutionExitCode = &code
	}
	event.JudgeDecision = judgeDecision.String
	event.JudgeConfidence = judgeConfidence.Float64
	event.JudgeReasoning = judgeReasoning.String
	event.JudgeApplied = judgeApplied.Valid && judgeApplied.Int64 != 0
	event.JudgeProvider = judgeProvider.String
	event.JudgeLatencyMs = judgeLatencyMs.Int64
	event.JudgeError = judgeError.String
	return event, nil
}

// SummarizeEvents aggregates counts across the full local event table
// using SQL GROUP BY queries to avoid loading all rows into memory.
func (d *DB) SummarizeEvents() (EventSummary, error) {
	summary := EventSummary{
		ByDecision:    map[string]int{},
		ByAgent:       map[string]int{},
		BySource:      map[string]int{},
		BySourceAgent: map[string]int{},
		ByWorkspace:   map[string]int{},
	}

	if err := d.db.QueryRow("SELECT COUNT(*) FROM events").Scan(&summary.Total); err != nil {
		return EventSummary{}, fmt.Errorf("count events: %w", err)
	}

	// Queries are constant strings — no dynamic SQL.
	dimQueries := []struct {
		query string
		dest  map[string]int
	}{
		{"SELECT COALESCE(decision, ''), COUNT(*) FROM events GROUP BY decision", summary.ByDecision},
		{"SELECT COALESCE(agent, ''), COUNT(*) FROM events GROUP BY agent", summary.ByAgent},
		{"SELECT COALESCE(source, ''), COUNT(*) FROM events GROUP BY source", summary.BySource},
		{"SELECT COALESCE(workspace_root, ''), COUNT(*) FROM events GROUP BY workspace_root", summary.ByWorkspace},
	}
	for _, dim := range dimQueries {
		rows, err := d.db.Query(dim.query)
		if err != nil {
			return EventSummary{}, fmt.Errorf("summarize events: %w", err)
		}
		for rows.Next() {
			var key string
			var count int
			if err := rows.Scan(&key, &count); err != nil {
				rows.Close()
				return EventSummary{}, fmt.Errorf("scan summary: %w", err)
			}
			if key == "" {
				key = "(unknown)"
			}
			dim.dest[key] = count
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return EventSummary{}, fmt.Errorf("iterate summary: %w", err)
		}
	}

	rows, err := d.db.Query("SELECT COALESCE(source, ''), COALESCE(agent, ''), COUNT(*) FROM events GROUP BY source, agent")
	if err != nil {
		return EventSummary{}, fmt.Errorf("summarize events source/agent: %w", err)
	}
	for rows.Next() {
		var source string
		var agent string
		var count int
		if err := rows.Scan(&source, &agent, &count); err != nil {
			rows.Close()
			return EventSummary{}, fmt.Errorf("scan source/agent summary: %w", err)
		}
		summary.BySourceAgent[sourceAgentKey(source, agent)] = count
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return EventSummary{}, fmt.Errorf("iterate source/agent summary: %w", err)
	}

	return summary, nil
}

func sourceAgentKey(source, agent string) string {
	if source == "" {
		source = "(unknown)"
	}
	if agent == "" {
		agent = "(unknown)"
	}
	return source + "/" + agent
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

// PolicyRecommendation represents a frequently-approved command pattern.
type PolicyRecommendation struct {
	Command   string `json:"command"`
	Count     int    `json:"count"`
	Decision  string `json:"decision"`
	Reason    string `json:"reason"`
	Suggested string `json:"suggested"` // suggested policy.yaml rule
}

// FrequentApprovals returns commands that were classified as APPROVAL and approved
// by the user multiple times. These are candidates for policy rules.
func (d *DB) FrequentApprovals(minCount int) ([]PolicyRecommendation, error) {
	if minCount <= 0 {
		minCount = 3
	}
	rows, err := d.db.Query(`
		SELECT command, COUNT(*) as cnt, decision, reason
		FROM events
		WHERE decision = 'APPROVAL'
		GROUP BY command
		HAVING cnt >= ?
		ORDER BY cnt DESC
		LIMIT 20
	`, minCount)
	if err != nil {
		return nil, fmt.Errorf("query frequent approvals: %w", err)
	}
	defer rows.Close()

	var recs []PolicyRecommendation
	for rows.Next() {
		var r PolicyRecommendation
		if err := rows.Scan(&r.Command, &r.Count, &r.Decision, &r.Reason); err != nil {
			continue
		}
		// Generate suggested policy rule
		escaped := regexp.QuoteMeta(r.Command)
		escaped = strings.ReplaceAll(escaped, `"`, `\"`)
		r.Suggested = fmt.Sprintf(`- pattern: "^%s$"\n  action: "allow"\n  reason: "approved %d times"`, escaped, r.Count)
		recs = append(recs, r)
	}
	return recs, rows.Err()
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
