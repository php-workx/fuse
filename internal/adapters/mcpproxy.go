package adapters

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"sync"

	"github.com/runger/fuse/internal/approve"
	"github.com/runger/fuse/internal/config"
	"github.com/runger/fuse/internal/core"
)

type inFlightRequests struct {
	mu      sync.Mutex
	methods map[string]string
}

func newInFlightRequests() *inFlightRequests {
	return &inFlightRequests{methods: make(map[string]string)}
}

func (r *inFlightRequests) add(id interface{}, method string) {
	key := jsonRPCIDKey(id)
	if key == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.methods[key] = method
}

func (r *inFlightRequests) pop(id interface{}) (string, bool) {
	key := jsonRPCIDKey(id)
	if key == "" {
		return "", false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	method, ok := r.methods[key]
	if ok {
		delete(r.methods, key)
	}
	return method, ok
}

func RunMCPProxy(downstreamName string, stdin io.Reader, stdout, stderr io.Writer) error {
	cfg, err := config.LoadConfig(config.ConfigPath())
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	proxyCfg, err := findMCPProxy(cfg, downstreamName)
	if err != nil {
		return err
	}

	cmd := exec.Command(proxyCfg.Command, proxyCfg.Args...)
	cmd.Env = buildProxyEnv(proxyCfg.Env)
	cmd.Stderr = stderr

	downstreamIn, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("downstream stdin: %w", err)
	}
	defer func() { _ = downstreamIn.Close() }()

	downstreamOut, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("downstream stdout: %w", err)
	}

	if startErr := cmd.Start(); startErr != nil {
		return fmt.Errorf("start downstream %s: %w", downstreamName, startErr)
	}
	defer func() {
		_ = downstreamIn.Close()
		_ = downstreamOut.Close()
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		_ = cmd.Wait()
	}()

	requests := newInFlightRequests()
	agentWriter := &lockedWriter{w: stdout}
	errCh := make(chan error, 2)

	go func() {
		proxyErr := proxyAgentToDownstream(stdin, downstreamIn, agentWriter, requests)
		_ = downstreamIn.Close()
		errCh <- proxyErr
	}()
	go func() {
		errCh <- proxyDownstreamToAgent(downstreamOut, agentWriter, requests)
	}()

	err = <-errCh
	if errors.Is(err, io.EOF) {
		return nil
	}
	return err
}

func proxyAgentToDownstream(stdin io.Reader, downstream, agent io.Writer, requests *inFlightRequests) error {
	reader := bufio.NewReader(stdin)
	for {
		payload, err := readMCPFrame(reader)
		if err != nil {
			var frameErr *mcpFrameTooLargeError
			if errors.As(err, &frameErr) {
				slog.Warn("rejecting oversized MCP agent request", "content_length", frameErr.contentLength)
				data, encodeErr := encodeJSONRPC(jsonRPCErrorResponse(nil, -32600, frameErr.Error()))
				if encodeErr != nil {
					return encodeErr
				}
				if writeErr := writeMCPFrame(agent, data); writeErr != nil {
					return writeErr
				}
				return nil
			}
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}

		msg, err := decodeJSONRPC(payload)
		if err != nil {
			slog.Warn("rejecting malformed MCP agent request", "error", err)
			data, encodeErr := encodeJSONRPC(jsonRPCErrorResponse(nil, -32700, fmt.Sprintf("invalid JSON-RPC request: %v", err)))
			if encodeErr != nil {
				return encodeErr
			}
			if writeErr := writeMCPFrame(agent, data); writeErr != nil {
				return writeErr
			}
			continue
		}

		if method, _ := msg["method"].(string); method != "" {
			allowed, response, err := interceptProxyRequest(msg)
			if err != nil {
				return err
			}
			if !allowed {
				if response != nil {
					data, err := encodeJSONRPC(response)
					if err != nil {
						return err
					}
					if err := writeMCPFrame(agent, data); err != nil {
						return err
					}
				}
				continue
			}
			requests.add(msg["id"], method)
		}

		if err := writeMCPFrame(downstream, payload); err != nil {
			return err
		}
	}
}

func proxyDownstreamToAgent(downstream io.Reader, agent io.Writer, requests *inFlightRequests) error {
	reader := bufio.NewReader(downstream)
	for {
		payload, err := readMCPFrame(reader)
		if err != nil {
			slog.Warn("downstream MCP frame error", "error", err)
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}

		msg, err := decodeJSONRPC(payload)
		if err != nil {
			slog.Warn("forwarding malformed downstream payload", "error", err)
			if writeErr := writeMCPFrame(agent, payload); writeErr != nil {
				return writeErr
			}
			continue
		}
		if !isJSONRPCResponseEnvelope(msg) {
			slog.Warn("forwarding malformed downstream JSON-RPC envelope", "message", msg)
		}

		if _, hasID := msg["id"]; hasID {
			method, ok := requests.pop(msg["id"])
			if !ok {
				slog.Warn("dropping unsolicited downstream response", "id", msg["id"])
				continue
			}
			if method == "tools/list" {
				logToolNames(msg)
			}
		} else {
			slog.Warn("dropping downstream notification without matching request")
			continue
		}

		if err := writeMCPFrame(agent, payload); err != nil {
			return err
		}
	}
}

func interceptProxyRequest(msg jsonRPCMessage) (bool, jsonRPCMessage, error) {
	method, _ := msg["method"].(string)
	switch method {
	case "tools/call":
		return interceptToolCall(msg)
	case "resources/read", "resources/subscribe":
		if sensitive, target := isSensitiveResourceRequest(msg); sensitive {
			return false, jsonRPCErrorResponse(msg["id"], -32000, fmt.Sprintf("fuse denied sensitive resource access: %s", target)), nil
		}
	}
	return true, nil, nil
}

func interceptToolCall(msg jsonRPCMessage) (bool, jsonRPCMessage, error) {
	params, _ := msg["params"].(map[string]interface{})
	name, _ := params["name"].(string)
	arguments, _ := params["arguments"].(map[string]interface{})

	decision := core.ClassifyMCPTool(name, arguments)
	switch decision {
	case core.DecisionBlocked:
		return false, jsonRPCErrorResponse(msg["id"], -32000, fmt.Sprintf("fuse blocked MCP tool %s", name)), nil
	case core.DecisionApproval:
		approved, err := requestMCPApproval(name, arguments)
		if err != nil {
			return false, jsonRPCErrorResponse(msg["id"], -32000, err.Error()), nil
		}
		if !approved {
			return false, jsonRPCErrorResponse(msg["id"], -32000, fmt.Sprintf("fuse denied MCP tool %s", name)), nil
		}
	}

	return true, nil, nil
}

func requestMCPApproval(name string, arguments map[string]interface{}) (bool, error) {
	database, secret, err := openDBAndSecret()
	if err != nil {
		return false, err
	}
	defer func() { _ = database.Close() }()

	mgr, err := approve.NewManager(database, secret)
	if err != nil {
		return false, err
	}

	result := &core.ClassifyResult{
		Decision:    core.DecisionApproval,
		Reason:      fmt.Sprintf("MCP tool %s requires approval", name),
		DecisionKey: computeMCPDecisionKey(name, arguments),
		SubResults: []core.SubCommandResult{
			{
				Command:  formatMCPCommand(name, arguments),
				Decision: core.DecisionApproval,
				Reason:   fmt.Sprintf("MCP tool %s requires approval", name),
			},
		},
	}

	decision, err := mgr.RequestApproval(result.DecisionKey, extractCommandFromResult(result), result.Reason, "", false)
	if err != nil {
		return false, err
	}
	return decision != core.DecisionBlocked, nil
}

func isSensitiveResourceRequest(msg jsonRPCMessage) (bool, string) {
	params, _ := msg["params"].(map[string]interface{})
	candidates := []string{}
	if uri, _ := params["uri"].(string); uri != "" {
		candidates = append(candidates, uri)
	}
	if path, _ := params["path"].(string); path != "" {
		candidates = append(candidates, path)
	}
	for _, candidate := range candidates {
		if isSensitiveResourceTarget(candidate) {
			return true, candidate
		}
	}
	return false, ""
}

func isSensitiveResourceTarget(target string) bool {
	sensitive := []string{"~/.fuse", "~/.ssh", ".claude", "secret.key", "fuse.db"}
	for _, token := range sensitive {
		if strings.Contains(target, token) {
			return true
		}
	}
	return false
}

func jsonRPCErrorResponse(id interface{}, code int, message string) jsonRPCMessage {
	return jsonRPCMessage{
		"jsonrpc": "2.0",
		"id":      id,
		"error": map[string]interface{}{
			"code":    code,
			"message": message,
		},
	}
}

func logToolNames(msg jsonRPCMessage) {
	result, _ := msg["result"].(map[string]interface{})
	tools, _ := result["tools"].([]interface{})
	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		toolMap, _ := tool.(map[string]interface{})
		name, _ := toolMap["name"].(string)
		if name != "" {
			names = append(names, name)
		}
	}
	if len(names) > 0 {
		slog.Info("downstream tools listed", "tools", names)
	}
}

func findMCPProxy(cfg *config.Config, name string) (*config.MCPProxy, error) {
	if cfg == nil {
		return nil, fmt.Errorf("no config loaded")
	}
	for i := range cfg.MCPProxies {
		if cfg.MCPProxies[i].Name == name {
			return &cfg.MCPProxies[i], nil
		}
	}
	return nil, fmt.Errorf("no mcp proxy configured for %q", name)
}

func buildProxyEnv(extra map[string]string) []string {
	env := os.Environ()
	for k, v := range extra {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}
	return env
}

func isJSONRPCResponseEnvelope(msg jsonRPCMessage) bool {
	if jsonrpc, _ := msg["jsonrpc"].(string); jsonrpc != "2.0" {
		return false
	}
	if _, ok := msg["id"]; !ok {
		return false
	}
	_, hasResult := msg["result"]
	_, hasError := msg["error"]
	return hasResult != hasError
}

type lockedWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func (w *lockedWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.w.Write(p)
}
