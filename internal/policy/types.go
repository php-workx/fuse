package policy

import (
	"regexp"

	"github.com/runger/fuse/internal/core"
)

// HardcodedRule represents a non-overridable safety rule compiled into the binary.
// These rules cannot be disabled by user policy.
type HardcodedRule struct {
	Pattern *regexp.Regexp
	Reason  string
}

// BuiltinRule represents a built-in preset rule that ships with fuse.
// These can be disabled by users via disabled_builtins in policy.yaml.
type BuiltinRule struct {
	ID        string
	Pattern   *regexp.Regexp
	Action    core.Decision
	Reason    string
	Predicate func(string) bool // optional additional check; nil means pattern-only
}

// BuiltinRules is populated by init() in builtins_core.go and builtins_security.go.
var BuiltinRules []BuiltinRule

// EvaluateHardcoded checks the normalized command string against all hardcoded
// BLOCKED rules. Returns the decision and reason if matched, or empty strings
// if no hardcoded rule applies. Hardcoded rules cannot be overridden.
func EvaluateHardcoded(classNorm string) (core.Decision, string) {
	for _, r := range HardcodedBlocked {
		if r.Pattern.MatchString(classNorm) {
			return core.DecisionBlocked, r.Reason
		}
	}
	return "", "" // no match
}

// EvaluateBuiltins checks the normalized command string against all registered
// built-in rules, skipping any whose ID is in the disabledIDs set. Returns the
// decision, reason, and rule ID if matched, or empty strings if no rule applies.
func EvaluateBuiltins(classNorm string, disabledIDs map[string]bool) (core.Decision, string, string) {
	for _, r := range BuiltinRules {
		if disabledIDs[r.ID] {
			continue
		}
		if r.Pattern.MatchString(classNorm) {
			if r.Predicate != nil && !r.Predicate(classNorm) {
				continue
			}
			return r.Action, r.Reason, r.ID
		}
	}
	return "", "", "" // no match
}
