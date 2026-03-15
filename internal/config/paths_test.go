package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMode_Disabled(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("FUSE_HOME", dir)
	if err := os.MkdirAll(StateDir(), 0o700); err != nil {
		t.Fatal(err)
	}
	if m := Mode(); m != ModeDisabled {
		t.Errorf("Mode() = %d, want ModeDisabled (%d)", m, ModeDisabled)
	}
}

func TestMode_Enabled(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("FUSE_HOME", dir)
	if err := os.MkdirAll(StateDir(), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(EnabledMarkerPath(), []byte("1"), 0o600); err != nil {
		t.Fatal(err)
	}
	if m := Mode(); m != ModeEnabled {
		t.Errorf("Mode() = %d, want ModeEnabled (%d)", m, ModeEnabled)
	}
}

func TestMode_DryRun(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("FUSE_HOME", dir)
	if err := os.MkdirAll(StateDir(), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(DryRunMarkerPath(), []byte("1"), 0o600); err != nil {
		t.Fatal(err)
	}
	if m := Mode(); m != ModeDryRun {
		t.Errorf("Mode() = %d, want ModeDryRun (%d)", m, ModeDryRun)
	}
}

func TestMode_EnabledTakesPrecedence(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("FUSE_HOME", dir)
	if err := os.MkdirAll(StateDir(), 0o700); err != nil {
		t.Fatal(err)
	}
	// Both markers — enabled wins.
	_ = os.WriteFile(EnabledMarkerPath(), []byte("1"), 0o600)
	_ = os.WriteFile(DryRunMarkerPath(), []byte("1"), 0o600)
	if m := Mode(); m != ModeEnabled {
		t.Errorf("Mode() = %d, want ModeEnabled when both markers exist", m)
	}
}

func TestDryRunMarkerPath(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("FUSE_HOME", dir)
	want := filepath.Join(dir, "state", "dryrun")
	if got := DryRunMarkerPath(); got != want {
		t.Errorf("DryRunMarkerPath() = %q, want %q", got, want)
	}
}
