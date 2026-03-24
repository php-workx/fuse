package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/php-workx/fuse/internal/config"
)

var disableCmd = &cobra.Command{
	Use:     "disable",
	Short:   "Fully disable fuse (zero processing, instant pass-through)",
	GroupID: groupSetup,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Remove both markers — fully disabled.
		_ = os.Remove(config.EnabledMarkerPath())
		_ = os.Remove(config.DryRunMarkerPath())

		fmt.Println("fuse is now disabled. All commands pass through with zero processing.")
		fmt.Println("Run 'fuse dryrun' to classify without blocking, or 'fuse enable' for full enforcement.")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(disableCmd)
}
