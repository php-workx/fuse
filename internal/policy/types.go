package policy

import (
	"regexp"
	"sort"
	"strings"

	"github.com/php-workx/fuse/internal/core"
)

// TagOverrideMode represents a per-tag enforcement mode override.
type TagOverrideMode int

const (
	// TagOverrideNone means no override — use global mode.
	TagOverrideNone TagOverrideMode = iota
	// TagOverrideDisabled means skip the rule entirely (no log, no match).
	TagOverrideDisabled
	// TagOverrideDryRun means evaluate and log, but don't enforce.
	TagOverrideDryRun
	// TagOverrideEnabled means enforce regardless of global mode.
	TagOverrideEnabled
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
// Returns a BuiltinMatch if a rule matched, or nil if no match.
// The DryRun field indicates the match should be logged but not enforced.
func EvaluateBuiltins(
	classNorm string,
	disabledIDs, disabledTags map[string]bool,
	tagOverrides map[string]TagOverrideMode,
	index *RuleIndex,
) *core.BuiltinMatch {
	candidates := index.CandidateRules(classNorm)

	for _, i := range candidates {
		r := &BuiltinRules[i]
		if disabledIDs[r.ID] {
			continue
		}
		if isTagDisabled(r.Tags, disabledTags) {
			continue
		}
		// Check tag override — disabled overrides skip entirely.
		mode, explicit := effectiveTagMode(r.Tags, tagOverrides)
		if mode == TagOverrideDisabled {
			continue
		}
		if r.Pattern.MatchString(classNorm) {
			if r.Predicate != nil && !r.Predicate(classNorm) {
				continue
			}
			return &core.BuiltinMatch{
				Decision:            r.Action,
				Reason:              r.Reason,
				RuleID:              r.ID,
				DryRun:              mode == TagOverrideDryRun,
				TagOverrideEnforced: explicit && mode == TagOverrideEnabled,
			}
		}
	}
	return nil // no match
}

// effectiveTagMode determines the enforcement mode for a rule based on its tags.
// Most restrictive tag override wins (enabled > dryrun > disabled).
// If no override applies, returns (enabled, false) — rule fires normally,
// global dryrun is handled by the adapter layer, not here.
// The second return value indicates whether an explicit tag_override matched.
func effectiveTagMode(tags []string, overrides map[string]TagOverrideMode) (TagOverrideMode, bool) {
	if len(overrides) == 0 {
		return TagOverrideEnabled, false
	}

	best := TagOverrideNone
	for _, tag := range tags {
		if m, ok := overrides[tag]; ok {
			if m > best { // enabled > dryrun > disabled > none
				best = m
			}
		}
	}

	if best != TagOverrideNone {
		return best, true
	}
	return TagOverrideEnabled, false
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
