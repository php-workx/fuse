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
	fuseHome := withFuseHome(t)
	enableFuseForTest(t)

	command := "printf blocked > " + filepath.Join(fuseHome, "config", "policy.yaml")
	stdout, stderr, exitCode, err := executeCodexShellCommand(command, "", time.Minute)
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

	_, _, exitCode, err := executeCodexShellCommand("python nonexistent_script.py", "", time.Minute)
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
