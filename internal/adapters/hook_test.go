package adapters

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/php-workx/fuse/internal/approve"
	"github.com/php-workx/fuse/internal/config"
	"github.com/php-workx/fuse/internal/core"
	"github.com/php-workx/fuse/internal/db"
	"github.com/php-workx/fuse/internal/judge"
)

func TestRunHook_SafeCommand(t *testing.T) {
	enableHookForTest(t)

	input := `{"tool_name":"Bash","tool_input":{"command":"ls -la"},"session_id":"test","cwd":"/tmp"}`
	stdin := strings.NewReader(input)
	stderr := &bytes.Buffer{}

	exitCode := RunHook(stdin, stderr)

	if exitCode != 0 {
		t.Errorf("expected exit code 0 for safe command, got %d", exitCode)
	}
}

func TestRunHook_BlockedCommand(t *testing.T) {
	enableHookForTest(t)

	input := `{"tool_name":"Bash","tool_input":{"command":"rm -rf /"},"session_id":"test","cwd":"/tmp"}`
	stdin := strings.NewReader(input)
	stderr := &bytes.Buffer{}

	exitCode := RunHook(stdin, stderr)

	if exitCode != 2 {
		t.Errorf("expected exit code 2 for blocked command, got %d", exitCode)
	}

	stderrStr := stderr.String()
	if stderrStr == "" {
		t.Error("expected stderr to contain block reason, got empty string")
	}
	if !strings.Contains(stderrStr, "fuse:POLICY_BLOCK") {
		t.Errorf("expected stderr to contain 'fuse:POLICY_BLOCK', got: %s", stderrStr)
	}
}

func TestRunHook_NonBashTool(t *testing.T) {
	enableHookForTest(t)

	input := `{"tool_name":"Glob","tool_input":{"pattern":"*.go"},"session_id":"test","cwd":"/tmp"}`
	stdin := strings.NewReader(input)
	stderr := &bytes.Buffer{}

	exitCode := RunHook(stdin, stderr)

	if exitCode != 0 {
		t.Errorf("expected exit code 0 for non-Bash tool, got %d", exitCode)
	}
}

func TestRunHook_InvalidJSON(t *testing.T) {
	enableHookForTest(t)

	input := `{invalid json}`
	stdin := strings.NewReader(input)
	stderr := &bytes.Buffer{}

	exitCode := RunHook(stdin, stderr)

	if exitCode != 2 {
		t.Errorf("expected exit code 2 for invalid JSON (fail-closed), got %d", exitCode)
	}
}

func TestRunHook_EmptyCommand(t *testing.T) {
	enableHookForTest(t)

	input := `{"tool_name":"Bash","tool_input":{"command":""},"session_id":"test","cwd":"/tmp"}`
	stdin := strings.NewReader(input)
	stderr := &bytes.Buffer{}

	exitCode := RunHook(stdin, stderr)

	if exitCode != 2 {
		t.Errorf("expected exit code 2 for empty command, got %d", exitCode)
	}

	stderrStr := stderr.String()
	if !strings.Contains(stderrStr, "fuse:POLICY_BLOCK") {
		t.Errorf("expected stderr to contain 'fuse:POLICY_BLOCK', got: %s", stderrStr)
	}
}

func TestRunHook_MCP(t *testing.T) {
	enableHookForTest(t)

	// MCP tool with a safe-prefix action should be allowed.
	input := `{"tool_name":"mcp__server__list_items","tool_input":{"query":"test"},"session_id":"test","cwd":"/tmp"}`
	stdin := strings.NewReader(input)
	stderr := &bytes.Buffer{}

	exitCode := RunHook(stdin, stderr)

	if exitCode != 0 {
		t.Errorf("expected exit code 0 for MCP safe tool, got %d", exitCode)
	}
}

func TestRunHook_MCP_DestructiveAction(t *testing.T) {
	enableHookForTest(t)
	t.Setenv("FUSE_NON_INTERACTIVE", "1")
	// MCP tool with a destructive-prefix action should emit CAUTION and continue.
	input := `{"tool_name":"mcp__server__delete_items","tool_input":{"id":"123"},"session_id":"test","cwd":"/tmp"}`
	stdin := strings.NewReader(input)
	stderr := &bytes.Buffer{}

	exitCode := RunHook(stdin, stderr)

	if exitCode != 0 {
		t.Errorf("expected exit code 0 for MCP caution tool, got %d", exitCode)
	}

	stderrStr := stderr.String()
	if !strings.Contains(stderrStr, "CAUTION") {
		t.Errorf("expected CAUTION directive in stderr, got: %s", stderrStr)
	}
}

func TestRunHook_MissingToolName(t *testing.T) {
	enableHookForTest(t)

	input := `{"tool_input":{"command":"ls"},"session_id":"test","cwd":"/tmp"}`
	stdin := strings.NewReader(input)
	stderr := &bytes.Buffer{}

	exitCode := RunHook(stdin, stderr)

	if exitCode != 2 {
		t.Errorf("expected exit code 2 for missing tool_name, got %d", exitCode)
	}
}

func TestRunHook_MissingToolInput(t *testing.T) {
	enableHookForTest(t)

	input := `{"tool_name":"Bash","session_id":"test","cwd":"/tmp"}`
	stdin := strings.NewReader(input)
	stderr := &bytes.Buffer{}

	exitCode := RunHook(stdin, stderr)

	if exitCode != 2 {
		t.Errorf("expected exit code 2 for missing tool_input, got %d", exitCode)
	}
}

func TestRunHook_DryRunAllowsBlockedCommand(t *testing.T) {
	withFuseHome(t)
	enableDryRunForTest(t)

	input := `{"tool_name":"Bash","tool_input":{"command":"rm -rf /"},"session_id":"dry-run-test","cwd":"/tmp"}`
	stdin := strings.NewReader(input)
	stderr := &bytes.Buffer{}

	exitCode := RunHook(stdin, stderr)

	// In dry-run mode, even BLOCKED commands return exit 0 (allow).
	if exitCode != 0 {
		t.Errorf("expected exit code 0 in dry-run mode, got %d; stderr: %s", exitCode, stderr.String())
	}
}

func TestRunHook_DryRunAllowsApprovalCommand(t *testing.T) {
	withFuseHome(t)
	enableDryRunForTest(t)

	input := `{"tool_name":"mcp__server__delete_items","tool_input":{"id":"123"},"session_id":"dry-run-test","cwd":"/tmp"}`
	stdin := strings.NewReader(input)
	stderr := &bytes.Buffer{}

	exitCode := RunHook(stdin, stderr)

	if exitCode != 0 {
		t.Errorf("expected exit code 0 in dry-run mode for MCP approval, got %d; stderr: %s", exitCode, stderr.String())
	}
}

func TestRunHook_DisabledPassesThrough(t *testing.T) {
	withFuseHome(t)
	// Neither enabled nor dry-run — fully disabled.

	input := `{"tool_name":"Bash","tool_input":{"command":"rm -rf /"},"session_id":"disabled-test","cwd":"/tmp"}`
	stdin := strings.NewReader(input)
	stderr := &bytes.Buffer{}

	exitCode := RunHook(stdin, stderr)
	if exitCode != 0 {
		t.Errorf("expected exit code 0 when disabled, got %d", exitCode)
	}
	if stderr.Len() != 0 {
		t.Errorf("expected no stderr when disabled, got: %s", stderr.String())
	}
}

func TestExtractMCPAction(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"standard format", "mcp__server__list_items", "list_items"},
		{"nested server name", "mcp__my_server__delete_records", "delete_records"},
		{"no second separator", "mcp__simple", "simple"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractMCPAction(tt.input)
			if got != tt.expected {
				t.Errorf("extractMCPAction(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestRunHook_NativeFileReadSafe(t *testing.T) {
	enableHookForTest(t)
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	input := `{"tool_name":"Read","tool_input":{"file_path":"docs/readme.md"},"session_id":"test","cwd":"/tmp/project"}`
	stdin := strings.NewReader(input)
	stderr := &bytes.Buffer{}

	exitCode := RunHook(stdin, stderr)

	if exitCode != 0 {
		t.Fatalf("expected exit code 0 for safe file read, got %d with stderr %q", exitCode, stderr.String())
	}
}

func TestRunHook_NativeFileReadSecretRequiresApproval(t *testing.T) {
	enableHookForTest(t)
	t.Setenv("FUSE_NON_INTERACTIVE", "1")
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	input := `{"tool_name":"Read","tool_input":{"file_path":".env"},"session_id":"test","cwd":"/tmp/project"}`
	stdin := strings.NewReader(input)
	stderr := &bytes.Buffer{}

	exitCode := RunHook(stdin, stderr)

	if exitCode != 2 {
		t.Fatalf("expected exit code 2 for approval-required file read without interactive tty, got %d", exitCode)
	}
	stderrStr := stderr.String()
	// With TUI approval support, non-interactive hooks now return PENDING_APPROVAL
	// to encourage the agent to retry. Legacy messages are also accepted.
	if !strings.Contains(stderrStr, "PENDING_APPROVAL") &&
		!strings.Contains(stderrStr, "APPROVAL_NOT_AVAILABLE") &&
		!strings.Contains(stderrStr, "NON_INTERACTIVE_MODE") &&
		!strings.Contains(stderrStr, "USER_DENIED") &&
		!strings.Contains(stderrStr, "TIMEOUT_WAITING_FOR_USER") {
		t.Fatalf("expected approval-related directive in stderr, got %q", stderrStr)
	}
}

func TestRunHook_NativeFileProtectedPathsAreBlocked(t *testing.T) {
	enableHookForTest(t)
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	tests := []struct {
		name     string
		toolName string
		input    string
	}{
		{
			name:     "write fuse state blocked",
			toolName: "Write",
			input:    `{"file_path":"~/.fuse/state/fuse.db","content":"nope"}`,
		},
		{
			name:     "edit project claude settings blocked",
			toolName: "Edit",
			input:    `{"file_path":".claude/settings.json","old_string":"a","new_string":"b"}`,
		},
		{
			name:     "multiedit claude settings blocked",
			toolName: "MultiEdit",
			input:    `{"file_path":".claude/settings.json","edits":[{"old_string":"a","new_string":"b"}]}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := `{"tool_name":"` + tt.toolName + `","tool_input":` + tt.input + `,"session_id":"test","cwd":"/tmp/project"}`
			stdin := strings.NewReader(input)
			stderr := &bytes.Buffer{}

			exitCode := RunHook(stdin, stderr)

			if exitCode != 2 {
				t.Fatalf("expected exit code 2 for blocked native file tool, got %d", exitCode)
			}
			if !strings.Contains(stderr.String(), "fuse:POLICY_BLOCK") {
				t.Fatalf("expected policy block directive, got %q", stderr.String())
			}
		})
	}
}

func TestRunHook_NativeFileAbsoluteProjectConfigPathsAreBlocked(t *testing.T) {
	enableHookForTest(t)
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	projectDir := t.TempDir()
	subdir := filepath.Join(projectDir, "subdir")
	tests := []struct {
		name     string
		toolName string
		path     string
		cwd      string
	}{
		{
			name:     "subdir relative parent claude settings blocked",
			toolName: "Edit",
			path:     "../.claude/settings.json",
			cwd:      subdir,
		},
		{
			name:     "absolute project claude settings blocked",
			toolName: "Edit",
			path:     filepath.Join(projectDir, ".claude", "settings.json"),
			cwd:      subdir,
		},
		{
			name:     "subdir relative parent codex config blocked",
			toolName: "Write",
			path:     "../.codex/config.toml",
			cwd:      subdir,
		},
		{
			name:     "absolute project codex config blocked",
			toolName: "Write",
			path:     filepath.Join(projectDir, ".codex", "config.toml"),
			cwd:      subdir,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := `{"tool_name":"` + tt.toolName + `","tool_input":{"file_path":"` + filepath.ToSlash(tt.path) + `"},"session_id":"test","cwd":"` + filepath.ToSlash(tt.cwd) + `"}`
			stdin := strings.NewReader(input)
			stderr := &bytes.Buffer{}

			exitCode := RunHook(stdin, stderr)

			if exitCode != 2 {
				t.Fatalf("expected exit code 2 for blocked absolute project config path, got %d", exitCode)
			}
			if !strings.Contains(stderr.String(), "fuse:POLICY_BLOCK") {
				t.Fatalf("expected policy block directive, got %q", stderr.String())
			}
		})
	}
}

func TestRunHook_NativeFileIgnoresNestedMetadataPaths(t *testing.T) {
	enableHookForTest(t)
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	input := `{"tool_name":"Read","tool_input":{"file_path":"docs/readme.md","metadata":{"path":".env","file_path":"secrets/prod.env"}},"session_id":"test","cwd":"/tmp/project"}`
	stdin := strings.NewReader(input)
	stderr := &bytes.Buffer{}

	exitCode := RunHook(stdin, stderr)

	if exitCode != 0 {
		t.Fatalf("expected exit code 0 when only nested metadata mentions sensitive paths, got %d with stderr %q", exitCode, stderr.String())
	}
}

func TestRunHook_NativeFileParentTraversalSecretsRequiresApproval(t *testing.T) {
	enableHookForTest(t)
	t.Setenv("FUSE_NON_INTERACTIVE", "1")
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	// From a subdirectory, ../secrets/prod.json should still require approval.
	projectDir := filepath.Join(tmpHome, "project")
	subdir := filepath.Join(projectDir, "src")
	secretsDir := filepath.Join(projectDir, "secrets")
	if err := os.MkdirAll(secretsDir, 0o755); err != nil {
		t.Fatalf("mkdir secrets: %v", err)
	}
	if err := os.MkdirAll(subdir, 0o755); err != nil {
		t.Fatalf("mkdir subdir: %v", err)
	}

	input := `{"tool_name":"Read","tool_input":{"file_path":"../secrets/prod.json"},"session_id":"test","cwd":"` + filepath.ToSlash(subdir) + `"}`
	stdin := strings.NewReader(input)
	stderr := &bytes.Buffer{}

	exitCode := RunHook(stdin, stderr)

	if exitCode != 2 {
		t.Fatalf("expected exit code 2 for ../secrets/prod.json traversal, got %d; stderr: %s", exitCode, stderr.String())
	}
}

func TestRunHook_NativeFileBlockedLogsProfileAndStructuralDecision(t *testing.T) {
	enableHookForTest(t)
	writeProfileConfigForBehaviorTest(t, profileConfigContents(config.ProfileBalanced, "22s"))

	projectDir := filepath.Join(t.TempDir(), "project")
	subdir := filepath.Join(projectDir, "src")
	protectedPath := filepath.Join(projectDir, ".claude", "settings.json")
	if err := os.MkdirAll(subdir, 0o755); err != nil {
		t.Fatalf("mkdir subdir: %v", err)
	}

	input := `{"tool_name":"Edit","tool_input":{"file_path":"` + filepath.ToSlash(protectedPath) + `"},"session_id":"native-file-log","cwd":"` + filepath.ToSlash(subdir) + `"}`
	stderr := &bytes.Buffer{}
	exitCode := RunHook(strings.NewReader(input), stderr)
	if exitCode != 2 {
		t.Fatalf("expected exit code 2 for blocked native file access, got %d; stderr: %s", exitCode, stderr.String())
	}

	database, err := db.OpenDB(config.DBPath())
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	defer database.Close()

	events, err := database.ListEvents(&db.EventFilter{Limit: 10, Source: "hook"})
	if err != nil {
		t.Fatalf("ListEvents: %v", err)
	}
	if len(events) == 0 {
		t.Fatal("expected at least one hook event")
	}

	event := events[0]
	if event.Profile != config.ProfileBalanced {
		t.Fatalf("Profile = %q, want %q", event.Profile, config.ProfileBalanced)
	}
	if event.StructuralDecision != string(core.DecisionBlocked) {
		t.Fatalf("StructuralDecision = %q, want %q", event.StructuralDecision, core.DecisionBlocked)
	}
	if event.Decision != string(core.DecisionBlocked) {
		t.Fatalf("Decision = %q, want %q", event.Decision, core.DecisionBlocked)
	}
}

func TestRunHook_ApprovalLogsProfileAndStructuralDecision(t *testing.T) {
	enableHookForTest(t)
	writeProfileConfigForBehaviorTest(t, profileConfigContents(config.ProfileStrict, "53s"))

	projectDir := t.TempDir()
	req := core.ShellRequest{
		RawCommand: "HOME=/tmp /bin/pwd",
		Cwd:        projectDir,
		Source:     "hook",
		SessionID:  "approval-log",
	}
	result, err := core.Classify(req, loadPolicyEvaluator())
	if err != nil {
		t.Fatalf("Classify: %v", err)
	}

	database, err := db.OpenDB(config.DBPath())
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	defer database.Close()

	secret, err := db.EnsureSecret(config.SecretPath())
	if err != nil {
		t.Fatalf("EnsureSecret: %v", err)
	}
	manager, err := approve.NewManager(database, secret)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	if err := manager.CreateApproval(result.DecisionKey, string(core.DecisionApproval), "once", "approval-log"); err != nil {
		t.Fatalf("CreateApproval: %v", err)
	}

	input := `{"tool_name":"Bash","tool_input":{"command":"HOME=/tmp /bin/pwd"},"session_id":"approval-log","cwd":"` + filepath.ToSlash(projectDir) + `"}`
	stderr := &bytes.Buffer{}
	exitCode := RunHook(strings.NewReader(input), stderr)
	if exitCode != 0 {
		t.Fatalf("expected approved command to exit 0, got %d; stderr: %s", exitCode, stderr.String())
	}

	events, err := database.ListEvents(&db.EventFilter{Limit: 10, Source: "hook"})
	if err != nil {
		t.Fatalf("ListEvents: %v", err)
	}
	if len(events) == 0 {
		t.Fatal("expected at least one hook event")
	}

	var event db.EventRecord
	found := false
	for _, candidate := range events {
		if strings.Contains(candidate.Command, "HOME=/tmp /bin/pwd") {
			event = candidate
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected approval event for command, got %#v", events)
	}
	if event.Profile != config.ProfileStrict {
		t.Fatalf("Profile = %q, want %q", event.Profile, config.ProfileStrict)
	}
	if event.StructuralDecision != string(core.DecisionApproval) {
		t.Fatalf("StructuralDecision = %q, want %q", event.StructuralDecision, core.DecisionApproval)
	}
	if event.Decision != string(core.DecisionApproval) {
		t.Fatalf("Decision = %q, want %q", event.Decision, core.DecisionApproval)
	}
}

// --- applyVerdict tests ---

func TestApplyVerdict_Nil(t *testing.T) {
	event := &db.EventRecord{Decision: "CAUTION"}
	applyVerdict(event, nil)
	// Should be a no-op.
	if event.JudgeDecision != "" {
		t.Errorf("nil verdict should not set judge fields, got %q", event.JudgeDecision)
	}
}

func TestApplyVerdict_NotApplied(t *testing.T) {
	event := &db.EventRecord{Decision: "CAUTION"}
	v := &judge.Verdict{
		OriginalDecision: core.DecisionCaution,
		JudgeDecision:    core.DecisionSafe,
		Confidence:       0.8,
		Reasoning:        "safe command",
		Applied:          false,
		ProviderName:     "claude",
		LatencyMs:        200,
	}
	applyVerdict(event, v)
	if event.JudgeDecision != "SAFE" {
		t.Errorf("JudgeDecision = %q, want SAFE", event.JudgeDecision)
	}
	// Decision should remain CAUTION (not applied).
	if event.Decision != "CAUTION" {
		t.Errorf("Decision = %q, want CAUTION (unchanged)", event.Decision)
	}
}

func TestApplyVerdict_Applied(t *testing.T) {
	event := &db.EventRecord{Decision: "APPROVAL"}
	v := &judge.Verdict{
		OriginalDecision: core.DecisionApproval,
		JudgeDecision:    core.DecisionSafe,
		Confidence:       0.97,
		Reasoning:        "actually safe",
		Applied:          true,
		ProviderName:     "codex",
		LatencyMs:        350,
	}
	applyVerdict(event, v)
	// When applied, Decision should be restored to OriginalDecision.
	if event.Decision != "APPROVAL" {
		t.Errorf("Decision = %q, want APPROVAL (original restored)", event.Decision)
	}
	if event.JudgeDecision != "SAFE" {
		t.Errorf("JudgeDecision = %q, want SAFE", event.JudgeDecision)
	}
	if !event.JudgeApplied {
		t.Error("JudgeApplied should be true")
	}
	if event.JudgeProvider != "codex" {
		t.Errorf("JudgeProvider = %q, want codex", event.JudgeProvider)
	}
}

func TestApplyVerdict_Error(t *testing.T) {
	event := &db.EventRecord{Decision: "CAUTION"}
	v := &judge.Verdict{
		OriginalDecision: core.DecisionCaution,
		Error:            "timeout",
		ProviderName:     "claude",
		LatencyMs:        10000,
	}
	applyVerdict(event, v)
	if event.JudgeError != "timeout" {
		t.Errorf("JudgeError = %q, want timeout", event.JudgeError)
	}
	if event.Decision != "CAUTION" {
		t.Errorf("Decision should remain CAUTION on error, got %q", event.Decision)
	}
}

// --- buildJudgeContext tests ---

func TestBuildJudgeContext_Basic(t *testing.T) {
	result := &core.ClassifyResult{
		Decision: core.DecisionCaution,
		Reason:   "test reason",
		RuleID:   "test-rule",
	}
	ctx := buildJudgeContext("echo hello", "/Users/dev/project", "Bash", result)
	if ctx.Command != "echo hello" {
		t.Errorf("Command = %q", ctx.Command)
	}
	if ctx.Cwd != "/Users/dev/project" {
		t.Errorf("Cwd = %q", ctx.Cwd)
	}
	if ctx.WorkspaceRoot != "dev/project" {
		t.Errorf("WorkspaceRoot = %q, want dev/project", ctx.WorkspaceRoot)
	}
	if ctx.CurrentDecision != "CAUTION" {
		t.Errorf("CurrentDecision = %q", ctx.CurrentDecision)
	}
	if ctx.ToolName != "Bash" {
		t.Errorf("ToolName = %q", ctx.ToolName)
	}
}

func TestBuildJudgeContext_EmptyCwd(t *testing.T) {
	result := &core.ClassifyResult{Decision: core.DecisionCaution}
	ctx := buildJudgeContext("python script.py", "", "Bash", result)
	// Empty cwd should skip script detection entirely.
	if ctx.ScriptContents != "" {
		t.Error("expected empty ScriptContents with empty cwd")
	}
}

func TestBuildJudgeContext_PathTraversalBlocked(t *testing.T) {
	dir := t.TempDir()
	// Create a file outside cwd that a traversal path would reach.
	outsideFile := filepath.Join(dir, "secret.sh")
	if err := os.WriteFile(outsideFile, []byte("#!/bin/bash\nrm -rf /"), 0o644); err != nil {
		t.Fatal(err)
	}
	cwd := filepath.Join(dir, "project")
	if err := os.MkdirAll(cwd, 0o755); err != nil {
		t.Fatal(err)
	}

	result := &core.ClassifyResult{Decision: core.DecisionApproval}
	ctx := buildJudgeContext("bash ../secret.sh", cwd, "Bash", result)
	// ../secret.sh resolves outside cwd — should NOT be read.
	if ctx.ScriptContents != "" {
		t.Error("path traversal should be blocked — ScriptContents should be empty")
	}
}

func TestBuildJudgeContext_ScriptWithinCwd(t *testing.T) {
	dir := t.TempDir()
	scriptContent := "#!/bin/bash\necho safe\n"
	if err := os.WriteFile(filepath.Join(dir, "deploy.sh"), []byte(scriptContent), 0o644); err != nil {
		t.Fatal(err)
	}

	result := &core.ClassifyResult{Decision: core.DecisionApproval}
	ctx := buildJudgeContext("bash deploy.sh", dir, "Bash", result)
	if ctx.ScriptContents != scriptContent {
		t.Errorf("ScriptContents = %q, want %q", ctx.ScriptContents, scriptContent)
	}
	if ctx.ScriptPath != "deploy.sh" {
		t.Errorf("ScriptPath = %q, want deploy.sh", ctx.ScriptPath)
	}
}

func enableHookForTest(t *testing.T) {
	t.Helper()

	// Use a very short hook timeout for tests so approval-waiting tests complete quickly.
	old := hookTimeout
	hookTimeout = 500 * time.Millisecond
	t.Cleanup(func() { hookTimeout = old })

	fuseHome := t.TempDir()
	t.Setenv("FUSE_HOME", fuseHome)
	enabledPath := filepath.Join(fuseHome, "state", "enabled")
	if err := os.MkdirAll(filepath.Dir(enabledPath), 0o755); err != nil {
		t.Fatalf("mkdir state dir: %v", err)
	}
	if err := os.WriteFile(enabledPath, []byte("enabled\n"), 0o644); err != nil {
		t.Fatalf("write enabled marker: %v", err)
	}
}
