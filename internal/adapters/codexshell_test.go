package adapters

import (
	"bufio"
	"bytes"
	"database/sql"
	"errors"
	"io"
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
	homeDir := t.TempDir()
	fuseHome := filepath.Join(homeDir, ".fuse")
	oldFuseHome := os.Getenv("FUSE_HOME")
	oldHome := os.Getenv("HOME")
	if err := os.Setenv("FUSE_HOME", fuseHome); err != nil {
		t.Fatalf("set FUSE_HOME: %v", err)
	}
	if err := os.Setenv("HOME", homeDir); err != nil {
		t.Fatalf("set HOME: %v", err)
	}
	t.Cleanup(func() {
		if oldFuseHome == "" {
			_ = os.Unsetenv("FUSE_HOME")
		} else {
			_ = os.Setenv("FUSE_HOME", oldFuseHome)
		}
		if oldHome == "" {
			_ = os.Unsetenv("HOME")
		} else {
			_ = os.Setenv("HOME", oldHome)
		}
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

func enableDryRunForTest(t *testing.T) {
	t.Helper()
	if err := os.WriteFile(config.DryRunMarkerPath(), []byte("1"), 0o600); err != nil {
		t.Fatalf("write dry-run marker: %v", err)
	}
}

func runCodexShellServerRequests(t *testing.T, requests ...jsonRPCMessage) []jsonRPCMessage {
	t.Helper()

	var input bytes.Buffer
	for _, request := range requests {
		payload, err := encodeJSONRPC(request)
		if err != nil {
			t.Fatalf("encode JSON-RPC request: %v", err)
		}
		if _, err := input.WriteString(rawMCPFrame(payload)); err != nil {
			t.Fatalf("write input frame: %v", err)
		}
	}

	var output bytes.Buffer
	if err := RunCodexShellServer(&input, &output); err != nil {
		t.Fatalf("RunCodexShellServer: %v", err)
	}

	reader := bufio.NewReader(&output)
	var responses []jsonRPCMessage
	for {
		payload, err := readMCPFrame(reader)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("readMCPFrame(response): %v", err)
		}
		msg, err := decodeJSONRPC(payload)
		if err != nil {
			t.Fatalf("decodeJSONRPC(response): %v", err)
		}
		responses = append(responses, msg)
	}

	return responses
}

func runCodexShellServerLineRequests(t *testing.T, requests ...jsonRPCMessage) []jsonRPCMessage {
	t.Helper()

	var input bytes.Buffer
	for _, request := range requests {
		payload, err := encodeJSONRPC(request)
		if err != nil {
			t.Fatalf("encode JSON-RPC request: %v", err)
		}
		if _, err := input.Write(payload); err != nil {
			t.Fatalf("write line request payload: %v", err)
		}
		if err := input.WriteByte('\n'); err != nil {
			t.Fatalf("write line request newline: %v", err)
		}
	}

	var output bytes.Buffer
	if err := RunCodexShellServer(&input, &output); err != nil {
		t.Fatalf("RunCodexShellServer: %v", err)
	}

	scanner := bufio.NewScanner(&output)
	scanner.Buffer(make([]byte, 0, 1024), maxMCPFrameBytes)
	var responses []jsonRPCMessage
	for scanner.Scan() {
		msg, err := decodeJSONRPC(scanner.Bytes())
		if err != nil {
			t.Fatalf("decodeJSONRPC(line response): %v", err)
		}
		responses = append(responses, msg)
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan line responses: %v", err)
	}

	return responses
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
	withFuseHome(t)
	enabledMarker := config.EnabledMarkerPath()
	if err := os.Remove(enabledMarker); err != nil && !os.IsNotExist(err) {
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

	_, _, exitCode, err := executeCodexShellCommand("rm -rf "+targetDir, "", "test-session", time.Minute)
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
		if _, _, exitCode, err := executeCodexShellCommand(command, "", "test-session", time.Minute); err != nil {
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

	stdout, stderr, exitCode, err := executeCodexShellCommand("printf safe", "", "test-session", time.Minute)
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
	fuseHome := withFuseHome(t)
	enableFuseForTest(t)

	command := "printf blocked > " + filepath.Join(fuseHome, "config", "policy.yaml")
	stdout, stderr, exitCode, err := executeCodexShellCommand(command, "", "test-session", time.Minute)
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
	t.Setenv("FUSE_NON_INTERACTIVE", "1")

	_, _, exitCode, err := executeCodexShellCommand("python nonexistent_script.py", "", "test-session", time.Minute)
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

func TestRunCodexShellServer_InitializeAndToolsList(t *testing.T) {
	responses := runCodexShellServerRequests(t,
		jsonRPCMessage{
			"jsonrpc": "2.0",
			"id":      1,
			"method":  "initialize",
		},
		jsonRPCMessage{
			"jsonrpc": "2.0",
			"method":  "notifications/initialized",
		},
		jsonRPCMessage{
			"jsonrpc": "2.0",
			"id":      2,
			"method":  "tools/list",
		},
	)

	if len(responses) != 2 {
		t.Fatalf("expected 2 responses, got %d", len(responses))
	}

	initResult, _ := responses[0]["result"].(map[string]interface{})
	if initResult == nil {
		t.Fatalf("expected initialize result, got %#v", responses[0])
	}
	if protocolVersion, _ := initResult["protocolVersion"].(string); protocolVersion != "2024-11-05" {
		t.Fatalf("protocolVersion = %q, want %q", protocolVersion, "2024-11-05")
	}
	serverInfo, _ := initResult["serverInfo"].(map[string]interface{})
	if name, _ := serverInfo["name"].(string); name != "fuse-shell" {
		t.Fatalf("serverInfo.name = %q, want %q", name, "fuse-shell")
	}

	listResult, _ := responses[1]["result"].(map[string]interface{})
	tools, _ := listResult["tools"].([]interface{})
	if len(tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(tools))
	}
	tool, _ := tools[0].(map[string]interface{})
	if name, _ := tool["name"].(string); name != "run_command" {
		t.Fatalf("tool name = %q, want %q", name, "run_command")
	}
}

func TestRunCodexShellServer_InitializeAndToolsList_LineDelimited(t *testing.T) {
	responses := runCodexShellServerLineRequests(t,
		jsonRPCMessage{
			"jsonrpc": "2.0",
			"id":      0,
			"method":  "initialize",
			"params": map[string]interface{}{
				"protocolVersion": "2025-06-18",
				"capabilities":    map[string]interface{}{},
				"clientInfo": map[string]interface{}{
					"name":    "codex-mcp-client",
					"title":   "Codex",
					"version": "0.114.0",
				},
			},
		},
		jsonRPCMessage{
			"jsonrpc": "2.0",
			"method":  "notifications/initialized",
		},
		jsonRPCMessage{
			"jsonrpc": "2.0",
			"id":      1,
			"method":  "tools/list",
		},
	)

	if len(responses) != 2 {
		t.Fatalf("expected 2 responses, got %d", len(responses))
	}

	initResult, _ := responses[0]["result"].(map[string]interface{})
	if initResult == nil {
		t.Fatalf("expected initialize result, got %#v", responses[0])
	}
	if protocolVersion, _ := initResult["protocolVersion"].(string); protocolVersion == "" {
		t.Fatalf("expected protocolVersion in initialize result, got %#v", initResult)
	}

	listResult, _ := responses[1]["result"].(map[string]interface{})
	tools, _ := listResult["tools"].([]interface{})
	if len(tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(tools))
	}
	tool, _ := tools[0].(map[string]interface{})
	if name, _ := tool["name"].(string); name != "run_command" {
		t.Fatalf("tool name = %q, want %q", name, "run_command")
	}
}

func TestRunCodexShellServer_ToolCallSafe(t *testing.T) {
	withFuseHome(t)
	enableFuseForTest(t)

	responses := runCodexShellServerRequests(t, jsonRPCMessage{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/call",
		"params": map[string]interface{}{
			"name": "run_command",
			"arguments": map[string]interface{}{
				"command": "printf safe",
			},
		},
	})

	if len(responses) != 1 {
		t.Fatalf("expected 1 response, got %d", len(responses))
	}

	result, _ := responses[0]["result"].(map[string]interface{})
	if result == nil {
		t.Fatalf("expected success result, got %#v", responses[0])
	}
	if stdout, _ := result["stdout"].(string); stdout != "safe" {
		t.Fatalf("stdout = %q, want %q", stdout, "safe")
	}
	if exitCode, _ := result["exit_code"].(float64); exitCode != 0 {
		t.Fatalf("exit_code = %v, want 0", exitCode)
	}
}

func TestRunCodexShellServer_ToolCallSafe_LineDelimited(t *testing.T) {
	withFuseHome(t)
	enableFuseForTest(t)

	responses := runCodexShellServerLineRequests(t, jsonRPCMessage{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/call",
		"params": map[string]interface{}{
			"name": "run_command",
			"arguments": map[string]interface{}{
				"command": "printf safe",
			},
		},
	})

	if len(responses) != 1 {
		t.Fatalf("expected 1 response, got %d", len(responses))
	}

	result, _ := responses[0]["result"].(map[string]interface{})
	if result == nil {
		t.Fatalf("expected success result, got %#v", responses[0])
	}
	if stdout, _ := result["stdout"].(string); stdout != "safe" {
		t.Fatalf("stdout = %q, want %q", stdout, "safe")
	}
	if exitCode, _ := result["exit_code"].(float64); exitCode != 0 {
		t.Fatalf("exit_code = %v, want 0", exitCode)
	}
}

func TestRunCodexShellServer_ToolCallBlocked(t *testing.T) {
	fuseHome := withFuseHome(t)
	enableFuseForTest(t)

	responses := runCodexShellServerRequests(t, jsonRPCMessage{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/call",
		"params": map[string]interface{}{
			"name": "run_command",
			"arguments": map[string]interface{}{
				"command": "printf blocked > " + filepath.Join(fuseHome, "config", "policy.yaml"),
			},
		},
	})

	if len(responses) != 1 {
		t.Fatalf("expected 1 response, got %d", len(responses))
	}

	errObj, _ := responses[0]["error"].(map[string]interface{})
	if errObj == nil {
		t.Fatalf("expected JSON-RPC error, got %#v", responses[0])
	}
	if code, _ := errObj["code"].(float64); code != -32000 {
		t.Fatalf("error code = %v, want -32000", code)
	}
	if message, _ := errObj["message"].(string); !strings.Contains(message, "fuse blocked command") {
		t.Fatalf("error message = %q, want fuse blocked command", message)
	}
}

func TestRunCodexShellServer_ToolCallApprovalWithoutTTY(t *testing.T) {
	withFuseHome(t)
	enableFuseForTest(t)
	t.Setenv("FUSE_NON_INTERACTIVE", "1")

	responses := runCodexShellServerRequests(t, jsonRPCMessage{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/call",
		"params": map[string]interface{}{
			"name": "run_command",
			"arguments": map[string]interface{}{
				"command": "python nonexistent_script.py",
			},
		},
	})

	if len(responses) != 1 {
		t.Fatalf("expected 1 response, got %d", len(responses))
	}

	errObj, _ := responses[0]["error"].(map[string]interface{})
	if errObj == nil {
		t.Fatalf("expected JSON-RPC error, got %#v", responses[0])
	}
	if code, _ := errObj["code"].(float64); code != -32000 {
		t.Fatalf("error code = %v, want -32000", code)
	}
	if message, _ := errObj["message"].(string); !strings.Contains(message, "NON_INTERACTIVE_MODE") {
		t.Fatalf("error message = %q, want NON_INTERACTIVE_MODE", message)
	}
}

// --- fu-8fj: Test session_id attribution end-to-end ---

func TestExecuteCodexShellCommand_SessionIDAttribution(t *testing.T) {
	withFuseHome(t)
	enableFuseForTest(t)

	sessionID := "codex-test-attribution"
	_, _, exitCode, err := executeCodexShellCommand("printf hello", "", sessionID, time.Minute)
	if err != nil {
		t.Fatalf("executeCodexShellCommand: %v", err)
	}
	if exitCode != 0 {
		t.Fatalf("exit code = %d, want 0", exitCode)
	}

	// Query the events table to verify session_id was stored correctly.
	sqlDB, err := sql.Open("sqlite", config.DBPath())
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer func() { _ = sqlDB.Close() }()

	var storedSessionID, storedDecision, storedSource string
	err = sqlDB.QueryRow(`
		SELECT session_id, decision, source FROM events
		ORDER BY id DESC LIMIT 1
	`).Scan(&storedSessionID, &storedDecision, &storedSource)
	if err != nil {
		t.Fatalf("query event: %v", err)
	}
	if storedSessionID != sessionID {
		t.Errorf("session_id = %q, want %q", storedSessionID, sessionID)
	}
	if storedDecision != "SAFE" {
		t.Errorf("decision = %q, want %q", storedDecision, "SAFE")
	}
	if storedSource != "codex-shell" {
		t.Errorf("source = %q, want %q", storedSource, "codex-shell")
	}
}

func TestExecuteCodexShellCommand_DistinctSessionIDs(t *testing.T) {
	withFuseHome(t)
	enableFuseForTest(t)

	// Run commands from two different sessions.
	for _, sess := range []string{"codex-sess-1", "codex-sess-2"} {
		_, _, _, err := executeCodexShellCommand("printf "+sess, "", sess, time.Minute)
		if err != nil {
			t.Fatalf("session %s: %v", sess, err)
		}
	}

	// Query events and verify each has the correct session_id.
	sqlDB, err := sql.Open("sqlite", config.DBPath())
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer func() { _ = sqlDB.Close() }()

	rows, err := sqlDB.Query("SELECT session_id, command FROM events ORDER BY id")
	if err != nil {
		t.Fatalf("query events: %v", err)
	}
	defer rows.Close()

	type eventRow struct {
		sessionID string
		command   string
	}
	var events []eventRow
	for rows.Next() {
		var e eventRow
		if err := rows.Scan(&e.sessionID, &e.command); err != nil {
			t.Fatalf("scan: %v", err)
		}
		events = append(events, e)
	}

	if len(events) < 2 {
		t.Fatalf("expected at least 2 events, got %d", len(events))
	}

	// Verify distinct session IDs in the events.
	sessions := map[string]bool{}
	for _, e := range events {
		sessions[e.sessionID] = true
	}
	if !sessions["codex-sess-1"] {
		t.Error("missing event for codex-sess-1")
	}
	if !sessions["codex-sess-2"] {
		t.Error("missing event for codex-sess-2")
	}
}

func TestGenerateSessionID_Unique(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 1000; i++ {
		id := generateSessionID()
		if !strings.HasPrefix(id, "codex-") {
			t.Fatalf("session ID %q missing codex- prefix", id)
		}
		if seen[id] {
			t.Fatalf("duplicate session ID %q after %d iterations", id, i)
		}
		seen[id] = true
	}
}
