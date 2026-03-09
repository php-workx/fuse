package db

import "database/sql"

// currentSchemaVersion is the latest schema version applied by migrate.
const currentSchemaVersion = "1"

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
