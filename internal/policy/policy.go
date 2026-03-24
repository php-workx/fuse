package policy

import (
	"crypto/sha256"
	"fmt"
	"log/slog"
	"os"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/php-workx/fuse/internal/core"
)

// PolicyConfig represents user-defined policy loaded from a YAML file.
type PolicyConfig struct {
	Version          string            `yaml:"version"`
	Rules            []PolicyRule      `yaml:"rules"`
	DisabledBuiltins []string          `yaml:"disabled_builtins"`
	DisabledTags     []string          `yaml:"disabled_tags"`
	TagOverrides     map[string]string `yaml:"tag_overrides"`
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

	if _, err := ParseTagOverrides(&cfg); err != nil {
		return nil, err
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

// ParseTagOverrides converts string override values to FuseMode.
// Returns an error if any value is invalid.
func ParseTagOverrides(policy *PolicyConfig) (map[string]TagOverrideMode, error) {
	if policy == nil || len(policy.TagOverrides) == 0 {
		return nil, nil
	}
	m := make(map[string]TagOverrideMode, len(policy.TagOverrides))
	for tag, mode := range policy.TagOverrides {
		switch mode {
		case "enabled":
			m[tag] = TagOverrideEnabled
		case "dryrun":
			m[tag] = TagOverrideDryRun
		case "disabled":
			m[tag] = TagOverrideDisabled
		default:
			return nil, fmt.Errorf("invalid tag_override value %q for tag %q (must be enabled/dryrun/disabled)", mode, tag)
		}
	}
	return m, nil
}

// DisabledTagSet returns a map for quick lookup of disabled tags.
func DisabledTagSet(policy *PolicyConfig) map[string]bool {
	if policy == nil || len(policy.DisabledTags) == 0 {
		return nil
	}
	m := make(map[string]bool, len(policy.DisabledTags))
	for _, tag := range policy.DisabledTags {
		m[tag] = true
	}
	return m
}

// defaultLKGMaxAge is the default maximum age for LKG policy files (7 days).
const defaultLKGMaxAge = 7 * 24 * time.Hour

// lkgSuffix is appended to the policy path to form the LKG path.
const lkgSuffix = ".lkg"

// LoadPolicyWithLKG tries to load policy.yaml. On success, saves as LKG.
// On failure, falls back to last known good policy if available and recent.
// maxAge controls how old the LKG can be (0 = use default 7 days).
func LoadPolicyWithLKG(path string, maxAge time.Duration) (*PolicyConfig, error) {
	if maxAge <= 0 {
		maxAge = defaultLKGMaxAge
	}
	lkgPath := path + lkgSuffix

	// Try loading the primary policy.
	data, readErr := os.ReadFile(path)
	if readErr != nil {
		if os.IsNotExist(readErr) {
			// File doesn't exist — this is normal (no user policy configured).
			return nil, readErr
		}
		// File exists but unreadable — try LKG.
		slog.Warn("policy file unreadable, attempting LKG fallback",
			"path", path, "error", readErr)
		lkgCfg, lkgErr := loadLKG(lkgPath, maxAge)
		if lkgErr != nil {
			return nil, fmt.Errorf("policy unreadable (%w) and no valid LKG fallback (%w)", readErr, lkgErr)
		}
		slog.Warn("[fuse] WARNING: using fallback policy (policy file unreadable)",
			"lkg_path", lkgPath)
		return lkgCfg, nil
	}

	cfg, err := loadPolicyFromBytes(data)
	if err == nil {
		// Success — save the already-read bytes as LKG (no TOCTOU).
		if saveErr := saveLKGBytes(data, path, lkgPath); saveErr != nil {
			slog.Warn("failed to refresh policy LKG", "path", lkgPath, "error", saveErr)
		}
		return cfg, nil
	}

	// Primary failed — try LKG fallback.
	slog.Warn("policy.yaml load failed, attempting LKG fallback",
		"path", path, "error", err)

	lkgCfg, lkgErr := loadLKG(lkgPath, maxAge)
	if lkgErr != nil {
		return nil, fmt.Errorf("policy load failed (%w) and no valid LKG fallback (%w)", err, lkgErr)
	}

	slog.Warn("[fuse] WARNING: using fallback policy (policy.yaml has errors)",
		"lkg_path", lkgPath)
	return lkgCfg, nil
}

// loadPolicyFromBytes parses and validates a policy from already-read bytes.
func loadPolicyFromBytes(data []byte) (*PolicyConfig, error) {
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
		if _, ok := actionToDecision[r.Action]; !ok {
			return nil, fmt.Errorf("invalid action %q in rule with pattern %q", r.Action, r.Pattern)
		}
	}

	if _, err := ParseTagOverrides(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// saveLKGBytes writes already-read policy bytes to the LKG path with a timestamp header.
// Uses pre-read bytes to avoid TOCTOU race with the filesystem.
// Writes atomically via temp file + rename to prevent partial LKG on crash.
func saveLKGBytes(data []byte, sourcePath, lkgPath string) error {
	hash := fmt.Sprintf("%x", sha256.Sum256(data))
	header := fmt.Sprintf("# LKG saved: %s\n# Original: %s (sha256: %s)\n",
		time.Now().UTC().Format(time.RFC3339), sourcePath, hash)

	lkgData := header + string(data)
	tmpPath := lkgPath + ".tmp"
	if err := os.WriteFile(tmpPath, []byte(lkgData), 0o600); err != nil {
		return err
	}
	return os.Rename(tmpPath, lkgPath)
}

// loadLKG loads and validates an LKG policy file.
func loadLKG(lkgPath string, maxAge time.Duration) (*PolicyConfig, error) {
	data, err := os.ReadFile(lkgPath)
	if err != nil {
		return nil, fmt.Errorf("LKG file not found: %w", err)
	}

	// Check freshness from file modification time.
	info, err := os.Stat(lkgPath)
	if err != nil {
		return nil, fmt.Errorf("LKG stat failed: %w", err)
	}
	if time.Since(info.ModTime()) > maxAge {
		return nil, fmt.Errorf("LKG too old (modified %s, max age %s)",
			info.ModTime().Format(time.RFC3339), maxAge)
	}

	// Strip comment lines before parsing.
	lines := strings.Split(string(data), "\n")
	var yamlLines []string
	for _, line := range lines {
		if strings.HasPrefix(line, "# LKG saved:") || strings.HasPrefix(line, "# Original:") {
			continue
		}
		yamlLines = append(yamlLines, line)
	}

	var cfg PolicyConfig
	if err := yaml.Unmarshal([]byte(strings.Join(yamlLines, "\n")), &cfg); err != nil {
		return nil, fmt.Errorf("LKG parse failed: %w", err)
	}

	// Compile rules.
	for i := range cfg.Rules {
		r := &cfg.Rules[i]
		compiled, compErr := regexp.Compile(r.Pattern)
		if compErr != nil {
			return nil, fmt.Errorf("LKG rule pattern %q: %w", r.Pattern, compErr)
		}
		r.compiled = compiled
		if _, ok := actionToDecision[r.Action]; !ok {
			return nil, fmt.Errorf("LKG invalid action %q", r.Action)
		}
	}

	if _, tagErr := ParseTagOverrides(&cfg); tagErr != nil {
		return nil, tagErr
	}

	return &cfg, nil
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
