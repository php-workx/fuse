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
