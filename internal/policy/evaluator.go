package policy

import "github.com/php-workx/fuse/internal/core"

// Evaluator implements core.PolicyEvaluator by delegating to the policy
// package's hardcoded, user, and built-in rule evaluation functions.
type Evaluator struct {
	config       *PolicyConfig
	disabledIDs  map[string]bool
	disabledTags map[string]bool
	tagOverrides map[string]TagOverrideMode
	safeJust     map[string]bool
	ruleIndex    *RuleIndex
}

// NewEvaluator creates a PolicyEvaluator from a PolicyConfig.
// The config may be nil (no user rules, no disabled builtins/tags).
// Builds a keyword index for progressive rule activation.
func NewEvaluator(cfg *PolicyConfig) *Evaluator {
	overrides, _ := ParseTagOverrides(cfg)
	return &Evaluator{
		config:       cfg,
		disabledIDs:  DisabledBuiltinSet(cfg),
		disabledTags: DisabledTagSet(cfg),
		tagOverrides: overrides,
		safeJust:     SafeJustRecipeSet(cfg),
		ruleIndex:    BuildRuleIndex(BuiltinRules),
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

// EvaluateBuiltins checks built-in preset rules with progressive activation
// and per-tag enforcement mode. Returns a BuiltinMatch or nil.
func (e *Evaluator) EvaluateBuiltins(classNorm string) *core.BuiltinMatch {
	return EvaluateBuiltins(classNorm, e.disabledIDs, e.disabledTags, e.tagOverrides, e.ruleIndex)
}

// IsSafeJustRecipe checks project policy's exact just recipe allowlist.
func (e *Evaluator) IsSafeJustRecipe(recipe string) bool {
	if e == nil || len(e.safeJust) == 0 {
		return false
	}
	return e.safeJust[recipe]
}
