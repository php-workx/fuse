package core

import (
	"fmt"
	"regexp"
	"strings"
)

// MCP tool name prefix classifications (§6.6 Layer 1).
var (
	mcpSafePrefixes = []string{
		"read_", "get_", "list_", "search_", "describe_",
		"show_", "view_", "count_", "check_", "validate_", "verify_",
	}
	mcpCautionPrefixes = []string{
		"create_", "update_", "modify_", "set_", "put_",
		"add_", "enable_", "configure_", "install_",
	}
	mcpApprovalPrefixes = []string{
		"delete_", "remove_", "destroy_", "drop_", "purge_",
		"revoke_", "disable_", "terminate_", "stop_", "kill_",
	}
)

// Compiled destructive patterns for MCP argument content scanning (§6.6 Layer 2).
var mcpDestructivePatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\brm\s+-rf\b`),
	regexp.MustCompile(`(?i)\bdrop\s+table\b`),
	regexp.MustCompile(`(?i)\bdrop\s+database\b`),
	regexp.MustCompile(`(?i)\bdelete\s+from\b`),
	regexp.MustCompile(`(?i)\btruncate\b`),
	regexp.MustCompile(`(?i)\bformat\b`),
}

// ClassifyMCPTool classifies an MCP tool call using two-layer analysis (§6.6).
func ClassifyMCPTool(toolName string, args map[string]interface{}) Decision {
	// Layer 1: Tool name prefix matching.
	nameDecision := classifyMCPByName(toolName)

	// Layer 2: Argument content scanning.
	argsDecision := DecisionSafe
	if args != nil {
		values, complete := flattenStringValues(args)
		for _, v := range values {
			if matchesDestructivePattern(v) {
				argsDecision = DecisionApproval
				break
			}
		}
		// Depth exhaustion: incomplete extraction gets at least CAUTION.
		if !complete && argsDecision == DecisionSafe {
			argsDecision = DecisionCaution
		}
	}

	// Layer 3: URL inspection in arguments (SEC-006).
	urlDecision := DecisionSafe
	if args != nil {
		if d, _ := InspectURLsInArgs(args); d != "" {
			urlDecision = d
		}
	}

	// Most restrictive wins.
	return MaxDecision(MaxDecision(nameDecision, argsDecision), urlDecision)
}

// classifyMCPByName classifies an MCP tool by its name prefix.
// Falls back to CAUTION for unmatched tool names.
func classifyMCPByName(toolName string) Decision {
	lower := strings.ToLower(toolName)

	// Defense-in-depth: strip mcp__<server>__ prefix if present.
	// The hook adapter already strips this via extractMCPAction(),
	// and the proxy receives raw MCP names. This protects future callers.
	if strings.HasPrefix(lower, "mcp__") {
		parts := strings.SplitN(lower, "__", 3)
		if len(parts) == 3 {
			lower = parts[2]
		}
	}

	// Check all prefix sets and take the most restrictive match.
	bestDecision := Decision("")

	for _, prefix := range mcpSafePrefixes {
		if strings.HasPrefix(lower, prefix) {
			if bestDecision == "" {
				bestDecision = DecisionSafe
			}
			break
		}
	}

	for _, prefix := range mcpCautionPrefixes {
		if strings.HasPrefix(lower, prefix) {
			if bestDecision == "" {
				bestDecision = DecisionCaution
			} else {
				bestDecision = MaxDecision(bestDecision, DecisionCaution)
			}
			break
		}
	}

	for _, prefix := range mcpApprovalPrefixes {
		if strings.HasPrefix(lower, prefix) {
			if bestDecision == "" {
				bestDecision = DecisionApproval
			} else {
				bestDecision = MaxDecision(bestDecision, DecisionApproval)
			}
			break
		}
	}

	// Fallback: CAUTION for unmatched tool names.
	if bestDecision == "" {
		return DecisionCaution
	}

	return bestDecision
}

// flattenStringValues recursively extracts all string values from a map,
// including values nested in maps and slices. Returns (values, complete).
// complete is false when maxExtractDepth was reached and content was skipped.
func flattenStringValues(m map[string]interface{}) ([]string, bool) {
	var result []string
	complete := true
	for _, v := range m {
		strs, c := extractStringsWithFlag(v, 0)
		result = append(result, strs...)
		if !c {
			complete = false
		}
	}
	return result, complete
}

// maxExtractDepth limits recursion into nested JSON structures.
const maxExtractDepth = 32

func extractStringsWithFlag(v interface{}, depth int) ([]string, bool) {
	if v == nil {
		return nil, true
	}
	if depth > maxExtractDepth {
		return nil, false // depth exhausted, extraction incomplete
	}

	complete := true
	switch val := v.(type) {
	case string:
		return []string{val}, true
	case map[string]interface{}:
		var result []string
		for _, mv := range val {
			strs, c := extractStringsWithFlag(mv, depth+1)
			result = append(result, strs...)
			if !c {
				complete = false
			}
		}
		return result, complete
	case []interface{}:
		var result []string
		for _, av := range val {
			strs, c := extractStringsWithFlag(av, depth+1)
			result = append(result, strs...)
			if !c {
				complete = false
			}
		}
		return result, complete
	default:
		s := fmt.Sprintf("%v", val)
		if s != "" {
			return []string{s}, true
		}
		return nil, true
	}
}

// matchesDestructivePattern checks if a string matches any of the
// destructive patterns used for MCP argument scanning.
func matchesDestructivePattern(s string) bool {
	for _, re := range mcpDestructivePatterns {
		if re.MatchString(s) {
			return true
		}
	}
	return false
}
