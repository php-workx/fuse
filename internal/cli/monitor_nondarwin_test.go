//go:build !darwin

package cli

import (
	"testing"

	"github.com/php-workx/fuse/internal/config"
	"github.com/php-workx/fuse/internal/tui"
)

func TestNonDarwinMonitorHandoffDoesNotInitializeNotifications(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Notifications.Enabled = false

	options := effectiveNotificationOptions(cfg, false)
	if options.Enabled() {
		t.Fatal("notification options enabled = true, want false from config")
	}
	if !options.Sound() {
		t.Fatal("notification options sound = false, want configured true")
	}

	_ = tui.NewModel(nil, "disabled", nil, options, nil)
}
