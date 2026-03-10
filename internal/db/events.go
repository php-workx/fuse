package db

import (
	"fmt"
	"regexp"
)

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

// LogEvent inserts an event record with credential scrubbing applied to command.
func (d *DB) LogEvent(sessionID, command, decision, ruleID, reason string, durationMs int64, metadata string) error {
	scrubbed := ScrubCredentials(command)

	_, err := d.db.Exec(`
		INSERT INTO events (session_id, command, decision, rule_id, reason, duration_ms, metadata)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, sessionID, scrubbed, decision, ruleID, reason, durationMs, metadata)
	if err != nil {
		return fmt.Errorf("log event: %w", err)
	}
	return nil
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
