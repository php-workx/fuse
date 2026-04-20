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

func TestCodexVersionSupportsNativeHooks(t *testing.T) {
	tests := []struct {
		name   string
		goos   string
		output string
		want   bool
	}{
		{name: "minimum supported version", goos: "linux", output: "codex-cli 0.115.0", want: true},
		{name: "newer supported version", goos: "darwin", output: "codex-cli 0.120.0", want: true},
		{name: "older version", goos: "linux", output: "codex-cli 0.114.9", want: false},
		{name: "windows disabled", goos: "windows", output: "codex-cli 0.120.0", want: false},
		{name: "unparseable version", goos: "linux", output: "codex-cli dev", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := codexVersionSupportsNativeHooks(tt.goos, tt.output); got != tt.want {
				t.Fatalf("codexVersionSupportsNativeHooks(%q, %q) = %v, want %v", tt.goos, tt.output, got, tt.want)
			}
		})
	}
}

func TestMergeCodexHooksJSON_PreservesUnrelatedHooksAndReplacesFuseHook(t *testing.T) {
	existing := `{
  "hooks": {
    "PreToolUse": [
      {
        "matcher": "Bash",
        "hooks": [
          {"type": "command", "command": "old fuse hook evaluate", "timeout": 1},
          {"type": "command", "command": "other hook", "timeout": 2}
        ]
      },
      {
        "matcher": "Read",
        "hooks": [
          {"type": "command", "command": "read hook", "timeout": 3}
        ]
      }
    ]
  }
}`
	got, err := mergeCodexHooksJSON([]byte(existing))
	if err != nil {
		t.Fatalf("mergeCodexHooksJSON: %v", err)
	}

	var hooks map[string]interface{}
	if err := json.Unmarshal(got, &hooks); err != nil {
		t.Fatalf("unmarshal merged hooks: %v\n%s", err, got)
	}
	preToolUse := hooks["hooks"].(map[string]interface{})["PreToolUse"].([]interface{})
	if len(preToolUse) != 2 {
		t.Fatalf("PreToolUse entry count = %d, want 2: %#v", len(preToolUse), preToolUse)
	}
	bashEntry := preToolUse[0].(map[string]interface{})
	if bashEntry["matcher"] != "Bash" {
		t.Fatalf("first matcher = %v, want Bash", bashEntry["matcher"])
	}
	bashHooks := bashEntry["hooks"].([]interface{})
	var hasFuse, hasOldFuse, hasOther bool
	for _, raw := range bashHooks {
		entry := raw.(map[string]interface{})
		command, _ := entry["command"].(string)
		switch {
		case command == "old fuse hook evaluate":
			hasOldFuse = true
		case strings.Contains(command, "fuse hook evaluate"):
			hasFuse = true
			if entry["statusMessage"] != "Fuse checking command" {
				t.Fatalf("statusMessage = %v, want Fuse checking command", entry["statusMessage"])
			}
		case command == "other hook":
			hasOther = true
		}
	}
	if !hasFuse || hasOldFuse || !hasOther {
		t.Fatalf("merged Bash hooks wrong: hasFuse=%v hasOldFuse=%v hasOther=%v hooks=%#v", hasFuse, hasOldFuse, hasOther, bashHooks)
	}
}

func TestInstallCodexUsesNativeHooksWhenSupported(t *testing.T) {
	tmpHome := t.TempDir()
	codexHome := filepath.Join(tmpHome, ".codex")
	t.Setenv("HOME", tmpHome)
	t.Setenv("CODEX_HOME", codexHome)
	t.Setenv("FUSE_HOME", filepath.Join(tmpHome, ".fuse"))
	t.Setenv("PATH", t.TempDir())

	prev := detectCodexNativeHooksSupport
	detectCodexNativeHooksSupport = func() bool { return true }
	t.Cleanup(func() { detectCodexNativeHooksSupport = prev })

	if err := installCodexWithProfile(config.ProfileRelaxed); err != nil {
		t.Fatalf("installCodexWithProfile: %v", err)
	}

	configData, err := os.ReadFile(filepath.Join(codexHome, "config.toml"))
	if err != nil {
		t.Fatalf("read config.toml: %v", err)
	}
	configText := string(configData)
	if !strings.Contains(configText, "codex_hooks = true") {
		t.Fatalf("expected codex_hooks enabled, got:\n%s", configText)
	}
	if strings.Contains(configText, "fuse-shell") || strings.Contains(configText, "shell_tool = false") {
		t.Fatalf("expected native hook install not to install MCP fallback config, got:\n%s", configText)
	}

	hooksData, err := os.ReadFile(filepath.Join(codexHome, "hooks.json"))
	if err != nil {
		t.Fatalf("read hooks.json: %v", err)
	}
	if !strings.Contains(string(hooksData), "fuse hook evaluate") || !strings.Contains(string(hooksData), "PreToolUse") {
		t.Fatalf("expected Codex hooks file to contain fuse PreToolUse hook, got:\n%s", hooksData)
	}
}

func TestInstallCodexFallsBackToMCPWhenNativeHooksUnsupported(t *testing.T) {
	tmpHome := t.TempDir()
	codexHome := filepath.Join(tmpHome, ".codex")
	t.Setenv("HOME", tmpHome)
	t.Setenv("CODEX_HOME", codexHome)
	t.Setenv("FUSE_HOME", filepath.Join(tmpHome, ".fuse"))
	t.Setenv("PATH", t.TempDir())

	prev := detectCodexNativeHooksSupport
	detectCodexNativeHooksSupport = func() bool { return false }
	t.Cleanup(func() { detectCodexNativeHooksSupport = prev })

	if err := installCodexWithProfile(config.ProfileRelaxed); err != nil {
		t.Fatalf("installCodexWithProfile: %v", err)
	}

	configData, err := os.ReadFile(filepath.Join(codexHome, "config.toml"))
	if err != nil {
		t.Fatalf("read config.toml: %v", err)
	}
	configText := string(configData)
	for _, want := range []string{"shell_tool = false", "[mcp_servers.fuse-shell]", `args = ["proxy", "codex-shell"]`} {
		if !strings.Contains(configText, want) {
			t.Fatalf("expected fallback config to contain %q, got:\n%s", want, configText)
		}
	}
	if _, err := os.Stat(filepath.Join(codexHome, "hooks.json")); !os.IsNotExist(err) {
		t.Fatalf("expected fallback install not to create hooks.json, stat err=%v", err)
	}
}

func TestRemoveCodexIntegration(t *testing.T) {
	input := `[features]
shell_tool = false

[mcp_servers.fuse-shell]
command = "fuse"
args = ["proxy", "codex-shell"]
[mcp_servers.fuse-shell.tools.run_command]
approval_mode = "approve"

[other]
value = "keep"
`
	got := removeCodexIntegration(input)
	if strings.Contains(got, "fuse-shell") {
		t.Fatalf("expected fuse-shell sections removed:\n%s", got)
	}
	if strings.Contains(got, "shell_tool = false") {
		t.Fatalf("expected shell_tool override removed:\n%s", got)
	}
	if !strings.Contains(got, "[other]") {
		t.Fatalf("expected unrelated config preserved:\n%s", got)
	}
}

func TestRemoveCodexHooksJSON(t *testing.T) {
	input := []byte(`{
  "hooks": {
    "PreToolUse": [
      {
        "matcher": "Bash",
        "hooks": [
          {"type": "command", "command": "FUSE_HOOK_AGENT=codex fuse hook evaluate", "timeout": 300},
          {"type": "command", "command": "other hook", "timeout": 2}
        ]
      },
      {
        "matcher": "Read",
        "hooks": [
          {"type": "command", "command": "read hook", "timeout": 3}
        ]
      }
    ]
  }
}`)
	got, modified, err := removeCodexHooksJSON(input)
	if err != nil {
		t.Fatalf("removeCodexHooksJSON: %v", err)
	}
	if !modified {
		t.Fatal("removeCodexHooksJSON modified = false, want true")
	}
	gotText := string(got)
	if strings.Contains(gotText, "fuse hook evaluate") {
		t.Fatalf("expected fuse hook removed, got:\n%s", gotText)
	}
	for _, want := range []string{"other hook", "read hook"} {
		if !strings.Contains(gotText, want) {
			t.Fatalf("expected unrelated hook %q preserved, got:\n%s", want, gotText)
		}
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
	binDir := t.TempDir()
	fusePath := writeFuseVersionExecutable(t, binDir, "fuse 1.2.3 (install-test) built 2026-04-16")
	t.Setenv("HOME", tmpHome)
	t.Setenv("FUSE_HOME", filepath.Join(tmpHome, ".fuse"))
	t.Setenv("PATH", binDir)

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
		"Hook binary: path: " + fusePath,
		// PATH fuse is a shell fixture; doctor must surface it as
		// unverified rather than executing it to read a version.
		"unverified",
		"not executed",
	} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("expected stdout to contain %q, got:\n%s", want, stdout)
		}
	}
	if strings.Contains(stdout, "fuse 1.2.3") || strings.Contains(stdout, "install-test") {
		t.Fatalf("stdout must not leak unexecuted PATH binary's claimed version, got:\n%s", stdout)
	}
	if !strings.Contains(stderr, "judge provider") {
		t.Fatalf("expected stderr warning about missing judge provider, got:\n%s", stderr)
	}
	assertProfileAwareConfigScaffold(t, config.ConfigPath(), config.ProfileBalanced)
}

func TestInstallCodexDefaultsToRelaxedProfileWhenInputIsEmpty(t *testing.T) {
	tmpHome := t.TempDir()
	binDir := t.TempDir()
	fusePath := writeFuseVersionExecutable(t, binDir, "fuse 2.3.4 (codex-install-test) built 2026-04-16")
	t.Setenv("HOME", tmpHome)
	t.Setenv("CODEX_HOME", filepath.Join(tmpHome, ".codex"))
	t.Setenv("FUSE_HOME", filepath.Join(tmpHome, ".fuse"))
	t.Setenv("PATH", binDir)

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
		t.Fatalf("expected relaxed selection output, got:\n%s", stdout)
	}
	for _, want := range []string{
		"Hook binary: path: " + fusePath,
		// PATH fuse is a shell fixture; doctor must surface it as
		// unverified rather than executing it to read a version.
		"unverified",
		"not executed",
	} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("expected stdout to contain %q, got:\n%s", want, stdout)
		}
	}
	if strings.Contains(stdout, "fuse 2.3.4") || strings.Contains(stdout, "codex-install-test") {
		t.Fatalf("stdout must not leak unexecuted PATH binary's claimed version, got:\n%s", stdout)
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

func TestInstallClaudeWarnsWhenHookBinaryIsStale(t *testing.T) {
	// Simulate the case fus-dv6k described: the user rebuilt fuse but the
	// stale binary is still on PATH. The install command must surface that
	// the hook will invoke the old binary so the user reinstalls rather
	// than running silently with stale policy.
	withBuildMetadata(t, "1.4.0", "fresh-commit", "2026-04-18")

	tmpHome := t.TempDir()
	binDir := t.TempDir()
	writeFuseVersionExecutable(t, binDir, "fuse 1.3.0 (stale-commit) built 2026-04-01")
	t.Setenv("HOME", tmpHome)
	t.Setenv("FUSE_HOME", filepath.Join(tmpHome, ".fuse"))
	t.Setenv("PATH", binDir)

	if err := installClaude(false); err != nil {
		t.Fatalf("installClaude(false): %v", err)
	}
}

func TestPrintHookBinaryInfo_WarnsWhenHookBinaryIsStale(t *testing.T) {
	// Current build metadata is known and the PATH fuse is an untrusted
	// third-party file (different bytes from os.Executable). doctor /
	// install must flag the mismatch from static metadata alone without
	// executing the PATH binary to read its version string.
	withBuildMetadata(t, "1.4.0", "fresh-commit", "2026-04-18")

	binDir := t.TempDir()
	fusePath := writeFuseVersionExecutable(t, binDir, "fuse 1.3.0 (stale-commit) built 2026-04-01")
	t.Setenv("PATH", binDir)

	stdout, _, err := captureCLIOutput(t, func() error {
		printHookBinaryInfo()
		return nil
	})
	if err != nil {
		t.Fatalf("captureCLIOutput: %v", err)
	}
	for _, want := range []string{
		"Hook binary: path: " + fusePath,
		"unverified",
		"not executed",
		"WARNING",
		"stale or mismatched",
		"fuse 1.4.0 (fresh-commit)",
		"Reinstall fuse",
	} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("expected stdout to contain %q, got:\n%s", want, stdout)
		}
	}
	// The PATH script's advertised version must not leak — we did not run it.
	for _, forbidden := range []string{"fuse 1.3.0", "stale-commit"} {
		if strings.Contains(stdout, forbidden) {
			t.Fatalf("stdout must not leak unexecuted PATH binary's %q, got:\n%s", forbidden, stdout)
		}
	}
}

func TestPrintHookBinaryInfo_NoWarningWhenHookBinaryMatches(t *testing.T) {
	// After reinstalling from current source the hook binary on PATH is the
	// same on-disk artifact as the running process (identical SHA-256). No
	// stale warning should be printed — emitting one would train users to
	// ignore real stale-binary alerts.
	withBuildMetadata(t, "1.4.0", "matching-commit", "2026-04-18")

	fusePath := stageSelfAsFuseInPath(t)

	stdout, _, err := captureCLIOutput(t, func() error {
		printHookBinaryInfo()
		return nil
	})
	if err != nil {
		t.Fatalf("captureCLIOutput: %v", err)
	}
	for _, want := range []string{"Hook binary: path: " + fusePath, "fuse 1.4.0 (matching-commit)"} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("expected stdout to contain %q, got:\n%s", want, stdout)
		}
	}
	for _, unwanted := range []string{"WARNING", "stale", "Reinstall fuse", "unverified"} {
		if strings.Contains(stdout, unwanted) {
			t.Fatalf("unexpected %q in stdout:\n%s", unwanted, stdout)
		}
	}
}

func assertProfileAwareConfigScaffold(t *testing.T, configPath, wantProfile string) {
	t.Helper()

	info, err := os.Stat(configPath)
	if err != nil {
		t.Fatalf("stat scaffold: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("scaffold permissions = %o, want %o", got, 0o600)
	}

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
