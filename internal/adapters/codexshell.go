package adapters

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"time"

	"github.com/runger/fuse/internal/approve"
	"github.com/runger/fuse/internal/config"
	"github.com/runger/fuse/internal/core"
	"github.com/runger/fuse/internal/policy"
)

func RunCodexShellServer(stdin io.Reader, stdout io.Writer) error {
	reader := bufio.NewReader(stdin)
	for {
		payload, err := readMCPFrame(reader)
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}

		msg, err := decodeJSONRPC(payload)
		if err != nil {
			return err
		}

		response, respond, err := handleCodexShellMessage(msg)
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
		if err := writeMCPFrame(stdout, data); err != nil {
			return err
		}
	}
}

func handleCodexShellMessage(msg jsonRPCMessage) (jsonRPCMessage, bool, error) {
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
		return handleCodexShellToolCall(msg)
	default:
		if _, ok := msg["id"]; ok {
			return jsonRPCErrorResponse(msg["id"], -32601, fmt.Sprintf("method %s not found", method)), true, nil
		}
		return nil, false, nil
	}
}

func handleCodexShellToolCall(msg jsonRPCMessage) (jsonRPCMessage, bool, error) {
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

	out, errOut, exitCode, err := executeCodexShellCommand(command, cwd, 30*time.Minute)
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

func executeCodexShellCommand(command, cwd string, timeout time.Duration) (string, string, int, error) {
	policyCfg, _ := policy.LoadPolicy(config.PolicyPath())
	evaluator := policy.NewEvaluator(policyCfg)
	req := core.ShellRequest{
		RawCommand: command,
		Cwd:        cwd,
		Source:     "codex",
	}

	result, err := core.Classify(req, evaluator)
	if err != nil {
		return "", "", 0, err
	}

	switch result.Decision {
	case core.DecisionBlocked:
		return "", "", 0, fmt.Errorf("fuse blocked command: %s", result.Reason)
	case core.DecisionApproval:
		database, secret, err := openDBAndSecret()
		if err != nil {
			return "", "", 0, err
		}
		defer func() { _ = database.Close() }()

		mgr, err := approve.NewManager(database, secret)
		if err != nil {
			return "", "", 0, err
		}
		decision, err := mgr.RequestApproval(result.DecisionKey, command, result.Reason, "", false)
		if err != nil {
			return "", "", 0, err
		}
		if decision == core.DecisionBlocked {
			return "", "", 0, fmt.Errorf("fuse denied command")
		}
	}

	if err := reverifyDecisionKey(req, evaluator, result.DecisionKey); err != nil {
		return "", "", 0, err
	}

	ctx := context.Background()
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	cmd := exec.CommandContext(ctx, "/bin/sh", "-c", command)
	cmd.Dir = cwd
	cmd.Env = BuildChildEnv(cmd.Environ())
	var stdoutBuf bytes.Buffer
	var stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	err = cmd.Run()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return stdoutBuf.String(), stderrBuf.String(), exitErr.ExitCode(), nil
		}
		return stdoutBuf.String(), stderrBuf.String(), 0, err
	}
	return stdoutBuf.String(), stderrBuf.String(), 0, nil
}
