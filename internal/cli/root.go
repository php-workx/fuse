package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "fuse",
	Short: "Command-safety runtime for AI coding agents",
	Long:  "fuse classifies shell commands from AI coding agents as SAFE, CAUTION, APPROVAL, or BLOCKED.",
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
