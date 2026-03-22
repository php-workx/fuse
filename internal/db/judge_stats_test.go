package db

import (
	"path/filepath"
	"testing"
)

func TestSummarizeJudgeAccuracy_Empty(t *testing.T) {
	dir := t.TempDir()
	d, err := OpenDB(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	defer d.Close()

	s, err := d.SummarizeJudgeAccuracy()
	if err != nil {
		t.Fatalf("SummarizeJudgeAccuracy: %v", err)
	}
	if s.Evaluated != 0 {
		t.Errorf("Evaluated = %d, want 0", s.Evaluated)
	}
	if s.Errors != 0 {
		t.Errorf("Errors = %d, want 0", s.Errors)
	}
}

func TestSummarizeJudgeAccuracy_Mixed(t *testing.T) {
	dir := t.TempDir()
	d, err := OpenDB(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	defer d.Close()

	// Agreed: judge says CAUTION, original is CAUTION.
	_ = d.LogEvent(&EventRecord{
		Command:         "git push origin feat/x",
		Decision:        "CAUTION",
		JudgeDecision:   "CAUTION",
		JudgeConfidence: 0.9,
		JudgeProvider:   "claude",
		JudgeLatencyMs:  200,
	})

	// Would upgrade: judge says APPROVAL, original is CAUTION.
	_ = d.LogEvent(&EventRecord{
		Command:         "rm -rf /important",
		Decision:        "CAUTION",
		JudgeDecision:   "APPROVAL",
		JudgeConfidence: 0.85,
		JudgeProvider:   "claude",
		JudgeLatencyMs:  300,
	})

	// Would downgrade: judge says SAFE, original is APPROVAL.
	_ = d.LogEvent(&EventRecord{
		Command:         "ls -la",
		Decision:        "APPROVAL",
		JudgeDecision:   "SAFE",
		JudgeConfidence: 0.95,
		JudgeProvider:   "claude",
		JudgeLatencyMs:  150,
	})

	// Error: judge failed (no judge_decision, just error).
	_ = d.LogEvent(&EventRecord{
		Command:    "echo test",
		Decision:   "CAUTION",
		JudgeError: "connection timeout",
	})

	s, err := d.SummarizeJudgeAccuracy()
	if err != nil {
		t.Fatalf("SummarizeJudgeAccuracy: %v", err)
	}

	if s.Evaluated != 4 {
		t.Errorf("Evaluated = %d, want 4", s.Evaluated)
	}
	if s.Agreed != 1 {
		t.Errorf("Agreed = %d, want 1", s.Agreed)
	}
	if s.WouldUpgrade != 1 {
		t.Errorf("WouldUpgrade = %d, want 1", s.WouldUpgrade)
	}
	if s.WouldDowngrade != 1 {
		t.Errorf("WouldDowngrade = %d, want 1", s.WouldDowngrade)
	}
	if s.Errors != 1 {
		t.Errorf("Errors = %d, want 1", s.Errors)
	}
	if s.AvgLatencyMs <= 0 {
		t.Errorf("AvgLatencyMs = %f, want > 0", s.AvgLatencyMs)
	}
}
