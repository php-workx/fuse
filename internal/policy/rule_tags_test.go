package policy

import (
	"testing"
)

func TestBuiltinRuleTagParity(t *testing.T) {
	tags := builtinRuleTags()

	// Build a set of all registered rule IDs.
	ruleIDs := make(map[string]bool)
	for _, r := range BuiltinRules {
		ruleIDs[r.ID] = true
	}

	// Every tag entry must reference a real rule.
	for id := range tags {
		if !ruleIDs[id] {
			t.Errorf("rule_tags.go references %q but no BuiltinRule with that ID exists", id)
		}
	}

	// Every rule that starts with "builtin:" should have tags.
	// This catches new rules added without corresponding tag registration.
	for _, r := range BuiltinRules {
		if _, ok := tags[r.ID]; !ok {
			t.Errorf("BuiltinRule %q has no tag entry in rule_tags.go", r.ID)
		}
	}
}
