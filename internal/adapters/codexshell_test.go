package adapters

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/runger/fuse/internal/config"
)

func TestExecuteCodexShellCommand_AllowsBlockedCommandWhenDisabled(t *testing.T) {
	enabledMarker := config.EnabledMarkerPath()
	if err := os.Remove(enabledMarker); err != nil {
		t.Fatalf("remove enabled marker: %v", err)
	}
	defer func() {
		if err := os.WriteFile(enabledMarker, []byte("1"), 0600); err != nil {
			t.Fatalf("restore enabled marker: %v", err)
		}
	}()

	targetDir := filepath.Join(t.TempDir(), "to-remove")
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		t.Fatalf("mkdir target: %v", err)
	}

	_, _, exitCode, err := executeCodexShellCommand("rm -rf "+targetDir, "", time.Minute)
	if err != nil {
		t.Fatalf("expected disabled mode to bypass classification, got error: %v", err)
	}
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}
	if _, err := os.Stat(targetDir); !os.IsNotExist(err) {
		t.Fatalf("expected target dir to be removed, stat err = %v", err)
	}
}

func TestExecuteCodexShellCommand_LogsAndPrunesEvents(t *testing.T) {
	if err := config.EnsureDirectories(); err != nil {
		t.Fatalf("ensure directories: %v", err)
	}
	configYAML := "max_event_log_rows: 1\n"
	if err := os.WriteFile(config.ConfigPath(), []byte(configYAML), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	for _, command := range []string{"printf one", "printf two"} {
		if _, _, exitCode, err := executeCodexShellCommand(command, "", time.Minute); err != nil {
			t.Fatalf("executeCodexShellCommand(%q): %v", command, err)
		} else if exitCode != 0 {
			t.Fatalf("executeCodexShellCommand(%q) exit code = %d", command, exitCode)
		}
	}

	sqlDB, err := sql.Open("sqlite", config.DBPath())
	if err != nil {
		t.Fatalf("open sqlite db: %v", err)
	}
	defer func() { _ = sqlDB.Close() }()

	var count int
	if err := sqlDB.QueryRow("SELECT COUNT(*) FROM events").Scan(&count); err != nil {
		t.Fatalf("count events: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected event log to be pruned to 1 row, got %d", count)
	}
}
