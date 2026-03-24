package cli

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/php-workx/fuse/internal/adapters"
)

const defaultRunTimeout = 30 * time.Minute

var runTimeout time.Duration

var runCmd = &cobra.Command{
	Use:     "run",
	Short:   "Classify and execute a command with safety controls",
	Long:    "Classify a shell command, prompt for approval if needed, then execute with environment sanitization.",
	GroupID: groupRuntime,
	RunE: func(cmd *cobra.Command, args []string) error {
		command, err := parseSingleCommandArg(args)
		if err != nil {
			return withExitCode(err, 2)
		}
		cwd, _ := os.Getwd()
		exitCode, err := adapters.ExecuteCommand(command, cwd, runTimeout)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
		}
		os.Exit(exitCode)
		return nil
	},
}

func init() {
	runCmd.Flags().DurationVar(&runTimeout, "timeout", defaultRunTimeout, "Maximum execution time for the spawned command")
	rootCmd.AddCommand(runCmd)
}

func parseSingleCommandArg(args []string) (string, error) {
	if len(args) != 1 {
		return "", fmt.Errorf("run requires exactly one shell command string after --")
	}
	if args[0] == "" {
		return "", fmt.Errorf("run requires a non-empty shell command string")
	}
	return args[0], nil
}
