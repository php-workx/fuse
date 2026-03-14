package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/runger/fuse/internal/config"
	"github.com/runger/fuse/internal/db"
)

func TestRunEvents_PrintsRecentActivity(t *testing.T) {
	fuseHome := t.TempDir()
	t.Setenv("FUSE_HOME", fuseHome)
	if err := config.EnsureDirectories(); err != nil {
		t.Fatalf("EnsureDirectories: %v", err)
	}

	database, err := db.OpenDB(config.DBPath())
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	defer func() { _ = database.Close() }()

	if err := database.LogEvent(db.EventRecord{
		Command:       "git status",
		Decision:      "SAFE",
		Source:        "codex-shell",
		Agent:         "codex",
		Cwd:           filepath.Join(fuseHome, "repo", "subdir"),
		WorkspaceRoot: filepath.Join(fuseHome, "repo"),
	}); err != nil {
		t.Fatalf("LogEvent: %v", err)
	}

	stdout, stderr, err := captureCLIOutput(t, func() error {
		return runEvents(eventsOptions{limit: 10})
	})
	if err != nil {
		t.Fatalf("runEvents: %v", err)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
	for _, want := range []string{"codex-shell", "codex", "git status"} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("expected output to contain %q, got:\n%s", want, stdout)
		}
	}
}

func TestRunStats_SummarizesActivity(t *testing.T) {
	fuseHome := t.TempDir()
	t.Setenv("FUSE_HOME", fuseHome)
	if err := config.EnsureDirectories(); err != nil {
		t.Fatalf("EnsureDirectories: %v", err)
	}

	database, err := db.OpenDB(config.DBPath())
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	defer func() { _ = database.Close() }()

	records := []db.EventRecord{
		{
			Command:       "git status",
			Decision:      "SAFE",
			Source:        "codex-shell",
			Agent:         "codex",
			Cwd:           filepath.Join(fuseHome, "repo-a"),
			WorkspaceRoot: filepath.Join(fuseHome, "repo-a"),
		},
		{
			Command:       "terraform destroy prod",
			Decision:      "APPROVAL",
			Source:        "hook",
			Agent:         "claude",
			Cwd:           filepath.Join(fuseHome, "repo-b"),
			WorkspaceRoot: filepath.Join(fuseHome, "repo-b"),
		},
	}
	for i, record := range records {
		if err := database.LogEvent(record); err != nil {
			t.Fatalf("LogEvent(%d): %v", i, err)
		}
	}

	stdout, stderr, err := captureCLIOutput(t, runStats)
	if err != nil {
		t.Fatalf("runStats: %v", err)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
	for _, want := range []string{"Total events", "codex", "claude", "codex-shell", "hook"} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("expected output to contain %q, got:\n%s", want, stdout)
		}
	}
}

func captureCLIOutput(t *testing.T, fn func() error) (string, string, error) {
	t.Helper()

	origStdout := os.Stdout
	origStderr := os.Stderr

	stdoutR, stdoutW, err := os.Pipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	stderrR, stderrW, err := os.Pipe()
	if err != nil {
		t.Fatalf("stderr pipe: %v", err)
	}

	os.Stdout = stdoutW
	os.Stderr = stderrW
	defer func() {
		os.Stdout = origStdout
		os.Stderr = origStderr
	}()

	runErr := fn()
	_ = stdoutW.Close()
	_ = stderrW.Close()

	var stdoutBuf bytes.Buffer
	var stderrBuf bytes.Buffer
	if _, err := stdoutBuf.ReadFrom(stdoutR); err != nil {
		t.Fatalf("read stdout: %v", err)
	}
	if _, err := stderrBuf.ReadFrom(stderrR); err != nil {
		t.Fatalf("read stderr: %v", err)
	}
	return stdoutBuf.String(), stderrBuf.String(), runErr
}
