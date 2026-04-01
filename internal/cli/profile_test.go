package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/php-workx/fuse/internal/config"
)

func TestRunProfile_ReportsCurrentProfileAndEffectiveSettings(t *testing.T) {
	fuseHome := t.TempDir()
	t.Setenv("FUSE_HOME", fuseHome)

	if err := config.EnsureDirectories(); err != nil {
		t.Fatalf("EnsureDirectories: %v", err)
	}
	path := config.ConfigPath()
	if err := writeProfileConfigFixture(path, `
profile: strict
caution_fallback: approve
llm_judge:
  provider: claude
  timeout: 15s
`); err != nil {
		t.Fatalf("write config fixture: %v", err)
	}

	stdout, stderr, err := captureCLIOutput(t, func() error {
		rootCmd.SetArgs([]string{"profile"})
		defer rootCmd.SetArgs(nil)
		return rootCmd.Execute()
	})
	if err != nil {
		t.Fatalf("profile command: %v", err)
	}
	for _, want := range []string{
		"Current profile: strict",
		"caution_fallback: approve",
		"llm_judge.provider: claude",
		"llm_judge.timeout: 15s",
	} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("expected profile output to contain %q, got:\n%s", want, stdout)
		}
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
}

func TestRunProfileSet_UpdatesConfigAndPreservesExistingSettings(t *testing.T) {
	fuseHome := t.TempDir()
	t.Setenv("FUSE_HOME", fuseHome)

	if err := config.EnsureDirectories(); err != nil {
		t.Fatalf("EnsureDirectories: %v", err)
	}
	path := config.ConfigPath()
	if err := writeProfileConfigFixture(path, `
profile: relaxed
log_level: info
max_event_log_rows: 42
caution_fallback: approve
llm_judge:
  mode: active
  provider: codex
  model: o4-mini
  timeout: 15s
  trigger_decisions:
    - caution
    - approval
`); err != nil {
		t.Fatalf("write config fixture: %v", err)
	}

	stdout, stderr, err := captureCLIOutput(t, func() error {
		rootCmd.SetArgs([]string{"profile", "set", "balanced"})
		defer rootCmd.SetArgs(nil)
		return rootCmd.Execute()
	})
	if err != nil {
		t.Fatalf("profile set command: %v", err)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
	if !strings.Contains(stdout, "profile set to balanced") {
		t.Fatalf("expected confirmation output, got:\n%s", stdout)
	}

	cfg, err := config.LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig after update: %v", err)
	}
	if cfg.Profile != config.ProfileBalanced {
		t.Fatalf("Profile = %q, want %q", cfg.Profile, config.ProfileBalanced)
	}
	if cfg.LogLevel != "info" {
		t.Fatalf("LogLevel = %q, want info", cfg.LogLevel)
	}
	if cfg.MaxEventLogRows != 42 {
		t.Fatalf("MaxEventLogRows = %d, want 42", cfg.MaxEventLogRows)
	}
	if cfg.CautionFallback != "approve" {
		t.Fatalf("CautionFallback = %q, want approve", cfg.CautionFallback)
	}
	if cfg.LLMJudge.Mode != "active" {
		t.Fatalf("LLMJudge.Mode = %q, want active", cfg.LLMJudge.Mode)
	}
	if cfg.LLMJudge.Provider != "codex" {
		t.Fatalf("LLMJudge.Provider = %q, want codex", cfg.LLMJudge.Provider)
	}
	if cfg.LLMJudge.Model != "o4-mini" {
		t.Fatalf("LLMJudge.Model = %q, want o4-mini", cfg.LLMJudge.Model)
	}
	if cfg.LLMJudge.Timeout != "15s" {
		t.Fatalf("LLMJudge.Timeout = %q, want 15s", cfg.LLMJudge.Timeout)
	}
}

func TestRunProfileSet_RejectsInvalidProfileName(t *testing.T) {
	fuseHome := t.TempDir()
	t.Setenv("FUSE_HOME", fuseHome)

	if err := config.EnsureDirectories(); err != nil {
		t.Fatalf("EnsureDirectories: %v", err)
	}
	path := config.ConfigPath()
	original := `
profile: relaxed
log_level: warn
`
	if err := writeProfileConfigFixture(path, original); err != nil {
		t.Fatalf("write config fixture: %v", err)
	}

	stdout, stderr, err := captureCLIOutput(t, func() error {
		rootCmd.SetArgs([]string{"profile", "set", "turbo"})
		defer rootCmd.SetArgs(nil)
		return rootCmd.Execute()
	})
	if err == nil {
		t.Fatal("expected profile set to reject invalid name")
	}
	if stdout != "" {
		t.Fatalf("stdout = %q, want empty", stdout)
	}
	if !strings.Contains(stderr, "invalid profile") || !strings.Contains(stderr, "relaxed") {
		t.Fatalf("expected invalid profile error, got stderr:\n%s", stderr)
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config after failed update: %v", err)
	}
	want := strings.TrimSpace(original) + "\n"
	if string(after) != want {
		t.Fatalf("config changed after failed update:\n--- got ---\n%s\n--- want ---\n%s", string(after), want)
	}
}

func TestProfileHelpEntryExists(t *testing.T) {
	withNoColor(t)
	output := captureHelp(t, "--help")
	if !strings.Contains(output, "profile") {
		t.Fatalf("expected root help to list profile command, got:\n%s", output)
	}
}

func writeProfileConfigFixture(path, contents string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(strings.TrimSpace(contents)+"\n"), 0o644)
}
