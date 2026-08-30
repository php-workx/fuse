//go:build darwin

package cli

import (
	"fmt"
	"os"
	"path/filepath"
)

func uninstallNotifier() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	bundle := filepath.Join(home, ".fuse", "libexec", "Fuse Notifier.app")
	if err := os.RemoveAll(bundle); err != nil {
		return fmt.Errorf("removing %s: %w", bundle, err)
	}
	return nil
}
