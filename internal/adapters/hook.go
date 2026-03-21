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
	"github.com/runger/fuse/internal/events"
	"github.com/runger/fuse/internal/policy"
)

// hookTimeout is fuse's internal safety timeout.
// Claude Code's default hook timeout is 600s; users can configure per-hook
// via the "timeout" field in settings. We default to 300s — long enough for a
// human to notice and approve via fuse monitor, but well under Claude Code's
// limit. Override with FUSE_HOOK_TIMEOUT env var or test helpers.
var hookTimeout = 300 * time.Second

// pendingApprovalMsg tells the agent the approval is still pending.
// Used when the hook times out before the user approves via the TUI.
const pendingApprovalMsg = "fuse:PENDING_APPROVAL WAIT. This command requires user approval " +
	"via fuse monitor. The approval request has been queued. " +
	"Wait 30-60 seconds, then retry the same command."

func init() {
	if v := os.Getenv("FUSE_HOOK_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			hookTimeout = d
		}
	}
}

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
		fmt.Fprintln(stderr, pendingApprovalMsg)
		return 2
	}
}

// runHookInternal contains the core hook logic without timeout management.
func runHookInternal(stdin io.Reader, stderr io.Writer) int {
	mode := config.Mode()
	if mode == config.ModeDisabled {
		return 0 // fully disabled: zero processing
	}
	dryRun := mode == config.ModeDryRun

	// Load config for log level.
	cfg := loadRuntimeConfig()
	if cfg != nil && cfg.LogLevel != "" {
		events.SetLogLevel(cfg.LogLevel)
	}

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

	// Route based on tool_name and classify.
	var exitCode int
	switch {
	case strings.HasPrefix(req.ToolName, "mcp__"):
		exitCode = handleMCPTool(req, stderr, cfg, dryRun)
	case req.ToolName == "Bash":
		exitCode = handleBashTool(req, stderr, cfg, dryRun)
	case isNativeClaudeFileTool(req.ToolName):
		exitCode = handleNativeFileTool(req, stderr, cfg, dryRun)
	default:
		// Non-Bash, non-MCP, non-native-file tool: allow.
		return 0
	}

	return exitCode
}

// handleMCPTool handles MCP tool classification.
func handleMCPTool(req HookRequest, stderr io.Writer, cfg *config.Config, dryRun bool) int {
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
	mcpCommand := formatMCPCommand(req.ToolName, args)

	// MCP tools don't use tag_overrides — dryrun always allows.
	if dryRun {
		logHookEventFields(req.SessionID, mcpCommand, "", string(decision), "", "")
		return 0
	}

	switch decision {
	case core.DecisionBlocked:
		fmt.Fprintln(stderr, "fuse:POLICY_BLOCK STOP. MCP tool call blocked by policy. Do not retry this exact command. Ask the user for guidance.")
		logHookEventFields(req.SessionID, mcpCommand, "", string(core.DecisionBlocked), "", "MCP tool blocked by policy")
		return 2
	case core.DecisionCaution:
		fmt.Fprintf(stderr, "[fuse] CAUTION: MCP tool %s classified as CAUTION\n", req.ToolName)
		logHookEventFields(req.SessionID, mcpCommand, "", string(core.DecisionCaution), "", fmt.Sprintf("MCP tool %s classified as CAUTION", req.ToolName))
		return 0
	case core.DecisionApproval:
		result := &core.ClassifyResult{
			Decision:    core.DecisionApproval,
			Reason:      fmt.Sprintf("MCP tool %s requires approval", req.ToolName),
			DecisionKey: computeMCPDecisionKey(req.ToolName, args),
			SubResults: []core.SubCommandResult{
				{
					Command:  formatMCPCommand(req.ToolName, args),
					Decision: core.DecisionApproval,
					Reason:   fmt.Sprintf("MCP tool %s requires approval", req.ToolName),
				},
			},
		}
		return handleApproval(req, result, stderr, cfg, dryRun)
	default:
		// SAFE
		logHookEventFields(req.SessionID, mcpCommand, "", string(core.DecisionSafe), "", "")
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
func handleBashTool(req HookRequest, stderr io.Writer, cfg *config.Config, dryRun bool) int {
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

	// Empty command is blocked per spec §3.1.
	if input.Command == "" {
		fmt.Fprintln(stderr, "fuse:POLICY_BLOCK STOP. Empty command. Do not retry this exact command. Ask the user for guidance.")
		return 2
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

	// Log per-tag dryrun matches for observability.
	for i := range result.DryRunMatches {
		m := &result.DryRunMatches[i]
		logHookEventFields(req.SessionID, input.Command, req.Cwd,
			string(m.Decision), m.RuleID, m.Reason+" (dryrun-override)")
	}

	// In dryrun mode, only enforce decisions from tag_overrides.
	// Without TagOverrideEnforced, log and allow all commands.
	if dryRun && !result.TagOverrideEnforced {
		logHookEvent(req.SessionID, input.Command, req.Cwd, result)
		if result.Decision == core.DecisionCaution {
			fmt.Fprintf(stderr, "[fuse] CAUTION: %s\n", result.Reason)
		}
		return 0
	}

	switch result.Decision {
	case core.DecisionSafe:
		logHookEvent(req.SessionID, input.Command, req.Cwd, result)
		return 0

	case core.DecisionBlocked:
		msg := fmt.Sprintf("fuse:POLICY_BLOCK STOP. %s Do not retry this exact command. Ask the user for guidance.", result.Reason)
		fmt.Fprintln(stderr, msg)
		logHookEvent(req.SessionID, input.Command, req.Cwd, result)
		return 2

	case core.DecisionCaution:
		fmt.Fprintf(stderr, "[fuse] CAUTION: %s\n", result.Reason)
		logHookEvent(req.SessionID, input.Command, req.Cwd, result)
		return 0

	case core.DecisionApproval:
		return handleApproval(req, result, stderr, cfg, dryRun)

	default:
		// Unknown decision: fail-closed.
		fmt.Fprintln(stderr, "fuse:POLICY_BLOCK STOP. Unknown classification result. Do not retry this exact command. Ask the user for guidance.")
		return 2
	}
}

// handleApproval handles the APPROVAL decision path, which requires DB access.
func handleApproval(req HookRequest, result *core.ClassifyResult, stderr io.Writer, cfg *config.Config, dryRun bool) int {
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

	// Use the hook timeout (hookTimeout - 2s margin) as the approval context.
	// This ensures the DB poll respects the hook's timeout budget and doesn't
	// wait indefinitely with context.Background().
	approvalCtx, approvalCancel := context.WithTimeout(context.Background(), hookTimeout-2*time.Second)
	defer approvalCancel()
	decision, err := mgr.RequestApproval(approvalCtx, result.DecisionKey, extractCommandFromResult(result), result.Reason, req.SessionID, "hook", true, dryRun)
	if err != nil {
		slog.Error("approval error", "error", err)
		// If the error is from a non-interactive prompt timeout, tell the agent
		// to retry — the user may approve via fuse monitor.
		if strings.Contains(err.Error(), "NON_INTERACTIVE_MODE") || strings.Contains(err.Error(), "TIMEOUT_WAITING") {
			fmt.Fprintln(stderr, pendingApprovalMsg)
			return 2
		}
		if msg := extractFuseDirective(err); msg != "" {
			fmt.Fprintln(stderr, msg)
			return 2
		}
		fmt.Fprintln(stderr, "fuse:POLICY_BLOCK STOP. Approval process failed. Do not retry this exact command. Ask the user for guidance.")
		return 2
	}

	cleanupExecutionState(database, cfg)

	cmd := extractCommandFromResult(result)
	switch decision {
	case core.DecisionApproval, core.DecisionSafe, core.DecisionCaution:
		_ = database.LogEvent(&db.EventRecord{
			SessionID: req.SessionID, Command: cmd, Decision: string(decision),
			RuleID: result.RuleID, Reason: result.Reason, Source: "hook", Agent: "claude", Cwd: req.Cwd,
		})
		return 0
	default:
		_ = database.LogEvent(&db.EventRecord{
			SessionID: req.SessionID, Command: cmd, Decision: "BLOCKED",
			RuleID: result.RuleID, Reason: "user denied", Source: "hook", Agent: "claude", Cwd: req.Cwd,
		})
		fmt.Fprintln(stderr, "fuse:USER_DENIED STOP. Do not retry this exact command without new user input.")
		return 2
	}
}

// logHookEvent logs a classification event best-effort (non-blocking).
func logHookEvent(sessionID, command, cwd string, result *core.ClassifyResult) {
	logHookEventFields(sessionID, command, cwd, string(result.Decision), result.RuleID, result.Reason)
}

// logHookEventFields logs a hook event with individual fields. Best-effort.
func logHookEventFields(sessionID, command, cwd, decision, ruleID, reason string) {
	database, err := db.OpenDB(config.DBPath())
	if err != nil {
		return // best-effort: skip if DB unavailable
	}
	defer func() { _ = database.Close() }()
	_ = database.LogEvent(&db.EventRecord{
		SessionID: sessionID,
		Command:   command,
		Decision:  decision,
		RuleID:    ruleID,
		Reason:    reason,
		Source:    "hook",
		Agent:     "claude",
		Cwd:       cwd,
	})
	cleanupExecutionState(database, loadRuntimeConfig())
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

	secret, err := db.EnsureSecret(config.SecretPath())
	if err != nil {
		_ = database.Close()
		return nil, nil, fmt.Errorf("read secret: %w", err)
	}

	return database, secret, nil
}

func computeMCPDecisionKey(toolName string, args map[string]interface{}) string {
	return core.ComputeDecisionKey("mcp", formatMCPCommand(toolName, args), "")
}

func formatMCPCommand(toolName string, args map[string]interface{}) string {
	if len(args) == 0 {
		return toolName
	}
	data, err := json.Marshal(args)
	if err != nil {
		return toolName
	}
	return toolName + ":" + string(data)
}

func extractFuseDirective(err error) string {
	msg := err.Error()
	idx := strings.Index(msg, "fuse:")
	if idx < 0 {
		return ""
	}
	return msg[idx:]
}
