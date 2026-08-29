package cli

import (
	"context"
	"testing"

	"github.com/php-workx/fuse/internal/attention"
	"github.com/php-workx/fuse/internal/config"
	"github.com/php-workx/fuse/internal/tui"
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


func TestMonitorAttentionDisabledSkipsServiceConstruction(t *testing.T) {
	previous := newAttentionService
	t.Cleanup(func() { newAttentionService = previous })
	calls := 0
	newAttentionService = func(context.Context, attention.Options) (attention.Service, error) {
		calls++
		return nil, nil
	}

	service, err := newMonitorAttentionService(tui.NewNotificationOptions(false, true))
	if err != nil {
		t.Fatalf("newMonitorAttentionService: %v", err)
	}
	if service != nil {
		t.Fatalf("disabled notifications returned service %T, want nil", service)
	}
	if calls != 0 {
		t.Fatalf("attention.New calls = %d, want 0 when notifications are disabled", calls)
	}
}