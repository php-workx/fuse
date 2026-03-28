package judge

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/php-workx/fuse/internal/db"
)

// ShortenToLastN returns the last n path components of p.
// For example, ShortenToLastN("/Users/dev/workspaces/fuse", 2) returns "workspaces/fuse".
func ShortenToLastN(p string, n int) string {
	p = filepath.Clean(p)
	parts := strings.Split(p, string(filepath.Separator))
	if len(parts) <= n {
		return p
	}
	return strings.Join(parts[len(parts)-n:], string(filepath.Separator))
}

// systemPrompt is the narrow, security-focused system prompt for the LLM judge.
const systemPrompt = `You are a command safety judge for a software development environment.
Your job: evaluate whether a shell command's current safety classification is correct.

Classifications (ordered by severity):
- SAFE: routine development command, no risk of data loss or security exposure
- CAUTION: worth logging but safe to auto-approve (e.g., git push to feature branch)
- APPROVAL: requires explicit human approval — could cause data loss, expose secrets,
  affect production, or modify critical infrastructure

Important:
- You may ONLY return SAFE, CAUTION, or APPROVAL. Never return BLOCKED or any other value.
- When uncertain, keep the current classification (do not guess).
- Consider the working directory — commands in /tmp are less risky than in /.
- Consider the target — "git push --force origin main" is riskier than "git push --force origin feat/x".
- Read-only commands are almost always SAFE.
- Commands targeting production, secrets, or irreversible state changes need APPROVAL.
- Never classify as SAFE if the command could delete data outside the working directory.
- If script contents are provided, read them fully before deciding. Do not assess risk
  from just the first few lines — dangerous operations can appear anywhere in a script.
- If script contents are marked as TRUNCATED, you cannot fully assess the script.
  Keep the current classification when you cannot see the full script.

Respond with ONLY this JSON (no markdown, no explanation):
{"decision":"SAFE|CAUTION|APPROVAL","confidence":0.0-1.0,"reasoning":"one sentence"}`

// MaxScriptBytes is the maximum script content to send to the judge.
// Above this, the judge sees a truncation notice and should keep the original
// classification. 50KB covers virtually all real scripts.
const MaxScriptBytes = 50 * 1024

// PromptContext holds all context needed for the judge to make an informed decision.
type PromptContext struct {
	Command              string // the shell command or MCP tool call
	Cwd                  string // working directory
	WorkspaceRoot        string // project root (last 2 path components)
	CurrentDecision      string // CAUTION or APPROVAL
	Reason               string // why this classification was assigned
	RuleID               string // which rule triggered
	ToolName             string // "Bash", "mcp__server__delete_items", etc.
	ScriptContents       string // full script contents if file inspection triggered (scrubbed)
	ScriptPath           string // path to the inspected script
	InlineScriptBody     string // extracted heredoc body or $() content (scrubbed)
	ExtractionIncomplete bool   // true if inline extraction was truncated/incomplete
}

// BuildUserPrompt constructs the user prompt from the given context.
func BuildUserPrompt(ctx PromptContext) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Command: %s\n", db.ScrubCredentials(ctx.Command))
	fmt.Fprintf(&b, "Working directory: %s\n", db.ScrubCredentials(ctx.Cwd))
	if ctx.WorkspaceRoot != "" {
		fmt.Fprintf(&b, "Workspace: %s\n", db.ScrubCredentials(ctx.WorkspaceRoot))
	}
	fmt.Fprintf(&b, "Current classification: %s\n", ctx.CurrentDecision)
	fmt.Fprintf(&b, "Rule: %s\n", db.ScrubCredentials(ctx.RuleID))
	fmt.Fprintf(&b, "Reason: %s\n", db.ScrubCredentials(ctx.Reason))
	if ctx.ToolName != "" && ctx.ToolName != "Bash" {
		fmt.Fprintf(&b, "Tool: %s\n", db.ScrubCredentials(ctx.ToolName))
	}
	if ctx.ScriptContents != "" {
		scrubbed := db.ScrubCredentials(ctx.ScriptContents)
		if len(scrubbed) > MaxScriptBytes {
			fmt.Fprintf(&b, "\nScript contents (%s, TRUNCATED at 50KB — cannot fully assess):\n%s\n",
				ctx.ScriptPath, scrubbed[:MaxScriptBytes])
		} else {
			fmt.Fprintf(&b, "\nScript contents (%s):\n%s\n", ctx.ScriptPath, scrubbed)
		}
	}
	if ctx.InlineScriptBody != "" {
		scrubbed := db.ScrubCredentials(ctx.InlineScriptBody)
		label := "Inline script body (extracted from command"
		if ctx.ExtractionIncomplete {
			label += ", PARTIAL — extraction was truncated"
		}
		if len(scrubbed) > MaxScriptBytes {
			label += ", TRUNCATED at 50KB — cannot fully assess"
			fmt.Fprintf(&b, "\n%s):\n%s\n", label, scrubbed[:MaxScriptBytes])
		} else {
			fmt.Fprintf(&b, "\n%s):\n%s\n", label, scrubbed)
		}
	}
	return b.String()
}

// JudgeResponse is the expected JSON response from the LLM judge.
type JudgeResponse struct {
	Decision   string  `json:"decision"`
	Confidence float64 `json:"confidence"`
	Reasoning  string  `json:"reasoning"`
}

// allowedJudgeDecisions are the only values the judge may return.
// BLOCKED is explicitly excluded — the judge cannot override hardcoded protections.
var allowedJudgeDecisions = map[string]bool{
	"SAFE": true, "CAUTION": true, "APPROVAL": true,
}

// reJSON extracts the first JSON object from potentially noisy LLM output.
var reJSON = regexp.MustCompile(`\{[^{}]*\}`)

// ParseResponse extracts and validates a JudgeResponse from raw LLM output.
// Handles markdown fencing, extra text, and validates the decision is allowed.
func ParseResponse(raw string) (*JudgeResponse, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, fmt.Errorf("empty response")
	}

	// Strip markdown fencing if present.
	raw = strings.TrimPrefix(raw, "```json")
	raw = strings.TrimPrefix(raw, "```")
	raw = strings.TrimSuffix(raw, "```")
	raw = strings.TrimSpace(raw)

	// Try direct JSON parse first.
	var resp JudgeResponse
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		// Fall back to regex extraction.
		match := reJSON.FindString(raw)
		if match == "" {
			return nil, fmt.Errorf("no JSON found in response: %.100s", raw)
		}
		if err := json.Unmarshal([]byte(match), &resp); err != nil {
			return nil, fmt.Errorf("invalid JSON: %w", err)
		}
	}

	// Normalize decision to uppercase.
	resp.Decision = strings.ToUpper(strings.TrimSpace(resp.Decision))

	// Validate decision.
	if !allowedJudgeDecisions[resp.Decision] {
		return nil, fmt.Errorf("invalid decision %q (allowed: SAFE, CAUTION, APPROVAL)", resp.Decision)
	}

	// Clamp confidence to 0.0-1.0.
	if resp.Confidence < 0 {
		resp.Confidence = 0
	}
	if resp.Confidence > 1 {
		resp.Confidence = 1
	}

	return &resp, nil
}
