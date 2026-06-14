package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/php-workx/fuse/internal/config"
	"github.com/php-workx/fuse/internal/db"
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
	for _, want := range []string{"Total events", "codex", "claude", "codex-shell", "hook", "By source/agent", "codex-shell/codex", "hook/claude"} {
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
	if !strings.Contains(stdout, `"total"`) || !strings.Contains(stdout, `"by_source_agent"`) || !strings.Contains(stdout, `"hook/claude"`) {
		t.Fatalf("expected JSON with source/agent stats, got:\n%s", stdout)
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

func TestDryrunCommand(t *testing.T) {
	fuseHome := t.TempDir()
	t.Setenv("FUSE_HOME", fuseHome)
	if err := config.EnsureDirectories(); err != nil {
		t.Fatalf("EnsureDirectories: %v", err)
	}

	stdout, _, err := captureCLIOutput(t, func() error {
		return dryrunCmd.RunE(dryrunCmd, nil)
	})
	if err != nil {
		t.Fatalf("dryrun: %v", err)
	}
	if !strings.Contains(stdout, "dry-run") {
		t.Fatalf("expected dry-run message, got: %s", stdout)
	}
	if config.Mode() != config.ModeDryRun {
		t.Fatalf("mode = %d, want ModeDryRun", config.Mode())
	}
}

func TestDisableCommand(t *testing.T) {
	fuseHome := t.TempDir()
	t.Setenv("FUSE_HOME", fuseHome)
	if err := config.EnsureDirectories(); err != nil {
		t.Fatalf("EnsureDirectories: %v", err)
	}
	// Enable first, then disable.
	_ = enableCmd.RunE(enableCmd, nil)

	stdout, _, err := captureCLIOutput(t, func() error {
		return disableCmd.RunE(disableCmd, nil)
	})
	if err != nil {
		t.Fatalf("disable: %v", err)
	}
	if !strings.Contains(stdout, "disabled") {
		t.Fatalf("expected disabled message, got: %s", stdout)
	}
	if config.Mode() != config.ModeDisabled {
		t.Fatalf("mode = %d, want ModeDisabled", config.Mode())
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

// seedEvents opens a test DB at FUSE_HOME and writes the supplied records.
// FUSE_HOME must already be set via t.Setenv.
func seedEvents(t *testing.T, records ...db.EventRecord) {
	t.Helper()
	if err := config.EnsureDirectories(); err != nil {
		t.Fatalf("EnsureDirectories: %v", err)
	}
	database, err := db.OpenDB(config.DBPath())
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	defer func() { _ = database.Close() }()
	for i := range records {
		if err := database.LogEvent(&records[i]); err != nil {
			t.Fatalf("LogEvent(%d): %v", i, err)
		}
	}
}

// Gap 1 — filter flag routing -----------------------------------------------

func TestRunEvents_FilterMatrix(t *testing.T) {
	t.Setenv("FUSE_HOME", t.TempDir())
	seedEvents(t,
		db.EventRecord{Command: "git status", Decision: "SAFE", Source: "hook", Agent: "claude", SessionID: "sess-1", WorkspaceRoot: "/Users/x/proj-a"},
		db.EventRecord{Command: "kubectl get", Decision: "CAUTION", Source: "codex-shell", Agent: "codex", SessionID: "sess-2", WorkspaceRoot: "/Users/x/proj-b"},
		db.EventRecord{Command: "rm -rf /", Decision: "BLOCKED", Source: "run", Agent: "manual", SessionID: "sess-3", WorkspaceRoot: "/Users/x/proj-c"},
	)

	cases := []struct {
		name     string
		opts     eventsOptions
		want     []string // commands that must appear
		notWant  []string // commands that must NOT appear
		wantSize int      // exact count when > 0
	}{
		{name: "source hook only", opts: eventsOptions{limit: 10, source: "hook"}, want: []string{"git status"}, notWant: []string{"kubectl get", "rm -rf /"}},
		{name: "agent codex only", opts: eventsOptions{limit: 10, agent: "codex"}, want: []string{"kubectl get"}, notWant: []string{"git status", "rm -rf /"}},
		{name: "decision uppercase", opts: eventsOptions{limit: 10, decision: "BLOCKED"}, want: []string{"rm -rf /"}, notWant: []string{"git status", "kubectl get"}},
		{name: "decision lowercase", opts: eventsOptions{limit: 10, decision: "blocked"}, want: []string{"rm -rf /"}, notWant: []string{"git status", "kubectl get"}},
		{name: "decision mixed case", opts: eventsOptions{limit: 10, decision: "Blocked"}, want: []string{"rm -rf /"}, notWant: []string{"git status", "kubectl get"}},
		{name: "session filter", opts: eventsOptions{limit: 10, session: "sess-2"}, want: []string{"kubectl get"}, notWant: []string{"git status", "rm -rf /"}},
		{name: "workspace filter", opts: eventsOptions{limit: 10, workspace: "/Users/x/proj-c"}, want: []string{"rm -rf /"}, notWant: []string{"git status", "kubectl get"}},
		{name: "intersection source+agent", opts: eventsOptions{limit: 10, source: "hook", agent: "claude"}, want: []string{"git status"}, notWant: []string{"kubectl get", "rm -rf /"}},
		{name: "limit caps result", opts: eventsOptions{limit: 1}, want: nil, wantSize: 1},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			opts := tc.opts
			stdout, _, err := captureCLIOutput(t, func() error { return runEvents(&opts) })
			if err != nil {
				t.Fatalf("runEvents: %v", err)
			}
			for _, w := range tc.want {
				if !strings.Contains(stdout, w) {
					t.Errorf("want %q in output:\n%s", w, stdout)
				}
			}
			for _, nw := range tc.notWant {
				if strings.Contains(stdout, nw) {
					t.Errorf("did NOT want %q in output:\n%s", nw, stdout)
				}
			}
			if tc.wantSize > 0 {
				dataRows := countDataRows(stdout)
				if dataRows != tc.wantSize {
					t.Errorf("got %d data rows, want %d:\n%s", dataRows, tc.wantSize, stdout)
				}
			}
		})
	}
}

// Gap 2 — tabwriter output format -------------------------------------------

func TestRunEvents_TabwriterHeader(t *testing.T) {
	t.Setenv("FUSE_HOME", t.TempDir())
	seedEvents(t, db.EventRecord{Command: "ls", Decision: "SAFE", Source: "hook", Agent: "claude"})

	stdout, _, err := captureCLIOutput(t, func() error { return runEvents(&eventsOptions{limit: 10}) })
	if err != nil {
		t.Fatalf("runEvents: %v", err)
	}
	if !strings.Contains(stdout, "TIME") || !strings.Contains(stdout, "AGENT") || !strings.Contains(stdout, "SOURCE") ||
		!strings.Contains(stdout, "DECISION") || !strings.Contains(stdout, "WORKSPACE") || !strings.Contains(stdout, "COMMAND") {
		t.Errorf("missing header columns:\n%s", stdout)
	}
}

func TestRunEvents_FallbackDashForEmptyFields(t *testing.T) {
	t.Setenv("FUSE_HOME", t.TempDir())
	// All optional fields empty.
	seedEvents(t, db.EventRecord{Command: "naked", Decision: "SAFE"})

	stdout, _, err := captureCLIOutput(t, func() error { return runEvents(&eventsOptions{limit: 10}) })
	if err != nil {
		t.Fatalf("runEvents: %v", err)
	}
	if !strings.Contains(stdout, "naked") {
		t.Fatalf("command missing:\n%s", stdout)
	}
	// "-" must appear at least once for a missing optional field.
	if !strings.Contains(stdout, "-") {
		t.Errorf("expected '-' fallback for empty fields:\n%s", stdout)
	}
}

func TestRunEvents_LongCommandShortened(t *testing.T) {
	// Use a realistic-looking long command that won't trip the credential
	// scrubber (which masks long base64-shaped tokens). 200 chars of repeated
	// `ls /tmp/file_XX ` segments stay through scrubbing and exceed the
	// 96-char shorten threshold.
	t.Setenv("FUSE_HOME", t.TempDir())
	seg := "ls /tmp/aaa /tmp/bbb /tmp/ccc /tmp/ddd "
	long := strings.Repeat(seg, 6) // 234 chars
	seedEvents(t, db.EventRecord{Command: long, Decision: "SAFE", Source: "hook", Agent: "claude"})

	stdout, _, err := captureCLIOutput(t, func() error { return runEvents(&eventsOptions{limit: 10}) })
	if err != nil {
		t.Fatalf("runEvents: %v", err)
	}
	if !strings.Contains(stdout, "...") {
		t.Errorf("expected '...' in shortened output:\n%s", stdout)
	}
}

// Gap 3 — runStats breakdown shape ------------------------------------------

func TestRunStats_BreakdownSectionsOrder(t *testing.T) {
	origStatsJSON := statsJSON
	statsJSON = false
	defer func() { statsJSON = origStatsJSON }()

	t.Setenv("FUSE_HOME", t.TempDir())
	seedEvents(t,
		db.EventRecord{Command: "a", Decision: "SAFE", Source: "hook", Agent: "claude", WorkspaceRoot: "/p"},
		db.EventRecord{Command: "b", Decision: "BLOCKED", Source: "run", Agent: "manual", WorkspaceRoot: "/p"},
	)

	stdout, _, err := captureCLIOutput(t, runStats)
	if err != nil {
		t.Fatalf("runStats: %v", err)
	}

	sections := []string{"By decision", "By agent", "By source", "By source/agent", "By workspace"}
	prev := -1
	for _, s := range sections {
		idx := strings.Index(stdout, s)
		if idx < 0 {
			t.Fatalf("section %q missing:\n%s", s, stdout)
		}
		if idx <= prev {
			t.Fatalf("section %q out of order (idx=%d, prev=%d):\n%s", s, idx, prev, stdout)
		}
		prev = idx
	}
}

func TestRunStats_JSONShape(t *testing.T) {
	origStatsJSON := statsJSON
	statsJSON = true
	defer func() { statsJSON = origStatsJSON }()

	t.Setenv("FUSE_HOME", t.TempDir())
	seedEvents(t, db.EventRecord{Command: "ls", Decision: "SAFE", Source: "hook", Agent: "claude"})

	stdout, _, err := captureCLIOutput(t, runStats)
	if err != nil {
		t.Fatalf("runStats: %v", err)
	}
	for _, want := range []string{`"total"`, `"by_decision"`, `"by_agent"`, `"by_source"`, `"by_source_agent"`, `"by_workspace"`} {
		if !strings.Contains(stdout, want) {
			t.Errorf("missing JSON key %q:\n%s", want, stdout)
		}
	}
}

// Gap 5 — empty-state JSON shapes -------------------------------------------

func TestRunEvents_JSONEmptyArrayWhenDBPresentButEmpty(t *testing.T) {
	t.Setenv("FUSE_HOME", t.TempDir())
	if err := config.EnsureDirectories(); err != nil {
		t.Fatalf("EnsureDirectories: %v", err)
	}
	// Open and close an empty DB so the file exists but no rows.
	database, err := db.OpenDB(config.DBPath())
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	_ = database.Close()

	stdout, _, err := captureCLIOutput(t, func() error { return runEvents(&eventsOptions{limit: 10, json: true}) })
	if err != nil {
		t.Fatalf("runEvents: %v", err)
	}
	trimmed := strings.TrimSpace(stdout)
	if trimmed != "[]" && trimmed != "null" {
		t.Errorf("expected empty JSON array or null, got %q", trimmed)
	}
}

func TestRunEvents_JSONEmptyArrayWhenNoDB(t *testing.T) {
	t.Setenv("FUSE_HOME", filepath.Join(t.TempDir(), "nonexistent"))

	stdout, _, err := captureCLIOutput(t, func() error { return runEvents(&eventsOptions{limit: 10, json: true}) })
	if err != nil {
		t.Fatalf("runEvents: %v", err)
	}
	if strings.TrimSpace(stdout) != "[]" {
		t.Errorf("expected '[]' when no DB, got %q", strings.TrimSpace(stdout))
	}
}

func TestRunStats_JSONEmptyObjectWhenNoDB(t *testing.T) {
	origStatsJSON := statsJSON
	statsJSON = true
	defer func() { statsJSON = origStatsJSON }()

	t.Setenv("FUSE_HOME", filepath.Join(t.TempDir(), "nonexistent"))

	stdout, _, err := captureCLIOutput(t, runStats)
	if err != nil {
		t.Fatalf("runStats: %v", err)
	}
	if strings.TrimSpace(stdout) != "{}" {
		t.Errorf("expected '{}' when no DB, got %q", strings.TrimSpace(stdout))
	}
}

// Gap 6 — limit edge cases ---------------------------------------------------

func TestRunEvents_LimitZeroIsUnbounded(t *testing.T) {
	// Pin the existing contract: limit=0 is treated as "no limit" by the DB
	// layer (i.e. all rows returned). If this contract changes, update both
	// this test and the docstring on the --limit flag.
	t.Setenv("FUSE_HOME", t.TempDir())
	seedEvents(t,
		db.EventRecord{Command: "a", Decision: "SAFE", Source: "hook", Agent: "claude"},
		db.EventRecord{Command: "b", Decision: "SAFE", Source: "hook", Agent: "claude"},
	)

	stdout, _, err := captureCLIOutput(t, func() error { return runEvents(&eventsOptions{limit: 0}) })
	if err != nil {
		t.Fatalf("runEvents: %v", err)
	}
	if !strings.Contains(stdout, "a") || !strings.Contains(stdout, "b") {
		t.Errorf("limit=0 should return all rows; got:\n%s", stdout)
	}
}

// Gap 7 — dryrun/disable idempotency ---------------------------------------

func TestDisableCommand_Idempotent(t *testing.T) {
	t.Setenv("FUSE_HOME", t.TempDir())
	if err := config.EnsureDirectories(); err != nil {
		t.Fatalf("EnsureDirectories: %v", err)
	}
	_ = enableCmd.RunE(enableCmd, nil)
	if err := disableCmd.RunE(disableCmd, nil); err != nil {
		t.Fatalf("first disable: %v", err)
	}
	if err := disableCmd.RunE(disableCmd, nil); err != nil {
		t.Fatalf("second disable should be idempotent: %v", err)
	}
	if config.Mode() != config.ModeDisabled {
		t.Fatalf("mode = %d, want ModeDisabled", config.Mode())
	}
}

func TestDryrunThenDisable(t *testing.T) {
	t.Setenv("FUSE_HOME", t.TempDir())
	if err := config.EnsureDirectories(); err != nil {
		t.Fatalf("EnsureDirectories: %v", err)
	}
	if err := dryrunCmd.RunE(dryrunCmd, nil); err != nil {
		t.Fatalf("dryrun: %v", err)
	}
	if err := disableCmd.RunE(disableCmd, nil); err != nil {
		t.Fatalf("disable: %v", err)
	}
	if config.Mode() != config.ModeDisabled {
		t.Fatalf("mode = %d, want ModeDisabled", config.Mode())
	}
}

// Helpers --------------------------------------------------------------------

func countDataRows(stdout string) int {
	lines := strings.Split(stdout, "\n")
	rows := 0
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if l == "" {
			continue
		}
		// Skip header + section title.
		if strings.HasPrefix(l, "Recent fuse events") || strings.HasPrefix(l, "TIME") {
			continue
		}
		if strings.HasPrefix(l, "No matching") || strings.HasPrefix(l, "No fuse events") {
			continue
		}
		rows++
	}
	return rows
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
