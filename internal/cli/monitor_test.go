package cli

import (
	"testing"

	"github.com/php-workx/fuse/internal/config"
)

func TestMonitorNoNotificationsFlagDisablesEffectiveOptions(t *testing.T) {
	cmd := newMonitorCommand()
	if err := cmd.ParseFlags([]string{"--no-notifications"}); err != nil {
		t.Fatalf("ParseFlags: %v", err)
	}
	disabled, err := cmd.Flags().GetBool("no-notifications")
	if err != nil {
		t.Fatalf("GetBool: %v", err)
	}

	options := effectiveNotificationOptions(config.DefaultConfig(), disabled)

	if options.Enabled() {
		t.Fatal("notification options enabled = true, want false after --no-notifications")
	}
}
