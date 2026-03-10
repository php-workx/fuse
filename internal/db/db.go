// Package db provides SQLite-backed state storage for fuse.
package db

import (
	"database/sql"
	"fmt"
	"os"
	"sync/atomic"

	_ "modernc.org/sqlite"
)

// cleanupCycleCount tracks how many cleanup cycles have been performed.
var cleanupCycleCount int64

// DB wraps a *sql.DB connection to the fuse state database.
type DB struct {
	db *sql.DB
}

// OpenDB opens (or creates) a SQLite database at path with correct
// pragmas and migrations applied. The caller is responsible for calling Close.
//
// OpenDB is meant to be called lazily — the SAFE/BLOCKED hot path should
// never reach this function.
func OpenDB(path string) (*DB, error) {
	// Ensure parent directory exists.
	if dir := parentDir(path); dir != "" {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return nil, fmt.Errorf("create state directory: %w", err)
		}
	}

	// Create the file with correct permissions atomically (avoids TOCTOU race).
	// If the file already exists, this is a no-op; if new, it's created with 0600.
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, fmt.Errorf("create database file: %w", err)
	}
	_ = f.Close()

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	// Enable WAL mode for better concurrency.
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("enable WAL mode: %w", err)
	}

	// Set busy timeout so concurrent writers wait instead of failing.
	if _, err := db.Exec("PRAGMA busy_timeout=5000"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("set busy timeout: %w", err)
	}

	// Enable foreign key enforcement.
	if _, err := db.Exec("PRAGMA foreign_keys=ON"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("enable foreign keys: %w", err)
	}

	// Run schema migrations.
	if err := migrate(db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("run migrations: %w", err)
	}

	return &DB{db: db}, nil
}

// Close closes the underlying database connection.
func (d *DB) Close() error {
	if d.db == nil {
		return nil
	}
	return d.db.Close()
}

// WalCheckpoint performs a WAL checkpoint with TRUNCATE mode to reclaim space.
func (d *DB) WalCheckpoint() error {
	_, err := d.db.Exec("PRAGMA wal_checkpoint(TRUNCATE)")
	if err != nil {
		return fmt.Errorf("wal checkpoint: %w", err)
	}
	atomic.AddInt64(&cleanupCycleCount, 1)
	return nil
}

// Vacuum runs VACUUM to rebuild the database file and reclaim unused space.
func (d *DB) Vacuum() error {
	_, err := d.db.Exec("VACUUM")
	if err != nil {
		return fmt.Errorf("vacuum: %w", err)
	}
	return nil
}

// parentDir returns the parent directory of path.
func parentDir(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' || path[i] == '\\' {
			return path[:i]
		}
	}
	return ""
}
