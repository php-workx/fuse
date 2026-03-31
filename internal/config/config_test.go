package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfig_RelaxedProfile(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Profile != ProfileRelaxed {
		t.Fatalf("Profile = %q, want %q", cfg.Profile, ProfileRelaxed)
	}
	if cfg.CautionFallback != "log" {
		t.Fatalf("CautionFallback = %q, want log", cfg.CautionFallback)
	}
	if cfg.LLMJudge.Mode != "off" {
		t.Fatalf("LLMJudge.Mode = %q, want off", cfg.LLMJudge.Mode)
	}
	if len(cfg.LLMJudge.TriggerDecisions) != 0 {
		t.Fatalf("TriggerDecisions = %#v, want empty", cfg.LLMJudge.TriggerDecisions)
	}
}

func TestLoadConfig_NoFileUsesRelaxedDefaults(t *testing.T) {
	cfg, err := LoadConfig(filepath.Join(t.TempDir(), "missing.yaml"))
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.Profile != ProfileRelaxed {
		t.Fatalf("Profile = %q, want %q", cfg.Profile, ProfileRelaxed)
	}
}

func TestLoadConfig_LegacyActiveJudgeMigratesToBalanced(t *testing.T) {
	path := writeConfigFixture(t, `
llm_judge:
  mode: active
`)

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.Profile != ProfileBalanced {
		t.Fatalf("Profile = %q, want %q", cfg.Profile, ProfileBalanced)
	}
	if cfg.LLMJudge.Mode != "active" {
		t.Fatalf("LLMJudge.Mode = %q, want active", cfg.LLMJudge.Mode)
	}
	if got, want := cfg.LLMJudge.DowngradeThreshold, 0.9; got != want {
		t.Fatalf("DowngradeThreshold = %v, want %v", got, want)
	}
	if len(cfg.LLMJudge.TriggerDecisions) != 2 {
		t.Fatalf("TriggerDecisions = %#v, want 2 entries", cfg.LLMJudge.TriggerDecisions)
	}
}

func TestLoadConfig_ProfileDefaultsThenExplicitOverrides(t *testing.T) {
	path := writeConfigFixture(t, `
profile: strict
caution_fallback: approve
llm_judge:
  trigger_decisions:
    - approval
`)

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.Profile != ProfileStrict {
		t.Fatalf("Profile = %q, want %q", cfg.Profile, ProfileStrict)
	}
	if cfg.CautionFallback != "approve" {
		t.Fatalf("CautionFallback = %q, want approve", cfg.CautionFallback)
	}
	if len(cfg.LLMJudge.TriggerDecisions) != 1 || cfg.LLMJudge.TriggerDecisions[0] != "approval" {
		t.Fatalf("TriggerDecisions = %#v, want [approval]", cfg.LLMJudge.TriggerDecisions)
	}
}

func TestLoadConfig_StrictUsesCautionOnlyTriggers(t *testing.T) {
	path := writeConfigFixture(t, `
profile: strict
`)

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.Profile != ProfileStrict {
		t.Fatalf("Profile = %q, want %q", cfg.Profile, ProfileStrict)
	}
	if cfg.LLMJudge.Mode != "active" {
		t.Fatalf("LLMJudge.Mode = %q, want active", cfg.LLMJudge.Mode)
	}
	if len(cfg.LLMJudge.TriggerDecisions) != 1 || cfg.LLMJudge.TriggerDecisions[0] != "caution" {
		t.Fatalf("TriggerDecisions = %#v, want [caution]", cfg.LLMJudge.TriggerDecisions)
	}
}

func writeConfigFixture(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write config fixture: %v", err)
	}
	return path
}
