package cli

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/runger/fuse/internal/adapters"
)

var hookCmd = &cobra.Command{
	Use:     "hook",
	Short:   "Hook commands for AI agent integration",
	GroupID: groupRuntime,
}

var hookEvaluateCmd = &cobra.Command{
	Use:   "evaluate",
	Short: "Evaluate a tool call from stdin (Claude Code hook protocol)",
	Run: func(cmd *cobra.Command, args []string) {
		exitCode := adapters.RunHook(os.Stdin, os.Stderr)
		os.Exit(exitCode)
	},
}

func init() {
	hookCmd.AddCommand(hookEvaluateCmd)
	rootCmd.AddCommand(hookCmd)
}
