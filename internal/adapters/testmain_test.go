package adapters

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMain(m *testing.M) {
	// Set up a temporary FUSE_HOME with the enabled marker so that
	// config.IsDisabled() returns false during tests.
	tmpDir, err := os.MkdirTemp("", "fuse-test-*")
	if err != nil {
		panic("failed to create temp dir: " + err.Error())
	}
	defer os.RemoveAll(tmpDir)

	stateDir := filepath.Join(tmpDir, "state")
	if err := os.MkdirAll(stateDir, 0700); err != nil {
		panic("failed to create state dir: " + err.Error())
	}
	if err := os.WriteFile(filepath.Join(stateDir, "enabled"), []byte("1"), 0600); err != nil {
		panic("failed to create enabled marker: " + err.Error())
	}

	os.Setenv("FUSE_HOME", tmpDir)
	defer os.Unsetenv("FUSE_HOME")

	os.Exit(m.Run())
}
