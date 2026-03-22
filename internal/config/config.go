package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

// Config represents the fuse configuration from config.yaml.
type Config struct {
	LogLevel        string         `yaml:"log_level"`
	MaxEventLogRows int            `yaml:"max_event_log_rows"`
	MCPProxies      []MCPProxy     `yaml:"mcp_proxies"`
	LLMJudge        LLMJudgeConfig `yaml:"llm_judge"`
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
	return &Config{
		LogLevel:        "warn",
		MaxEventLogRows: 10000,
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
	cfg := DefaultConfig()
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}
