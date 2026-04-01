package judge

import (
	"strings"
	"testing"
)

func TestBuildUserPrompt_AllFields(t *testing.T) {
	ctx := PromptContext{
		Command:         "terraform destroy",
		Cwd:             "/workspace/infra",
		WorkspaceRoot:   "workspace/infra",
		CurrentDecision: "APPROVAL",
		Reason:          "IaC destruction command",
		RuleID:          "terraform-destroy",
		ToolName:        "Bash",
	}
	prompt := BuildUserPrompt(ctx)

	for _, want := range []string{"terraform destroy", "/workspace/infra", "workspace/infra", "APPROVAL", "IaC destruction command", "terraform-destroy"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt missing %q:\n%s", want, prompt)
		}
	}
	// Bash toolName should not be shown (it's the default).
	if strings.Contains(prompt, "Tool: Bash") {
		t.Error("prompt should not show 'Tool: Bash' (default tool)")
	}
}

func TestBuildUserPrompt_NonBashTool(t *testing.T) {
	ctx := PromptContext{
		Command:         "delete_items",
		Cwd:             "/tmp",
		CurrentDecision: "CAUTION",
		ToolName:        "mcp__server__delete_items",
	}
	prompt := BuildUserPrompt(ctx)
	if !strings.Contains(prompt, "Tool: mcp__server__delete_items") {
		t.Error("prompt should show non-Bash tool name")
	}
}

func TestBuildUserPrompt_EmptyFields(t *testing.T) {
	ctx := PromptContext{
		Command:         "ls",
		Cwd:             "/tmp",
		CurrentDecision: "CAUTION",
	}
	prompt := BuildUserPrompt(ctx)
	if strings.Contains(prompt, "Workspace:") {
		t.Error("should not show empty workspace")
	}
	if strings.Contains(prompt, "Tool:") {
		t.Error("should not show empty tool")
	}
}

func TestBuildUserPrompt_WithScriptContents(t *testing.T) {
	ctx := PromptContext{
		Command:         "bash deploy.sh",
		Cwd:             "/workspace",
		CurrentDecision: "APPROVAL",
		ScriptContents:  "#!/bin/bash\nkubectl apply -f k8s/\n",
		ScriptPath:      "deploy.sh",
	}
	prompt := BuildUserPrompt(ctx)
	if !strings.Contains(prompt, "Script contents (deploy.sh)") {
		t.Error("prompt should include script contents header")
	}
	if !strings.Contains(prompt, "kubectl apply") {
		t.Error("prompt should include script body")
	}
}

func TestBuildUserPrompt_TruncatedScript(t *testing.T) {
	// Create a script larger than MaxScriptBytes.
	bigScript := strings.Repeat("echo hello\n", MaxScriptBytes/11+1)
	ctx := PromptContext{
		Command:         "bash big.sh",
		Cwd:             "/workspace",
		CurrentDecision: "APPROVAL",
		ScriptContents:  bigScript,
		ScriptPath:      "big.sh",
	}
	prompt := BuildUserPrompt(ctx)
	if !strings.Contains(prompt, "TRUNCATED") {
		t.Error("prompt should mark truncated scripts")
	}
}

func TestBuildUserPrompt_ScrubsContextFields(t *testing.T) {
	ctx := PromptContext{
		Command:         "echo hello",
		Cwd:             "/tmp/api_key=EXAMPLE_API_KEY",
		WorkspaceRoot:   "/workspace/Cookie: session=EXAMPLE_SESSION_ID",
		CurrentDecision: "APPROVAL",
		Reason:          "Bearer EXAMPLE_TOKEN detected",
		RuleID:          "rule-token=ghp_EXAMPLE_GITHUB_TOKEN",
		ToolName:        "mcp__server__Authorization: Basic ZXhhbXBsZTp1c2Vy",
	}
	prompt := BuildUserPrompt(ctx)
	for _, forbidden := range []string{
		"EXAMPLE_API_KEY",
		"session=EXAMPLE_SESSION_ID",
		"EXAMPLE_TOKEN",
		"ghp_EXAMPLE_GITHUB_TOKEN",
		"ZXhhbXBsZTp1c2Vy",
	} {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("prompt leaked %q:\n%s", forbidden, prompt)
		}
	}
	if !strings.Contains(prompt, "[REDACTED") {
		t.Fatalf("expected scrubbed markers in prompt:\n%s", prompt)
	}
}

func TestSystemPrompt_DescribesRiskNotRuntimeEnforcement(t *testing.T) {
	for _, forbidden := range []string{
		"auto-approved unless you escalate it",
		"When downgrading APPROVAL, use CAUTION, not SAFE.",
	} {
		if strings.Contains(systemPrompt, forbidden) {
			t.Fatalf("systemPrompt should not contain %q:\n%s", forbidden, systemPrompt)
		}
	}
	for _, want := range []string{
		"potentially risky or state-changing",
		"extra scrutiny but does not justify APPROVAL",
	} {
		if !strings.Contains(systemPrompt, want) {
			t.Fatalf("systemPrompt missing %q:\n%s", want, systemPrompt)
		}
	}
}

func TestParseResponse_ValidJSON(t *testing.T) {
	resp, err := ParseResponse(`{"decision":"SAFE","confidence":0.92,"reasoning":"safe command"}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Decision != "SAFE" {
		t.Errorf("decision = %q, want SAFE", resp.Decision)
	}
	if resp.Confidence != 0.92 {
		t.Errorf("confidence = %f, want 0.92", resp.Confidence)
	}
	if resp.Reasoning != "safe command" {
		t.Errorf("reasoning = %q", resp.Reasoning)
	}
}

func TestParseResponse_MarkdownFenced(t *testing.T) {
	resp, err := ParseResponse("```json\n{\"decision\":\"CAUTION\",\"confidence\":0.8,\"reasoning\":\"risky\"}\n```")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Decision != "CAUTION" {
		t.Errorf("decision = %q, want CAUTION", resp.Decision)
	}
}

func TestParseResponse_ExtraText(t *testing.T) {
	resp, err := ParseResponse("Here is my assessment:\n{\"decision\":\"APPROVAL\",\"confidence\":0.88,\"reasoning\":\"needs approval\"}\nHope this helps!")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Decision != "APPROVAL" {
		t.Errorf("decision = %q, want APPROVAL", resp.Decision)
	}
}

func TestParseResponse_InvalidJSON(t *testing.T) {
	_, err := ParseResponse("I cannot evaluate this command safely.")
	if err == nil {
		t.Error("expected error for non-JSON response")
	}
}

func TestParseResponse_EmptyResponse(t *testing.T) {
	_, err := ParseResponse("")
	if err == nil {
		t.Error("expected error for empty response")
	}
}

func TestParseResponse_InvalidDecision_Blocked(t *testing.T) {
	_, err := ParseResponse(`{"decision":"BLOCKED","confidence":1.0,"reasoning":"block it"}`)
	if err == nil {
		t.Error("expected error for BLOCKED decision")
	}
	if !strings.Contains(err.Error(), "invalid decision") {
		t.Errorf("expected 'invalid decision' error, got: %v", err)
	}
}

func TestParseResponse_UnknownDecision(t *testing.T) {
	_, err := ParseResponse(`{"decision":"DELETE","confidence":0.9,"reasoning":"delete it"}`)
	if err == nil {
		t.Error("expected error for unknown decision")
	}
}

func TestParseResponse_ConfidenceClamping(t *testing.T) {
	resp, err := ParseResponse(`{"decision":"SAFE","confidence":1.5,"reasoning":"very safe"}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Confidence != 1.0 {
		t.Errorf("confidence = %f, want 1.0 (clamped from 1.5)", resp.Confidence)
	}

	resp2, err := ParseResponse(`{"decision":"SAFE","confidence":-0.5,"reasoning":"negative"}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp2.Confidence != 0.0 {
		t.Errorf("confidence = %f, want 0.0 (clamped from -0.5)", resp2.Confidence)
	}
}

func TestShortenToLastN(t *testing.T) {
	tests := []struct {
		path string
		n    int
		want string
	}{
		{"/Users/dev/workspaces/fuse", 2, "workspaces/fuse"},
		{"/Users/dev/workspaces/fuse", 1, "fuse"},
		{"/short", 5, "/short"},
		{"relative/path", 2, "relative/path"},
		{"", 2, "."},
	}
	for _, tt := range tests {
		got := ShortenToLastN(tt.path, tt.n)
		if got != tt.want {
			t.Errorf("ShortenToLastN(%q, %d) = %q, want %q", tt.path, tt.n, got, tt.want)
		}
	}
}

func TestParseResponse_LowercaseDecision(t *testing.T) {
	resp, err := ParseResponse(`{"decision":"safe","confidence":0.9,"reasoning":"ok"}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Decision != "SAFE" {
		t.Errorf("decision = %q, want SAFE (normalized)", resp.Decision)
	}
}
