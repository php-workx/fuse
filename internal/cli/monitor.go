package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	tea "charm.land/bubbletea/v2"

	"github.com/php-workx/fuse/internal/config"
	"github.com/php-workx/fuse/internal/db"
	"github.com/php-workx/fuse/internal/tui"
)

var monitorCmd = newMonitorCommand()

func newMonitorCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "monitor",
		Aliases: []string{"tui"},
		Short:   "Live dashboard for monitoring fuse activity",
		GroupID: groupObserve,
		RunE: func(cmd *cobra.Command, args []string) error {
			noNotifications, err := cmd.Flags().GetBool("no-notifications")
			if err != nil {
				return err
			}
			return runMonitor(noNotifications)
		},
	}
	cmd.Flags().Bool("no-notifications", false, "disable approval notifications for this run")
	return cmd
}

func init() {
	rootCmd.AddCommand(monitorCmd)
}

func runMonitor(noNotifications bool) error {
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

	cfg, err := config.LoadConfig(config.ConfigPath())
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	notificationOptions := effectiveNotificationOptions(cfg, noNotifications)

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

	m := tui.NewModel(database, modeStr, secret, notificationOptions)
	p := tea.NewProgram(m)
	_, err = p.Run()
	return err
}

func effectiveNotificationOptions(cfg *config.Config, noNotifications bool) tui.NotificationOptions {
	return tui.NewNotificationOptions(!noNotifications && cfg.Notifications.Enabled, cfg.Notifications.Sound)
}
