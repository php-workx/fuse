package db

import (
	"database/sql"
	"strings"
)

// currentSchemaVersion is the latest schema version applied by migrate.
const currentSchemaVersion = "4"

// migrate creates or updates the database schema.
func migrate(db *sql.DB) error {
	// Create the schema_meta table first so we can track versions.
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS schema_meta (
			key   TEXT PRIMARY KEY,
			value TEXT NOT NULL
		)
	`); err != nil {
		return err
	}

	// Read current version (empty string if table is empty).
	var version string
	row := db.QueryRow(`SELECT value FROM schema_meta WHERE key = 'version'`)
	if err := row.Scan(&version); err != nil && err != sql.ErrNoRows {
		return err
	}

	if version == currentSchemaVersion {
		return nil // already up to date
	}

	// Apply schema v1.
	if version == "" {
		if err := applyV1(db); err != nil {
			return err
		}
		version = "1"
	}

	if version == "1" {
		if err := applyV2(db); err != nil {
			return err
		}
		version = "2"
	}

	if version == "2" {
		if err := applyV3(db); err != nil {
			return err
		}
		version = "3"
	}

	if version == "3" {
		if err := applyV4(db); err != nil {
			return err
		}
	}

	return nil
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

func isDuplicateColumnError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "duplicate column name")
}
