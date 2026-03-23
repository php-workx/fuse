package adapters

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/runger/fuse/internal/approve"
	"github.com/runger/fuse/internal/config"
	"github.com/runger/fuse/internal/core"
	"github.com/runger/fuse/internal/db"
	"github.com/runger/fuse/internal/judge"
	"github.com/runger/fuse/internal/policy"
)

// codexDrainTimeout is the time to wait for in-flight requests before cancelling.
// codexDrainPostCancel is the extra drain after cancel for response writes.
// Overridable in tests to avoid 5s+ waits.
var (
	codexDrainTimeout    = 5 * time.Second
	codexDrainPostCancel = 1 * time.Second
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
	sessionID := generateSessionID()
	transport, err := detectCodexShellTransport(reader)
	if err != nil {
		if errors.Is(err, io.EOF) {
			return nil
		}
		return err
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	writer := &codexShellWriter{writer: bufio.NewWriter(stdout), transport: transport}
	var wg sync.WaitGroup

	for {
		payload, err := readCodexShellPayload(reader, transport)
		if err != nil {
			break // EOF or read error — shut down
		}

		msg, err := decodeJSONRPC(payload)
		if err != nil {
			_ = writer.writeResponse(jsonRPCErrorResponse(nil, -32700, err.Error()))
			continue
		}

		wg.Add(1)
		go func(m jsonRPCMessage) {
			defer wg.Done()
			processRequest(ctx, m, sessionID, writer)
		}(msg)
	}

	// Wait for in-flight requests to complete naturally, then cancel
	// any stragglers (kills child processes via exec.CommandContext).
	waitGroupWithTimeout(&wg, codexDrainTimeout)
	cancel()
	// Brief extra drain after cancel to let goroutines finish writing responses.
	waitGroupWithTimeout(&wg, codexDrainPostCancel)
	return nil
}

// processRequest handles a single MCP request in its own goroutine.
// Errors are converted to JSON-RPC error responses (non-fatal to server).
func processRequest(ctx context.Context, msg jsonRPCMessage, sessionID string, writer *codexShellWriter) {
	defer func() {
		if r := recover(); r != nil {
			if id, ok := msg["id"]; ok {
				_ = writer.writeResponse(jsonRPCErrorResponse(id, -32603, fmt.Sprintf("internal error: %v", r)))
			}
		}
	}()

	response, respond, err := handleCodexShellMessage(ctx, msg, sessionID)
	if err != nil {
		if id, ok := msg["id"]; ok {
			_ = writer.writeResponse(jsonRPCErrorResponse(id, -32603, err.Error()))
		}
		return
	}
	if !respond || response == nil {
		return
	}
	_ = writer.writeResponse(response)
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

func handleCodexShellMessage(ctx context.Context, msg jsonRPCMessage, sessionID string) (jsonRPCMessage, bool, error) {
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
		return handleCodexShellToolCall(ctx, msg, sessionID)
	default:
		if _, ok := msg["id"]; ok {
			return jsonRPCErrorResponse(msg["id"], -32601, fmt.Sprintf("method %s not found", method)), true, nil
		}
		return nil, false, nil
	}
}

func handleCodexShellToolCall(ctx context.Context, msg jsonRPCMessage, sessionID string) (jsonRPCMessage, bool, error) {
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

	out, errOut, exitCode, err := executeCodexShellCommand(ctx, command, cwd, sessionID, 30*time.Minute)
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

func executeCodexShellCommand(ctx context.Context, command, cwd, sessionID string, timeout time.Duration) (string, string, int, error) {
	cfg := loadRuntimeConfig()
	mode := config.Mode()
	if mode == config.ModeDisabled {
		execResult, err := executeCapturedShellCommand(ctx, command, cwd, timeout)
		return execResult.Stdout, execResult.Stderr, execResult.ExitCode, err
	}
	dryRun := mode == config.ModeDryRun

	if ctx.Err() != nil {
		return "", "", 0, ctx.Err()
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

	// LLM judge: get a second opinion on the classification.
	var verdict *judge.Verdict
	result, verdict = judge.MaybeJudge(ctx, cfg, result, buildJudgeContext(command, cwd, "Bash", result))

	if ctx.Err() != nil {
		return "", "", 0, ctx.Err()
	}

	database, dbErr := db.OpenDB(config.DBPath())
	if dbErr != nil {
		database = nil
	}
	if database != nil {
		defer func() { _ = database.Close() }()
	}

	// logWithVerdict is a helper to log events with judge verdict fields.
	logWithVerdict := func(outcome string) {
		event := newEvent(result, "codex-shell", "codex", sessionID, command, cwd, outcome)
		applyVerdict(event, verdict)
		logEvent(database, event)
	}

	switch result.Decision {
	case core.DecisionBlocked:
		logWithVerdict("blocked")
		cleanupExecutionState(database, cfg)
		if !dryRun {
			return "", "", 0, fmt.Errorf("fuse blocked command: %s", result.Reason)
		}
	case core.DecisionSafe, core.DecisionCaution:
		// Execute directly.
	case core.DecisionApproval:
		if !dryRun {
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
			decision, promptErr := mgr.RequestApproval(ctx, approve.ApprovalRequest{
				DecisionKey:    result.DecisionKey,
				Command:        command,
				Reason:         result.Reason,
				SessionID:      sessionID,
				Source:         "codex-shell",
				NonInteractive: dryRun,
			})
			if promptErr != nil {
				return "", "", 0, promptErr
			}
			if decision == core.DecisionBlocked {
				logWithVerdict("denied")
				cleanupExecutionState(database, cfg)
				return "", "", 0, errApprovalDenied
			}
		} else {
			logWithVerdict("dry-run")
		}

	default:
		// Unknown decision — execute directly (safe fallback).
	}

	if ctx.Err() != nil {
		return "", "", 0, ctx.Err()
	}

	if !dryRun {
		if verifyErr := reverifyDecisionKey(req, evaluator, result.DecisionKey); verifyErr != nil {
			return "", "", 0, verifyErr
		}
	}

	execResult, err := executeCapturedShellCommand(ctx, command, cwd, timeout)
	outcome := "executed"
	if err != nil {
		outcome = "error"
	}
	logWithVerdict(outcome)
	cleanupExecutionState(database, cfg)
	return execResult.Stdout, execResult.Stderr, execResult.ExitCode, err
}

// codexShellWriter serializes MCP response writes to stdout.
// Encode + frame + flush happen under one mutex hold to prevent
// frame interleaving from concurrent goroutines.
type codexShellWriter struct {
	mu        sync.Mutex
	writer    *bufio.Writer
	transport codexShellTransport
}

func (w *codexShellWriter) writeResponse(msg jsonRPCMessage) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	data, err := encodeJSONRPC(msg)
	if err != nil {
		return err
	}
	return writeCodexShellPayload(w.writer, data, w.transport)
}

// waitGroupWithTimeout blocks until all goroutines in wg finish or timeout
// elapses. Used for bounded drain during shutdown.
func waitGroupWithTimeout(wg *sync.WaitGroup, timeout time.Duration) {
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(timeout):
	}
}
