package policy

import "github.com/runger/fuse/internal/core"

// Evaluator implements core.PolicyEvaluator by delegating to the policy
// package's hardcoded, user, and built-in rule evaluation functions.
type Evaluator struct {
	config      *PolicyConfig
	disabledIDs map[string]bool
}

// NewEvaluator creates a PolicyEvaluator from a PolicyConfig.
// The config may be nil (no user rules, no disabled builtins).
func NewEvaluator(config *PolicyConfig) *Evaluator {
	return &Evaluator{
		config:      config,
		disabledIDs: DisabledBuiltinSet(config),
	}
}

// EvaluateHardcoded checks hardcoded BLOCKED rules.
func (e *Evaluator) EvaluateHardcoded(classNorm string) (core.Decision, string) {
	return EvaluateHardcoded(classNorm)
}

// EvaluateUserRules checks user-defined policy rules.
func (e *Evaluator) EvaluateUserRules(classNorm string) (core.Decision, string) {
	return EvaluateUserRules(classNorm, e.config)
}

// EvaluateBuiltins checks built-in preset rules.
func (e *Evaluator) EvaluateBuiltins(classNorm string) (core.Decision, string, string) {
	return EvaluateBuiltins(classNorm, e.disabledIDs)
}
