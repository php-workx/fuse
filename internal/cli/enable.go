package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/runger/fuse/internal/config"
	"github.com/spf13/cobra"
)

var enableCmd = &cobra.Command{
	Use:   "enable",
	Short: "Re-enable fuse after it has been disabled",
	RunE: func(cmd *cobra.Command, args []string) error {
		markerPath := filepath.Join(config.StateDir(), "enabled")

		// Ensure the state directory exists.
		if err := os.MkdirAll(config.StateDir(), 0700); err != nil {
			return fmt.Errorf("creating state directory: %w", err)
		}

		// Create the enabled marker file.
		if err := os.WriteFile(markerPath, []byte("1\n"), 0600); err != nil {
			return fmt.Errorf("creating enabled marker: %w", err)
		}

		fmt.Println("fuse is now enabled.")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(enableCmd)
}
