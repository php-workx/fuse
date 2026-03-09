package adapters

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunHook_SafeCommand(t *testing.T) {
	input := `{"tool_name":"Bash","tool_input":{"command":"ls -la"},"session_id":"test","cwd":"/tmp"}`
	stdin := strings.NewReader(input)
	stderr := &bytes.Buffer{}

	exitCode := RunHook(stdin, stderr)

	if exitCode != 0 {
		t.Errorf("expected exit code 0 for safe command, got %d", exitCode)
	}
}

func TestRunHook_BlockedCommand(t *testing.T) {
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
	input := `{"tool_name":"Read","tool_input":{"file_path":"/tmp/test.txt"},"session_id":"test","cwd":"/tmp"}`
	stdin := strings.NewReader(input)
	stderr := &bytes.Buffer{}

	exitCode := RunHook(stdin, stderr)

	if exitCode != 0 {
		t.Errorf("expected exit code 0 for non-Bash tool, got %d", exitCode)
	}
}

func TestRunHook_InvalidJSON(t *testing.T) {
	input := `{invalid json}`
	stdin := strings.NewReader(input)
	stderr := &bytes.Buffer{}

	exitCode := RunHook(stdin, stderr)

	if exitCode != 2 {
		t.Errorf("expected exit code 2 for invalid JSON (fail-closed), got %d", exitCode)
	}
}

func TestRunHook_EmptyCommand(t *testing.T) {
	input := `{"tool_name":"Bash","tool_input":{"command":""},"session_id":"test","cwd":"/tmp"}`
	stdin := strings.NewReader(input)
	stderr := &bytes.Buffer{}

	exitCode := RunHook(stdin, stderr)

	if exitCode != 0 {
		t.Errorf("expected exit code 0 for empty command, got %d", exitCode)
	}
}

func TestRunHook_MCP(t *testing.T) {
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
	// MCP tool with a destructive-prefix action should trigger caution/approval path.
	input := `{"tool_name":"mcp__server__delete_items","tool_input":{"id":"123"},"session_id":"test","cwd":"/tmp"}`
	stdin := strings.NewReader(input)
	stderr := &bytes.Buffer{}

	exitCode := RunHook(stdin, stderr)

	// delete_ prefix triggers APPROVAL, which in v1 auto-allows with CAUTION message.
	if exitCode != 0 {
		t.Errorf("expected exit code 0 for MCP approval tool (auto-allow in v1), got %d", exitCode)
	}

	stderrStr := stderr.String()
	if !strings.Contains(stderrStr, "CAUTION") {
		t.Errorf("expected stderr to contain CAUTION for destructive MCP tool, got: %s", stderrStr)
	}
}

func TestRunHook_MissingToolName(t *testing.T) {
	input := `{"tool_input":{"command":"ls"},"session_id":"test","cwd":"/tmp"}`
	stdin := strings.NewReader(input)
	stderr := &bytes.Buffer{}

	exitCode := RunHook(stdin, stderr)

	if exitCode != 2 {
		t.Errorf("expected exit code 2 for missing tool_name, got %d", exitCode)
	}
}

func TestRunHook_MissingToolInput(t *testing.T) {
	input := `{"tool_name":"Bash","session_id":"test","cwd":"/tmp"}`
	stdin := strings.NewReader(input)
	stderr := &bytes.Buffer{}

	exitCode := RunHook(stdin, stderr)

	if exitCode != 2 {
		t.Errorf("expected exit code 2 for missing tool_input, got %d", exitCode)
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
