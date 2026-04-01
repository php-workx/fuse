package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProfileMigrationNotice_PrintsOnceForLegacyConfig(t *testing.T) {
	fuseHome := t.TempDir()
	t.Setenv("FUSE_HOME", fuseHome)

	configPath := filepath.Join(fuseHome, "config", "config.yaml")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	if err := os.WriteFile(configPath, []byte("log_level: warn\n"), 0o644); err != nil {
		t.Fatalf("write legacy config: %v", err)
	}

	stdout, stderr, err := captureCLIOutput(t, func() error {
		rootCmd.SetArgs([]string{"profile"})
		defer rootCmd.SetArgs(nil)
		return rootCmd.Execute()
	})
	if err != nil {
		t.Fatalf("first profile run: %v", err)
	}
	if !strings.Contains(stderr, profileMigrationNoticeText) {
		t.Fatalf("expected migration notice on first run, got stderr:\n%s", stderr)
	}
	if stdout == "" {
		t.Fatal("expected profile command output on stdout")
	}

	stdout, stderr, err = captureCLIOutput(t, func() error {
		rootCmd.SetArgs([]string{"profile"})
		defer rootCmd.SetArgs(nil)
		return rootCmd.Execute()
	})
	if err != nil {
		t.Fatalf("second profile run: %v", err)
	}
	if stderr != "" {
		t.Fatalf("expected no migration notice on second run, got stderr:\n%s", stderr)
	}
	if stdout == "" {
		t.Fatal("expected profile command output on stdout")
	}
}

func TestProfileMigrationNotice_SkipsConfiguredProfile(t *testing.T) {
	fuseHome := t.TempDir()
	t.Setenv("FUSE_HOME", fuseHome)

	configPath := filepath.Join(fuseHome, "config", "config.yaml")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	if err := os.WriteFile(configPath, []byte("profile: relaxed\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	_, stderr, err := captureCLIOutput(t, func() error {
		rootCmd.SetArgs([]string{"profile"})
		defer rootCmd.SetArgs(nil)
		return rootCmd.Execute()
	})
	if err != nil {
		t.Fatalf("profile run: %v", err)
	}
	if stderr != "" {
		t.Fatalf("expected no migration notice for configured profile, got stderr:\n%s", stderr)
	}
	if _, err := os.Stat(profileMigrationNoticeMarkerPath()); !os.IsNotExist(err) {
		t.Fatalf("expected no migration marker, got err=%v", err)
	}
}

func TestProfileMigrationNotice_SkipsHelp(t *testing.T) {
	fuseHome := t.TempDir()
	t.Setenv("FUSE_HOME", fuseHome)

	configPath := filepath.Join(fuseHome, "config", "config.yaml")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	if err := os.WriteFile(configPath, []byte("log_level: warn\n"), 0o644); err != nil {
		t.Fatalf("write legacy config: %v", err)
	}

	output := captureHelp(t, "--help")
	if strings.Contains(output, profileMigrationNoticeText) {
		t.Fatalf("expected help output to skip migration notice, got:\n%s", output)
	}
}

func TestProfileMigrationNotice_MarkerWriteFailureDoesNotAbortCommand(t *testing.T) {
	fuseHome := t.TempDir()
	t.Setenv("FUSE_HOME", fuseHome)

	configPath := filepath.Join(fuseHome, "config", "config.yaml")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	if err := os.WriteFile(configPath, []byte("log_level: warn\n"), 0o644); err != nil {
		t.Fatalf("write legacy config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(fuseHome, "state"), []byte("not-a-dir\n"), 0o644); err != nil {
		t.Fatalf("write blocking state path: %v", err)
	}

	stdout, stderr, err := captureCLIOutput(t, func() error {
		rootCmd.SetArgs([]string{"profile"})
		defer rootCmd.SetArgs(nil)
		return rootCmd.Execute()
	})
	if err != nil {
		t.Fatalf("profile run should tolerate marker write failure, got: %v", err)
	}
	if stdout == "" {
		t.Fatal("expected profile command output on stdout")
	}
	if !strings.Contains(stderr, profileMigrationNoticeText) {
		t.Fatalf("expected migration notice on stderr, got:\n%s", stderr)
	}
	if !strings.Contains(stderr, "warning: unable to persist profile migration notice") {
		t.Fatalf("expected warning about marker write failure, got:\n%s", stderr)
	}
}
