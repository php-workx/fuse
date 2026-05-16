package cli

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/php-workx/fuse/internal/config"
	"github.com/php-workx/fuse/internal/db"
)

func TestRunReplayEvents_JSONReportsDecisionMatrixAndClusters(t *testing.T) {
	fuseHome := t.TempDir()
	t.Setenv("FUSE_HOME", fuseHome)
	if err := config.EnsureDirectories(); err != nil {
		t.Fatalf("EnsureDirectories: %v", err)
	}

	database, err := db.OpenDB(config.DBPath())
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	records := []db.EventRecord{
		{
			Command:       "just lint",
			Decision:      "CAUTION",
			Source:        "hook",
			Agent:         "codex",
			Cwd:           filepath.Join(fuseHome, "repo"),
			WorkspaceRoot: filepath.Join(fuseHome, "repo"),
		},
		{
			Command:       "just generate-docs",
			Decision:      "APPROVAL",
			Source:        "hook",
			Agent:         "codex",
			Cwd:           filepath.Join(fuseHome, "repo"),
			WorkspaceRoot: filepath.Join(fuseHome, "repo"),
		},
		{
			Command:       "rm -rf /",
			Decision:      "APPROVAL",
			Source:        "hook",
			Agent:         "codex",
			Cwd:           filepath.Join(fuseHome, "repo"),
			WorkspaceRoot: filepath.Join(fuseHome, "repo"),
		},
	}
	for i := range records {
		if err := database.LogEvent(&records[i]); err != nil {
			t.Fatalf("LogEvent(%d): %v", i, err)
		}
	}
	_ = database.Close()

	stdout, stderr, err := captureCLIOutput(t, func() error {
		return runReplayEvents(&replayEventsOptions{json: true, top: 10})
	})
	if err != nil {
		t.Fatalf("runReplayEvents: %v", err)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}

	var report replayEventsReport
	if err := json.Unmarshal([]byte(stdout), &report); err != nil {
		t.Fatalf("decode report: %v\n%s", err, stdout)
	}
	if report.Total != 3 {
		t.Fatalf("total = %d, want 3", report.Total)
	}
	if got := report.DecisionMatrix["CAUTION"]["SAFE"]; got != 1 {
		t.Fatalf("CAUTION->SAFE = %d, want 1; matrix=%v", got, report.DecisionMatrix)
	}
	if got := report.DecisionMatrix["APPROVAL"]["CAUTION"]; got != 1 {
		t.Fatalf("APPROVAL->CAUTION = %d, want 1; matrix=%v", got, report.DecisionMatrix)
	}
	if got := report.DecisionMatrix["APPROVAL"]["BLOCKED"]; got != 1 {
		t.Fatalf("APPROVAL->BLOCKED = %d, want 1; matrix=%v", got, report.DecisionMatrix)
	}
	if report.CurrentApprovalEvents != 0 {
		t.Fatalf("current approval events = %d, want 0", report.CurrentApprovalEvents)
	}
	if report.EstimatedLivePromptKeys != 0 {
		t.Fatalf("estimated live prompt keys = %d, want 0", report.EstimatedLivePromptKeys)
	}
	if len(report.RemainingClusters) == 0 {
		t.Fatalf("expected remaining CAUTION/BLOCKED-adjacent cluster data, got none")
	}
}

func TestRunReplayEvents_SeparatesFileNotFoundReplayDrift(t *testing.T) {
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
		Command:       "bash -n scripts/missing-at-replay-time.sh",
		Decision:      "APPROVAL",
		Cwd:           filepath.Join(fuseHome, "repo"),
		WorkspaceRoot: filepath.Join(fuseHome, "repo"),
	}); err != nil {
		t.Fatalf("LogEvent: %v", err)
	}
	_ = database.Close()

	stdout, _, err := captureCLIOutput(t, func() error {
		return runReplayEvents(&replayEventsOptions{json: true, top: 10})
	})
	if err != nil {
		t.Fatalf("runReplayEvents: %v", err)
	}

	var report replayEventsReport
	if err := json.Unmarshal([]byte(stdout), &report); err != nil {
		t.Fatalf("decode report: %v\n%s", err, stdout)
	}
	if report.CurrentApprovalEvents != 1 {
		t.Fatalf("current approval events = %d, want 1", report.CurrentApprovalEvents)
	}
	if report.ReplayDriftApprovalEvents != 1 {
		t.Fatalf("replay drift approval events = %d, want 1", report.ReplayDriftApprovalEvents)
	}
	if report.EstimatedLivePromptKeys != 0 {
		t.Fatalf("estimated live prompt keys = %d, want 0", report.EstimatedLivePromptKeys)
	}
	if len(report.ReplayDriftClusters) != 1 {
		t.Fatalf("replay drift clusters = %d, want 1", len(report.ReplayDriftClusters))
	}
	if len(report.RemainingClusters) != 0 {
		t.Fatalf("remaining clusters = %d, want 0 for replay drift-only approval", len(report.RemainingClusters))
	}
}

func TestRunReplayEvents_HumanSummary(t *testing.T) {
	fuseHome := t.TempDir()
	t.Setenv("FUSE_HOME", fuseHome)
	if err := config.EnsureDirectories(); err != nil {
		t.Fatalf("EnsureDirectories: %v", err)
	}
	database, err := db.OpenDB(config.DBPath())
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	if err := database.LogEvent(&db.EventRecord{Command: "just lint", Decision: "CAUTION"}); err != nil {
		t.Fatalf("LogEvent: %v", err)
	}
	_ = database.Close()

	stdout, _, err := captureCLIOutput(t, func() error {
		return runReplayEvents(&replayEventsOptions{top: 5})
	})
	if err != nil {
		t.Fatalf("runReplayEvents: %v", err)
	}
	for _, want := range []string{"Classifier replay audit", "Events replayed: 1", "Old -> current decision matrix", "CAUTION", "SAFE"} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("expected output to contain %q, got:\n%s", want, stdout)
		}
	}
}
