package core

import "testing"

func TestDecisionSeverity_AllDecisions(t *testing.T) {
	tests := []struct {
		decision Decision
		want     int
	}{
		{DecisionSafe, 0},
		{DecisionCaution, 1},
		{DecisionApproval, 2},
		{DecisionBlocked, 3},
	}
	for _, tt := range tests {
		got := DecisionSeverity(tt.decision)
		if got != tt.want {
			t.Errorf("DecisionSeverity(%q) = %d, want %d", tt.decision, got, tt.want)
		}
	}
}

func TestDecisionSeverity_Unknown(t *testing.T) {
	got := DecisionSeverity(Decision("INVALID"))
	if got != -1 {
		t.Errorf("DecisionSeverity(INVALID) = %d, want -1", got)
	}
}

func TestWithDecision_ChangesDecisionAndReason(t *testing.T) {
	original := &ClassifyResult{
		Decision: DecisionApproval,
		Reason:   "original reason",
		RuleID:   "test-rule",
	}

	modified := original.WithDecision(DecisionSafe, "judge override")

	if modified.Decision != DecisionSafe {
		t.Errorf("modified.Decision = %q, want SAFE", modified.Decision)
	}
	if modified.Reason != "judge override" {
		t.Errorf("modified.Reason = %q, want 'judge override'", modified.Reason)
	}
	// Original must be unchanged.
	if original.Decision != DecisionApproval {
		t.Errorf("original.Decision mutated to %q", original.Decision)
	}
	if original.Reason != "original reason" {
		t.Errorf("original.Reason mutated to %q", original.Reason)
	}
}

func TestWithDecision_PreservesOtherFields(t *testing.T) {
	original := &ClassifyResult{
		Decision:            DecisionCaution,
		Reason:              "caution reason",
		RuleID:              "rule-123",
		DecisionKey:         "key-abc",
		TagOverrideEnforced: true,
	}

	modified := original.WithDecision(DecisionSafe, "new reason")

	if modified.RuleID != "rule-123" {
		t.Errorf("RuleID not preserved: %q", modified.RuleID)
	}
	if modified.DecisionKey != "key-abc" {
		t.Errorf("DecisionKey not preserved: %q", modified.DecisionKey)
	}
	if !modified.TagOverrideEnforced {
		t.Error("TagOverrideEnforced not preserved")
	}
}

func TestWithDecision_DeepCopiesSlices(t *testing.T) {
	original := &ClassifyResult{
		Decision: DecisionApproval,
		Reason:   "reason",
		SubResults: []SubCommandResult{
			{Command: "cmd1", Decision: DecisionSafe},
			{Command: "cmd2", Decision: DecisionCaution},
		},
		DryRunMatches: []BuiltinMatch{
			{Decision: DecisionCaution, RuleID: "dry-1"},
		},
	}

	modified := original.WithDecision(DecisionSafe, "override")

	// Modify the copy's slices.
	modified.SubResults[0].Command = "mutated"
	modified.DryRunMatches[0].RuleID = "mutated"

	// Original must be unchanged.
	if original.SubResults[0].Command != "cmd1" {
		t.Errorf("SubResults aliased: original[0].Command = %q", original.SubResults[0].Command)
	}
	if original.DryRunMatches[0].RuleID != "dry-1" {
		t.Errorf("DryRunMatches aliased: original[0].RuleID = %q", original.DryRunMatches[0].RuleID)
	}
}
