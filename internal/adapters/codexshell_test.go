package adapters

import (
	"bufio"
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/php-workx/fuse/internal/config"
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

func shortTestDrains(t *testing.T) {
	t.Helper()
	oldDrain, oldPost := codexDrainTimeout, codexDrainPostCancel
	codexDrainTimeout = 750 * time.Millisecond
	codexDrainPostCancel = 100 * time.Millisecond
	oldHook := hookTimeout
	hookTimeout = 750 * time.Millisecond
	t.Cleanup(func() {
		codexDrainTimeout = oldDrain
		codexDrainPostCancel = oldPost
		hookTimeout = oldHook
	})
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

	result, err := executeCapturedShellCommand(context.Background(), "cat", "", time.Second)
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

	_, _, exitCode, err := executeCodexShellCommand(context.Background(), "rm -rf "+targetDir, "", "test-session", time.Minute)
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
		if _, _, exitCode, err := executeCodexShellCommand(context.Background(), command, "", "test-session", time.Minute); err != nil {
			t.Fatalf("executeCodexShellCommand(context.Background(),%q): %v", command, err)
		} else if exitCode != 0 {
			t.Fatalf("executeCodexShellCommand(context.Background(),%q) exit code = %d", command, exitCode)
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

	stdout, stderr, exitCode, err := executeCodexShellCommand(context.Background(), "printf safe", "", "test-session", time.Minute)
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
	stdout, stderr, exitCode, err := executeCodexShellCommand(context.Background(), command, "", "test-session", time.Minute)
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

	ctx, cancel := context.WithTimeout(context.Background(), 750*time.Millisecond)
	defer cancel()
	_, _, exitCode, err := executeCodexShellCommand(ctx, "python nonexistent_script.py", "", "test-session", time.Minute)
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
			"id":      float64(1),
			"method":  "initialize",
		},
		jsonRPCMessage{
			"jsonrpc": "2.0",
			"method":  "notifications/initialized",
		},
		jsonRPCMessage{
			"jsonrpc": "2.0",
			"id":      float64(2),
			"method":  "tools/list",
		},
	)

	if len(responses) != 2 {
		t.Fatalf("expected 2 responses, got %d", len(responses))
	}

	// Match responses by ID since concurrent processing may reorder them.
	var initResp, listResp jsonRPCMessage
	for _, r := range responses {
		switch id, _ := r["id"].(float64); id {
		case 1:
			initResp = r
		case 2:
			listResp = r
		}
	}

	initResult, _ := initResp["result"].(map[string]interface{})
	if initResult == nil {
		t.Fatalf("expected initialize result, got %#v", initResp)
	}
	if protocolVersion, _ := initResult["protocolVersion"].(string); protocolVersion != "2024-11-05" {
		t.Fatalf("protocolVersion = %q, want %q", protocolVersion, "2024-11-05")
	}
	serverInfo, _ := initResult["serverInfo"].(map[string]interface{})
	if name, _ := serverInfo["name"].(string); name != "fuse-shell" {
		t.Fatalf("serverInfo.name = %q, want %q", name, "fuse-shell")
	}

	listResult, _ := listResp["result"].(map[string]interface{})
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
			"id":      float64(0),
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
			"id":      float64(1),
			"method":  "tools/list",
		},
	)

	if len(responses) != 2 {
		t.Fatalf("expected 2 responses, got %d", len(responses))
	}

	// Match responses by ID since concurrent processing may reorder them.
	var initResp, listResp jsonRPCMessage
	for _, r := range responses {
		switch id, _ := r["id"].(float64); id {
		case 0:
			initResp = r
		case 1:
			listResp = r
		}
	}

	initResult, _ := initResp["result"].(map[string]interface{})
	if initResult == nil {
		t.Fatalf("expected initialize result, got %#v", initResp)
	}
	if protocolVersion, _ := initResult["protocolVersion"].(string); protocolVersion == "" {
		t.Fatalf("expected protocolVersion in initialize result, got %#v", initResult)
	}

	listResult, _ := listResp["result"].(map[string]interface{})
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
	shortTestDrains(t)

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
	_, _, exitCode, err := executeCodexShellCommand(context.Background(), "printf hello", "", sessionID, time.Minute)
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
		_, _, _, err := executeCodexShellCommand(context.Background(), "printf "+sess, "", sess, time.Minute)
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

func TestRunCodexShellServer_ConcurrentToolCalls(t *testing.T) {
	withFuseHome(t)
	enableFuseForTest(t)

	responses := runCodexShellServerRequests(t,
		jsonRPCMessage{"jsonrpc": "2.0", "id": float64(1), "method": "tools/call", "params": map[string]interface{}{
			"name": "run_command", "arguments": map[string]interface{}{"command": "printf one"},
		}},
		jsonRPCMessage{"jsonrpc": "2.0", "id": float64(2), "method": "tools/call", "params": map[string]interface{}{
			"name": "run_command", "arguments": map[string]interface{}{"command": "printf two"},
		}},
		jsonRPCMessage{"jsonrpc": "2.0", "id": float64(3), "method": "tools/call", "params": map[string]interface{}{
			"name": "run_command", "arguments": map[string]interface{}{"command": "printf three"},
		}},
	)

	if len(responses) != 3 {
		t.Fatalf("expected 3 responses, got %d", len(responses))
	}

	// All IDs must be present (order may vary).
	seen := map[float64]bool{}
	for _, r := range responses {
		id, _ := r["id"].(float64)
		seen[id] = true
	}
	for _, want := range []float64{1, 2, 3} {
		if !seen[want] {
			t.Errorf("missing response for id %v", want)
		}
	}
}

func TestRunCodexShellServer_SlowRequestDoesNotBlockFast(t *testing.T) {
	withFuseHome(t)
	enableFuseForTest(t)

	// Use io.Pipe so we can control timing: send slow request, then fast request.
	pr, pw := io.Pipe()
	var output bytes.Buffer
	done := make(chan error, 1)
	go func() {
		done <- RunCodexShellServer(pr, &output)
	}()

	// Send slow command (sleep 0.2).
	slow := jsonRPCMessage{"jsonrpc": "2.0", "id": float64(1), "method": "tools/call", "params": map[string]interface{}{
		"name": "run_command", "arguments": map[string]interface{}{"command": "sleep 0.2 && printf slow"},
	}}
	writeFramedRequest(t, pw, slow)

	// Small delay to ensure slow request is being processed.
	time.Sleep(50 * time.Millisecond)

	// Send fast command.
	fast := jsonRPCMessage{"jsonrpc": "2.0", "id": float64(2), "method": "tools/call", "params": map[string]interface{}{
		"name": "run_command", "arguments": map[string]interface{}{"command": "printf fast"},
	}}
	writeFramedRequest(t, pw, fast)

	// Close stdin after a brief delay to let both process.
	time.Sleep(400 * time.Millisecond)
	_ = pw.Close()
	<-done

	reader := bufio.NewReader(&output)
	var responses []jsonRPCMessage
	for {
		payload, err := readMCPFrame(reader)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("readMCPFrame: %v", err)
		}
		msg, _ := decodeJSONRPC(payload)
		responses = append(responses, msg)
	}

	if len(responses) != 2 {
		t.Fatalf("expected 2 responses, got %d", len(responses))
	}

	// The fast response (id=2) should arrive before the slow response (id=1).
	firstID, _ := responses[0]["id"].(float64)
	if firstID != 2 {
		t.Errorf("expected fast response (id=2) first, got id=%v", firstID)
	}
}

func TestRunCodexShellServer_ParseErrorContinuesProcessing(t *testing.T) {
	// Send malformed JSON followed by a valid initialize request.
	var input bytes.Buffer
	// Malformed frame: valid MCP framing but invalid JSON body.
	input.WriteString("Content-Length: 3\r\n\r\n{x}")
	// Valid initialize request.
	initReq, _ := encodeJSONRPC(jsonRPCMessage{"jsonrpc": "2.0", "id": float64(1), "method": "initialize"})
	input.WriteString(rawMCPFrame(initReq))

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
			t.Fatalf("readMCPFrame: %v", err)
		}
		msg, _ := decodeJSONRPC(payload)
		responses = append(responses, msg)
	}

	if len(responses) < 2 {
		t.Fatalf("expected at least 2 responses (parse error + init), got %d", len(responses))
	}

	// One should be an error response, one should be the init result.
	var hasError, hasInit bool
	for _, r := range responses {
		if _, ok := r["error"]; ok {
			hasError = true
		}
		if result, ok := r["result"]; ok && result != nil {
			hasInit = true
		}
	}
	if !hasError {
		t.Error("expected a parse error response for malformed JSON")
	}
	if !hasInit {
		t.Error("expected an initialize response after the parse error")
	}
}

func TestRunCodexShellServer_GracefulShutdownOnEOF(t *testing.T) {
	responses := runCodexShellServerRequests(t, jsonRPCMessage{
		"jsonrpc": "2.0",
		"id":      float64(1),
		"method":  "initialize",
	})

	if len(responses) != 1 {
		t.Fatalf("expected 1 response, got %d", len(responses))
	}
	if _, ok := responses[0]["result"]; !ok {
		t.Fatal("expected initialize result")
	}
}

func TestRunCodexShellServer_ShutdownKillsInFlight(t *testing.T) {
	withFuseHome(t)
	enableFuseForTest(t)
	shortTestDrains(t)

	pr, pw := io.Pipe()
	var output bytes.Buffer
	done := make(chan error, 1)
	go func() {
		done <- RunCodexShellServer(pr, &output)
	}()

	// Send a long-running command.
	req := jsonRPCMessage{"jsonrpc": "2.0", "id": float64(1), "method": "tools/call", "params": map[string]interface{}{
		"name": "run_command", "arguments": map[string]interface{}{"command": "sleep 10"},
	}}
	writeFramedRequest(t, pw, req)

	// Give it a moment to start, then close stdin.
	time.Sleep(200 * time.Millisecond)
	_ = pw.Close()

	// Server should return within drain timeout + margin, not 10s.
	select {
	case <-done:
		// Good — server shut down promptly.
	case <-time.After(1500 * time.Millisecond):
		t.Fatal("RunCodexShellServer did not shut down within 1.5s after stdin close")
	}
}

func TestCodexShellWriter_ConcurrentWrites(t *testing.T) {
	var output bytes.Buffer
	w := &codexShellWriter{
		writer:    bufio.NewWriter(&output),
		transport: codexShellTransportFramed,
	}

	const n = 20
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(id int) {
			defer wg.Done()
			_ = w.writeResponse(jsonRPCMessage{
				"jsonrpc": "2.0",
				"id":      float64(id),
				"result":  "ok",
			})
		}(i)
	}
	wg.Wait()

	// Read all frames and verify none are corrupted.
	reader := bufio.NewReader(&output)
	count := 0
	for {
		payload, err := readMCPFrame(reader)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("readMCPFrame after %d frames: %v", count, err)
		}
		msg, err := decodeJSONRPC(payload)
		if err != nil {
			t.Fatalf("decodeJSONRPC frame %d: %v", count, err)
		}
		if _, ok := msg["jsonrpc"]; !ok {
			t.Fatalf("frame %d missing jsonrpc field", count)
		}
		count++
	}
	if count != n {
		t.Fatalf("expected %d frames, got %d", n, count)
	}
}

// writeFramedRequest encodes and writes a single MCP-framed request to a writer.
func writeFramedRequest(t *testing.T, w io.Writer, msg jsonRPCMessage) {
	t.Helper()
	payload, err := encodeJSONRPC(msg)
	if err != nil {
		t.Fatalf("encode request: %v", err)
	}
	if _, err := fmt.Fprintf(w, "Content-Length: %d\r\n\r\n%s", len(payload), payload); err != nil {
		t.Fatalf("write request: %v", err)
	}
}
