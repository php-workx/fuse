package db

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLogEvent_PersistsObservabilityFields(t *testing.T) {
	d := openTestDB(t)

	err := d.LogEvent(EventRecord{
		SessionID:     "claude-session-1",
		Command:       "git status",
		Decision:      "SAFE",
		RuleID:        "builtin:git:status",
		Reason:        "read-only git command",
		DurationMs:    12,
		Metadata:      "executed",
		Source:        "codex-shell",
		Agent:         "codex",
		Cwd:           "/tmp/work/repo/subdir",
		WorkspaceRoot: "/tmp/work/repo",
	})
	if err != nil {
		t.Fatalf("LogEvent: %v", err)
	}

	events, err := d.ListEvents(EventFilter{Limit: 10})
	if err != nil {
		t.Fatalf("ListEvents: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("len(events) = %d, want 1", len(events))
	}

	event := events[0]
	if event.Source != "codex-shell" {
		t.Fatalf("Source = %q, want %q", event.Source, "codex-shell")
	}
	if event.Agent != "codex" {
		t.Fatalf("Agent = %q, want %q", event.Agent, "codex")
	}
	if event.Cwd != "/tmp/work/repo/subdir" {
		t.Fatalf("Cwd = %q, want %q", event.Cwd, "/tmp/work/repo/subdir")
	}
	if event.WorkspaceRoot != "/tmp/work/repo" {
		t.Fatalf("WorkspaceRoot = %q, want %q", event.WorkspaceRoot, "/tmp/work/repo")
	}
}

func TestSummarizeEvents_AggregatesByDecisionAgentSourceAndWorkspace(t *testing.T) {
	d := openTestDB(t)

	records := []EventRecord{
		{
			Command:       "git status",
			Decision:      "SAFE",
			Source:        "codex-shell",
			Agent:         "codex",
			Cwd:           "/tmp/repo-a",
			WorkspaceRoot: "/tmp/repo-a",
		},
		{
			Command:       "rm -rf /",
			Decision:      "BLOCKED",
			Source:        "codex-shell",
			Agent:         "codex",
			Cwd:           "/tmp/repo-a",
			WorkspaceRoot: "/tmp/repo-a",
		},
		{
			Command:       "terraform destroy prod",
			Decision:      "APPROVAL",
			Source:        "hook",
			Agent:         "claude",
			Cwd:           "/tmp/repo-b",
			WorkspaceRoot: "/tmp/repo-b",
		},
	}
	for i, record := range records {
		if err := d.LogEvent(record); err != nil {
			t.Fatalf("LogEvent(%d): %v", i, err)
		}
	}

	summary, err := d.SummarizeEvents()
	if err != nil {
		t.Fatalf("SummarizeEvents: %v", err)
	}

	if summary.Total != 3 {
		t.Fatalf("Total = %d, want 3", summary.Total)
	}
	if summary.ByDecision["SAFE"] != 1 || summary.ByDecision["BLOCKED"] != 1 || summary.ByDecision["APPROVAL"] != 1 {
		t.Fatalf("unexpected decision counts: %#v", summary.ByDecision)
	}
	if summary.ByAgent["codex"] != 2 || summary.ByAgent["claude"] != 1 {
		t.Fatalf("unexpected agent counts: %#v", summary.ByAgent)
	}
	if summary.BySource["codex-shell"] != 2 || summary.BySource["hook"] != 1 {
		t.Fatalf("unexpected source counts: %#v", summary.BySource)
	}
	if summary.ByWorkspace["/tmp/repo-a"] != 2 || summary.ByWorkspace["/tmp/repo-b"] != 1 {
		t.Fatalf("unexpected workspace counts: %#v", summary.ByWorkspace)
	}
}

func TestLogEvent_DetectsWorkspaceRootFromNestedCwd(t *testing.T) {
	d := openTestDB(t)

	workspaceRoot := filepath.Join(t.TempDir(), "repo")
	nested := filepath.Join(workspaceRoot, "cmd", "service")
	if err := os.MkdirAll(filepath.Join(workspaceRoot, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatalf("mkdir nested cwd: %v", err)
	}

	if err := d.LogEvent(EventRecord{
		Command:  "go test ./...",
		Decision: "SAFE",
		Source:   "codex-shell",
		Agent:    "codex",
		Cwd:      nested,
	}); err != nil {
		t.Fatalf("LogEvent: %v", err)
	}

	events, err := d.ListEvents(EventFilter{Limit: 1})
	if err != nil {
		t.Fatalf("ListEvents: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("len(events) = %d, want 1", len(events))
	}
	wantRoot := normalizeEventPath(workspaceRoot)
	if events[0].WorkspaceRoot != wantRoot {
		t.Fatalf("WorkspaceRoot = %q, want %q", events[0].WorkspaceRoot, wantRoot)
	}
}
