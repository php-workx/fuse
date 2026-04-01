package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/php-workx/fuse/internal/config"
)

func TestMergeCodexConfig(t *testing.T) {
	got := mergeCodexConfig("")
	for _, want := range []string{
		"[features]",
		"shell_tool = false",
		`[mcp_servers.fuse-shell]`,
		`command = "fuse"`,
		`args = ["proxy", "codex-shell"]`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("merged config missing %q:\n%s", want, got)
		}
	}
}

func TestRemoveCodexIntegration(t *testing.T) {
	input := `[features]
shell_tool = false

[mcp_servers.fuse-shell]
command = "fuse"
args = ["proxy", "codex-shell"]

[other]
value = "keep"
`
	got := removeCodexIntegration(input)
	if strings.Contains(got, "fuse-shell") {
		t.Fatalf("expected fuse-shell section removed:\n%s", got)
	}
	if strings.Contains(got, "shell_tool = false") {
		t.Fatalf("expected shell_tool override removed:\n%s", got)
	}
	if !strings.Contains(got, "[other]") {
		t.Fatalf("expected unrelated config preserved:\n%s", got)
	}
}

func TestCodexConfigPath_PrefersLocalRepoConfig(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("CODEX_HOME", "")

	localConfigDir := filepath.Join(tmpDir, ".codex")
	if err := os.MkdirAll(localConfigDir, 0o755); err != nil {
		t.Fatalf("mkdir .codex: %v", err)
	}
	localConfigPath := filepath.Join(localConfigDir, "config.toml")
	if err := os.WriteFile(localConfigPath, []byte("[mcp_servers]\n"), 0o644); err != nil {
		t.Fatalf("write local config: %v", err)
	}

	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer func() {
		if err := os.Chdir(origWd); err != nil {
			t.Fatalf("restore wd: %v", err)
		}
	}()

	got := codexConfigPath()
	gotEval, err := filepath.EvalSymlinks(got)
	if err != nil {
		t.Fatalf("EvalSymlinks(got): %v", err)
	}
	wantEval, err := filepath.EvalSymlinks(localConfigPath)
	if err != nil {
		t.Fatalf("EvalSymlinks(want): %v", err)
	}
	if gotEval != wantEval {
		t.Fatalf("codexConfigPath() = %q (%q), want %q (%q)", got, gotEval, localConfigPath, wantEval)
	}
}

func TestInstallCodex_RejectsSymlinkedLocalConfig(t *testing.T) {
	tmpDir := t.TempDir()
	targetPath := filepath.Join(t.TempDir(), "target.toml")
	if err := os.WriteFile(targetPath, []byte("original\n"), 0o644); err != nil {
		t.Fatalf("write target: %v", err)
	}

	localConfigDir := filepath.Join(tmpDir, ".codex")
	if err := os.MkdirAll(localConfigDir, 0o755); err != nil {
		t.Fatalf("mkdir .codex: %v", err)
	}
	localConfigPath := filepath.Join(localConfigDir, "config.toml")
	if err := os.Symlink(targetPath, localConfigPath); err != nil {
		t.Fatalf("symlink local config: %v", err)
	}

	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer func() {
		if err := os.Chdir(origWd); err != nil {
			t.Fatalf("restore wd: %v", err)
		}
	}()

	err = installCodex()
	if err == nil {
		t.Fatal("expected installCodex to reject symlinked local config")
	}
	if !strings.Contains(err.Error(), "symlinked") {
		t.Fatalf("expected symlink rejection error, got %v", err)
	}

	data, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("read target: %v", err)
	}
	if string(data) != "original\n" {
		t.Fatalf("expected symlink target to remain unchanged, got %q", string(data))
	}
}

func TestInstallClaudePreservesCurrentBehaviorByDefault(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	if err := installClaude(false); err != nil {
		t.Fatalf("installClaude(false): %v", err)
	}

	settingsPath := filepath.Join(tmpHome, ".claude", "settings.json")
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("read settings: %v", err)
	}

	var settings map[string]interface{}
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("unmarshal settings: %v", err)
	}

	if _, ok := settings["permissions"]; ok {
		t.Fatalf("expected plain install to avoid secure permissions config, got %#v", settings["permissions"])
	}
	if _, ok := settings["sandbox"]; ok {
		t.Fatalf("expected plain install to avoid secure sandbox config, got %#v", settings["sandbox"])
	}

	hooksObj, ok := settings["hooks"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected hooks object, got %#v", settings["hooks"])
	}
	if _, ok := hooksObj["PreToolUse"]; !ok {
		t.Fatalf("expected PreToolUse hook configuration, got %#v", hooksObj)
	}
	preToolUse, ok := hooksObj["PreToolUse"].([]interface{})
	if !ok {
		t.Fatalf("expected PreToolUse array, got %#v", hooksObj["PreToolUse"])
	}
	matchers := claudeMatchersFromHooks(t, preToolUse)
	want := []string{"Bash", "mcp__.*"}
	if strings.Join(matchers, ",") != strings.Join(want, ",") {
		t.Fatalf("plain install matchers = %v, want %v", matchers, want)
	}
}

func TestInstallClaudePromptsForBalancedProfileAndWarnsWhenNoProvider(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("FUSE_HOME", filepath.Join(tmpHome, ".fuse"))
	t.Setenv("PATH", t.TempDir())

	stdout, stderr, err := captureCLIOutput(t, func() error {
		rootCmd.SetArgs([]string{"install", "claude"})
		rootCmd.SetIn(strings.NewReader("2\n"))
		defer rootCmd.SetArgs(nil)
		defer rootCmd.SetIn(nil)
		return rootCmd.Execute()
	})
	if err != nil {
		t.Fatalf("install claude: %v", err)
	}
	for _, want := range []string{
		"Pick a profile [1-3] (default: 1):",
		"profile selected: balanced",
	} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("expected stdout to contain %q, got:\\n%s", want, stdout)
		}
	}
	if !strings.Contains(stderr, "judge provider") {
		t.Fatalf("expected stderr warning about missing judge provider, got:\\n%s", stderr)
	}
	assertProfileAwareConfigScaffold(t, config.ConfigPath(), config.ProfileBalanced)
}

func TestInstallCodexDefaultsToRelaxedProfileWhenInputIsEmpty(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("CODEX_HOME", filepath.Join(tmpHome, ".codex"))
	t.Setenv("FUSE_HOME", filepath.Join(tmpHome, ".fuse"))
	t.Setenv("PATH", t.TempDir())

	stdout, stderr, err := captureCLIOutput(t, func() error {
		rootCmd.SetArgs([]string{"install", "codex"})
		rootCmd.SetIn(strings.NewReader("\n"))
		defer rootCmd.SetArgs(nil)
		defer rootCmd.SetIn(nil)
		return rootCmd.Execute()
	})
	if err != nil {
		t.Fatalf("install codex: %v", err)
	}
	if !strings.Contains(stdout, "profile selected: relaxed") {
		t.Fatalf("expected relaxed selection output, got:\\n%s", stdout)
	}
	if stderr != "" {
		t.Fatalf("expected no stderr for relaxed default, got %q", stderr)
	}
	assertProfileAwareConfigScaffold(t, config.ConfigPath(), config.ProfileRelaxed)
}

func TestInstallPreservesExistingFuseConfigScaffold(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("FUSE_HOME", filepath.Join(tmpHome, ".fuse"))

	configPath := config.ConfigPath()
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	want := []byte("profile: strict\ncustom_key: keep\n")
	if err := os.WriteFile(configPath, want, 0o644); err != nil {
		t.Fatalf("write existing config: %v", err)
	}

	if err := installClaude(false); err != nil {
		t.Fatalf("installClaude(false): %v", err)
	}

	got, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if string(got) != string(want) {
		t.Fatalf("config.yaml changed unexpectedly:\n--- want ---\n%s--- got ---\n%s", want, got)
	}
}

func TestInstallClaude_RejectsSymlinkedSettingsPath(t *testing.T) {
	tmpHome := t.TempDir()
	targetPath := filepath.Join(t.TempDir(), "target.json")
	if err := os.WriteFile(targetPath, []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("write target: %v", err)
	}
	t.Setenv("HOME", tmpHome)

	claudeDir := filepath.Join(tmpHome, ".claude")
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		t.Fatalf("mkdir .claude: %v", err)
	}
	settingsPath := filepath.Join(claudeDir, "settings.json")
	if err := os.Symlink(targetPath, settingsPath); err != nil {
		t.Fatalf("symlink settings: %v", err)
	}

	err := installClaude(false)
	if err == nil {
		t.Fatal("expected installClaude to reject symlinked settings path")
	}
	if !strings.Contains(err.Error(), "symlinked") {
		t.Fatalf("expected symlink rejection error, got %v", err)
	}

	data, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("read target: %v", err)
	}
	if string(data) != "{}\n" {
		t.Fatalf("expected symlink target unchanged, got %q", string(data))
	}
}

func TestInstallCommand_RejectsSecureFlagForCodex(t *testing.T) {
	prevSecure := installClaudeSecure
	installClaudeSecure = true
	t.Setenv("CODEX_HOME", t.TempDir())
	t.Cleanup(func() {
		installClaudeSecure = prevSecure
	})

	err := installCmd.RunE(installCmd, []string{"codex"})
	if err == nil {
		t.Fatal("expected install command to reject codex --secure")
	}
	if !strings.Contains(err.Error(), "--secure") || !strings.Contains(err.Error(), "claude") {
		t.Fatalf("expected secure-flag rejection, got %v", err)
	}
}

func TestInstallClaudeSecureMergesHooksAndSecureSettingsOnDisk(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	settingsPath := filepath.Join(tmpHome, ".claude", "settings.json")
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		t.Fatalf("mkdir settings dir: %v", err)
	}

	existing := map[string]interface{}{
		"permissions": map[string]interface{}{
			"defaultMode": "askUser",
			"deny": []interface{}{
				"Read(./customer-secrets/**)",
			},
		},
		"sandbox": map[string]interface{}{
			"enabled": true,
			"filesystem": map[string]interface{}{
				"denyRead": []interface{}{
					"~/private",
				},
			},
		},
		"theme": "light",
	}
	data, err := json.Marshal(existing)
	if err != nil {
		t.Fatalf("marshal existing settings: %v", err)
	}
	if err := os.WriteFile(settingsPath, append(data, '\n'), 0o644); err != nil {
		t.Fatalf("write settings: %v", err)
	}

	if err := installClaude(true); err != nil {
		t.Fatalf("installClaude(true): %v", err)
	}

	updatedData, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("read updated settings: %v", err)
	}

	var settings map[string]interface{}
	if err := json.Unmarshal(updatedData, &settings); err != nil {
		t.Fatalf("unmarshal updated settings: %v", err)
	}

	if settings["theme"] != "light" {
		t.Fatalf("expected unrelated theme preserved, got %#v", settings["theme"])
	}
	assertClaudeSecureDefaults(t, settings, "askUser", "disable")

	hooksObj, ok := settings["hooks"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected hooks object, got %#v", settings["hooks"])
	}
	preToolUse, ok := hooksObj["PreToolUse"].([]interface{})
	if !ok || len(preToolUse) == 0 {
		t.Fatalf("expected PreToolUse hooks after secure install, got %#v", hooksObj["PreToolUse"])
	}
	matchers := claudeMatchersFromHooks(t, preToolUse)
	want := []string{"Bash", "Edit", "MultiEdit", "Read", "Write", "mcp__.*"}
	if strings.Join(matchers, ",") != strings.Join(want, ",") {
		t.Fatalf("secure install matchers = %v, want %v", matchers, want)
	}
}

func claudeMatchersFromHooks(t *testing.T, preToolUse []interface{}) []string {
	t.Helper()

	matchers := make([]string, 0, len(preToolUse))
	for _, raw := range preToolUse {
		entry, ok := raw.(map[string]interface{})
		if !ok {
			t.Fatalf("expected matcher entry object, got %#v", raw)
		}
		matcher, ok := entry["matcher"].(string)
		if !ok {
			t.Fatalf("expected matcher string, got %#v", entry["matcher"])
		}
		matchers = append(matchers, matcher)
	}
	sort.Strings(matchers)
	return matchers
}

func assertProfileAwareConfigScaffold(t *testing.T, configPath, wantProfile string) {
	t.Helper()

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read scaffold: %v", err)
	}

	for _, want := range []string{
		"# Fuse configuration",
		"# Profile sets defaults. Override individual settings below.",
		"profile: " + wantProfile,
		"# llm_judge:",
		"#   provider: auto",
		"# caution_fallback: log",
	} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("config scaffold missing %q:\n%s", want, string(data))
		}
	}
}
