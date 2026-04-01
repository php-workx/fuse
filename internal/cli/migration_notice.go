package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/php-workx/fuse/internal/config"
)

const profileMigrationNoticeText = "Fuse now supports profiles. Run `fuse profile` to see your settings."

func maybePrintProfileMigrationNotice(cmd *cobra.Command) error {
	if shouldSkipProfileMigrationNotice(cmd) {
		return nil
	}
	if !needsProfileMigrationNotice() {
		return nil
	}
	if err := writeMigrationNotice(cmd.ErrOrStderr()); err != nil {
		return err
	}
	return markProfileMigrationNoticeSeen()
}

func shouldSkipProfileMigrationNotice(cmd *cobra.Command) bool {
	if cmd == nil {
		return false
	}
	if helpFlag := cmd.Flags().Lookup("help"); helpFlag != nil && helpFlag.Changed {
		return true
	}
	switch cmd.Name() {
	case "completion", "help", "version":
		return true
	}
	if parent := cmd.Parent(); parent != nil && parent.Name() == "completion" {
		return true
	}
	return false
}

func needsProfileMigrationNotice() bool {
	if _, err := os.Stat(profileMigrationNoticeMarkerPath()); err == nil {
		return false
	}
	raw, err := loadRawConfig(config.ConfigPath())
	if err != nil || raw == nil {
		return false
	}
	_, hasProfile := raw["profile"]
	return !hasProfile
}

func markProfileMigrationNoticeSeen() error {
	if err := os.MkdirAll(config.StateDir(), 0o700); err != nil {
		return fmt.Errorf("create state directory: %w", err)
	}
	if err := os.WriteFile(profileMigrationNoticeMarkerPath(), []byte("1\n"), 0o600); err != nil {
		return fmt.Errorf("write migration marker: %w", err)
	}
	return nil
}

func profileMigrationNoticeMarkerPath() string {
	return filepath.Join(config.StateDir(), "profile-migration-notice-seen")
}

func loadRawConfig(path string) (map[string]interface{}, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var raw map[string]interface{}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	return raw, nil
}

func writeMigrationNotice(w io.Writer) error {
	_, err := fmt.Fprintln(w, profileMigrationNoticeText)
	return err
}
