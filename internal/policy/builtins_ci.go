package policy

import (
	"regexp"
	"strings"

	"github.com/php-workx/fuse/internal/core"
)

func init() {
	BuiltinRules = append(BuiltinRules,
		// ===================================================================
		// CI/CD protections
		// ===================================================================
		BuiltinRule{
			ID:      "builtin:cicd:gh-secret-delete",
			Pattern: regexp.MustCompile(`\bgh\s+secret\s+delete\b`),
			Action:  core.DecisionCaution,
			Reason:  "Deletes a GitHub CLI secret",
		},
		BuiltinRule{
			ID:      "builtin:cicd:gh-variable-delete",
			Pattern: regexp.MustCompile(`\bgh\s+variable\s+delete\b`),
			Action:  core.DecisionCaution,
			Reason:  "Deletes a GitHub CLI variable",
		},
		BuiltinRule{
			ID:      "builtin:cicd:gh-api-actions-admin",
			Pattern: regexp.MustCompile(`\bgh\s+api\b.*\/actions\/(secrets|variables)\b`),
			Action:  core.DecisionCaution,
			Reason:  "Modifies GitHub Actions secrets or variables via gh api",
			Predicate: func(cmd string) bool {
				lower := strings.ToLower(cmd)
				return strings.Contains(lower, "-x delete") ||
					strings.Contains(lower, "-x patch") ||
					strings.Contains(lower, "-x put") ||
					strings.Contains(lower, "--method delete") ||
					strings.Contains(lower, "--method patch") ||
					strings.Contains(lower, "--method put")
			},
		},
		BuiltinRule{
			ID:      "builtin:cicd:gitlab-runner-unregister",
			Pattern: regexp.MustCompile(`\bgitlab-runner\s+unregister\b`),
			Action:  core.DecisionCaution,
			Reason:  "Unregisters a GitLab runner",
		},
		BuiltinRule{
			ID:      "builtin:cicd:glab-variable-delete",
			Pattern: regexp.MustCompile(`\bglab\s+variable\s+delete\b`),
			Action:  core.DecisionCaution,
			Reason:  "Deletes a GitLab CI variable",
		},
		BuiltinRule{
			ID:      "builtin:cicd:jenkins-delete-job",
			Pattern: regexp.MustCompile(`\b(?:jenkins-cli|java\s+-jar\s+\S*jenkins-cli\.jar)\s+delete-job\b`),
			Action:  core.DecisionCaution,
			Reason:  "Deletes a Jenkins job",
		},
		BuiltinRule{
			ID:      "builtin:cicd:circleci-remove-secret",
			Pattern: regexp.MustCompile(`\bcircleci\s+context\s+remove-secret\b`),
			Action:  core.DecisionCaution,
			Reason:  "Removes a CircleCI context secret",
		},
	)
}
