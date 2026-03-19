package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/runger/fuse/internal/config"
)

var enableCmd = &cobra.Command{
	Use:     "enable",
	Short:   "Enable fuse enforcement (classify, block, prompt)",
	GroupID: groupSetup,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := os.MkdirAll(config.StateDir(), 0o700); err != nil {
			return fmt.Errorf("creating state directory: %w", err)
		}

		// Set enabled marker, remove dry-run marker.
		if err := os.WriteFile(config.EnabledMarkerPath(), []byte("1\n"), 0o600); err != nil {
			return fmt.Errorf("creating enabled marker: %w", err)
		}
		_ = os.Remove(config.DryRunMarkerPath())

		fmt.Println("fuse is now enabled. Commands will be classified, blocked, and may require approval.")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(enableCmd)
}
