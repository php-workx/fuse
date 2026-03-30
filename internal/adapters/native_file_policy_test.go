package adapters

import (
	"path/filepath"
	"testing"
)

// --- Tests for standalone path comparison helpers ---

func TestPathContains(t *testing.T) {
	tests := []struct {
		name   string
		path   string
		substr string
		ci     bool
		want   bool
	}{
		{"exact match case-sensitive", "/home/user/.claude/settings.json", ".claude/", false, true},
		{"no match case-sensitive", "/home/user/.Claude/settings.json", ".claude/", false, false},
		{"match case-insensitive", "/home/user/.Claude/settings.json", ".claude/", true, true},
		{"upper case-insensitive", "/HOME/USER/.CLAUDE/SETTINGS.JSON", ".claude/", true, true},
		{"no match either mode", "/home/user/foo.txt", ".claude/", false, false},
		{"no match ci either", "/home/user/foo.txt", ".claude/", true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := pathContains(tt.path, tt.substr, tt.ci)
			if got != tt.want {
				t.Errorf("pathContains(%q, %q, %v) = %v, want %v", tt.path, tt.substr, tt.ci, got, tt.want)
			}
		})
	}
}

func TestPathHasSuffix(t *testing.T) {
	tests := []struct {
		name   string
		path   string
		suffix string
		ci     bool
		want   bool
	}{
		{"exact suffix case-sensitive", "/home/user/.claude/settings.json", "settings.json", false, true},
		{"no match case-sensitive", "/home/user/.claude/Settings.json", "settings.json", false, false},
		{"match case-insensitive", "/home/user/.claude/Settings.JSON", "settings.json", true, true},
		{"no suffix match", "/home/user/foo.txt", "settings.json", false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := pathHasSuffix(tt.path, tt.suffix, tt.ci)
			if got != tt.want {
				t.Errorf("pathHasSuffix(%q, %q, %v) = %v, want %v", tt.path, tt.suffix, tt.ci, got, tt.want)
			}
		})
	}
}

func TestPathHasPrefix(t *testing.T) {
	tests := []struct {
		name   string
		path   string
		prefix string
		ci     bool
		want   bool
	}{
		{"exact prefix case-sensitive", "/home/user/.claude/", "/home/user", false, true},
		{"no match case-sensitive", "/Home/User/.claude/", "/home/user", false, false},
		{"match case-insensitive", "/Home/User/.claude/", "/home/user", true, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := pathHasPrefix(tt.path, tt.prefix, tt.ci)
			if got != tt.want {
				t.Errorf("pathHasPrefix(%q, %q, %v) = %v, want %v", tt.path, tt.prefix, tt.ci, got, tt.want)
			}
		})
	}
}

func TestPathEquals(t *testing.T) {
	tests := []struct {
		name string
		a    string
		b    string
		ci   bool
		want bool
	}{
		{"equal case-sensitive", "/home/user/.env", "/home/user/.env", false, true},
		{"not equal case-sensitive", "/home/user/.Env", "/home/user/.env", false, false},
		{"equal case-insensitive", "/home/user/.Env", "/home/user/.env", true, true},
		{"upper case-insensitive", "/HOME/USER/.ENV", "/home/user/.env", true, true},
		{"not equal either", "/home/user/foo", "/home/user/bar", true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := pathEquals(tt.a, tt.b, tt.ci)
			if got != tt.want {
				t.Errorf("pathEquals(%q, %q, %v) = %v, want %v", tt.a, tt.b, tt.ci, got, tt.want)
			}
		})
	}
}

// --- Tests for filePathInfo methods with caseInsensitive flag ---

// newTestFilePathInfo creates a filePathInfo directly for testing, bypassing
// nativeFilePathInfo which relies on the real filesystem and os.UserHomeDir.
func newTestFilePathInfo(raw, abs, homeDir, cwd string, ci bool) filePathInfo {
	cleanRaw := filepath.Clean(raw)
	return filePathInfo{
		raw:             raw,
		cleanRaw:        cleanRaw,
		abs:             filepath.Clean(abs),
		slashRaw:        filepath.ToSlash(cleanRaw),
		slashAbs:        filepath.ToSlash(filepath.Clean(abs)),
		homeDir:         homeDir,
		cwd:             cwd,
		caseInsensitive: ci,
	}
}

func TestFilePathInfo_MatchesRelative_CaseInsensitive(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
		ci   bool
		exp  bool
	}{
		{"exact match cs", ".git/hooks/pre-commit", ".git/hooks", false, true},
		{"no match cs", ".Git/Hooks/pre-commit", ".git/hooks", false, false},
		{"match ci", ".Git/Hooks/pre-commit", ".git/hooks", true, true},
		{"exact match ci", ".git/hooks", ".git/hooks", true, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := newTestFilePathInfo(tt.raw, "/tmp/"+tt.raw, "/home/user", "/tmp", tt.ci)
			got := info.matchesRelative(tt.want)
			if got != tt.exp {
				t.Errorf("matchesRelative(%q) with ci=%v = %v, want %v", tt.want, tt.ci, got, tt.exp)
			}
		})
	}
}

func TestFilePathInfo_MatchesAbsolute_CaseInsensitive(t *testing.T) {
	tests := []struct {
		name string
		abs  string
		want string
		ci   bool
		exp  bool
	}{
		{"exact match cs", "/home/user/.claude/settings.json", "/home/user/.claude/settings.json", false, true},
		{"no match cs", "/home/user/.Claude/Settings.json", "/home/user/.claude/settings.json", false, false},
		{"match ci", "/home/user/.Claude/Settings.json", "/home/user/.claude/settings.json", true, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := newTestFilePathInfo("raw", tt.abs, "/home/user", "/tmp", tt.ci)
			got := info.matchesAbsolute(tt.want)
			if got != tt.exp {
				t.Errorf("matchesAbsolute(%q) with ci=%v = %v, want %v", tt.want, tt.ci, got, tt.exp)
			}
		})
	}
}

func TestFilePathInfo_ContainsSegment_CaseInsensitive(t *testing.T) {
	tests := []struct {
		name    string
		abs     string
		segment string
		ci      bool
		exp     bool
	}{
		{"found cs", "/home/user/secrets/key.pem", "secrets", false, true},
		{"not found cs", "/home/user/Secrets/key.pem", "secrets", false, false},
		{"found ci", "/home/user/Secrets/key.pem", "secrets", true, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := newTestFilePathInfo("raw", tt.abs, "/home/user", "/tmp", tt.ci)
			got := info.containsSegment(tt.segment)
			if got != tt.exp {
				t.Errorf("containsSegment(%q) with ci=%v = %v, want %v", tt.segment, tt.ci, got, tt.exp)
			}
		})
	}
}

func TestFilePathInfo_EndsWithPathSuffix_CaseInsensitive(t *testing.T) {
	tests := []struct {
		name   string
		raw    string
		abs    string
		suffix string
		ci     bool
		exp    bool
	}{
		{"match via slashAbs cs", "raw", "/home/user/.claude/settings.json", ".claude/settings.json", false, true},
		{"no match cs", "raw", "/home/user/.Claude/Settings.json", ".claude/settings.json", false, false},
		{"match ci", "raw", "/home/user/.Claude/Settings.json", ".claude/settings.json", true, true},
		{"match via slashRaw cs", ".claude/settings.json", "/abs", ".claude/settings.json", false, true},
		{"match via slashRaw ci", ".Claude/Settings.json", "/abs", ".claude/settings.json", true, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := newTestFilePathInfo(tt.raw, tt.abs, "/home/user", "/tmp", tt.ci)
			got := info.endsWithPathSuffix(tt.suffix)
			if got != tt.exp {
				t.Errorf("endsWithPathSuffix(%q) with ci=%v = %v, want %v", tt.suffix, tt.ci, got, tt.exp)
			}
		})
	}
}

func TestFilePathInfo_IsUnder_CaseInsensitive(t *testing.T) {
	tests := []struct {
		name string
		abs  string
		base string
		ci   bool
		exp  bool
	}{
		{"under cs", "/home/user/.fuse/state/db", "/home/user/.fuse", false, true},
		{"not under cs", "/home/user/.Fuse/state/db", "/home/user/.fuse", false, false},
		{"under ci", "/home/user/.Fuse/state/db", "/home/user/.fuse", true, true},
		{"exact match ci", "/home/user/.fuse", "/home/user/.Fuse", true, true},
		{"not under either", "/home/user/other", "/home/user/.fuse", true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := newTestFilePathInfo("raw", tt.abs, "/home/user", "/tmp", tt.ci)
			got := info.isUnder(tt.base)
			if got != tt.exp {
				t.Errorf("isUnder(%q) with abs=%q ci=%v = %v, want %v", tt.base, tt.abs, tt.ci, got, tt.exp)
			}
		})
	}
}

func TestFilePathInfo_HasBase_CaseInsensitive(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		abs  string
		base string
		ci   bool
		exp  bool
	}{
		{"match raw cs", "/path/to/.env", "/path/to/.env", ".env", false, true},
		{"no match cs", "/path/to/.ENV", "/path/to/.ENV", ".env", false, false},
		{"match ci", "/path/to/.ENV", "/path/to/.ENV", ".env", true, true},
		{"match abs ci", "/path/to/other", "/path/to/.Env", ".env", true, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := newTestFilePathInfo(tt.raw, tt.abs, "/home/user", "/tmp", tt.ci)
			got := info.hasBase(tt.base)
			if got != tt.exp {
				t.Errorf("hasBase(%q) with ci=%v = %v, want %v", tt.base, tt.ci, got, tt.exp)
			}
		})
	}
}

func TestFilePathInfo_IsGitHookPath_CaseInsensitive(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		abs  string
		ci   bool
		exp  bool
	}{
		{"match relative cs", ".git/hooks/pre-commit", "/repo/.git/hooks/pre-commit", false, true},
		{"no match cs", ".Git/Hooks/pre-commit", "/repo/.Git/Hooks/pre-commit", false, false},
		{"match relative ci", ".Git/Hooks/pre-commit", "/repo/.Git/Hooks/pre-commit", true, true},
		{"match abs ci", "hooks/pre-commit", "/repo/.GIT/HOOKS/pre-commit", true, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := newTestFilePathInfo(tt.raw, tt.abs, "/home/user", "/repo", tt.ci)
			got := info.isGitHookPath()
			if got != tt.exp {
				t.Errorf("isGitHookPath() with ci=%v = %v, want %v", tt.ci, got, tt.exp)
			}
		})
	}
}

func TestFilePathInfo_IsEnvFile_CaseInsensitive(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		ci   bool
		exp  bool
	}{
		{"match cs", "/path/.env", false, true},
		{"match prefix cs", "/path/.env.local", false, true},
		{"no match cs", "/path/.ENV", false, false},
		{"match ci", "/path/.ENV", true, true},
		{"match prefix ci", "/path/.ENV.LOCAL", true, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := newTestFilePathInfo(tt.raw, tt.raw, "/home/user", "/tmp", tt.ci)
			got := info.isEnvFile()
			if got != tt.exp {
				t.Errorf("isEnvFile() for %q with ci=%v = %v, want %v", tt.raw, tt.ci, got, tt.exp)
			}
		})
	}
}

func TestFilePathInfo_IsClaudeSettingsPath_CaseInsensitive(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		abs  string
		ci   bool
		exp  bool
	}{
		{"match suffix cs", ".claude/settings.json", "/home/user/.claude/settings.json", false, true},
		{"no match cs", ".Claude/Settings.json", "/home/user/.Claude/Settings.json", false, false},
		{"match ci", ".Claude/Settings.json", "/home/user/.Claude/Settings.json", true, true},
		{"match abs ci", "foo", "/home/user/.CLAUDE/SETTINGS.JSON", true, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := newTestFilePathInfo(tt.raw, tt.abs, "/home/user", "/tmp", tt.ci)
			got := info.isClaudeSettingsPath()
			if got != tt.exp {
				t.Errorf("isClaudeSettingsPath() for raw=%q abs=%q ci=%v = %v, want %v", tt.raw, tt.abs, tt.ci, got, tt.exp)
			}
		})
	}
}

func TestFilePathInfo_IsCodexConfigPath_CaseInsensitive(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		abs  string
		ci   bool
		exp  bool
	}{
		{"match suffix cs", ".codex/config.toml", "/home/user/.codex/config.toml", false, true},
		{"no match cs", ".Codex/Config.toml", "/home/user/.Codex/Config.toml", false, false},
		{"match ci", ".Codex/Config.toml", "/home/user/.Codex/Config.toml", true, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := newTestFilePathInfo(tt.raw, tt.abs, "/home/user", "/tmp", tt.ci)
			got := info.isCodexConfigPath()
			if got != tt.exp {
				t.Errorf("isCodexConfigPath() for raw=%q abs=%q ci=%v = %v, want %v", tt.raw, tt.abs, tt.ci, got, tt.exp)
			}
		})
	}
}
