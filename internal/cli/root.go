package cli

import (
	"errors"
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
		var exitErr interface{ ExitCode() int }
		if errors.As(err, &exitErr) {
			os.Exit(exitErr.ExitCode())
		}
		os.Exit(1)
	}
}

type cliExitError struct {
	err      error
	exitCode int
}

func (e *cliExitError) Error() string {
	return e.err.Error()
}

func (e *cliExitError) Unwrap() error {
	return e.err
}

func (e *cliExitError) ExitCode() int {
	return e.exitCode
}

func withExitCode(err error, exitCode int) error {
	if err == nil {
		return nil
	}
	return &cliExitError{err: err, exitCode: exitCode}
}
