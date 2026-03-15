package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/runger/fuse/internal/config"
)

var dryrunCmd = &cobra.Command{
	Use:   "dryrun",
	Short: "Classify and log commands without blocking or prompting",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := os.MkdirAll(config.StateDir(), 0o700); err != nil {
			return fmt.Errorf("creating state directory: %w", err)
		}

		// Set dry-run marker, remove enabled marker.
		if err := os.WriteFile(config.DryRunMarkerPath(), []byte("1\n"), 0o600); err != nil {
			return fmt.Errorf("creating dry-run marker: %w", err)
		}
		_ = os.Remove(config.EnabledMarkerPath())

		fmt.Println("fuse is now in dry-run mode. Commands are classified and logged but never blocked.")
		fmt.Println("Run 'fuse events' to see decisions. Run 'fuse enable' for full enforcement.")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(dryrunCmd)
}
