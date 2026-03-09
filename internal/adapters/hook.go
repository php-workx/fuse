package adapters

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/runger/fuse/internal/approve"
	"github.com/runger/fuse/internal/config"
	"github.com/runger/fuse/internal/core"
	"github.com/runger/fuse/internal/db"
	"github.com/runger/fuse/internal/policy"
)

// hookTimeout is the internal timeout before Claude's 30s kill.
const hookTimeout = 25 * time.Second

// HookRequest is the JSON input from Claude Code's PreToolUse hook.
type HookRequest struct {
	ToolName  string          `json:"tool_name"`
	ToolInput json.RawMessage `json:"tool_input"`
	SessionID string          `json:"session_id"`
	Cwd       string          `json:"cwd"`
}

// BashToolInput is the tool_input for Bash tool calls.
type BashToolInput struct {
	Command string `json:"command"`
}

// RunHook processes a Claude Code hook request.
// Returns the exit code: 0 = allow, 2 = block.
// Writes directive messages to stderr.
// NEVER writes to stdout (stdout is for tool output in hook mode).
func RunHook(stdin io.Reader, stderr io.Writer) int {
	ctx, cancel := context.WithTimeout(context.Background(), hookTimeout)
	defer cancel()

	resultCh := make(chan int, 1)
	go func() {
		resultCh <- runHookInternal(stdin, stderr)
	}()

	select {
	case code := <-resultCh:
		return code
	case <-ctx.Done():
		fmt.Fprintln(stderr, "fuse:TIMEOUT_WAITING_FOR_USER STOP. The user did not approve this action in time. Do not retry this exact command.")
		return 2
	}
}

// runHookInternal contains the core hook logic without timeout management.
func runHookInternal(stdin io.Reader, stderr io.Writer) int {
	// Read and parse JSON from stdin.
	data, err := io.ReadAll(stdin)
	if err != nil {
		slog.Error("failed to read stdin", "error", err)
		fmt.Fprintln(stderr, "fuse:POLICY_BLOCK STOP. Failed to read input. Do not retry this exact command. Ask the user for guidance.")
		return 2 // fail-closed
	}

	var req HookRequest
	if err := json.Unmarshal(data, &req); err != nil {
		slog.Error("invalid JSON input", "error", err)
		fmt.Fprintln(stderr, "fuse:POLICY_BLOCK STOP. Invalid JSON input. Do not retry this exact command. Ask the user for guidance.")
		return 2 // fail-closed
	}

	// Validate tool_name is present.
	if req.ToolName == "" {
		fmt.Fprintln(stderr, "fuse:POLICY_BLOCK STOP. Missing tool_name. Do not retry this exact command. Ask the user for guidance.")
		return 2
	}

	// Route based on tool_name.
	if strings.HasPrefix(req.ToolName, "mcp__") {
		return handleMCPTool(req, stderr)
	}

	if req.ToolName != "Bash" {
		// Non-Bash, non-MCP tool: allow (fuse only mediates shell commands and MCP).
		return 0
	}

	return handleBashTool(req, stderr)
}

// handleMCPTool handles MCP tool classification.
func handleMCPTool(req HookRequest, stderr io.Writer) int {
	// Parse tool_input as a generic map for MCP argument scanning.
	var args map[string]interface{}
	if len(req.ToolInput) > 0 {
		if err := json.Unmarshal(req.ToolInput, &args); err != nil {
			// If we can't parse args, classify with nil args (name-only).
			slog.Warn("failed to parse MCP tool_input", "error", err)
			args = nil
		}
	}

	// Strip the "mcp__<server>__" prefix to get the action name for classification.
	toolAction := extractMCPAction(req.ToolName)
	decision := core.ClassifyMCPTool(toolAction, args)

	switch decision {
	case core.DecisionBlocked:
		fmt.Fprintln(stderr, "fuse:POLICY_BLOCK STOP. MCP tool call blocked by policy. Do not retry this exact command. Ask the user for guidance.")
		return 2
	case core.DecisionCaution:
		fmt.Fprintf(stderr, "[fuse] CAUTION: MCP tool %s classified as CAUTION\n", req.ToolName)
		return 0
	case core.DecisionApproval:
		// For now, approval for MCP tools in hook mode blocks without TTY prompt.
		// Future: add TTY approval flow.
		fmt.Fprintf(stderr, "[fuse] CAUTION: MCP tool %s requires approval (auto-allowing in v1)\n", req.ToolName)
		return 0
	default:
		// SAFE
		return 0
	}
}

// extractMCPAction strips the "mcp__<server>__" prefix from an MCP tool name
// to get the action portion for classification.
func extractMCPAction(toolName string) string {
	// Format: mcp__<server>__<action>
	// Strip "mcp__" prefix first.
	rest := strings.TrimPrefix(toolName, "mcp__")
	// Find the next "__" separator.
	idx := strings.Index(rest, "__")
	if idx >= 0 && idx+2 < len(rest) {
		return rest[idx+2:]
	}
	// If no second separator, use the whole remainder.
	return rest
}

// handleBashTool handles Bash tool classification.
func handleBashTool(req HookRequest, stderr io.Writer) int {
	// Parse tool_input to extract command.
	if len(req.ToolInput) == 0 {
		fmt.Fprintln(stderr, "fuse:POLICY_BLOCK STOP. Missing tool_input. Do not retry this exact command. Ask the user for guidance.")
		return 2
	}

	var input BashToolInput
	if err := json.Unmarshal(req.ToolInput, &input); err != nil {
		fmt.Fprintln(stderr, "fuse:POLICY_BLOCK STOP. Invalid tool_input JSON. Do not retry this exact command. Ask the user for guidance.")
		return 2
	}

	// Empty command is allowed (no-op).
	if input.Command == "" {
		return 0
	}

	// Load policy for classification.
	evaluator := loadPolicyEvaluator()

	// Classify the command.
	shellReq := core.ShellRequest{
		RawCommand: input.Command,
		Cwd:        req.Cwd,
		Source:     "hook",
		SessionID:  req.SessionID,
	}

	result, err := core.Classify(shellReq, evaluator)
	if err != nil {
		slog.Error("classification error", "error", err)
		fmt.Fprintln(stderr, "fuse:POLICY_BLOCK STOP. Classification error. Do not retry this exact command. Ask the user for guidance.")
		return 2 // fail-closed
	}
	if result == nil {
		slog.Error("classification returned nil result")
		fmt.Fprintln(stderr, "fuse:POLICY_BLOCK STOP. Classification error. Do not retry this exact command. Ask the user for guidance.")
		return 2 // fail-closed
	}

	switch result.Decision {
	case core.DecisionSafe:
		return 0

	case core.DecisionBlocked:
		msg := fmt.Sprintf("fuse:POLICY_BLOCK STOP. %s Do not retry this exact command. Ask the user for guidance.", result.Reason)
		fmt.Fprintln(stderr, msg)
		return 2

	case core.DecisionCaution:
		fmt.Fprintf(stderr, "[fuse] CAUTION: %s\n", result.Reason)
		return 0

	case core.DecisionApproval:
		return handleApproval(req, result, stderr)

	default:
		// Unknown decision: fail-closed.
		fmt.Fprintln(stderr, "fuse:POLICY_BLOCK STOP. Unknown classification result. Do not retry this exact command. Ask the user for guidance.")
		return 2
	}
}

// handleApproval handles the APPROVAL decision path, which requires DB access.
func handleApproval(req HookRequest, result *core.ClassifyResult, stderr io.Writer) int {
	// Lazy DB access: only APPROVAL path opens the database.
	database, secret, err := openDBAndSecret()
	if err != nil {
		slog.Error("failed to open database for approval", "error", err)
		fmt.Fprintln(stderr, "fuse:POLICY_BLOCK STOP. Internal error (database). Do not retry this exact command. Ask the user for guidance.")
		return 2
	}
	defer func() { _ = database.Close() }()

	mgr, mgrErr := approve.NewManager(database, secret)
	if mgrErr != nil {
		slog.Error("failed to create approval manager", "error", mgrErr)
		fmt.Fprintln(stderr, "fuse:POLICY_BLOCK STOP. Internal error (approval). Do not retry this exact command. Ask the user for guidance.")
		return 2
	}

	decision, err := mgr.RequestApproval(result.DecisionKey, extractCommandFromResult(result), result.Reason, req.SessionID, true)
	if err != nil {
		slog.Error("approval error", "error", err)
		fmt.Fprintln(stderr, "fuse:POLICY_BLOCK STOP. Approval process failed. Do not retry this exact command. Ask the user for guidance.")
		return 2
	}

	switch decision {
	case core.DecisionApproval, core.DecisionSafe, core.DecisionCaution:
		return 0
	default:
		fmt.Fprintln(stderr, "fuse:USER_DENIED STOP. Do not retry this exact command without new user input.")
		return 2
	}
}

// extractCommandFromResult returns the raw command from the first sub-result,
// or a placeholder if unavailable.
func extractCommandFromResult(result *core.ClassifyResult) string {
	if len(result.SubResults) > 0 {
		return result.SubResults[0].Command
	}
	return "(unknown command)"
}

// loadPolicyEvaluator loads user policy and returns a PolicyEvaluator.
// Returns a default evaluator if no policy file exists.
func loadPolicyEvaluator() core.PolicyEvaluator {
	policyPath := config.PolicyPath()
	policyCfg, err := policy.LoadPolicy(policyPath)
	if err != nil {
		// No policy file or parse error: use default (no user rules).
		slog.Debug("no user policy loaded", "path", policyPath, "error", err)
		return policy.NewEvaluator(nil)
	}
	return policy.NewEvaluator(policyCfg)
}

// openDBAndSecret opens the SQLite database and reads the HMAC secret.
func openDBAndSecret() (*db.DB, []byte, error) {
	if err := config.EnsureDirectories(); err != nil {
		return nil, nil, fmt.Errorf("ensure directories: %w", err)
	}

	database, err := db.OpenDB(config.DBPath())
	if err != nil {
		return nil, nil, fmt.Errorf("open database: %w", err)
	}

	secret, err := os.ReadFile(config.SecretPath())
	if err != nil {
		_ = database.Close()
		return nil, nil, fmt.Errorf("read secret: %w", err)
	}

	return database, secret, nil
}
