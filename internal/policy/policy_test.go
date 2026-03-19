package policy

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/runger/fuse/internal/core"
)

// TestHardcoded_AllCompile verifies that all 22 hardcoded patterns are valid
// compiled regexes (they use MustCompile, so this also confirms the count).
func TestHardcoded_AllCompile(t *testing.T) {
	if len(HardcodedBlocked) != 22 {
		t.Fatalf("expected 22 hardcoded rules, got %d", len(HardcodedBlocked))
	}
	for i, r := range HardcodedBlocked {
		if r.Pattern == nil {
			t.Errorf("rule %d has nil pattern", i)
		}
		if r.Reason == "" {
			t.Errorf("rule %d has empty reason", i)
		}
	}
}

// TestHardcoded_MatchExamples tests that hardcoded rules match known-dangerous
// commands and do NOT match benign ones.
func TestHardcoded_MatchExamples(t *testing.T) {
	shouldMatch := []struct {
		cmd    string
		reason string
	}{
		{"rm -rf /", "rm -rf / should be blocked"},
		{"rm -rf /*", "rm -rf /* should be blocked"},
		{"rm -rf ~", "rm -rf ~ should be blocked"},
		{"rm -rf $HOME", "rm -rf $HOME should be blocked"},
		// Note: -rfi (trailing chars after f) requires tokenized argv analysis per §6.2;
		// regex alone covers -rf but not -rfi. That logic is in the classifier, not here.
		{"rm -r -f /", "rm with split flags should be blocked"},
		{"rm -f -r /", "rm with reversed split flags should be blocked"},
		{"rm --recursive --force /", "rm with long flags should be blocked"},
		{"rm --force --recursive /", "rm with reversed long flags should be blocked"},
		{"rm -r --force /", "rm with mixed flags should be blocked"},
		{"rm --recursive -f /", "rm with mixed flags (reversed) should be blocked"},
		{"mkfs.ext4 /dev/sda1", "mkfs should be blocked"},
		{"mkswap /dev/sda2", "mkswap on device should be blocked"},
		{"dd if=/dev/zero of=/dev/sda", "dd writing to device should be blocked"},
		{"> /dev/sda", "redirect to device should be blocked"},
		{":() { :|: & }; :", "fork bomb should be blocked"},
		{"chmod 777 / ", "chmod 777 on root should be blocked"},
		{"chmod -R 777 / ", "chmod -R 777 on root should be blocked"},
		{"chown -R root:root / ", "chown -R on root should be blocked"},
		{"fuse disable", "fuse disable should be blocked"},
		{"fuse uninstall", "fuse uninstall should be blocked"},
		{"fuse enable", "fuse enable should be blocked"},
		{"tee ~/.fuse/config/policy.yaml", "writing to fuse config should be blocked"},
		{"cp malicious.json .claude/settings.json", "writing to claude settings should be blocked"},
		{"sed -i 's/a/b/' ~/.fuse/config/policy.yaml", "modifying fuse config should be blocked"},
		{"rm -rf ~/.fuse/", "deleting fuse directory should be blocked"},
		{"rm .claude/settings.json", "deleting claude settings should be blocked"},
		{"sqlite3 fuse.db \"DROP TABLE events\"", "destructive sqlite3 on fuse db should be blocked"},
		{"python -c 'import os' ~/.fuse/config", "python eval touching fuse files should be blocked"},
		{"node -e 'code' .claude/settings.json", "node eval touching claude settings should be blocked"},
		{"bash -c 'cat fuse.db'", "bash eval touching fuse db should be blocked"},
	}

	for _, tc := range shouldMatch {
		dec, reason := EvaluateHardcoded(tc.cmd)
		if dec != core.DecisionBlocked {
			t.Errorf("%s: got decision %q, want BLOCKED (reason: %s)", tc.reason, dec, reason)
		}
	}

	shouldNotMatch := []struct {
		cmd    string
		reason string
	}{
		{"rm file.txt", "simple rm should not be blocked"},
		{"rm -r mydir", "rm -r without -f on non-root should not be blocked"},
		{"rm -f file.txt", "rm -f without -r should not be blocked"},
		{"ls /dev/sda", "listing a device should not be blocked"},
		{"echo hello", "echo should not be blocked"},
		{"dd if=file.img of=backup.img", "dd not writing to device should not be blocked"},
		{"chmod 755 /home/user/script.sh", "chmod 755 should not be blocked"},
		{"fuse status", "fuse status should not be blocked"},
		{"cat ~/.fuse/config/policy.yaml", "reading fuse config should not be blocked"},
		{"git rm file.txt", "git rm should not be blocked"},
	}

	for _, tc := range shouldNotMatch {
		dec, _ := EvaluateHardcoded(tc.cmd)
		if dec == core.DecisionBlocked {
			t.Errorf("%s: got BLOCKED, want no match", tc.reason)
		}
	}
}

// TestPolicyLoad tests loading and parsing a sample policy YAML file.
func TestPolicyLoad(t *testing.T) {
	yamlContent := `
version: "1"
rules:
  - pattern: "\\bterraform\\s+destroy\\b"
    action: "approval"
    reason: "Terraform destroy requires approval"
  - pattern: "\\bgit\\s+push\\s+.*--force\\b"
    action: "caution"
    reason: "Force push is risky"
  - pattern: "\\bdrop\\s+database\\b"
    action: "block"
    reason: "Dropping databases is blocked"
disabled_builtins:
  - "git:reset-hard"
  - "docker:prune-system"
`
	tmpDir := t.TempDir()
	policyPath := filepath.Join(tmpDir, "policy.yaml")
	if err := os.WriteFile(policyPath, []byte(yamlContent), 0o644); err != nil {
		t.Fatalf("writing test policy: %v", err)
	}

	cfg, err := LoadPolicy(policyPath)
	if err != nil {
		t.Fatalf("LoadPolicy: %v", err)
	}

	if cfg.Version != "1" {
		t.Errorf("version = %q, want %q", cfg.Version, "1")
	}
	if len(cfg.Rules) != 3 {
		t.Fatalf("expected 3 rules, got %d", len(cfg.Rules))
	}
	if len(cfg.DisabledBuiltins) != 2 {
		t.Fatalf("expected 2 disabled builtins, got %d", len(cfg.DisabledBuiltins))
	}

	// Verify patterns compiled
	for i, r := range cfg.Rules {
		if r.compiled == nil {
			t.Errorf("rule %d pattern not compiled", i)
		}
	}

	// Verify disabled builtins set
	disabled := DisabledBuiltinSet(cfg)
	if !disabled["git:reset-hard"] {
		t.Error("git:reset-hard should be disabled")
	}
	if !disabled["docker:prune-system"] {
		t.Error("docker:prune-system should be disabled")
	}
}

// TestPolicyLoad_InvalidPattern tests that LoadPolicy returns an error for invalid regex.
func TestPolicyLoad_InvalidPattern(t *testing.T) {
	yamlContent := `
version: "1"
rules:
  - pattern: "[invalid"
    action: "block"
    reason: "Bad regex"
`
	tmpDir := t.TempDir()
	policyPath := filepath.Join(tmpDir, "policy.yaml")
	if err := os.WriteFile(policyPath, []byte(yamlContent), 0o644); err != nil {
		t.Fatalf("writing test policy: %v", err)
	}

	_, err := LoadPolicy(policyPath)
	if err == nil {
		t.Fatal("expected error for invalid regex pattern, got nil")
	}
}

// TestPolicyLoad_InvalidAction tests that LoadPolicy returns an error for invalid action.
func TestPolicyLoad_InvalidAction(t *testing.T) {
	yamlContent := `
version: "1"
rules:
  - pattern: "\\btest\\b"
    action: "invalid_action"
    reason: "Bad action"
`
	tmpDir := t.TempDir()
	policyPath := filepath.Join(tmpDir, "policy.yaml")
	if err := os.WriteFile(policyPath, []byte(yamlContent), 0o644); err != nil {
		t.Fatalf("writing test policy: %v", err)
	}

	_, err := LoadPolicy(policyPath)
	if err == nil {
		t.Fatal("expected error for invalid action, got nil")
	}
}

// TestPolicyEvaluation tests that user rules match correctly and the most
// restrictive decision wins.
func TestPolicyEvaluation(t *testing.T) {
	yamlContent := `
version: "1"
rules:
  - pattern: "\\bterraform\\s+destroy\\b"
    action: "approval"
    reason: "Terraform destroy requires approval"
  - pattern: "\\bterraform\\b"
    action: "caution"
    reason: "Terraform commands need review"
  - pattern: "\\bgit\\s+push\\b"
    action: "safe"
    reason: "Git push is allowed"
`
	tmpDir := t.TempDir()
	policyPath := filepath.Join(tmpDir, "policy.yaml")
	if err := os.WriteFile(policyPath, []byte(yamlContent), 0o644); err != nil {
		t.Fatalf("writing test policy: %v", err)
	}

	cfg, err := LoadPolicy(policyPath)
	if err != nil {
		t.Fatalf("LoadPolicy: %v", err)
	}

	tests := []struct {
		name       string
		cmd        string
		wantDec    core.Decision
		wantReason string
	}{
		{
			name:       "terraform destroy matches most restrictive (APPROVAL)",
			cmd:        "terraform destroy",
			wantDec:    core.DecisionApproval,
			wantReason: "Terraform destroy requires approval",
		},
		{
			name:       "terraform plan matches CAUTION only",
			cmd:        "terraform plan",
			wantDec:    core.DecisionCaution,
			wantReason: "Terraform commands need review",
		},
		{
			name:       "git push matches SAFE",
			cmd:        "git push origin main",
			wantDec:    core.DecisionSafe,
			wantReason: "Git push is allowed",
		},
		{
			name:    "no match returns empty",
			cmd:     "echo hello",
			wantDec: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dec, reason := EvaluateUserRules(tc.cmd, cfg)
			if dec != tc.wantDec {
				t.Errorf("decision = %q, want %q", dec, tc.wantDec)
			}
			if tc.wantReason != "" && reason != tc.wantReason {
				t.Errorf("reason = %q, want %q", reason, tc.wantReason)
			}
		})
	}
}

// TestPolicyEvaluation_NilPolicy tests that EvaluateUserRules handles nil policy gracefully.
func TestPolicyEvaluation_NilPolicy(t *testing.T) {
	dec, reason := EvaluateUserRules("rm -rf /", nil)
	if dec != "" || reason != "" {
		t.Errorf("expected empty result for nil policy, got dec=%q reason=%q", dec, reason)
	}
}

// TestEvaluateBuiltins_Empty verifies that EvaluateBuiltins returns no match
// when no builtin rules are registered (BuiltinRules will be populated by 5b/5c).
func TestEvaluateBuiltins_Empty(t *testing.T) {
	// Save and restore BuiltinRules in case other tests modify it
	saved := BuiltinRules
	BuiltinRules = nil
	defer func() { BuiltinRules = saved }()

	idx := BuildRuleIndex(BuiltinRules)
	match := EvaluateBuiltins("rm -rf /", nil, nil, nil, idx)
	if match != nil {
		t.Errorf("expected no match with empty builtins, got %+v", match)
	}
}

// TestEvaluateBuiltins_WithRule verifies that EvaluateBuiltins matches a registered
// builtin rule and respects disabled IDs.
func TestEvaluateBuiltins_WithRule(t *testing.T) {
	saved := BuiltinRules
	defer func() { BuiltinRules = saved }()

	BuiltinRules = []BuiltinRule{
		{
			ID:      "test:rule1",
			Pattern: regexp.MustCompile(`\btest\b`),
			Action:  core.DecisionCaution,
			Reason:  "test rule",
		},
	}

	idx := BuildRuleIndex(BuiltinRules)

	// Should match
	match := EvaluateBuiltins("run test now", nil, nil, nil, idx)
	if match == nil {
		t.Fatal("expected a match")
	}
	if match.Decision != core.DecisionCaution {
		t.Errorf("expected CAUTION, got %q", match.Decision)
	}
	if match.Reason != "test rule" {
		t.Errorf("expected 'test rule', got %q", match.Reason)
	}
	if match.RuleID != "test:rule1" {
		t.Errorf("expected 'test:rule1', got %q", match.RuleID)
	}

	// Should be skipped when disabled
	disabled := map[string]bool{"test:rule1": true}
	match = EvaluateBuiltins("run test now", disabled, nil, nil, idx)
	if match != nil {
		t.Errorf("expected no match when disabled, got %+v", match)
	}
}

func TestEvaluateBuiltins_TagOverrideDryRun(t *testing.T) {
	saved := BuiltinRules
	defer func() { BuiltinRules = saved }()

	BuiltinRules = []BuiltinRule{
		{
			ID:      "test:git:push",
			Pattern: regexp.MustCompile(`\bgit\s+push\b`),
			Action:  core.DecisionCaution,
			Reason:  "git push",
			Tags:    []string{"git"},
		},
	}

	idx := BuildRuleIndex(BuiltinRules)
	overrides := map[string]TagOverrideMode{"git": TagOverrideDryRun}

	match := EvaluateBuiltins("git push origin main", nil, nil, overrides, idx)
	if match == nil {
		t.Fatal("expected match")
	}
	if !match.DryRun {
		t.Error("expected DryRun=true for tag override dryrun")
	}
	if match.Decision != core.DecisionCaution {
		t.Errorf("expected CAUTION, got %q", match.Decision)
	}
}

func TestEvaluateBuiltins_TagOverrideDisabled(t *testing.T) {
	saved := BuiltinRules
	defer func() { BuiltinRules = saved }()

	BuiltinRules = []BuiltinRule{
		{
			ID:      "test:git:push",
			Pattern: regexp.MustCompile(`\bgit\s+push\b`),
			Action:  core.DecisionCaution,
			Reason:  "git push",
			Tags:    []string{"git"},
		},
	}

	idx := BuildRuleIndex(BuiltinRules)
	overrides := map[string]TagOverrideMode{"git": TagOverrideDisabled}

	match := EvaluateBuiltins("git push origin main", nil, nil, overrides, idx)
	if match != nil {
		t.Errorf("expected no match when tag override is disabled, got %+v", match)
	}
}

func TestEvaluateBuiltins_TagOverrideEnabled(t *testing.T) {
	saved := BuiltinRules
	defer func() { BuiltinRules = saved }()

	BuiltinRules = []BuiltinRule{
		{
			ID:      "test:git:push",
			Pattern: regexp.MustCompile(`\bgit\s+push\b`),
			Action:  core.DecisionCaution,
			Reason:  "git push",
			Tags:    []string{"git"},
		},
	}

	idx := BuildRuleIndex(BuiltinRules)
	overrides := map[string]TagOverrideMode{"git": TagOverrideEnabled}

	match := EvaluateBuiltins("git push origin main", nil, nil, overrides, idx)
	if match == nil {
		t.Fatal("expected match")
	}
	if match.DryRun {
		t.Error("expected DryRun=false for tag override enabled")
	}
}

func TestEffectiveTagMode_MostRestrictiveWins(t *testing.T) {
	// Rule has tags ["aws", "cloudformation"]
	// aws=enabled, cloudformation=dryrun → enabled wins (most restrictive)
	overrides := map[string]TagOverrideMode{
		"aws":            TagOverrideEnabled,
		"cloudformation": TagOverrideDryRun,
	}
	mode := effectiveTagMode([]string{"aws", "cloudformation"}, overrides)
	if mode != TagOverrideEnabled {
		t.Errorf("expected TagOverrideEnabled, got %d", mode)
	}
}

func TestEffectiveTagMode_NoOverrides(t *testing.T) {
	mode := effectiveTagMode([]string{"aws", "cloud"}, nil)
	if mode != TagOverrideEnabled {
		t.Errorf("expected TagOverrideEnabled when no overrides, got %d", mode)
	}
}

func TestEffectiveTagMode_UnmatchedTags(t *testing.T) {
	overrides := map[string]TagOverrideMode{"payment": TagOverrideDisabled}
	mode := effectiveTagMode([]string{"aws", "cloud"}, overrides)
	if mode != TagOverrideEnabled {
		t.Errorf("expected TagOverrideEnabled for unmatched tags, got %d", mode)
	}
}
