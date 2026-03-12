package adapters

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunHook_SafeCommand(t *testing.T) {
	enableHookForTest(t)

	input := `{"tool_name":"Bash","tool_input":{"command":"ls -la"},"session_id":"test","cwd":"/tmp"}`
	stdin := strings.NewReader(input)
	stderr := &bytes.Buffer{}

	exitCode := RunHook(stdin, stderr)

	if exitCode != 0 {
		t.Errorf("expected exit code 0 for safe command, got %d", exitCode)
	}
}

func TestRunHook_BlockedCommand(t *testing.T) {
	enableHookForTest(t)

	input := `{"tool_name":"Bash","tool_input":{"command":"rm -rf /"},"session_id":"test","cwd":"/tmp"}`
	stdin := strings.NewReader(input)
	stderr := &bytes.Buffer{}

	exitCode := RunHook(stdin, stderr)

	if exitCode != 2 {
		t.Errorf("expected exit code 2 for blocked command, got %d", exitCode)
	}

	stderrStr := stderr.String()
	if stderrStr == "" {
		t.Error("expected stderr to contain block reason, got empty string")
	}
	if !strings.Contains(stderrStr, "fuse:POLICY_BLOCK") {
		t.Errorf("expected stderr to contain 'fuse:POLICY_BLOCK', got: %s", stderrStr)
	}
}

func TestRunHook_NonBashTool(t *testing.T) {
	enableHookForTest(t)

	input := `{"tool_name":"Glob","tool_input":{"pattern":"*.go"},"session_id":"test","cwd":"/tmp"}`
	stdin := strings.NewReader(input)
	stderr := &bytes.Buffer{}

	exitCode := RunHook(stdin, stderr)

	if exitCode != 0 {
		t.Errorf("expected exit code 0 for non-Bash tool, got %d", exitCode)
	}
}

func TestRunHook_InvalidJSON(t *testing.T) {
	enableHookForTest(t)

	input := `{invalid json}`
	stdin := strings.NewReader(input)
	stderr := &bytes.Buffer{}

	exitCode := RunHook(stdin, stderr)

	if exitCode != 2 {
		t.Errorf("expected exit code 2 for invalid JSON (fail-closed), got %d", exitCode)
	}
}

func TestRunHook_EmptyCommand(t *testing.T) {
	enableHookForTest(t)

	input := `{"tool_name":"Bash","tool_input":{"command":""},"session_id":"test","cwd":"/tmp"}`
	stdin := strings.NewReader(input)
	stderr := &bytes.Buffer{}

	exitCode := RunHook(stdin, stderr)

	if exitCode != 2 {
		t.Errorf("expected exit code 2 for empty command, got %d", exitCode)
	}

	stderrStr := stderr.String()
	if !strings.Contains(stderrStr, "fuse:POLICY_BLOCK") {
		t.Errorf("expected stderr to contain 'fuse:POLICY_BLOCK', got: %s", stderrStr)
	}
}

func TestRunHook_MCP(t *testing.T) {
	enableHookForTest(t)

	// MCP tool with a safe-prefix action should be allowed.
	input := `{"tool_name":"mcp__server__list_items","tool_input":{"query":"test"},"session_id":"test","cwd":"/tmp"}`
	stdin := strings.NewReader(input)
	stderr := &bytes.Buffer{}

	exitCode := RunHook(stdin, stderr)

	if exitCode != 0 {
		t.Errorf("expected exit code 0 for MCP safe tool, got %d", exitCode)
	}
}

func TestRunHook_MCP_DestructiveAction(t *testing.T) {
	enableHookForTest(t)
	t.Setenv("FUSE_NON_INTERACTIVE", "1")
	// MCP tool with a destructive-prefix action should trigger caution/approval path.
	input := `{"tool_name":"mcp__server__delete_items","tool_input":{"id":"123"},"session_id":"test","cwd":"/tmp"}`
	stdin := strings.NewReader(input)
	stderr := &bytes.Buffer{}

	exitCode := RunHook(stdin, stderr)

	if exitCode != 2 {
		t.Errorf("expected exit code 2 for MCP approval tool without interactive tty, got %d", exitCode)
	}

	stderrStr := stderr.String()
	if !strings.Contains(stderrStr, "NON_INTERACTIVE_MODE") && !strings.Contains(stderrStr, "USER_DENIED") && !strings.Contains(stderrStr, "TIMEOUT_WAITING_FOR_USER") {
		t.Errorf("expected stderr to contain an approval-denial directive, got: %s", stderrStr)
	}
}

func TestRunHook_MissingToolName(t *testing.T) {
	enableHookForTest(t)

	input := `{"tool_input":{"command":"ls"},"session_id":"test","cwd":"/tmp"}`
	stdin := strings.NewReader(input)
	stderr := &bytes.Buffer{}

	exitCode := RunHook(stdin, stderr)

	if exitCode != 2 {
		t.Errorf("expected exit code 2 for missing tool_name, got %d", exitCode)
	}
}

func TestRunHook_MissingToolInput(t *testing.T) {
	enableHookForTest(t)

	input := `{"tool_name":"Bash","session_id":"test","cwd":"/tmp"}`
	stdin := strings.NewReader(input)
	stderr := &bytes.Buffer{}

	exitCode := RunHook(stdin, stderr)

	if exitCode != 2 {
		t.Errorf("expected exit code 2 for missing tool_input, got %d", exitCode)
	}
}

func TestRunHook_DryRunAllowsBlockedCommand(t *testing.T) {
	withFuseHome(t)
	enableDryRunForTest(t)

	input := `{"tool_name":"Bash","tool_input":{"command":"rm -rf /"},"session_id":"dry-run-test","cwd":"/tmp"}`
	stdin := strings.NewReader(input)
	stderr := &bytes.Buffer{}

	exitCode := RunHook(stdin, stderr)

	// In dry-run mode, even BLOCKED commands return exit 0 (allow).
	if exitCode != 0 {
		t.Errorf("expected exit code 0 in dry-run mode, got %d; stderr: %s", exitCode, stderr.String())
	}
}

func TestRunHook_DryRunAllowsApprovalCommand(t *testing.T) {
	withFuseHome(t)
	enableDryRunForTest(t)

	input := `{"tool_name":"mcp__server__delete_items","tool_input":{"id":"123"},"session_id":"dry-run-test","cwd":"/tmp"}`
	stdin := strings.NewReader(input)
	stderr := &bytes.Buffer{}

	exitCode := RunHook(stdin, stderr)

	if exitCode != 0 {
		t.Errorf("expected exit code 0 in dry-run mode for MCP approval, got %d; stderr: %s", exitCode, stderr.String())
	}
}

func TestRunHook_DisabledPassesThrough(t *testing.T) {
	withFuseHome(t)
	// Neither enabled nor dry-run — fully disabled.

	input := `{"tool_name":"Bash","tool_input":{"command":"rm -rf /"},"session_id":"disabled-test","cwd":"/tmp"}`
	stdin := strings.NewReader(input)
	stderr := &bytes.Buffer{}

	exitCode := RunHook(stdin, stderr)
	if exitCode != 0 {
		t.Errorf("expected exit code 0 when disabled, got %d", exitCode)
	}
	if stderr.Len() != 0 {
		t.Errorf("expected no stderr when disabled, got: %s", stderr.String())
	}
}

func TestExtractMCPAction(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"standard format", "mcp__server__list_items", "list_items"},
		{"nested server name", "mcp__my_server__delete_records", "delete_records"},
		{"no second separator", "mcp__simple", "simple"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractMCPAction(tt.input)
			if got != tt.expected {
				t.Errorf("extractMCPAction(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestRunHook_NativeFileReadSafe(t *testing.T) {
	enableHookForTest(t)
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	input := `{"tool_name":"Read","tool_input":{"file_path":"docs/readme.md"},"session_id":"test","cwd":"/tmp/project"}`
	stdin := strings.NewReader(input)
	stderr := &bytes.Buffer{}

	exitCode := RunHook(stdin, stderr)

	if exitCode != 0 {
		t.Fatalf("expected exit code 0 for safe file read, got %d with stderr %q", exitCode, stderr.String())
	}
}

func TestRunHook_NativeFileReadSecretRequiresApproval(t *testing.T) {
	enableHookForTest(t)
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	input := `{"tool_name":"Read","tool_input":{"file_path":".env"},"session_id":"test","cwd":"/tmp/project"}`
	stdin := strings.NewReader(input)
	stderr := &bytes.Buffer{}

	exitCode := RunHook(stdin, stderr)

	if exitCode != 2 {
		t.Fatalf("expected exit code 2 for approval-required file read without interactive tty, got %d", exitCode)
	}
	stderrStr := stderr.String()
	if !strings.Contains(stderrStr, "NON_INTERACTIVE_MODE") && !strings.Contains(stderrStr, "USER_DENIED") && !strings.Contains(stderrStr, "TIMEOUT_WAITING_FOR_USER") {
		t.Fatalf("expected approval denial directive, got %q", stderrStr)
	}
}

func TestRunHook_NativeFileProtectedPathsAreBlocked(t *testing.T) {
	enableHookForTest(t)
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	tests := []struct {
		name     string
		toolName string
		input    string
	}{
		{
			name:     "write fuse state blocked",
			toolName: "Write",
			input:    `{"file_path":"~/.fuse/state/fuse.db","content":"nope"}`,
		},
		{
			name:     "edit project claude settings blocked",
			toolName: "Edit",
			input:    `{"file_path":".claude/settings.json","old_string":"a","new_string":"b"}`,
		},
		{
			name:     "multiedit claude settings blocked",
			toolName: "MultiEdit",
			input:    `{"file_path":".claude/settings.json","edits":[{"old_string":"a","new_string":"b"}]}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := `{"tool_name":"` + tt.toolName + `","tool_input":` + tt.input + `,"session_id":"test","cwd":"/tmp/project"}`
			stdin := strings.NewReader(input)
			stderr := &bytes.Buffer{}

			exitCode := RunHook(stdin, stderr)

			if exitCode != 2 {
				t.Fatalf("expected exit code 2 for blocked native file tool, got %d", exitCode)
			}
			if !strings.Contains(stderr.String(), "fuse:POLICY_BLOCK") {
				t.Fatalf("expected policy block directive, got %q", stderr.String())
			}
		})
	}
}

func TestRunHook_NativeFileAbsoluteProjectConfigPathsAreBlocked(t *testing.T) {
	enableHookForTest(t)
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	projectDir := t.TempDir()
	subdir := filepath.Join(projectDir, "subdir")
	tests := []struct {
		name     string
		toolName string
		path     string
		cwd      string
	}{
		{
			name:     "subdir relative parent claude settings blocked",
			toolName: "Edit",
			path:     "../.claude/settings.json",
			cwd:      subdir,
		},
		{
			name:     "absolute project claude settings blocked",
			toolName: "Edit",
			path:     filepath.Join(projectDir, ".claude", "settings.json"),
			cwd:      subdir,
		},
		{
			name:     "subdir relative parent codex config blocked",
			toolName: "Write",
			path:     "../.codex/config.toml",
			cwd:      subdir,
		},
		{
			name:     "absolute project codex config blocked",
			toolName: "Write",
			path:     filepath.Join(projectDir, ".codex", "config.toml"),
			cwd:      subdir,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := `{"tool_name":"` + tt.toolName + `","tool_input":{"file_path":"` + filepath.ToSlash(tt.path) + `"},"session_id":"test","cwd":"` + filepath.ToSlash(tt.cwd) + `"}`
			stdin := strings.NewReader(input)
			stderr := &bytes.Buffer{}

			exitCode := RunHook(stdin, stderr)

			if exitCode != 2 {
				t.Fatalf("expected exit code 2 for blocked absolute project config path, got %d", exitCode)
			}
			if !strings.Contains(stderr.String(), "fuse:POLICY_BLOCK") {
				t.Fatalf("expected policy block directive, got %q", stderr.String())
			}
		})
	}
}

func TestRunHook_NativeFileIgnoresNestedMetadataPaths(t *testing.T) {
	enableHookForTest(t)
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	input := `{"tool_name":"Read","tool_input":{"file_path":"docs/readme.md","metadata":{"path":".env","file_path":"secrets/prod.env"}},"session_id":"test","cwd":"/tmp/project"}`
	stdin := strings.NewReader(input)
	stderr := &bytes.Buffer{}

	exitCode := RunHook(stdin, stderr)

	if exitCode != 0 {
		t.Fatalf("expected exit code 0 when only nested metadata mentions sensitive paths, got %d with stderr %q", exitCode, stderr.String())
	}
}

func enableHookForTest(t *testing.T) {
	t.Helper()

	fuseHome := t.TempDir()
	t.Setenv("FUSE_HOME", fuseHome)
	enabledPath := filepath.Join(fuseHome, "state", "enabled")
	if err := os.MkdirAll(filepath.Dir(enabledPath), 0o755); err != nil {
		t.Fatalf("mkdir state dir: %v", err)
	}
	if err := os.WriteFile(enabledPath, []byte("enabled\n"), 0o644); err != nil {
		t.Fatalf("write enabled marker: %v", err)
	}
}
