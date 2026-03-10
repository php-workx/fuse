package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunDoctor_ReportsMCPProxyChecks(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("FUSE_HOME", tmpDir)

	if err := os.MkdirAll(filepath.Dir(configPathForTest(t)), 0755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	configYAML := "mcp_proxies:\n  - name: missing\n    command: definitely-not-a-command\n    args: []\n    env: {}\n"
	if err := os.WriteFile(configPathForTest(t), []byte(configYAML), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	stdout, stderr, err := captureDoctorOutput(t, func() error {
		return runDoctor(false)
	})
	if err == nil {
		t.Fatal("expected doctor to return an error for a missing downstream command")
	}
	if stderr != "" {
		t.Fatalf("expected no stderr, got %q", stderr)
	}
	if !strings.Contains(stdout, "MCP proxy") {
		t.Fatalf("expected doctor output to include MCP proxy diagnostics, got:\n%s", stdout)
	}
}

func TestRunDoctorLive_ReportsTerminalCapabilityChecks(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("FUSE_HOME", tmpDir)

	stdout, _, err := captureDoctorOutput(t, func() error {
		return runDoctor(true)
	})
	if err != nil {
		t.Fatalf("unexpected doctor --live error: %v", err)
	}
	for _, want := range []string{"/dev/tty", "foreground process-group"} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("expected doctor --live output to include %q, got:\n%s", want, stdout)
		}
	}
}

func captureDoctorOutput(t *testing.T, fn func() error) (string, string, error) {
	t.Helper()

	origStdout := os.Stdout
	origStderr := os.Stderr

	stdoutR, stdoutW, err := os.Pipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	stderrR, stderrW, err := os.Pipe()
	if err != nil {
		t.Fatalf("stderr pipe: %v", err)
	}

	os.Stdout = stdoutW
	os.Stderr = stderrW
	defer func() {
		os.Stdout = origStdout
		os.Stderr = origStderr
	}()

	runErr := fn()
	_ = stdoutW.Close()
	_ = stderrW.Close()

	var stdoutBuf bytes.Buffer
	var stderrBuf bytes.Buffer
	if _, err := stdoutBuf.ReadFrom(stdoutR); err != nil {
		t.Fatalf("read stdout: %v", err)
	}
	if _, err := stderrBuf.ReadFrom(stderrR); err != nil {
		t.Fatalf("read stderr: %v", err)
	}
	return stdoutBuf.String(), stderrBuf.String(), runErr
}

func configPathForTest(t *testing.T) string {
	t.Helper()
	fuseHome := os.Getenv("FUSE_HOME")
	if fuseHome == "" {
		t.Fatal("FUSE_HOME not set")
	}
	return filepath.Join(fuseHome, "config", "config.yaml")
}
