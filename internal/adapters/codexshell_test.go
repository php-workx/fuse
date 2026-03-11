package adapters

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/runger/fuse/internal/config"
)

func withFuseHome(t *testing.T) string {
	t.Helper()
	fuseHome := filepath.Join(t.TempDir(), ".fuse")
	oldFuseHome := os.Getenv("FUSE_HOME")
	if err := os.Setenv("FUSE_HOME", fuseHome); err != nil {
		t.Fatalf("set FUSE_HOME: %v", err)
	}
	t.Cleanup(func() {
		if oldFuseHome == "" {
			_ = os.Unsetenv("FUSE_HOME")
			return
		}
		_ = os.Setenv("FUSE_HOME", oldFuseHome)
	})
	if err := config.EnsureDirectories(); err != nil {
		t.Fatalf("ensure directories: %v", err)
	}
	return fuseHome
}

func enableFuseForTest(t *testing.T) {
	t.Helper()
	if err := os.WriteFile(config.EnabledMarkerPath(), []byte("1"), 0o600); err != nil {
		t.Fatalf("write enabled marker: %v", err)
	}
}

func TestExecuteCapturedShellCommand_DoesNotInheritProcessStdin(t *testing.T) {
	origStdin := os.Stdin
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	if _, err := w.Write([]byte("transport-bytes")); err != nil {
		t.Fatalf("write pipe: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}
	os.Stdin = r
	defer func() {
		os.Stdin = origStdin
		_ = r.Close()
	}()

	result, err := executeCapturedShellCommand("cat", "", time.Second)
	if err != nil {
		t.Fatalf("executeCapturedShellCommand: %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", result.ExitCode)
	}
	if result.Stdout != "" {
		t.Fatalf("expected captured command to ignore process stdin, got %q", result.Stdout)
	}
}

func TestExecuteCodexShellCommand_AllowsBlockedCommandWhenDisabled(t *testing.T) {
	enabledMarker := config.EnabledMarkerPath()
	if err := os.Remove(enabledMarker); err != nil {
		t.Fatalf("remove enabled marker: %v", err)
	}
	defer func() {
		if err := os.WriteFile(enabledMarker, []byte("1"), 0o600); err != nil {
			t.Fatalf("restore enabled marker: %v", err)
		}
	}()

	targetDir := filepath.Join(t.TempDir(), "to-remove")
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		t.Fatalf("mkdir target: %v", err)
	}

	_, _, exitCode, err := executeCodexShellCommand("rm -rf "+targetDir, "", time.Minute)
	if err != nil {
		t.Fatalf("expected disabled mode to bypass classification, got error: %v", err)
	}
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}
	if _, err := os.Stat(targetDir); !os.IsNotExist(err) {
		t.Fatalf("expected target dir to be removed, stat err = %v", err)
	}
}

func TestExecuteCodexShellCommand_LogsAndPrunesEvents(t *testing.T) {
	withFuseHome(t)
	enableFuseForTest(t)
	configYAML := "max_event_log_rows: 1\n"
	if err := os.WriteFile(config.ConfigPath(), []byte(configYAML), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	for _, command := range []string{"printf one", "printf two"} {
		if _, _, exitCode, err := executeCodexShellCommand(command, "", time.Minute); err != nil {
			t.Fatalf("executeCodexShellCommand(%q): %v", command, err)
		} else if exitCode != 0 {
			t.Fatalf("executeCodexShellCommand(%q) exit code = %d", command, exitCode)
		}
	}

	sqlDB, err := sql.Open("sqlite", config.DBPath())
	if err != nil {
		t.Fatalf("open sqlite db: %v", err)
	}
	defer func() { _ = sqlDB.Close() }()

	var count int
	if err := sqlDB.QueryRow("SELECT COUNT(*) FROM events").Scan(&count); err != nil {
		t.Fatalf("count events: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected event log to be pruned to 1 row, got %d", count)
	}
}

func TestExecuteCodexShellCommand_EnabledSafeCommand(t *testing.T) {
	withFuseHome(t)
	enableFuseForTest(t)

	stdout, stderr, exitCode, err := executeCodexShellCommand("printf safe", "", time.Minute)
	if err != nil {
		t.Fatalf("executeCodexShellCommand: %v", err)
	}
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}
	if stdout != "safe" {
		t.Fatalf("stdout = %q, want %q", stdout, "safe")
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
}

func TestExecuteCodexShellCommand_EnabledBlockedCommand(t *testing.T) {
	withFuseHome(t)
	enableFuseForTest(t)

	stdout, stderr, exitCode, err := executeCodexShellCommand("rm -rf /", "", time.Minute)
	if err == nil {
		t.Fatal("expected blocked command to return an error")
	}
	if !strings.Contains(err.Error(), "fuse blocked command") {
		t.Fatalf("expected blocked error, got %v", err)
	}
	if stdout != "" || stderr != "" || exitCode != 0 {
		t.Fatalf("expected empty output and exit code 0, got stdout=%q stderr=%q exitCode=%d", stdout, stderr, exitCode)
	}
}

func TestExecuteCodexShellCommand_EnabledApprovalWithoutTTY(t *testing.T) {
	withFuseHome(t)
	enableFuseForTest(t)

	_, _, exitCode, err := executeCodexShellCommand("terraform destroy prod", "", time.Minute)
	if err == nil {
		t.Fatal("expected approval-required command without TTY to return an error")
	}
	if !strings.Contains(err.Error(), "NON_INTERACTIVE_MODE") {
		t.Fatalf("expected NON_INTERACTIVE_MODE error, got %v", err)
	}
	if exitCode != 0 {
		t.Fatalf("expected exit code 0 on approval error path, got %d", exitCode)
	}
}
