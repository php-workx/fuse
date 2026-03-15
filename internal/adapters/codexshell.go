package adapters

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/runger/fuse/internal/approve"
	"github.com/runger/fuse/internal/config"
	"github.com/runger/fuse/internal/core"
	"github.com/runger/fuse/internal/db"
	"github.com/runger/fuse/internal/policy"
)

type codexShellTransport int

const (
	codexShellTransportFramed codexShellTransport = iota
	codexShellTransportLineDelimited
)

func generateSessionID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return fmt.Sprintf("codex-%x", b)
}

func RunCodexShellServer(stdin io.Reader, stdout io.Writer) error {
	reader := bufio.NewReader(stdin)
	writer := bufio.NewWriter(stdout)
	sessionID := generateSessionID()
	transport, err := detectCodexShellTransport(reader)
	if err != nil {
		if errors.Is(err, io.EOF) {
			return nil
		}
		return err
	}

	for {
		payload, err := readCodexShellPayload(reader, transport)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}

		msg, err := decodeJSONRPC(payload)
		if err != nil {
			return err
		}

		response, respond, err := handleCodexShellMessage(msg, sessionID)
		if err != nil {
			return err
		}
		if !respond || response == nil {
			continue
		}

		data, err := encodeJSONRPC(response)
		if err != nil {
			return err
		}
		if err := writeCodexShellPayload(writer, data, transport); err != nil {
			return err
		}
	}
}

func detectCodexShellTransport(reader *bufio.Reader) (codexShellTransport, error) {
	for {
		b, err := reader.Peek(1)
		if err != nil {
			return codexShellTransportFramed, err
		}
		switch b[0] {
		case ' ', '\t', '\r', '\n':
			if _, err := reader.ReadByte(); err != nil {
				return codexShellTransportFramed, err
			}
		case '{', '[':
			return codexShellTransportLineDelimited, nil
		default:
			return codexShellTransportFramed, nil
		}
	}
}

func readCodexShellPayload(reader *bufio.Reader, transport codexShellTransport) ([]byte, error) {
	if transport == codexShellTransportLineDelimited {
		return readJSONRPCLine(reader)
	}
	return readMCPFrame(reader)
}

func writeCodexShellPayload(writer *bufio.Writer, payload []byte, transport codexShellTransport) error {
	if transport == codexShellTransportLineDelimited {
		if _, err := writer.Write(payload); err != nil {
			return err
		}
		if err := writer.WriteByte('\n'); err != nil {
			return err
		}
		return writer.Flush()
	}
	if err := writeMCPFrame(writer, payload); err != nil {
		return err
	}
	return writer.Flush()
}

func readJSONRPCLine(reader *bufio.Reader) ([]byte, error) {
	for {
		line, err := reader.ReadBytes('\n')
		line = bytes.TrimSpace(line)
		if len(line) > 0 {
			return line, nil
		}
		if err != nil {
			return nil, err
		}
	}
}

func handleCodexShellMessage(msg jsonRPCMessage, sessionID string) (jsonRPCMessage, bool, error) {
	method, _ := msg["method"].(string)
	switch method {
	case "initialize":
		return jsonRPCMessage{
			"jsonrpc": "2.0",
			"id":      msg["id"],
			"result": map[string]interface{}{
				"protocolVersion": "2024-11-05",
				"capabilities": map[string]interface{}{
					"tools": map[string]interface{}{},
				},
				"serverInfo": map[string]interface{}{
					"name":    "fuse-shell",
					"version": "dev",
				},
			},
		}, true, nil
	case "notifications/initialized":
		return nil, false, nil
	case "tools/list":
		return jsonRPCMessage{
			"jsonrpc": "2.0",
			"id":      msg["id"],
			"result": map[string]interface{}{
				"tools": []map[string]interface{}{
					{
						"name":        "run_command",
						"description": "Execute a shell command through fuse safety runtime",
						"inputSchema": map[string]interface{}{
							"type": "object",
							"properties": map[string]interface{}{
								"command": map[string]interface{}{
									"type":        "string",
									"description": "The shell command to execute",
								},
								"cwd": map[string]interface{}{
									"type":        "string",
									"description": "Working directory (optional)",
								},
							},
							"required": []string{"command"},
						},
					},
				},
			},
		}, true, nil
	case "tools/call":
		return handleCodexShellToolCall(msg, sessionID)
	default:
		if _, ok := msg["id"]; ok {
			return jsonRPCErrorResponse(msg["id"], -32601, fmt.Sprintf("method %s not found", method)), true, nil
		}
		return nil, false, nil
	}
}

func handleCodexShellToolCall(msg jsonRPCMessage, sessionID string) (jsonRPCMessage, bool, error) {
	params, _ := msg["params"].(map[string]interface{})
	name, _ := params["name"].(string)
	if name != "run_command" {
		return jsonRPCErrorResponse(msg["id"], -32601, fmt.Sprintf("tool %s not found", name)), true, nil
	}

	arguments, _ := params["arguments"].(map[string]interface{})
	command, _ := arguments["command"].(string)
	cwd, _ := arguments["cwd"].(string)
	if command == "" {
		return jsonRPCErrorResponse(msg["id"], -32602, "missing command"), true, nil
	}

	out, errOut, exitCode, err := executeCodexShellCommand(command, cwd, sessionID, 30*time.Minute)
	if err != nil {
		return jsonRPCErrorResponse(msg["id"], -32000, err.Error()), true, nil
	}

	return jsonRPCMessage{
		"jsonrpc": "2.0",
		"id":      msg["id"],
		"result": map[string]interface{}{
			"content": []map[string]interface{}{
				{
					"type": "text",
					"text": out + errOut,
				},
			},
			"stdout":    out,
			"stderr":    errOut,
			"exit_code": exitCode,
		},
	}, true, nil
}

func executeCodexShellCommand(command, cwd, sessionID string, timeout time.Duration) (string, string, int, error) {
	cfg := loadRuntimeConfig()
	if config.IsDisabled() {
		execResult, err := executeCapturedShellCommand(command, cwd, timeout)
		return execResult.Stdout, execResult.Stderr, execResult.ExitCode, err
	}

	policyCfg, _ := policy.LoadPolicy(config.PolicyPath())
	evaluator := policy.NewEvaluator(policyCfg)
	req := core.ShellRequest{
		RawCommand: command,
		Cwd:        cwd,
		Source:     "codex",
		SessionID:  sessionID,
	}

	result, err := core.Classify(req, evaluator)
	if err != nil {
		return "", "", 0, err
	}

	database, dbErr := db.OpenDB(config.DBPath())
	if dbErr != nil {
		database = nil
	}
	if database != nil {
		defer func() { _ = database.Close() }()
	}

	switch result.Decision {
	case core.DecisionBlocked:
		logEvent(database, newEvent(result, "codex-shell", "codex", sessionID, command, cwd, "blocked"))
		cleanupExecutionState(database, cfg)
		return "", "", 0, fmt.Errorf("fuse blocked command: %s", result.Reason)
	case core.DecisionSafe, core.DecisionCaution:
		// Execute directly.
	case core.DecisionApproval:
		if database == nil {
			return "", "", 0, fmt.Errorf("database unavailable for approval")
		}
		secret, secretErr := db.EnsureSecret(config.SecretPath())
		if secretErr != nil {
			return "", "", 0, secretErr
		}

		mgr, mgrErr := approve.NewManager(database, secret)
		if mgrErr != nil {
			return "", "", 0, mgrErr
		}
		decision, promptErr := mgr.RequestApproval(result.DecisionKey, command, result.Reason, sessionID, false)
		if promptErr != nil {
			return "", "", 0, promptErr
		}
		if decision == core.DecisionBlocked {
			logEvent(database, newEvent(result, "codex-shell", "codex", sessionID, command, cwd, "denied"))
			cleanupExecutionState(database, cfg)
			return "", "", 0, errApprovalDenied
		}

	default:
		// Unknown decision — execute directly (safe fallback).
	}

	if verifyErr := reverifyDecisionKey(req, evaluator, result.DecisionKey); verifyErr != nil {
		return "", "", 0, verifyErr
	}

	execResult, err := executeCapturedShellCommand(command, cwd, timeout)
	outcome := "executed"
	if err != nil {
		outcome = "error"
	}
	logEvent(database, newEvent(result, "codex-shell", "codex", sessionID, command, cwd, outcome))
	cleanupExecutionState(database, cfg)
	return execResult.Stdout, execResult.Stderr, execResult.ExitCode, err
}
