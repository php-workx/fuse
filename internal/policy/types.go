package policy

import (
	"regexp"
	"sort"
	"strings"

	"github.com/runger/fuse/internal/core"
)

// HardcodedRule represents a non-overridable safety rule compiled into the binary.
// These rules cannot be disabled by user policy.
type HardcodedRule struct {
	Pattern   *regexp.Regexp
	Reason    string
	Predicate func(string) bool // optional: if set, Pattern must match AND Predicate must return true
}

// BuiltinRule represents a built-in preset rule that ships with fuse.
// Rules are tagged for progressive activation and user-level enable/disable.
type BuiltinRule struct {
	ID        string
	Pattern   *regexp.Regexp
	Action    core.Decision
	Reason    string
	Tags      []string          // e.g. ["aws", "cloud", "s3"] — for filtering and progressive activation
	Keywords  []string          // fast substring pre-filter; regex only runs if a keyword is found
	Predicate func(string) bool // optional additional check; nil means pattern-only
}

// BuiltinRules is populated by init() in builtins_*.go files.
var BuiltinRules []BuiltinRule

// RuleIndex enables progressive rule activation: only evaluate rules whose
// keywords appear in the command. Built once at NewEvaluator time.
type RuleIndex struct {
	// keywordToRules maps each keyword to the indices of rules that use it.
	keywordToRules map[string][]int
	// noKeywordRules are rule indices with no keywords (always evaluated).
	noKeywordRules []int
}

// BuildRuleIndex creates a keyword index for progressive activation.
func BuildRuleIndex(rules []BuiltinRule) *RuleIndex {
	idx := &RuleIndex{
		keywordToRules: make(map[string][]int),
	}
	for i, r := range rules {
		if len(r.Keywords) == 0 {
			idx.noKeywordRules = append(idx.noKeywordRules, i)
		} else {
			for _, kw := range r.Keywords {
				idx.keywordToRules[strings.ToLower(kw)] = append(idx.keywordToRules[strings.ToLower(kw)], i)
			}
		}
	}
	return idx
}

// CandidateRules returns the indices of rules that might match the given command,
// based on keyword pre-filtering. Rules without keywords are always included.
func (idx *RuleIndex) CandidateRules(cmd string) []int {
	cmdLower := strings.ToLower(cmd)
	seen := make(map[int]bool)
	var candidates []int

	// Always include rules with no keywords.
	for _, i := range idx.noKeywordRules {
		candidates = append(candidates, i)
		seen[i] = true
	}

	// Include rules whose keywords appear in the command.
	for kw, indices := range idx.keywordToRules {
		if strings.Contains(cmdLower, kw) {
			for _, i := range indices {
				if !seen[i] {
					candidates = append(candidates, i)
					seen[i] = true
				}
			}
		}
	}
	// Sort by original rule index to preserve registration order (first match wins).
	sort.Ints(candidates)
	return candidates
}

// EvaluateHardcoded checks the normalized command string against all hardcoded
// BLOCKED rules. Returns the decision and reason if matched, or empty strings
// if no hardcoded rule applies. Hardcoded rules cannot be overridden.
func EvaluateHardcoded(classNorm string) (core.Decision, string) {
	for _, r := range HardcodedBlocked {
		if r.Pattern.MatchString(classNorm) {
			if r.Predicate != nil && !r.Predicate(classNorm) {
				continue
			}
			return core.DecisionBlocked, r.Reason
		}
	}
	return "", "" // no match
}

// EvaluateBuiltins checks built-in rules using progressive activation.
// Only rules whose keywords match the command are evaluated (plus rules with no keywords).
func EvaluateBuiltins(classNorm string, disabledIDs, disabledTags map[string]bool, index *RuleIndex) (core.Decision, string, string) {
	candidates := index.CandidateRules(classNorm)

	for _, i := range candidates {
		r := &BuiltinRules[i]
		if disabledIDs[r.ID] {
			continue
		}
		if isTagDisabled(r.Tags, disabledTags) {
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

// isTagDisabled returns true if any of the rule's tags are in the disabled set.
func isTagDisabled(tags []string, disabledTags map[string]bool) bool {
	for _, tag := range tags {
		if disabledTags[tag] {
			return true
		}
	}
	return false
}
