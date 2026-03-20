package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	tea "charm.land/bubbletea/v2"

	"github.com/runger/fuse/internal/config"
	"github.com/runger/fuse/internal/db"
	"github.com/runger/fuse/internal/tui"
)

var monitorCmd = &cobra.Command{
	Use:     "monitor",
	Aliases: []string{"tui"},
	Short:   "Live dashboard for monitoring fuse activity",
	GroupID: groupObserve,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runMonitor()
	},
}

func init() {
	rootCmd.AddCommand(monitorCmd)
}

func runMonitor() error {
	if !isTerminal(int(os.Stdin.Fd())) || !isTerminal(int(os.Stdout.Fd())) {
		return fmt.Errorf("fuse monitor requires an interactive terminal")
	}

	database, exists, err := openEventsDB()
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	if !exists {
		return fmt.Errorf("no fuse database found (run some commands first)")
	}
	defer func() { _ = database.Close() }()

	mode := config.Mode()
	var modeStr string
	switch mode {
	case config.ModeEnabled:
		modeStr = "enabled"
	case config.ModeDryRun:
		modeStr = "dryrun"
	default:
		modeStr = "disabled"
	}

	// Read HMAC secret for approval creation from the TUI.
	secret, secretErr := db.EnsureSecret(config.SecretPath())
	if secretErr != nil {
		return fmt.Errorf("read secret: %w", secretErr)
	}

	m := tui.NewModel(database, modeStr, secret)
	p := tea.NewProgram(m)
	_, err = p.Run()
	return err
}
