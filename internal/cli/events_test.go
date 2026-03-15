package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"sync"
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

	if err := database.LogEvent(&db.EventRecord{
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
		return runEvents(&eventsOptions{limit: 10})
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
	origStatsJSON := statsJSON
	statsJSON = false
	defer func() { statsJSON = origStatsJSON }()

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
		if err := database.LogEvent(&record); err != nil {
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

func TestRunEvents_JSONOutput(t *testing.T) {
	fuseHome := t.TempDir()
	t.Setenv("FUSE_HOME", fuseHome)
	if err := config.EnsureDirectories(); err != nil {
		t.Fatalf("EnsureDirectories: %v", err)
	}

	database, err := db.OpenDB(config.DBPath())
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	if err := database.LogEvent(&db.EventRecord{
		Command: "echo json", Decision: "SAFE", Source: "hook", Agent: "claude",
	}); err != nil {
		t.Fatalf("LogEvent: %v", err)
	}
	_ = database.Close()

	stdout, _, err := captureCLIOutput(t, func() error {
		return runEvents(&eventsOptions{limit: 10, json: true})
	})
	if err != nil {
		t.Fatalf("runEvents: %v", err)
	}
	if !strings.Contains(stdout, `"command"`) || !strings.Contains(stdout, "echo json") {
		t.Fatalf("expected JSON output with command, got:\n%s", stdout)
	}
}

func TestRunEvents_NoEvents(t *testing.T) {
	fuseHome := t.TempDir()
	t.Setenv("FUSE_HOME", fuseHome)
	if err := config.EnsureDirectories(); err != nil {
		t.Fatalf("EnsureDirectories: %v", err)
	}

	database, err := db.OpenDB(config.DBPath())
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	_ = database.Close()

	stdout, _, err := captureCLIOutput(t, func() error {
		return runEvents(&eventsOptions{limit: 10})
	})
	if err != nil {
		t.Fatalf("runEvents: %v", err)
	}
	if !strings.Contains(stdout, "No matching") {
		t.Fatalf("expected 'No matching' message, got:\n%s", stdout)
	}
}

func TestRunEvents_NoDB(t *testing.T) {
	t.Setenv("FUSE_HOME", filepath.Join(t.TempDir(), "nonexistent"))

	stdout, _, err := captureCLIOutput(t, func() error {
		return runEvents(&eventsOptions{limit: 10})
	})
	if err != nil {
		t.Fatalf("runEvents: %v", err)
	}
	if !strings.Contains(stdout, "No fuse events recorded") {
		t.Fatalf("expected 'No fuse events recorded' message, got:\n%s", stdout)
	}
}

func TestRunStats_JSONOutput(t *testing.T) {
	origStatsJSON := statsJSON
	statsJSON = true
	defer func() { statsJSON = origStatsJSON }()

	fuseHome := t.TempDir()
	t.Setenv("FUSE_HOME", fuseHome)
	if err := config.EnsureDirectories(); err != nil {
		t.Fatalf("EnsureDirectories: %v", err)
	}
	database, err := db.OpenDB(config.DBPath())
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	_ = database.LogEvent(&db.EventRecord{Command: "ls", Decision: "SAFE", Source: "hook", Agent: "claude"})
	_ = database.Close()

	stdout, _, err := captureCLIOutput(t, runStats)
	if err != nil {
		t.Fatalf("runStats: %v", err)
	}
	if !strings.Contains(stdout, `"total"`) {
		t.Fatalf("expected JSON with total, got:\n%s", stdout)
	}
}

func TestRunStats_NoDB(t *testing.T) {
	origStatsJSON := statsJSON
	statsJSON = false
	defer func() { statsJSON = origStatsJSON }()

	t.Setenv("FUSE_HOME", filepath.Join(t.TempDir(), "nonexistent"))
	stdout, _, err := captureCLIOutput(t, runStats)
	if err != nil {
		t.Fatalf("runStats: %v", err)
	}
	if !strings.Contains(stdout, "No fuse events recorded") {
		t.Fatalf("expected no-events message, got:\n%s", stdout)
	}
}

func TestShorten(t *testing.T) {
	if got := shorten("hello", 10); got != "hello" {
		t.Errorf("shorten(hello, 10) = %q", got)
	}
	if got := shorten("hello world", 5); got != "he..." {
		t.Errorf("shorten(hello world, 5) = %q, want he...", got)
	}
	if got := shorten("hello", 3); got != "hel" {
		t.Errorf("shorten(hello, 3) = %q, want hel", got)
	}
	if got := shorten("hi", 2); got != "hi" {
		t.Errorf("shorten(hi, 2) = %q", got)
	}
}

func TestFallbackValue(t *testing.T) {
	if got := fallbackValue(""); got != "-" {
		t.Errorf("fallbackValue('') = %q, want '-'", got)
	}
	if got := fallbackValue("x"); got != "x" {
		t.Errorf("fallbackValue('x') = %q, want 'x'", got)
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

	// Read pipes concurrently to prevent deadlock when fn() output
	// exceeds the OS pipe buffer (~64KB).
	var stdoutBuf, stderrBuf bytes.Buffer
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, _ = stdoutBuf.ReadFrom(stdoutR)
	}()
	go func() {
		defer wg.Done()
		_, _ = stderrBuf.ReadFrom(stderrR)
	}()

	runErr := fn()
	_ = stdoutW.Close()
	_ = stderrW.Close()
	wg.Wait()

	return stdoutBuf.String(), stderrBuf.String(), runErr
}
