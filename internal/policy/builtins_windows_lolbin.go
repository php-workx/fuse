package policy

import (
	"regexp"
	"strings"

	"github.com/php-workx/fuse/internal/core"
)

func init() {
	rules := []BuiltinRule{
		{
			ID:      "builtin:windows:certutil-decode",
			Pattern: regexp.MustCompile(`(?i)\bcertutil\b.*\s-(decode|urlcache)\b`),
			Action:  core.DecisionApproval,
			Reason:  "certutil used for decode or remote retrieval",
		},
		{
			ID:      "builtin:windows:bitsadmin-transfer",
			Pattern: regexp.MustCompile(`(?i)\bbitsadmin\b.*\s/transfer\b`),
			Action:  core.DecisionApproval,
			Reason:  "bitsadmin transfer downloads or moves content",
		},
		{
			ID:      "builtin:windows:mshta-remote",
			Pattern: regexp.MustCompile(`(?i)\bmshta\b.*(https?://|vbscript:)`),
			Action:  core.DecisionApproval,
			Reason:  "mshta executing remote or script content",
		},
		{
			ID:      "builtin:windows:regsvr32-remote",
			Pattern: regexp.MustCompile(`(?i)\bregsvr32\b.*\s/i:https?://`),
			Action:  core.DecisionApproval,
			Reason:  "regsvr32 remote scriptlet execution",
		},
		{
			ID:      "builtin:windows:rundll32-javascript",
			Pattern: regexp.MustCompile(`(?i)\brundll32\b.*javascript:`),
			Action:  core.DecisionApproval,
			Reason:  "rundll32 javascript execution",
		},
		{
			ID:      "builtin:windows:cmstp-inf",
			Pattern: regexp.MustCompile(`(?i)\bcmstp\b.*\s/s\b.*\.inf\b`),
			Action:  core.DecisionApproval,
			Reason:  "cmstp installing INF file silently",
		},
		{
			ID:      "builtin:windows:msiexec-remote",
			Pattern: regexp.MustCompile(`(?i)\bmsiexec\b.*\s/i\b.*https?://`),
			Action:  core.DecisionApproval,
			Reason:  "msiexec installing from remote URL",
		},
		{
			ID:      "builtin:windows:wscript-engine",
			Pattern: regexp.MustCompile(`(?i)\b(wscript|cscript)\b.*//e:`),
			Action:  core.DecisionApproval,
			Reason:  "wscript/cscript with explicit script engine",
		},
		{
			ID:      "builtin:windows:forfiles-command",
			Pattern: regexp.MustCompile(`(?i)\bforfiles\b.*\s/c\b`),
			Action:  core.DecisionApproval,
			Reason:  "forfiles indirect command execution",
		},
		{
			ID:      "builtin:windows:certutil-general",
			Pattern: regexp.MustCompile(`(?i)\bcertutil\b`),
			Action:  core.DecisionCaution,
			Reason:  "general certutil usage",
			Predicate: func(cmd string) bool {
				for _, safe := range []string{"-hashfile", "-verify", "-dump", "-store", "-viewstore"} {
					if strings.Contains(strings.ToLower(cmd), safe) {
						return false
					}
				}
				return true
			},
		},
		{
			ID:      "builtin:windows:wscript-general",
			Pattern: regexp.MustCompile(`(?i)\b(wscript|cscript)\b`),
			Action:  core.DecisionCaution,
			Reason:  "general Windows script host execution",
			Predicate: func(cmd string) bool {
				return !strings.Contains(strings.ToLower(cmd), "//e:")
			},
		},
	}

	BuiltinRules = append(rules, BuiltinRules...)
}
