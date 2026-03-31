package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

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

func init() {
	rootCmd.AddCommand(profileCmd)
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
