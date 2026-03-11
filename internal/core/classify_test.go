package core_test

import (
	"os"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/runger/fuse/internal/core"
	"github.com/runger/fuse/internal/policy"
)

// GoldenFixture represents a single golden test fixture entry.
type GoldenFixture struct {
	Command     string `yaml:"command"`
	Expected    string `yaml:"expected"`
	Description string `yaml:"description"`
}

// GoldenFixtures is the top-level structure for the fixtures YAML file.
type GoldenFixtures struct {
	Fixtures []GoldenFixture `yaml:"fixtures"`
}

func TestClassify_GoldenFixtures(t *testing.T) {
	data, err := os.ReadFile("../../testdata/fixtures/commands.yaml")
	if err != nil {
		t.Fatalf("failed to read fixtures: %v", err)
	}
	var fixtures GoldenFixtures
	if err := yaml.Unmarshal(data, &fixtures); err != nil {
		t.Fatalf("failed to parse fixtures: %v", err)
	}

	if len(fixtures.Fixtures) < 100 {
		t.Fatalf("expected at least 100 fixtures, got %d", len(fixtures.Fixtures))
	}

	evaluator := policy.NewEvaluator(nil) // no user policy for golden tests

	for _, f := range fixtures.Fixtures {
		t.Run(f.Description, func(t *testing.T) {
			req := core.ShellRequest{
				RawCommand: f.Command,
				Cwd:        "/tmp",
				Source:     "test",
				SessionID:  "test-session",
			}
			result, err := core.Classify(req, evaluator)
			if err != nil {
				t.Fatalf("classify error: %v", err)
			}
			if string(result.Decision) != f.Expected {
				t.Errorf("command %q: got %s, want %s (reason: %s, rule: %s)",
					f.Command, result.Decision, f.Expected, result.Reason, result.RuleID)
			}
		})
	}
}

func TestClassify_CompoundMostRestrictive(t *testing.T) {
	evaluator := policy.NewEvaluator(nil)

	req := core.ShellRequest{
		RawCommand: "ls && rm -rf /",
		Cwd:        "/tmp",
		Source:     "test",
		SessionID:  "test-session",
	}
	result, err := core.Classify(req, evaluator)
	if err != nil {
		t.Fatalf("classify error: %v", err)
	}
	if result.Decision != core.DecisionBlocked {
		t.Errorf("expected BLOCKED for 'ls && rm -rf /', got %s (reason: %s)",
			result.Decision, result.Reason)
	}
}

func TestClassify_SudoEscalation(t *testing.T) {
	evaluator := policy.NewEvaluator(nil)

	req := core.ShellRequest{
		RawCommand: "sudo echo hello",
		Cwd:        "/tmp",
		Source:     "test",
		SessionID:  "test-session",
	}
	result, err := core.Classify(req, evaluator)
	if err != nil {
		t.Fatalf("classify error: %v", err)
	}
	if result.Decision != core.DecisionCaution {
		t.Errorf("expected CAUTION for 'sudo echo hello', got %s (reason: %s)",
			result.Decision, result.Reason)
	}
}

func TestClassify_InputValidation_TooLong(t *testing.T) {
	evaluator := policy.NewEvaluator(nil)

	longCommand := strings.Repeat("a", 64*1024+1)
	req := core.ShellRequest{
		RawCommand: longCommand,
		Cwd:        "/tmp",
		Source:     "test",
		SessionID:  "test-session",
	}
	result, err := core.Classify(req, evaluator)
	if err != nil {
		t.Fatalf("expected oversized command to classify without error, got %v", err)
	}
	if result.Decision != core.DecisionApproval {
		t.Fatalf("expected oversized command to require APPROVAL, got %s", result.Decision)
	}
}

func TestClassify_InputValidation_NullBytes(t *testing.T) {
	evaluator := policy.NewEvaluator(nil)

	req := core.ShellRequest{
		RawCommand: "ls\x00-la",
		Cwd:        "/tmp",
		Source:     "test",
		SessionID:  "test-session",
	}
	result, err := core.Classify(req, evaluator)
	if err != nil {
		t.Fatalf("expected null bytes to be normalized away, got %v", err)
	}
	if result.Decision != core.DecisionSafe {
		t.Fatalf("expected null-byte command to remain SAFE after normalization, got %s", result.Decision)
	}
}

func TestClassify_EmptyCommand(t *testing.T) {
	evaluator := policy.NewEvaluator(nil)

	req := core.ShellRequest{
		RawCommand: "",
		Cwd:        "/tmp",
		Source:     "test",
		SessionID:  "test-session",
	}
	result, err := core.Classify(req, evaluator)
	if err != nil {
		t.Fatalf("classify error: %v", err)
	}
	if result.Decision != core.DecisionSafe {
		t.Errorf("expected SAFE for empty command, got %s (reason: %s)",
			result.Decision, result.Reason)
	}
}

func TestClassify_InlineScript(t *testing.T) {
	evaluator := policy.NewEvaluator(nil)

	tests := []struct {
		name     string
		command  string
		expected core.Decision
	}{
		{
			name:     "heredoc pattern",
			command:  "cat <<EOF",
			expected: core.DecisionApproval,
		},
		{
			name:     "eval command",
			command:  "eval 'echo dangerous'",
			expected: core.DecisionApproval,
		},
		{
			name:     "python -c inline",
			command:  "python -c 'print(1)'",
			expected: core.DecisionApproval,
		},
		{
			name:     "bash -c inline",
			command:  "bash -c 'echo test'",
			expected: core.DecisionApproval,
		},
		{
			name:     "node -e inline",
			command:  "node -e 'console.log(1)'",
			expected: core.DecisionApproval,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := core.ShellRequest{
				RawCommand: tt.command,
				Cwd:        "/tmp",
				Source:     "test",
				SessionID:  "test-session",
			}
			result, err := core.Classify(req, evaluator)
			if err != nil {
				t.Fatalf("classify error: %v", err)
			}
			if result.Decision != tt.expected {
				t.Errorf("command %q: got %s, want %s (reason: %s)",
					tt.command, result.Decision, tt.expected, result.Reason)
			}
		})
	}
}

func TestClassify_SensitiveEnvVars(t *testing.T) {
	evaluator := policy.NewEvaluator(nil)

	tests := []struct {
		name     string
		command  string
		expected core.Decision
	}{
		{
			name:     "echo GITHUB_TOKEN",
			command:  "echo $GITHUB_TOKEN",
			expected: core.DecisionCaution,
		},
		{
			name:     "echo AWS_SECRET_ACCESS_KEY",
			command:  "echo $AWS_SECRET_ACCESS_KEY",
			expected: core.DecisionCaution,
		},
		{
			name:     "echo DATABASE_URL",
			command:  "echo ${DATABASE_URL}",
			expected: core.DecisionCaution,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := core.ShellRequest{
				RawCommand: tt.command,
				Cwd:        "/tmp",
				Source:     "test",
				SessionID:  "test-session",
			}
			result, err := core.Classify(req, evaluator)
			if err != nil {
				t.Fatalf("classify error: %v", err)
			}
			if result.Decision != tt.expected {
				t.Errorf("command %q: got %s, want %s (reason: %s)",
					tt.command, result.Decision, tt.expected, result.Reason)
			}
		})
	}
}

func TestClassify_InterpreterBackedDangerousScriptUsesInspectionResult(t *testing.T) {
	evaluator := policy.NewEvaluator(nil)

	req := core.ShellRequest{
		RawCommand: "python dangerous_boto3.py",
		Cwd:        "../../testdata/scripts",
		Source:     "test",
		SessionID:  "test-session",
	}
	result, err := core.Classify(req, evaluator)
	if err != nil {
		t.Fatalf("classify error: %v", err)
	}
	if result.Decision != core.DecisionApproval {
		t.Fatalf("expected dangerous script execution to require APPROVAL, got %s (reason: %s)", result.Decision, result.Reason)
	}
}

func TestClassify_InterpreterBackedMissingScriptRequiresApproval(t *testing.T) {
	evaluator := policy.NewEvaluator(nil)

	req := core.ShellRequest{
		RawCommand: "python missing.py",
		Cwd:        "/tmp",
		Source:     "test",
		SessionID:  "test-session",
	}
	result, err := core.Classify(req, evaluator)
	if err != nil {
		t.Fatalf("classify error: %v", err)
	}
	if result.Decision != core.DecisionApproval {
		t.Fatalf("expected missing script execution to require APPROVAL, got %s (reason: %s)", result.Decision, result.Reason)
	}
}
