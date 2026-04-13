package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/php-workx/fuse/internal/config"
	"github.com/php-workx/fuse/internal/db"
)

func ensureFuseConfigScaffold(profile string) error {
	path := config.ConfigPath()

	info, err := os.Stat(path)
	if err == nil {
		if info.IsDir() {
			return fmt.Errorf("%s exists and is a directory", path)
		}
		return nil
	}
	if !os.IsNotExist(err) {
		return fmt.Errorf("checking %s: %w", path, err)
	}

	if err := config.EnsureDirectories(); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}

	if err := os.WriteFile(path, []byte(profileAwareConfigScaffold(profile)), 0o600); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	return nil
}

// ensureFuseStateDB materializes an empty state database so that commands
// like `fuse monitor` don't error on a freshly installed system. OpenDB
// creates the file, runs migrations, and is a no-op if the DB already exists.
func ensureFuseStateDB() error {
	database, err := db.OpenDB(config.DBPath())
	if err != nil {
		return fmt.Errorf("initialize state database: %w", err)
	}
	return database.Close()
}

func profileAwareConfigScaffold(profile string) string {
	if profile == "" {
		profile = config.ProfileRelaxed
	}
	profile = config.ProfileDefaults(profile).Profile

	return strings.TrimSpace(fmt.Sprintf(`
# Fuse configuration
# Profile sets defaults. Override individual settings below.
# See: https://github.com/php-workx/fuse/docs/profiles.md
profile: %s

# LLM Judge settings (set by profile, customize as needed)
# llm_judge:
#   mode: active
#   provider: auto
#   timeout: 10s

# Caution fallback when judge is unavailable
# log:     auto-approve and log (default)
# approve: ask for confirmation
# caution_fallback: log
`, profile)) + "\n"
}
