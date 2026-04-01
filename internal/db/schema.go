package db

import (
	"database/sql"
	"fmt"
	"strings"
)

// migrationStep maps a schema version to its migration function.
type migrationStep struct {
	fromVersion string // version that triggers this migration ("" for initial)
	apply       func(*sql.DB) error
	toVersion   string // version after successful migration
}

// migrationSteps defines the ordered sequence of schema migrations.
var migrationSteps = []migrationStep{
	{"", applyV1, "1"},
	{"1", applyV2, "2"},
	{"2", applyV3, "3"},
	{"3", applyV4, "4"},
	{"4", applyV5, "5"},
	{"5", applyV6, "6"},
	{"6", applyV7, "7"},
}

func currentSchemaVersion() string {
	if len(migrationSteps) == 0 {
		return ""
	}
	return migrationSteps[len(migrationSteps)-1].toVersion
}

// migrate creates or updates the database schema.
func migrate(db *sql.DB) error {
	if err := ensureSchemaMeta(db); err != nil {
		return fmt.Errorf("bootstrap schema_meta: %w", err)
	}

	version, err := readSchemaVersion(db)
	if err != nil {
		return fmt.Errorf("read schema version: %w", err)
	}
	if version == currentSchemaVersion() {
		return nil
	}

	for version != currentSchemaVersion() {
		applied := false
		for _, step := range migrationSteps {
			if version != step.fromVersion {
				continue
			}
			if err := step.apply(db); err != nil {
				return fmt.Errorf("migrate schema %s->%s: %w", step.fromVersion, step.toVersion, err)
			}
			version = step.toVersion
			applied = true
			break
		}
		if !applied {
			return fmt.Errorf("unknown schema version %q: no migration path to %s", version, currentSchemaVersion())
		}
	}
	return nil
}

// ensureSchemaMeta creates the schema_meta table if it doesn't exist.
func ensureSchemaMeta(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS schema_meta (
			key   TEXT PRIMARY KEY,
			value TEXT NOT NULL
		)
	`)
	return err
}

// readSchemaVersion returns the current schema version, or "" if unset.
func readSchemaVersion(db *sql.DB) (string, error) {
	var version string
	row := db.QueryRow(`SELECT value FROM schema_meta WHERE key = 'version'`)
	if err := row.Scan(&version); err != nil && err != sql.ErrNoRows {
		return "", err
	}
	return version, nil
}

func applyV1(db *sql.DB) error {
	stmts := []string{
		// Approval records.
		`CREATE TABLE IF NOT EXISTS approvals (
			id            TEXT PRIMARY KEY,
			decision_key  TEXT NOT NULL,
			decision      TEXT NOT NULL CHECK(decision IN ('SAFE','CAUTION','APPROVAL','BLOCKED')),
			scope         TEXT NOT NULL CHECK(scope IN ('once','command','session','forever')),
			session_id    TEXT,
			hmac          TEXT NOT NULL,
			created_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
			consumed_at   TEXT,
			expires_at    TEXT
		)`,
		`CREATE INDEX IF NOT EXISTS idx_approvals_key ON approvals(decision_key, consumed_at)`,

		// Event log.
		`CREATE TABLE IF NOT EXISTS events (
			id           INTEGER PRIMARY KEY AUTOINCREMENT,
			timestamp    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
			session_id   TEXT,
			command      TEXT,
			decision     TEXT,
			structural_decision TEXT DEFAULT '',
			profile      TEXT DEFAULT '',
			rule_id      TEXT,
			reason       TEXT,
			duration_ms  INTEGER,
			metadata     TEXT
		)`,

		// Record schema version.
		`INSERT OR REPLACE INTO schema_meta (key, value) VALUES ('version', '1')`,
	}

	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			return err
		}
	}
	return nil
}

func applyV2(db *sql.DB) error {
	stmts := []string{
		`ALTER TABLE approvals ADD COLUMN consumed INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE approvals ADD COLUMN source TEXT NOT NULL DEFAULT 'shell'`,
		`ALTER TABLE approvals ADD COLUMN command TEXT`,
		`ALTER TABLE approvals ADD COLUMN reason TEXT`,
		`ALTER TABLE approvals ADD COLUMN file_inspected TEXT`,
		`CREATE INDEX IF NOT EXISTS idx_approvals_expires ON approvals(expires_at)`,
		`UPDATE approvals SET consumed = 1 WHERE consumed_at IS NOT NULL`,
		`ALTER TABLE events ADD COLUMN source TEXT NOT NULL DEFAULT 'shell'`,
		`ALTER TABLE events ADD COLUMN agent TEXT`,
		`ALTER TABLE events ADD COLUMN file_inspected INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE events ADD COLUMN approval_id TEXT`,
		`ALTER TABLE events ADD COLUMN user_response TEXT`,
		`ALTER TABLE events ADD COLUMN execution_exit_code INTEGER`,
		`INSERT OR REPLACE INTO schema_meta (key, value) VALUES ('version', '2')`,
	}

	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil && !isDuplicateColumnError(err) {
			return err
		}
	}
	return nil
}

func applyV3(db *sql.DB) error {
	stmts := []string{
		`CREATE INDEX IF NOT EXISTS idx_events_ts ON events(timestamp)`,
		`INSERT OR REPLACE INTO schema_meta (key, value) VALUES ('version', '3')`,
	}

	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			return err
		}
	}
	return nil
}

func applyV4(db *sql.DB) error {
	stmts := []string{
		`ALTER TABLE events ADD COLUMN cwd TEXT`,
		`ALTER TABLE events ADD COLUMN workspace_root TEXT`,
		`INSERT OR REPLACE INTO schema_meta (key, value) VALUES ('version', '4')`,
	}

	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil && !isDuplicateColumnError(err) {
			return err
		}
	}
	return nil
}

func applyV5(db *sql.DB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS pending_requests (
			id             TEXT PRIMARY KEY,
			decision_key   TEXT NOT NULL,
			command        TEXT NOT NULL,
			reason         TEXT,
			source         TEXT NOT NULL DEFAULT 'hook',
			session_id     TEXT,
			cwd            TEXT,
			workspace_root TEXT,
			created_at     TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
		)`,
		`CREATE INDEX IF NOT EXISTS idx_pending_created ON pending_requests(created_at)`,
		`INSERT OR REPLACE INTO schema_meta (key, value) VALUES ('version', '5')`,
	}

	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			return err
		}
	}
	return nil
}

// applyV6 adds LLM judge columns to the events table.
func applyV6(db *sql.DB) error {
	stmts := []string{
		`ALTER TABLE events ADD COLUMN judge_decision TEXT DEFAULT ''`,
		`ALTER TABLE events ADD COLUMN judge_confidence REAL DEFAULT 0`,
		`ALTER TABLE events ADD COLUMN judge_reasoning TEXT DEFAULT ''`,
		`ALTER TABLE events ADD COLUMN judge_applied INTEGER DEFAULT 0`,
		`ALTER TABLE events ADD COLUMN judge_provider TEXT DEFAULT ''`,
		`ALTER TABLE events ADD COLUMN judge_latency_ms INTEGER DEFAULT 0`,
		`ALTER TABLE events ADD COLUMN judge_error TEXT DEFAULT ''`,
		`INSERT OR REPLACE INTO schema_meta (key, value) VALUES ('version', '6')`,
	}

	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil && !isDuplicateColumnError(err) {
			return err
		}
	}
	return nil
}

func applyV7(db *sql.DB) error {
	stmts := []string{
		`ALTER TABLE events ADD COLUMN structural_decision TEXT DEFAULT ''`,
		`ALTER TABLE events ADD COLUMN profile TEXT DEFAULT ''`,
		`INSERT OR REPLACE INTO schema_meta (key, value) VALUES ('version', '7')`,
	}

	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil && !isDuplicateColumnError(err) {
			return err
		}
	}
	return nil
}

func isDuplicateColumnError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "duplicate column name")
}
