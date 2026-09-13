package adapters

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/php-workx/fuse/internal/config"
)

// maxAgentDirectiveBytes is the upper bound the tests hold agent-visible
// messages to. A deny reason is replayed into the agent's context on every
// later turn (DCG v0.13.0), so it must not grow with attacker-controlled input.
const maxAgentDirectiveBytes = 1024

func hookInput(t *testing.T, toolName string, toolInput map[string]any) string {
	t.Helper()
	b, err := json.Marshal(map[string]any{
		"tool_name":  toolName,
		"tool_input": toolInput,
		"session_id": "test",
		"cwd":        "/tmp/project",
	})
	if err != nil {
		t.Fatalf("marshal hook input: %v", err)
	}
	return string(b)
}

func TestRunHook_NativeFileBlockedMessageIsBounded(t *testing.T) {
	enableHookForTest(t)
	t.Setenv("HOME", t.TempDir())

	longPath := "/tmp/project/" + strings.Repeat("dir/", 500) + ".git/hooks/pre-commit"
	stderr := &bytes.Buffer{}

	exitCode := RunHook(strings.NewReader(hookInput(t, "Write", map[string]any{
		"file_path": longPath,
		"content":   "x",
	})), stderr)

	if exitCode != 2 {
		t.Fatalf("expected exit code 2, got %d (stderr %.200q)", exitCode, stderr.String())
	}
	if !strings.Contains(stderr.String(), "fuse:POLICY_BLOCK") {
		t.Fatalf("expected policy block directive, got %.200q", stderr.String())
	}
	if !strings.Contains(stderr.String(), "git hooks path") {
		t.Fatalf("expected the rule's reason to survive truncation, got %.200q", stderr.String())
	}
	if stderr.Len() > maxAgentDirectiveBytes {
		t.Fatalf("agent-visible message is %d bytes, want <= %d", stderr.Len(), maxAgentDirectiveBytes)
	}
}

func TestRunHook_MCPCautionMessageIsBounded(t *testing.T) {
	enableHookForTest(t)

	toolName := "mcp__srv__" + strings.Repeat("delete_", 7000) + "everything"
	stderr := &bytes.Buffer{}

	exitCode := RunHook(strings.NewReader(hookInput(t, toolName, map[string]any{"path": "/"})), stderr)

	if exitCode != 0 {
		t.Fatalf("expected exit code 0 for CAUTION, got %d", exitCode)
	}
	if !strings.Contains(stderr.String(), "[fuse] CAUTION") {
		t.Fatalf("expected caution line, got %.200q", stderr.String())
	}
	if stderr.Len() > maxAgentDirectiveBytes {
		t.Fatalf("agent-visible message is %d bytes, want <= %d", stderr.Len(), maxAgentDirectiveBytes)
	}
}

func TestInterceptToolCall_BlockedMessageIsBounded(t *testing.T) {
	t.Setenv("FUSE_HOME", t.TempDir())
	if err := os.MkdirAll(filepath.Dir(config.PolicyPath()), 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	writeTestPolicy(t, config.PolicyPath(), `
version: "1"
rules:
  - pattern: '^delete_'
    action: block
    reason: deletes are blocked
`)
	msg := jsonRPCMessage{
		"jsonrpc": "2.0",
		"id":      float64(1),
		"method":  "tools/call",
		"params": map[string]interface{}{
			"name":      strings.Repeat("delete_", 7000) + "everything",
			"arguments": map[string]interface{}{},
		},
	}

	forward, response, err := interceptToolCall(msg)
	if err != nil {
		t.Fatalf("interceptToolCall: %v", err)
	}
	if forward {
		t.Fatal("expected blocked MCP tool not to be forwarded")
	}
	errObj, _ := response["error"].(map[string]interface{})
	message, _ := errObj["message"].(string)
	if !strings.Contains(message, "deletes are blocked") {
		t.Fatalf("expected policy reason to survive truncation, got %.200q", message)
	}
	if len(message) > maxAgentDirectiveBytes {
		t.Fatalf("agent-visible message is %d bytes, want <= %d", len(message), maxAgentDirectiveBytes)
	}
}

func TestBoundForAgent_CutsOnRuneBoundary(t *testing.T) {
	// 300 three-byte runes: any byte cut that ignores rune starts yields invalid UTF-8.
	got := boundForAgent(strings.Repeat("€", 300))

	if !utf8.ValidString(got) {
		t.Fatalf("truncated text is not valid UTF-8: %q", got)
	}
	if len(got) > 256 {
		t.Fatalf("truncated text is %d bytes, want <= 256", len(got))
	}
	if !strings.HasSuffix(got, "…[truncated]") {
		t.Fatalf("expected truncation marker, got %q", got)
	}
	if short := "access denied"; boundForAgent(short) != short {
		t.Fatalf("short text changed: %q", boundForAgent(short))
	}
}
