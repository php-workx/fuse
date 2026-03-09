package config

import (
	"os"
	"path/filepath"
)

// BaseDir returns the fuse root directory (~/.fuse/).
func BaseDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), ".fuse")
	}
	return filepath.Join(home, ".fuse")
}

// ConfigDir returns ~/.fuse/config/.
func ConfigDir() string {
	return filepath.Join(BaseDir(), "config")
}

// StateDir returns ~/.fuse/state/.
func StateDir() string {
	return filepath.Join(BaseDir(), "state")
}

// CacheDir returns ~/.fuse/cache/.
func CacheDir() string {
	return filepath.Join(BaseDir(), "cache")
}

// ConfigPath returns ~/.fuse/config/config.yaml.
func ConfigPath() string {
	return filepath.Join(ConfigDir(), "config.yaml")
}

// PolicyPath returns ~/.fuse/config/policy.yaml.
func PolicyPath() string {
	return filepath.Join(ConfigDir(), "policy.yaml")
}

// DBPath returns ~/.fuse/state/fuse.db.
func DBPath() string {
	return filepath.Join(StateDir(), "fuse.db")
}

// SecretPath returns ~/.fuse/state/secret.key.
func SecretPath() string {
	return filepath.Join(StateDir(), "secret.key")
}

// EnsureDirectories creates the fuse directory structure with correct permissions.
func EnsureDirectories() error {
	dirs := []struct {
		path string
		perm os.FileMode
	}{
		{ConfigDir(), 0755},
		{StateDir(), 0700},
		{CacheDir(), 0755},
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d.path, d.perm); err != nil {
			return err
		}
	}
	return nil
}
