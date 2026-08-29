package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/php-workx/fuse/internal/config"
)

func TestProfileAwareConfigScaffoldIncludesNotificationDefaults(t *testing.T) {
	contents := profileAwareConfigScaffold(config.ProfileRelaxed)
	for _, want := range []string{
		"notifications:",
		"  enabled: true",
		"  sound: true",
	} {
		if !strings.Contains(contents, want) {
			t.Fatalf("config scaffold missing %q:\n%s", want, contents)
		}
	}

	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("write scaffold: %v", err)
	}
	cfg, err := config.LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig scaffold: %v", err)
	}
	if !cfg.Notifications.Enabled || !cfg.Notifications.Sound {
		t.Fatalf("scaffold notifications = %#v, want enabled sound defaults", cfg.Notifications)
	}
}
