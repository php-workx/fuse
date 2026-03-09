package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// Version information, injected at build time via ldflags.
var (
	Version   = "dev"
	GitCommit = "unknown"
	BuildDate = "unknown"
)

var rootCmd = &cobra.Command{
	Use:   "fuse",
	Short: "Command-safety runtime for AI coding agents",
	Long:  "fuse classifies shell commands from AI coding agents as SAFE, CAUTION, APPROVAL, or BLOCKED.",
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print fuse version information",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("fuse %s (%s) built %s\n", Version, GitCommit, BuildDate)
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
