package config

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	ProfileRelaxed  = "relaxed"
	ProfileBalanced = "balanced"
	ProfileStrict   = "strict"
	ProfileCustom   = "custom"
)

// Config represents the fuse configuration from config.yaml.
type Config struct {
	Profile         string               `yaml:"profile"`
	LogLevel        string               `yaml:"log_level"`
	MaxEventLogRows int                  `yaml:"max_event_log_rows"`
	MCPProxies      []MCPProxy           `yaml:"mcp_proxies"`
	LLMJudge        LLMJudgeConfig       `yaml:"llm_judge"`
	URLTrustPolicy  URLTrustPolicyConfig `yaml:"url_trust_policy"`
	PolicyLKG       PolicyLKGConfig      `yaml:"policy_lkg"`
	CautionFallback string               `yaml:"caution_fallback"`
}

// URLTrustPolicyConfig controls URL inspection behavior.
type URLTrustPolicyConfig struct {
	TrustedDomains []string `yaml:"trusted_domains"` // empty = no domain trust enforcement
	BlockSchemes   []string `yaml:"block_schemes"`   // additional blocked schemes
}

// PolicyLKGConfig controls last-known-good policy fallback.
type PolicyLKGConfig struct {
	Enabled    bool `yaml:"enabled"`      // default true
	MaxAgeDays int  `yaml:"max_age_days"` // default 7
}

// LLMJudgeConfig controls the optional LLM judge that provides a second opinion
// on CAUTION and APPROVAL classifications.
type LLMJudgeConfig struct {
	Mode               string   `yaml:"mode"`                 // "off", "shadow", "active"
	Provider           string   `yaml:"provider"`             // "claude", "codex", "auto"
	Model              string   `yaml:"model"`                // provider-specific model name
	Timeout            string   `yaml:"timeout"`              // duration string, default "10s"
	UpgradeThreshold   float64  `yaml:"upgrade_threshold"`    // min confidence for upgrades, default 0.7
	DowngradeThreshold float64  `yaml:"downgrade_threshold"`  // min confidence for downgrades, default 0.95
	TriggerDecisions   []string `yaml:"trigger_decisions"`    // default ["approval", "caution"]
	MaxCallsPerMinute  int      `yaml:"max_calls_per_minute"` // rate limit, default 30
}

// MCPProxy defines an MCP proxy configuration.
type MCPProxy struct {
	Name    string            `yaml:"name"`
	Command string            `yaml:"command"`
	Args    []string          `yaml:"args"`
	Env     map[string]string `yaml:"env"`
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() *Config {
	return ProfileDefaults(ProfileRelaxed)
}

// ProfileDefaults returns a Config seeded with defaults for the given profile.
func ProfileDefaults(profile string) *Config {
	profile = normalizeProfile(profile)
	cfg := &Config{
		Profile:         profile,
		LogLevel:        "warn",
		MaxEventLogRows: 100000,
		CautionFallback: "log",
		LLMJudge: LLMJudgeConfig{
			Mode:               "off",
			Provider:           "auto",
			Timeout:            "10s",
			UpgradeThreshold:   0.7,
			DowngradeThreshold: 0.95,
			TriggerDecisions:   []string{},
			MaxCallsPerMinute:  30,
		},
		PolicyLKG: PolicyLKGConfig{
			Enabled:    true,
			MaxAgeDays: 7,
		},
	}

	switch profile {
	case ProfileBalanced:
		cfg.LLMJudge.Mode = "active"
		cfg.LLMJudge.DowngradeThreshold = 0.9
		cfg.LLMJudge.TriggerDecisions = []string{"caution", "approval"}
	case ProfileStrict:
		cfg.LLMJudge.Mode = "active"
		cfg.LLMJudge.TriggerDecisions = []string{"caution"}
	case ProfileCustom:
		cfg.Profile = ProfileCustom
	}

	return cfg
}

func normalizeProfile(profile string) string {
	switch strings.ToLower(strings.TrimSpace(profile)) {
	case "", ProfileRelaxed:
		return ProfileRelaxed
	case ProfileBalanced:
		return ProfileBalanced
	case ProfileStrict:
		return ProfileStrict
	case ProfileCustom:
		return ProfileCustom
	default:
		return ProfileCustom
	}
}

// LoadConfig reads and parses config.yaml from the given path.
// Returns DefaultConfig if the file does not exist.
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return DefaultConfig(), nil
		}
		return nil, err
	}
	var overlay Config
	err = yaml.Unmarshal(data, &overlay)
	if err != nil {
		return nil, err
	}

	var raw map[string]interface{}
	err = yaml.Unmarshal(data, &raw)
	if err != nil {
		return nil, err
	}

	profile, err := resolveProfile(raw, &overlay)
	if err != nil {
		return nil, err
	}
	validationErr := validateCautionFallback(raw)
	if validationErr != nil {
		return nil, validationErr
	}
	cfg := ProfileDefaults(profile)
	err = yaml.Unmarshal(data, cfg)
	if err != nil {
		return nil, err
	}
	cfg.Profile = profile
	return cfg, nil
}

func resolveProfile(raw map[string]interface{}, overlay *Config) (string, error) {
	if raw != nil {
		if value, ok := raw["profile"]; ok {
			profile, ok := value.(string)
			if !ok {
				return "", fmt.Errorf("invalid profile: must be one of %s, %s, %s, %s", ProfileRelaxed, ProfileBalanced, ProfileStrict, ProfileCustom)
			}
			return parseProfile(profile)
		}
	}

	if overlay != nil && strings.EqualFold(strings.TrimSpace(overlay.LLMJudge.Mode), "active") {
		return ProfileBalanced, nil
	}

	return ProfileRelaxed, nil
}

func parseProfile(profile string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(profile)) {
	case ProfileRelaxed:
		return ProfileRelaxed, nil
	case ProfileBalanced:
		return ProfileBalanced, nil
	case ProfileStrict:
		return ProfileStrict, nil
	case ProfileCustom:
		return ProfileCustom, nil
	default:
		return "", fmt.Errorf("invalid profile %q: must be one of %s, %s, %s, %s", profile, ProfileRelaxed, ProfileBalanced, ProfileStrict, ProfileCustom)
	}
}

func validateCautionFallback(raw map[string]interface{}) error {
	if raw == nil {
		return nil
	}
	value, ok := raw["caution_fallback"]
	if !ok {
		return nil
	}
	fallback, ok := value.(string)
	if !ok {
		return fmt.Errorf("invalid caution_fallback: must be %q or %q", "log", "approve")
	}
	switch strings.ToLower(strings.TrimSpace(fallback)) {
	case "log", "approve":
		return nil
	default:
		return fmt.Errorf("invalid caution_fallback %q: must be %q or %q", fallback, "log", "approve")
	}
}
