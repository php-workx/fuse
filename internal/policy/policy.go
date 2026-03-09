package policy

import (
	"fmt"
	"os"
	"regexp"

	"github.com/runger/fuse/internal/core"
	"gopkg.in/yaml.v3"
)

// PolicyConfig represents user-defined policy loaded from a YAML file.
type PolicyConfig struct {
	Version          string       `yaml:"version"`
	Rules            []PolicyRule `yaml:"rules"`
	DisabledBuiltins []string     `yaml:"disabled_builtins"`
}

// PolicyRule represents a single user-defined rule in policy.yaml.
type PolicyRule struct {
	Pattern  string `yaml:"pattern"`
	Action   string `yaml:"action"` // "allow", "caution", "approval", "block"
	Reason   string `yaml:"reason"`
	compiled *regexp.Regexp
}

// actionToDecision maps user-facing action strings to core.Decision values.
var actionToDecision = map[string]core.Decision{
	"allow":    core.DecisionSafe,
	"safe":     core.DecisionSafe,
	"caution":  core.DecisionCaution,
	"approval": core.DecisionApproval,
	"block":    core.DecisionBlocked,
}

// LoadPolicy reads and parses a policy YAML file, compiling all rule patterns.
// Returns an error if the file cannot be read, parsed, or if any pattern is invalid.
func LoadPolicy(path string) (*PolicyConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading policy file: %w", err)
	}

	var cfg PolicyConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing policy YAML: %w", err)
	}

	for i := range cfg.Rules {
		r := &cfg.Rules[i]
		compiled, err := regexp.Compile(r.Pattern)
		if err != nil {
			return nil, fmt.Errorf("compiling rule pattern %q: %w", r.Pattern, err)
		}
		r.compiled = compiled

		// Validate action string
		if _, ok := actionToDecision[r.Action]; !ok {
			return nil, fmt.Errorf("invalid action %q in rule with pattern %q", r.Action, r.Pattern)
		}
	}

	return &cfg, nil
}

// EvaluateUserRules checks the normalized command string against all user-defined
// policy rules. Returns the most restrictive matching decision and corresponding
// reason, or empty strings if no rule matches.
func EvaluateUserRules(classNorm string, policy *PolicyConfig) (core.Decision, string) {
	if policy == nil {
		return "", ""
	}

	var bestDecision core.Decision
	var bestReason string

	for _, r := range policy.Rules {
		if r.compiled == nil {
			continue
		}
		if r.compiled.MatchString(classNorm) {
			decision := actionToDecision[r.Action]
			if bestDecision == "" {
				bestDecision = decision
				bestReason = r.Reason
			} else {
				combined := core.MaxDecision(bestDecision, decision)
				if combined != bestDecision {
					bestDecision = combined
					bestReason = r.Reason
				}
			}
		}
	}

	return bestDecision, bestReason
}

// DisabledBuiltinSet returns a map for quick lookup of disabled builtin rule IDs.
func DisabledBuiltinSet(policy *PolicyConfig) map[string]bool {
	if policy == nil {
		return nil
	}
	if len(policy.DisabledBuiltins) == 0 {
		return nil
	}
	m := make(map[string]bool, len(policy.DisabledBuiltins))
	for _, id := range policy.DisabledBuiltins {
		m[id] = true
	}
	return m
}
