package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/php-workx/fuse/internal/config"
)

var profileCmd = &cobra.Command{
	Use:   "profile",
	Short: "Show the current profile and effective settings",
	Long:  "Shows the active profile resolved from config.yaml along with the effective settings in use.",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.LoadConfig(config.ConfigPath())
		if err != nil {
			return err
		}
		printProfileSummary(cfg)
		return nil
	},
	GroupID: groupObserve,
}

var profileSetCmd = &cobra.Command{
	Use:   "set <name>",
	Short: "Set the active profile",
	Long:  "Sets the active profile in config.yaml while preserving other settings where possible.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return setProfile(args[0])
	},
}

func init() {
	rootCmd.AddCommand(profileCmd)
	profileCmd.AddCommand(profileSetCmd)
}

func printProfileSummary(cfg *config.Config) {
	fmt.Println("fuse profile")
	fmt.Println("============")
	fmt.Printf("Current profile: %s\n", cfg.Profile)
	fmt.Println()
	fmt.Println("Effective settings")
	fmt.Println("------------------")
	fmt.Printf("profile: %s\n", cfg.Profile)
	fmt.Printf("log_level: %s\n", cfg.LogLevel)
	fmt.Printf("max_event_log_rows: %d\n", cfg.MaxEventLogRows)
	fmt.Printf("caution_fallback: %s\n", cfg.CautionFallback)
	fmt.Printf("llm_judge.mode: %s\n", cfg.LLMJudge.Mode)
	fmt.Printf("llm_judge.provider: %s\n", cfg.LLMJudge.Provider)
	if cfg.LLMJudge.Model != "" {
		fmt.Printf("llm_judge.model: %s\n", cfg.LLMJudge.Model)
	}
	fmt.Printf("llm_judge.timeout: %s\n", cfg.LLMJudge.Timeout)
	fmt.Printf("llm_judge.upgrade_threshold: %.2f\n", cfg.LLMJudge.UpgradeThreshold)
	fmt.Printf("llm_judge.downgrade_threshold: %.2f\n", cfg.LLMJudge.DowngradeThreshold)
	fmt.Printf("llm_judge.trigger_decisions: [%s]\n", strings.Join(cfg.LLMJudge.TriggerDecisions, " "))
	fmt.Printf("llm_judge.max_calls_per_minute: %d\n", cfg.LLMJudge.MaxCallsPerMinute)
}

func setProfile(profile string) error {
	normalized, err := normalizeProfileSelection(profile)
	if err != nil {
		return err
	}

	cfgPath := config.ConfigPath()
	if _, loadErr := config.LoadConfig(cfgPath); loadErr != nil {
		return loadErr
	}
	if ensureErr := config.EnsureDirectories(); ensureErr != nil {
		return ensureErr
	}

	raw, err := loadProfileConfigMap(cfgPath)
	if err != nil {
		return err
	}
	raw["profile"] = normalized

	if err := writeProfileConfigMap(cfgPath, raw); err != nil {
		return err
	}

	fmt.Printf("profile set to %s\n", normalized)
	return nil
}

func normalizeProfileSelection(profile string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(profile)) {
	case config.ProfileRelaxed:
		return config.ProfileRelaxed, nil
	case config.ProfileBalanced:
		return config.ProfileBalanced, nil
	case config.ProfileStrict:
		return config.ProfileStrict, nil
	default:
		return "", fmt.Errorf("invalid profile %q (supported: relaxed, balanced, strict)", profile)
	}
}

func loadProfileConfigMap(path string) (map[string]interface{}, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]interface{}{}, nil
		}
		return nil, fmt.Errorf("read config: %w", err)
	}

	var cfg map[string]interface{}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	if cfg == nil {
		cfg = map[string]interface{}{}
	}
	return cfg, nil
}

func writeProfileConfigMap(path string, cfg map[string]interface{}) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".config-*.yaml")
	if err != nil {
		return fmt.Errorf("create temp config: %w", err)
	}
	tmpName := tmp.Name()
	defer func() {
		_ = os.Remove(tmpName)
	}()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write temp config: %w", err)
	}
	if err := tmp.Chmod(0o644); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("set temp config permissions: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp config: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("replace config: %w", err)
	}
	return nil
}
