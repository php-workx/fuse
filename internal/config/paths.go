package config

import (
	"os"
	"path/filepath"
)

// BaseDir returns the fuse root directory (~/.fuse/).
// If FUSE_HOME is set, it is used instead (useful for testing).
func BaseDir() string {
	if env := os.Getenv("FUSE_HOME"); env != "" {
		return env
	}
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

// FuseMode represents the operational mode of fuse.
type FuseMode int

const (
	// ModeDisabled means fuse is fully off — zero processing, instant pass-through.
	ModeDisabled FuseMode = iota
	// ModeDryRun means fuse classifies and logs but never blocks or prompts.
	ModeDryRun
	// ModeEnabled means fuse enforces classification with blocking and approval prompts.
	ModeEnabled
)

// EnabledMarkerPath returns the path to the enabled marker file.
func EnabledMarkerPath() string {
	return filepath.Join(StateDir(), "enabled")
}

// DryRunMarkerPath returns the path to the dry-run marker file.
func DryRunMarkerPath() string {
	return filepath.Join(StateDir(), "dryrun")
}

// Mode returns the current operational mode of fuse.
func Mode() FuseMode {
	if _, err := os.Stat(EnabledMarkerPath()); err == nil {
		return ModeEnabled
	}
	if _, err := os.Stat(DryRunMarkerPath()); err == nil {
		return ModeDryRun
	}
	return ModeDisabled
}

// IsDisabled returns true when fuse is fully disabled (neither enabled nor dry-run).
func IsDisabled() bool {
	return Mode() == ModeDisabled
}

// EnsureDirectories creates the fuse directory structure with correct permissions.
func EnsureDirectories() error {
	dirs := []struct {
		path string
		perm os.FileMode
	}{
		{ConfigDir(), 0o755},
		{StateDir(), 0o700},
		{CacheDir(), 0o755},
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d.path, d.perm); err != nil {
			return err
		}
	}
	return nil
}
