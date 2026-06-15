package adapters

import (
	"fmt"

	"github.com/php-workx/fuse/internal/core"
)

func classifyMCPToolCall(toolName string, args map[string]interface{}, evaluator core.PolicyEvaluator) *core.ClassifyResult {
	mcpCommand := formatMCPCommand(toolName, args)
	if evaluator != nil {
		if d, reason := evaluator.EvaluateUserRules(mcpCommand); d != "" {
			return mcpToolResult(toolName, args, d, reason)
		}
	}

	decision := core.ClassifyMCPTool(toolName, args)
	return mcpToolResult(toolName, args, decision, fmt.Sprintf("MCP tool %s classified as %s", toolName, decision))
}

func mcpToolResult(toolName string, args map[string]interface{}, decision core.Decision, reason string) *core.ClassifyResult {
	command := formatMCPCommand(toolName, args)
	return &core.ClassifyResult{
		Decision:    decision,
		Reason:      reason,
		DecisionKey: computeMCPDecisionKey(toolName, args),
		SubResults: []core.SubCommandResult{
			{
				Command:  command,
				Decision: decision,
				Reason:   reason,
			},
		},
	}
}
