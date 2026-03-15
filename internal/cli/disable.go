package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/runger/fuse/internal/config"
)

var disableCmd = &cobra.Command{
	Use:   "disable",
	Short: "Temporarily disable fuse (allow-all mode)",
	RunE: func(cmd *cobra.Command, args []string) error {
		markerPath := filepath.Join(config.StateDir(), "enabled")

		// Remove the enabled marker file.
		if err := os.Remove(markerPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("removing enabled marker: %w", err)
		}

		fmt.Println("fuse is now in dry-run mode. Commands are classified and logged but never blocked.")
		fmt.Println("Run 'fuse events' to see decisions. Run 'fuse enable' to re-enable enforcement.")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(disableCmd)
}
