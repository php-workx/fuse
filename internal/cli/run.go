package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/runger/fuse/internal/adapters"
	"github.com/spf13/cobra"
)

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Classify and execute a command with safety controls",
	Long:  "Classify a shell command, prompt for approval if needed, then execute with environment sanitization.",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		command := strings.Join(args, " ")
		cwd, _ := os.Getwd()
		exitCode, err := adapters.ExecuteCommand(command, cwd)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
		}
		os.Exit(exitCode)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(runCmd)
}
