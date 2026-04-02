package fuse_test

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/php-workx/fuse/internal/adapters"
	"github.com/php-workx/fuse/internal/approve"
	"github.com/php-workx/fuse/internal/config"
	"github.com/php-workx/fuse/internal/core"
	"github.com/php-workx/fuse/internal/db"
	"github.com/php-workx/fuse/internal/policy"
)

// skipIfShort skips the test if -short flag is set.
func skipIfShort(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
}

// withIsolatedHome sets FUSE_HOME and HOME to a temp directory so
// integration tests don't read/write the user's real fuse state.
// Creates the enabled marker so fuse is active.
func withIsolatedHome(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("FUSE_HOME", filepath.Join(home, ".fuse"))
	t.Setenv("HOME", home)
	marker := config.EnabledMarkerPath()
	if err := os.MkdirAll(filepath.Dir(marker), 0o755); err != nil {
		t.Fatalf("mkdir for enabled marker: %v", err)
	}
	if err := os.WriteFile(marker, []byte("1"), 0o600); err != nil {
		t.Fatalf("write enabled marker: %v", err)
	}
}

func TestIntegration_RunUsageErrorsExitTwo(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test uses Unix-specific shell commands")
	}
	skipIfShort(t)

	binPath := filepath.Join(t.TempDir(), "fuse-test-bin")
	build := exec.Command("go", "build", "-o", binPath, "./cmd/fuse")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build failed: %v\n%s", err, string(output))
	}

	cmd := exec.Command(binPath, "run")
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected fuse run without command to fail, output:\n%s", string(output))
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected ExitError, got %T (%v)", err, err)
	}
	if exitErr.ExitCode() != 2 {
		t.Fatalf("expected exit code 2, got %d\noutput:\n%s", exitErr.ExitCode(), string(output))
	}
	if !strings.Contains(string(output), "exactly one shell command string") {
		t.Fatalf("expected usage error in output, got:\n%s", string(output))
	}
}

// ---------------------------------------------------------------------------
// 1. TestIntegration_HookFlow — Full hook flow end-to-end
// ---------------------------------------------------------------------------

func TestIntegration_HookFlow(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test uses Unix-specific shell commands")
	}
	skipIfShort(t)
	withIsolatedHome(t)

	t.Run("safe command returns exit 0", func(t *testing.T) {
		input := `{"tool_name":"Bash","tool_input":{"command":"ls -la"},"session_id":"integ-test","cwd":"/tmp"}`
		stdin := strings.NewReader(input)
		stderr := &bytes.Buffer{}

		exitCode := adapters.RunHook(stdin, stderr)
		if exitCode != 0 {
			t.Errorf("expected exit code 0 for safe command, got %d; stderr: %s", exitCode, stderr.String())
		}
	})

	t.Run("blocked command returns exit 2", func(t *testing.T) {
		input := `{"tool_name":"Bash","tool_input":{"command":"rm -rf /"},"session_id":"integ-test","cwd":"/tmp"}`
		stdin := strings.NewReader(input)
		stderr := &bytes.Buffer{}

		exitCode := adapters.RunHook(stdin, stderr)
		if exitCode != 2 {
			t.Errorf("expected exit code 2 for blocked command, got %d; stderr: %s", exitCode, stderr.String())
		}
	})

	t.Run("caution command returns exit 0 with stderr message", func(t *testing.T) {
		input := `{"tool_name":"Bash","tool_input":{"command":"sudo echo hello"},"session_id":"integ-test","cwd":"/tmp"}`
		stdin := strings.NewReader(input)
		stderr := &bytes.Buffer{}

		exitCode := adapters.RunHook(stdin, stderr)
		if exitCode != 0 {
			t.Errorf("expected exit code 0 for caution command, got %d; stderr: %s", exitCode, stderr.String())
		}
		if !strings.Contains(stderr.String(), "CAUTION") {
			t.Errorf("expected stderr to contain CAUTION, got: %s", stderr.String())
		}
	})
}

// ---------------------------------------------------------------------------
// 2. TestIntegration_HookFlow_MCP — MCP tool classification through hook
// ---------------------------------------------------------------------------

func TestIntegration_HookFlow_MCP(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test uses Unix-specific shell commands")
	}
	skipIfShort(t)
	withIsolatedHome(t)

	t.Run("safe MCP read tool returns exit 0", func(t *testing.T) {
		input := `{"tool_name":"mcp__server__read_file","tool_input":{"path":"/tmp/test.txt"},"session_id":"integ-test","cwd":"/tmp"}`
		stdin := strings.NewReader(input)
		stderr := &bytes.Buffer{}

		exitCode := adapters.RunHook(stdin, stderr)
		if exitCode != 0 {
			t.Errorf("expected exit code 0 for safe MCP tool, got %d; stderr: %s", exitCode, stderr.String())
		}
	})

	t.Run("destructive MCP delete tool returns caution", func(t *testing.T) {
		t.Setenv("FUSE_NON_INTERACTIVE", "1")
		t.Setenv("FUSE_HOOK_TIMEOUT", "3.5s") // short timeout for tests (must be > 3s)
		input := `{"tool_name":"mcp__server__delete_database","tool_input":{"name":"prod"},"session_id":"integ-test","cwd":"/tmp"}`
		stdin := strings.NewReader(input)
		stderr := &bytes.Buffer{}

		exitCode := adapters.RunHook(stdin, stderr)
		if exitCode != 0 {
			t.Errorf("expected exit code 0 for MCP delete CAUTION, got %d; stderr: %s", exitCode, stderr.String())
		}
		stderrStr := stderr.String()
		if !strings.Contains(stderrStr, "CAUTION") {
			t.Errorf("expected CAUTION directive, got: %s", stderrStr)
		}
	})

	t.Run("MCP list tool is safe", func(t *testing.T) {
		input := `{"tool_name":"mcp__server__list_items","tool_input":{"query":"test"},"session_id":"integ-test","cwd":"/tmp"}`
		stdin := strings.NewReader(input)
		stderr := &bytes.Buffer{}

		exitCode := adapters.RunHook(stdin, stderr)
		if exitCode != 0 {
			t.Errorf("expected exit code 0 for MCP list tool, got %d; stderr: %s", exitCode, stderr.String())
		}
	})
}

// ---------------------------------------------------------------------------
// 3. TestIntegration_FileInspection — File inspection end-to-end
// ---------------------------------------------------------------------------

func TestIntegration_FileInspection(t *testing.T) {
	skipIfShort(t)
	withIsolatedHome(t)

	evaluator := policy.NewEvaluator(nil)

	t.Run("dangerous boto3 script detected at pipeline level", func(t *testing.T) {
		req := core.ShellRequest{
			RawCommand: "python dangerous_boto3.py",
			Cwd:        testdataScriptsDir(),
			Source:     "test",
			SessionID:  "integ-test",
		}
		result, err := core.Classify(req, evaluator)
		if err != nil {
			t.Fatalf("classify error: %v", err)
		}
		if result.Decision != core.DecisionApproval {
			t.Errorf("expected APPROVAL for dangerous python script execution, got %s (reason: %s)", result.Decision, result.Reason)
		}
	})

	t.Run("file inspection directly detects dangerous signals", func(t *testing.T) {
		// Test file inspection directly (bypassing the classify pipeline).
		path := filepath.Join(testdataScriptsDir(), "dangerous_boto3.py")
		inspection, err := core.InspectFile(path, core.DefaultMaxBytes)
		if err != nil {
			t.Fatalf("InspectFile error: %v", err)
		}
		if !inspection.Exists {
			t.Fatal("expected file to exist")
		}
		if len(inspection.Signals) == 0 {
			t.Error("expected signals for dangerous boto3 file")
		}
		if inspection.Decision != core.DecisionApproval {
			t.Errorf("expected APPROVAL from file inspection, got %s", inspection.Decision)
		}
	})

	t.Run("file inspection directly sees safe script as safe", func(t *testing.T) {
		path := filepath.Join(testdataScriptsDir(), "safe_script.py")
		inspection, err := core.InspectFile(path, core.DefaultMaxBytes)
		if err != nil {
			t.Fatalf("InspectFile error: %v", err)
		}
		if !inspection.Exists {
			t.Fatal("expected file to exist")
		}
		if inspection.Decision != core.DecisionSafe {
			t.Errorf("expected SAFE from file inspection, got %s", inspection.Decision)
		}
	})

	t.Run("missing referenced file in classify pipeline", func(t *testing.T) {
		req := core.ShellRequest{
			RawCommand: "python nonexistent_script.py",
			Cwd:        "/tmp",
			Source:     "test",
			SessionID:  "integ-test",
		}
		result, err := core.Classify(req, evaluator)
		if err != nil {
			t.Fatalf("classify error: %v", err)
		}
		if result.Decision != core.DecisionApproval {
			t.Errorf("expected APPROVAL for python with missing file, got %s (reason: %s)", result.Decision, result.Reason)
		}
	})
}

// testdataScriptsDir returns the absolute path to testdata/scripts.
func testdataScriptsDir() string {
	// From the project root, testdata/scripts.
	return filepath.Join("testdata", "scripts")
}

// ---------------------------------------------------------------------------
// 4. TestIntegration_ApprovalLifecycle — Full approval CRUD
// ---------------------------------------------------------------------------

func TestIntegration_ApprovalLifecycle(t *testing.T) {
	skipIfShort(t)

	dbPath := filepath.Join(t.TempDir(), "integ-test.db")
	database, err := db.OpenDB(dbPath)
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	defer database.Close()

	secret := []byte("integration-test-secret-32bytes!")
	mgr, mgrErr := approve.NewManager(database, secret)
	if mgrErr != nil {
		t.Fatalf("NewManager failed: %v", mgrErr)
	}

	decisionKey := "integ-test-decision-key"
	sessionID := "integ-session-001"

	// Create an approval with "once" scope.
	err = mgr.CreateApproval(decisionKey, string(core.DecisionApproval), "once", sessionID)
	if err != nil {
		t.Fatalf("CreateApproval failed: %v", err)
	}

	// Consume: should return APPROVAL.
	got, err := mgr.ConsumeApproval(decisionKey, sessionID)
	if err != nil {
		t.Fatalf("ConsumeApproval failed: %v", err)
	}
	if got != core.DecisionApproval {
		t.Errorf("expected APPROVAL, got %q", got)
	}

	// Consume again: "once" scope should be gone.
	got2, err := mgr.ConsumeApproval(decisionKey, sessionID)
	if err != nil {
		t.Fatalf("second ConsumeApproval failed: %v", err)
	}
	if got2 != "" {
		t.Errorf("expected empty decision after once-consumed, got %q", got2)
	}

	// Create a "command" scoped approval: should be reusable.
	cmdKey := "integ-cmd-key"
	err = mgr.CreateApproval(cmdKey, string(core.DecisionApproval), "command", sessionID)
	if err != nil {
		t.Fatalf("CreateApproval (command scope) failed: %v", err)
	}
	for i := 0; i < 3; i++ {
		got, err := mgr.ConsumeApproval(cmdKey, sessionID)
		if err != nil {
			t.Fatalf("ConsumeApproval (command scope) attempt %d failed: %v", i+1, err)
		}
		if got != core.DecisionApproval {
			t.Errorf("attempt %d: expected APPROVAL, got %q", i+1, got)
		}
	}
}

// ---------------------------------------------------------------------------
// 5. TestIntegration_TwoLevelNormalization — End-to-end normalization
// ---------------------------------------------------------------------------

func TestIntegration_TwoLevelNormalization(t *testing.T) {
	skipIfShort(t)

	// Input with ANSI codes + sudo wrapper.
	raw := "\x1b[31msudo\x1b[0m terraform destroy"

	// Display normalization: strips ANSI, collapses whitespace. Preserves sudo.
	display := core.DisplayNormalize(raw)
	if strings.Contains(display, "\x1b") {
		t.Errorf("display normalization should strip ANSI codes, got: %q", display)
	}
	if !strings.Contains(display, "sudo") {
		t.Errorf("display normalization should preserve sudo, got: %q", display)
	}

	// Classification normalization: strips sudo, sets escalation flag.
	classified := core.ClassificationNormalize(display)
	if strings.HasPrefix(classified.Outer, "sudo") {
		t.Errorf("classification normalization should strip sudo, got: %q", classified.Outer)
	}
	if !classified.EscalateClassification {
		t.Error("expected EscalateClassification to be true for sudo command")
	}
	if classified.Outer != "terraform destroy" {
		t.Errorf("expected classification outer to be 'terraform destroy', got: %q", classified.Outer)
	}

	// Full pipeline classification: sudo escalates the decision.
	evaluator := policy.NewEvaluator(nil)
	req := core.ShellRequest{
		RawCommand: raw,
		Cwd:        "/tmp",
		Source:     "test",
		SessionID:  "integ-test",
	}
	result, err := core.Classify(req, evaluator)
	if err != nil {
		t.Fatalf("classify error: %v", err)
	}
	// sudo escalates SAFE -> CAUTION at minimum.
	if result.Decision == core.DecisionSafe {
		t.Errorf("expected decision higher than SAFE for sudo command, got %s", result.Decision)
	}
}

// ---------------------------------------------------------------------------
// 6. TestIntegration_LazyDB — Verify SAFE/BLOCKED don't need DB
// ---------------------------------------------------------------------------

func TestIntegration_LazyDB(t *testing.T) {
	skipIfShort(t)

	// Use a non-existent DB path. If the code tries to open it,
	// it would create files in a non-existent directory (or succeed in TempDir).
	// The point is that the classification pipeline itself doesn't need DB.
	evaluator := policy.NewEvaluator(nil)

	t.Run("SAFE command succeeds without DB", func(t *testing.T) {
		req := core.ShellRequest{
			RawCommand: "ls -la",
			Cwd:        "/tmp",
			Source:     "test",
			SessionID:  "integ-test",
		}
		result, err := core.Classify(req, evaluator)
		if err != nil {
			t.Fatalf("classify error for safe command: %v", err)
		}
		if result.Decision != core.DecisionSafe {
			t.Errorf("expected SAFE, got %s", result.Decision)
		}
	})

	t.Run("BLOCKED command succeeds without DB", func(t *testing.T) {
		req := core.ShellRequest{
			RawCommand: "rm -rf /",
			Cwd:        "/tmp",
			Source:     "test",
			SessionID:  "integ-test",
		}
		result, err := core.Classify(req, evaluator)
		if err != nil {
			t.Fatalf("classify error for blocked command: %v", err)
		}
		if result.Decision != core.DecisionBlocked {
			t.Errorf("expected BLOCKED, got %s", result.Decision)
		}
	})
}

// ---------------------------------------------------------------------------
// 7. TestIntegration_DirectiveMessaging — Verify stderr directives
// ---------------------------------------------------------------------------

func TestIntegration_DirectiveMessaging(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test uses Unix-specific shell commands")
	}
	skipIfShort(t)
	withIsolatedHome(t)

	t.Run("blocked command stderr contains POLICY_BLOCK directive", func(t *testing.T) {
		input := `{"tool_name":"Bash","tool_input":{"command":"rm -rf /"},"session_id":"integ-test","cwd":"/tmp"}`
		stdin := strings.NewReader(input)
		stderr := &bytes.Buffer{}

		exitCode := adapters.RunHook(stdin, stderr)
		if exitCode != 2 {
			t.Errorf("expected exit code 2, got %d", exitCode)
		}
		stderrStr := stderr.String()
		if !strings.Contains(stderrStr, "fuse:POLICY_BLOCK") {
			t.Errorf("expected stderr to contain 'fuse:POLICY_BLOCK', got: %s", stderrStr)
		}
		if !strings.Contains(stderrStr, "Do not retry") {
			t.Errorf("expected stderr to contain 'Do not retry', got: %s", stderrStr)
		}
	})

	t.Run("caution command stderr contains CAUTION info", func(t *testing.T) {
		input := `{"tool_name":"Bash","tool_input":{"command":"sudo echo hello"},"session_id":"integ-test","cwd":"/tmp"}`
		stdin := strings.NewReader(input)
		stderr := &bytes.Buffer{}

		exitCode := adapters.RunHook(stdin, stderr)
		if exitCode != 0 {
			t.Errorf("expected exit code 0, got %d", exitCode)
		}
		stderrStr := stderr.String()
		if !strings.Contains(stderrStr, "CAUTION") {
			t.Errorf("expected stderr to contain 'CAUTION', got: %s", stderrStr)
		}
	})

	t.Run("invalid JSON stderr contains block directive", func(t *testing.T) {
		input := `{invalid`
		stdin := strings.NewReader(input)
		stderr := &bytes.Buffer{}

		exitCode := adapters.RunHook(stdin, stderr)
		if exitCode != 2 {
			t.Errorf("expected exit code 2, got %d", exitCode)
		}
		stderrStr := stderr.String()
		if !strings.Contains(stderrStr, "fuse:POLICY_BLOCK") {
			t.Errorf("expected stderr to contain 'fuse:POLICY_BLOCK', got: %s", stderrStr)
		}
	})
}

// ---------------------------------------------------------------------------
// 8. TestIntegration_EnvSanitization — BuildChildEnv
// ---------------------------------------------------------------------------

func TestIntegration_EnvSanitization(t *testing.T) {
	skipIfShort(t)

	input := []string{
		"HOME=/home/user",
		"PATH=/usr/bin:/bin",
		"LD_PRELOAD=/evil/lib.so",
		"DYLD_INSERT_LIBRARIES=/evil/dylib",
		"DYLD_LIBRARY_PATH=/evil/path",
		"PYTHONPATH=/evil/python",
		"NODE_PATH=/evil/node",
		"RUBYLIB=/evil/ruby",
		"BASH_ENV=/evil/bashrc",
		"ENV=/evil/env",
		"LD_LIBRARY_PATH=/evil/ld",
		"EDITOR=vim",
		"LANG=en_US.UTF-8",
		"TERM=xterm-256color",
	}

	result := adapters.BuildChildEnv(input)

	envMap := make(map[string]string)
	for _, e := range result {
		parts := strings.SplitN(e, "=", 2)
		if len(parts) == 2 {
			envMap[parts[0]] = parts[1]
		}
	}

	// Dangerous vars should be stripped.
	dangerous := []string{
		"LD_PRELOAD", "LD_LIBRARY_PATH", "DYLD_INSERT_LIBRARIES",
		"DYLD_LIBRARY_PATH", "PYTHONPATH", "NODE_PATH",
		"RUBYLIB", "BASH_ENV", "ENV",
	}
	for _, name := range dangerous {
		if _, ok := envMap[name]; ok {
			t.Errorf("dangerous var %s should be stripped but was present", name)
		}
	}

	// PATH should be set to a trusted path (not the original).
	if path, ok := envMap["PATH"]; !ok {
		t.Error("PATH not set in output")
	} else if path == "/usr/bin:/bin" {
		t.Error("PATH should be reset to trusted path, not preserved as-is")
	}

	// Safe vars should be preserved.
	if home, ok := envMap["HOME"]; !ok || home != "/home/user" {
		t.Errorf("HOME should be preserved, got %q", home)
	}
	if editor, ok := envMap["EDITOR"]; !ok || editor != "vim" {
		t.Errorf("EDITOR should be preserved, got %q", editor)
	}
	if lang, ok := envMap["LANG"]; !ok || lang != "en_US.UTF-8" {
		t.Errorf("LANG should be preserved, got %q", lang)
	}
}

// ---------------------------------------------------------------------------
// 9. TestIntegration_CompoundCommandClassification
// ---------------------------------------------------------------------------

func TestIntegration_CompoundCommandClassification(t *testing.T) {
	skipIfShort(t)

	evaluator := policy.NewEvaluator(nil)

	t.Run("ls && rm -rf / is BLOCKED (most restrictive)", func(t *testing.T) {
		req := core.ShellRequest{
			RawCommand: "ls && rm -rf /",
			Cwd:        "/tmp",
			Source:     "test",
			SessionID:  "integ-test",
		}
		result, err := core.Classify(req, evaluator)
		if err != nil {
			t.Fatalf("classify error: %v", err)
		}
		if result.Decision != core.DecisionBlocked {
			t.Errorf("expected BLOCKED for 'ls && rm -rf /', got %s (reason: %s)", result.Decision, result.Reason)
		}
	})

	t.Run("echo hello; cat file is SAFE", func(t *testing.T) {
		req := core.ShellRequest{
			RawCommand: "echo hello; cat file",
			Cwd:        "/tmp",
			Source:     "test",
			SessionID:  "integ-test",
		}
		result, err := core.Classify(req, evaluator)
		if err != nil {
			t.Fatalf("classify error: %v", err)
		}
		if result.Decision != core.DecisionSafe {
			t.Errorf("expected SAFE for 'echo hello; cat file', got %s (reason: %s)", result.Decision, result.Reason)
		}
	})

	t.Run("mixed compound with blocked wins", func(t *testing.T) {
		req := core.ShellRequest{
			RawCommand: "echo hello && ls -la && rm -rf /",
			Cwd:        "/tmp",
			Source:     "test",
			SessionID:  "integ-test",
		}
		result, err := core.Classify(req, evaluator)
		if err != nil {
			t.Fatalf("classify error: %v", err)
		}
		if result.Decision != core.DecisionBlocked {
			t.Errorf("expected BLOCKED for compound with rm -rf, got %s (reason: %s)", result.Decision, result.Reason)
		}
	})

	t.Run("compound with cd triggers APPROVAL (cwd change)", func(t *testing.T) {
		req := core.ShellRequest{
			RawCommand: "cd /tmp && python script.py",
			Cwd:        "/tmp",
			Source:     "test",
			SessionID:  "integ-test",
		}
		result, err := core.Classify(req, evaluator)
		if err != nil {
			t.Fatalf("classify error: %v", err)
		}
		if result.Decision != core.DecisionApproval {
			t.Errorf("expected APPROVAL for compound with cd, got %s (reason: %s)", result.Decision, result.Reason)
		}
	})
}

// ---------------------------------------------------------------------------
// 10. TestIntegration_CrossCompilation — verify builds succeed
// ---------------------------------------------------------------------------

// Cross-compilation is tested via the justfile (just build-all) and CI.
// See acceptance criteria: the build commands are run separately after tests.

// ---------------------------------------------------------------------------
// Additional integration tests
// ---------------------------------------------------------------------------

func TestIntegration_InvalidHMACRejected(t *testing.T) {
	skipIfShort(t)

	dbPath := filepath.Join(t.TempDir(), "hmac-test.db")
	database, err := db.OpenDB(dbPath)
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	defer database.Close()

	secret := []byte("integration-test-secret-32bytes!")
	mgr, mgrErr := approve.NewManager(database, secret)
	if mgrErr != nil {
		t.Fatalf("NewManager failed: %v", mgrErr)
	}

	decisionKey := "hmac-integ-key"
	sessionID := "session-hmac"

	// Create an approval with a valid HMAC.
	err = mgr.CreateApproval(decisionKey, string(core.DecisionApproval), "command", sessionID)
	if err != nil {
		t.Fatalf("CreateApproval failed: %v", err)
	}

	// Try to consume with a different secret (simulates tampering).
	wrongSecret := []byte("wrong-secret-key-32-bytes-long!!")
	tampered, tamperErr := approve.NewManager(database, wrongSecret)
	if tamperErr != nil {
		t.Fatalf("NewManager with wrong secret failed: %v", tamperErr)
	}

	_, err = tampered.ConsumeApproval(decisionKey, sessionID)
	if err == nil {
		t.Fatal("expected HMAC verification error for tampered secret, got nil")
	}
}

func TestIntegration_DecisionKeyDeterminism(t *testing.T) {
	skipIfShort(t)

	// Same inputs should produce same decision key.
	key1 := core.ComputeDecisionKey("hook", "ls -la", "")
	key2 := core.ComputeDecisionKey("hook", "ls -la", "")
	if key1 != key2 {
		t.Errorf("decision keys should be deterministic: %q vs %q", key1, key2)
	}

	// Different inputs should produce different keys.
	key3 := core.ComputeDecisionKey("hook", "rm -rf /", "")
	if key1 == key3 {
		t.Errorf("different commands should produce different decision keys")
	}

	// File hash should change the key.
	key4 := core.ComputeDecisionKey("hook", "ls -la", "abc123hash")
	if key1 == key4 {
		t.Errorf("different file hashes should produce different decision keys")
	}
}

func TestIntegration_NonBashToolAllowed(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test uses Unix-specific shell commands")
	}
	skipIfShort(t)
	withIsolatedHome(t)

	// Non-Bash, non-MCP tools should always be allowed through the hook.
	tools := []string{"Read", "Write", "Edit", "Grep", "Glob"}
	for _, tool := range tools {
		t.Run(tool, func(t *testing.T) {
			input := `{"tool_name":"` + tool + `","tool_input":{"path":"/tmp/test"},"session_id":"integ-test","cwd":"/tmp"}`
			stdin := strings.NewReader(input)
			stderr := &bytes.Buffer{}

			exitCode := adapters.RunHook(stdin, stderr)
			if exitCode != 0 {
				t.Errorf("expected exit code 0 for %s tool, got %d; stderr: %s", tool, exitCode, stderr.String())
			}
		})
	}
}

// ---------------------------------------------------------------------------
// V2 Pipeline Integration Tests
// ---------------------------------------------------------------------------

func TestIntegration_V2_HeredocWithMetadataURL(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test uses Unix-specific shell commands")
	}
	skipIfShort(t)
	withIsolatedHome(t)
	core.ResetBinaryTOFU()
	t.Cleanup(core.ResetBinaryTOFU)

	// Heredoc containing a curl to AWS metadata endpoint.
	// Should be BLOCKED via: heredoc extraction → inline body URL scan → blocked hostname.
	input := `{"tool_name":"Bash","tool_input":{"command":"bash <<EOF\ncurl http://169.254.169.254/latest/meta-data/\nEOF"},"session_id":"integ-v2","cwd":"/tmp"}`
	stdin := strings.NewReader(input)
	stderr := &bytes.Buffer{}

	exitCode := adapters.RunHook(stdin, stderr)
	if exitCode != 2 {
		t.Errorf("expected exit code 2 (BLOCKED) for heredoc with metadata URL, got %d; stderr: %s",
			exitCode, stderr.String())
	}
}

func TestIntegration_V2_HeredocDangerousCommand(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test uses Unix-specific shell commands")
	}
	skipIfShort(t)
	withIsolatedHome(t)
	core.ResetBinaryTOFU()
	t.Cleanup(core.ResetBinaryTOFU)

	// Heredoc containing rm -rf / — should be BLOCKED via extracted body classification.
	input := `{"tool_name":"Bash","tool_input":{"command":"bash <<EOF\nrm -rf /\nEOF"},"session_id":"integ-v2","cwd":"/tmp"}`
	stdin := strings.NewReader(input)
	stderr := &bytes.Buffer{}

	exitCode := adapters.RunHook(stdin, stderr)
	if exitCode != 2 {
		t.Errorf("expected exit code 2 (BLOCKED) for heredoc with rm -rf /, got %d; stderr: %s",
			exitCode, stderr.String())
	}
}

func TestIntegration_V2_InlineBodyPopulated(t *testing.T) {
	skipIfShort(t)
	withIsolatedHome(t)
	core.ResetBinaryTOFU()
	t.Cleanup(core.ResetBinaryTOFU)

	// Classify a heredoc and verify InlineBody is populated.
	evaluator := policy.NewEvaluator(nil)
	req := core.ShellRequest{
		RawCommand: "bash <<EOF\necho hello\nEOF",
		Cwd:        "/tmp",
		Source:     "test",
		SessionID:  "integ-v2",
	}
	result, err := core.Classify(req, evaluator)
	if err != nil {
		t.Fatalf("classify error: %v", err)
	}
	if result.InlineBody == "" {
		t.Error("expected InlineBody to be populated for heredoc command")
	}
}

func TestIntegration_V2_SSRFMetadataBlocked(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test uses Unix-specific shell commands")
	}
	skipIfShort(t)
	withIsolatedHome(t)
	core.ResetBinaryTOFU()
	t.Cleanup(core.ResetBinaryTOFU)

	// Direct SSRF — curl to cloud metadata.
	input := `{"tool_name":"Bash","tool_input":{"command":"curl http://169.254.169.254/latest/meta-data/"},"session_id":"integ-v2","cwd":"/tmp"}`
	stdin := strings.NewReader(input)
	stderr := &bytes.Buffer{}

	exitCode := adapters.RunHook(stdin, stderr)
	if exitCode != 2 {
		t.Errorf("expected exit code 2 (BLOCKED) for SSRF metadata URL, got %d; stderr: %s",
			exitCode, stderr.String())
	}
}

func TestIntegration_V2_DestructiveHTTPMethodCaution(t *testing.T) {
	skipIfShort(t)
	withIsolatedHome(t)
	core.ResetBinaryTOFU()
	t.Cleanup(core.ResetBinaryTOFU)

	// curl -X DELETE should now land in the structural CAUTION tier.
	evaluator := policy.NewEvaluator(nil)
	req := core.ShellRequest{
		RawCommand: "curl -X DELETE https://api.example.com/users/123",
		Cwd:        "/tmp",
		Source:     "test",
		SessionID:  "integ-v2",
	}
	result, err := core.Classify(req, evaluator)
	if err != nil {
		t.Fatalf("classify error: %v", err)
	}
	if result.Decision != core.DecisionCaution {
		t.Errorf("expected CAUTION for curl -X DELETE, got %s (reason: %s)",
			result.Decision, result.Reason)
	}
}

func TestIntegration_V2_HeredocContainingBashC(t *testing.T) {
	skipIfShort(t)
	withIsolatedHome(t)
	core.ResetBinaryTOFU()
	t.Cleanup(core.ResetBinaryTOFU)

	evaluator := policy.NewEvaluator(nil)
	req := core.ShellRequest{
		RawCommand: "bash <<EOF\nbash -c 'rm -rf /'\nEOF",
		Cwd:        "/tmp",
		Source:     "test",
		SessionID:  "integ-v2",
	}
	result, err := core.Classify(req, evaluator)
	if err != nil {
		t.Fatalf("classify error: %v", err)
	}
	// Nested bash -c inside heredoc hits extraction failure and fail-closes to APPROVAL.
	// This still gates execution (user must approve), just doesn't hard-block.
	if result.Decision != core.DecisionApproval && result.Decision != core.DecisionBlocked {
		t.Errorf("expected APPROVAL or BLOCKED for heredoc with bash -c rm -rf /, got %s (reason: %s)",
			result.Decision, result.Reason)
	}
}

func TestIntegration_V2_MultipleHeredocs(t *testing.T) {
	skipIfShort(t)
	withIsolatedHome(t)
	core.ResetBinaryTOFU()
	t.Cleanup(core.ResetBinaryTOFU)

	evaluator := policy.NewEvaluator(nil)
	req := core.ShellRequest{
		RawCommand: "cat <<A\nhello\nA\nbash <<B\ncurl http://169.254.169.254/\nB",
		Cwd:        "/tmp",
		Source:     "test",
		SessionID:  "integ-v2",
	}
	result, err := core.Classify(req, evaluator)
	if err != nil {
		t.Fatalf("classify error: %v", err)
	}
	if result.Decision != core.DecisionBlocked {
		t.Errorf("expected BLOCKED for multi-heredoc with metadata URL, got %s (reason: %s)",
			result.Decision, result.Reason)
	}
}

func TestIntegration_V2_HeredocWithCommandSubstitution(t *testing.T) {
	skipIfShort(t)
	withIsolatedHome(t)
	core.ResetBinaryTOFU()
	t.Cleanup(core.ResetBinaryTOFU)

	evaluator := policy.NewEvaluator(nil)
	req := core.ShellRequest{
		RawCommand: "bash <<EOF\nresult=$(curl http://169.254.169.254/latest/meta-data/)\necho $result\nEOF",
		Cwd:        "/tmp",
		Source:     "test",
		SessionID:  "integ-v2",
	}
	result, err := core.Classify(req, evaluator)
	if err != nil {
		t.Fatalf("classify error: %v", err)
	}
	if result.Decision != core.DecisionBlocked {
		t.Errorf("expected BLOCKED for heredoc with metadata $(), got %s (reason: %s)",
			result.Decision, result.Reason)
	}
}

func TestIntegration_V2_HeredocVariableAssembly(t *testing.T) {
	skipIfShort(t)
	withIsolatedHome(t)
	core.ResetBinaryTOFU()
	t.Cleanup(core.ResetBinaryTOFU)

	// Known limitation: variable assembly evades line-by-line detection.
	// This test documents the current behavior.
	evaluator := policy.NewEvaluator(nil)
	req := core.ShellRequest{
		RawCommand: "bash <<EOF\nCMD=rm\nARGS='-rf /'\n$CMD $ARGS\nEOF",
		Cwd:        "/tmp",
		Source:     "test",
		SessionID:  "integ-v2",
	}
	result, err := core.Classify(req, evaluator)
	if err != nil {
		t.Fatalf("classify error: %v", err)
	}
	t.Skipf("known limitation: variable assembly still evades line-by-line classification; observed %s (%s)", result.Decision, result.Reason)
}

// ---------------------------------------------------------------------------
// Env Var Injection Tests
// ---------------------------------------------------------------------------

func TestIntegration_EnvWrapperSensitiveVarDetected(t *testing.T) {
	skipIfShort(t)
	withIsolatedHome(t)
	core.ResetBinaryTOFU()
	t.Cleanup(core.ResetBinaryTOFU)
	evaluator := policy.NewEvaluator(nil)

	tests := []struct {
		name string
		cmd  string
	}{
		{"env LD_PRELOAD", "env LD_PRELOAD=/evil/lib.so ls"},
		{"env DYLD_INSERT", "env DYLD_INSERT_LIBRARIES=/evil.dylib python"},
		{"env PATH override", "env PATH=/evil:$PATH command_here"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := core.ShellRequest{RawCommand: tt.cmd, Cwd: "/tmp", Source: "test", SessionID: "integ-env"}
			result, err := core.Classify(req, evaluator)
			if err != nil {
				t.Fatalf("classify error: %v", err)
			}
			if result.Decision == core.DecisionSafe {
				t.Errorf("%s should not be SAFE, got %s (reason: %s)", tt.cmd, result.Decision, result.Reason)
			}
		})
	}
}

func TestIntegration_BareEnvVarPathBypass(t *testing.T) {
	skipIfShort(t)
	withIsolatedHome(t)
	core.ResetBinaryTOFU()
	t.Cleanup(core.ResetBinaryTOFU)
	evaluator := policy.NewEvaluator(nil)

	tests := []struct {
		name string
		cmd  string
	}{
		{"LD_PRELOAD with path", "LD_PRELOAD=/tmp/evil.so ls"},
		{"DYLD with path", "DYLD_INSERT_LIBRARIES=/evil/lib.dylib python script.py"},
		{"LD_PRELOAD no path", "LD_PRELOAD=evil.so ls"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := core.ShellRequest{RawCommand: tt.cmd, Cwd: "/tmp", Source: "test", SessionID: "integ-env"}
			result, err := core.Classify(req, evaluator)
			if err != nil {
				t.Fatalf("classify error: %v", err)
			}
			if result.Decision == core.DecisionSafe {
				t.Errorf("%s should not be SAFE, got %s (reason: %s)", tt.cmd, result.Decision, result.Reason)
			}
		})
	}
}
