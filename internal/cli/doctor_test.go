package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/php-workx/fuse/internal/db"
)

func TestRunDoctor_ReportsMCPProxyChecks(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("FUSE_HOME", tmpDir)

	if err := os.MkdirAll(filepath.Dir(configPathForTest(t)), 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	configYAML := "mcp_proxies:\n  - name: missing\n    command: definitely-not-a-command\n    args: []\n    env: {}\n"
	if err := os.WriteFile(configPathForTest(t), []byte(configYAML), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	stdout, stderr, err := captureDoctorOutput(t, func() error {
		return runDoctor(false, false)
	})
	if err == nil {
		t.Fatal("expected doctor to return an error for a missing downstream command")
	}
	if stderr != "" {
		t.Fatalf("expected no stderr, got %q", stderr)
	}
	if !strings.Contains(stdout, "MCP proxy") {
		t.Fatalf("expected doctor output to include MCP proxy diagnostics, got:\n%s", stdout)
	}
}

func TestRunDoctor_ReportsCurrentProfile(t *testing.T) {
	fuseHome := t.TempDir()
	t.Setenv("FUSE_HOME", fuseHome)

	configPath := configPathForTest(t)
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	if err := os.WriteFile(configPath, []byte("profile: strict\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	binDir := t.TempDir()
	mustWriteExecutable(t, binDir, "fuse")
	mustWriteExecutable(t, binDir, "claude")
	t.Setenv("PATH", binDir)

	stdout, stderr, err := captureDoctorOutput(t, func() error {
		return runDoctor(false, false)
	})
	if err != nil {
		t.Fatalf("unexpected doctor error: %v\nstdout:\n%s", err, stdout)
	}
	if stderr != "" {
		t.Fatalf("expected no stderr, got %q", stderr)
	}
	for _, want := range []string{"Current profile", "strict", "Judge availability", "claude"} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("expected doctor output to include %q, got:\n%s", want, stdout)
		}
	}
}

func TestRunDoctor_WarnsWhenBalancedProfileHasNoJudgeProvider(t *testing.T) {
	fuseHome := t.TempDir()
	t.Setenv("FUSE_HOME", fuseHome)

	configPath := configPathForTest(t)
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	if err := os.WriteFile(configPath, []byte("profile: balanced\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	binDir := t.TempDir()
	mustWriteExecutable(t, binDir, "fuse")
	t.Setenv("PATH", binDir)

	stdout, stderr, err := captureDoctorOutput(t, func() error {
		return runDoctor(false, false)
	})
	if err != nil {
		t.Fatalf("unexpected doctor error: %v\nstdout:\n%s", err, stdout)
	}
	if stderr != "" {
		t.Fatalf("expected no stderr, got %q", stderr)
	}
	for _, want := range []string{"Judge availability", "balanced", "no judge provider"} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("expected doctor output to include %q, got:\n%s", want, stdout)
		}
	}
}

func TestRunDoctor_WarnsWhenBalancedProfileHasJudgeProviderButNoAuth(t *testing.T) {
	fuseHome := t.TempDir()
	t.Setenv("FUSE_HOME", fuseHome)
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("CLAUDE_API_KEY", "")

	configPath := configPathForTest(t)
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	if err := os.WriteFile(configPath, []byte("profile: balanced\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	binDir := t.TempDir()
	mustWriteExecutable(t, binDir, "fuse")
	mustWriteExecutable(t, binDir, "claude")
	t.Setenv("PATH", binDir)

	stdout, stderr, err := captureDoctorOutput(t, func() error {
		return runDoctor(false, false)
	})
	if err != nil {
		t.Fatalf("unexpected doctor error: %v\nstdout:\n%s", err, stdout)
	}
	if stderr != "" {
		t.Fatalf("expected no stderr, got %q", stderr)
	}
	for _, want := range []string{"Judge availability", "provider detected: claude", "auth not detected"} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("expected doctor output to include %q, got:\n%s", want, stdout)
		}
	}
}

func TestRunDoctor_PassesWhenBalancedProfileHasJudgeProviderAndAuth(t *testing.T) {
	fuseHome := t.TempDir()
	t.Setenv("FUSE_HOME", fuseHome)
	t.Setenv("ANTHROPIC_API_KEY", "test-key")

	configPath := configPathForTest(t)
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	if err := os.WriteFile(configPath, []byte("profile: balanced\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	binDir := t.TempDir()
	mustWriteExecutable(t, binDir, "fuse")
	mustWriteExecutable(t, binDir, "claude")
	t.Setenv("PATH", binDir)

	stdout, stderr, err := captureDoctorOutput(t, func() error {
		return runDoctor(false, false)
	})
	if err != nil {
		t.Fatalf("unexpected doctor error: %v\nstdout:\n%s", err, stdout)
	}
	if stderr != "" {
		t.Fatalf("expected no stderr, got %q", stderr)
	}
	for _, want := range []string{"Judge availability", "provider detected: claude", "auth configured"} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("expected doctor output to include %q, got:\n%s", want, stdout)
		}
	}
}

func TestRunDoctorSecurity_WarnsWhenClaudeHookExistsWithoutSecureSettings(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("CODEX_HOME", filepath.Join(t.TempDir(), ".codex"))
	t.Setenv("FUSE_HOME", t.TempDir())

	settingsPath := filepath.Join(tmpHome, ".claude", "settings.json")
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		t.Fatalf("mkdir settings dir: %v", err)
	}

	settings := mustClaudeSettings(t, defaultFuseHookEntries())
	writeJSONForTest(t, settingsPath, settings)

	stdout, stderr, err := captureDoctorOutput(t, func() error {
		return runDoctor(false, true)
	})
	if err != nil {
		t.Fatalf("unexpected doctor --security error: %v\nstdout:\n%s", err, stdout)
	}
	if stderr != "" {
		t.Fatalf("expected no stderr, got %q", stderr)
	}
	for _, want := range []string{"Claude security posture", "missing or weaker", "permissions block"} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("expected doctor --security output to include %q, got:\n%s", want, stdout)
		}
	}
}

func TestCheckClaudeSecurityPosture_WarnsWhenSettingsMissing(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	got := checkClaudeSecurityPosture()
	if got.status != "WARN" {
		t.Fatalf("checkClaudeSecurityPosture() status = %q, want WARN", got.status)
	}
	if !strings.Contains(got.detail, "not evaluated") && !strings.Contains(got.detail, "not found") {
		t.Fatalf("expected warning detail about missing settings, got %q", got.detail)
	}
}

func TestCheckClaudeSecurityPosture_WarnsWhenHookMissing(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	settingsPath := filepath.Join(tmpHome, ".claude", "settings.json")
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		t.Fatalf("mkdir settings dir: %v", err)
	}
	writeJSONForTest(t, settingsPath, map[string]interface{}{"theme": "light"})

	got := checkClaudeSecurityPosture()
	if got.status != "WARN" {
		t.Fatalf("checkClaudeSecurityPosture() status = %q, want WARN", got.status)
	}
	if !strings.Contains(got.detail, "not evaluated") && !strings.Contains(got.detail, "missing") {
		t.Fatalf("expected warning detail about missing hook, got %q", got.detail)
	}
}

func TestRunDoctorSecurity_PassesWithSecureClaudeSettingsPresent(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("CODEX_HOME", filepath.Join(t.TempDir(), ".codex"))
	t.Setenv("FUSE_HOME", t.TempDir())

	settingsPath := filepath.Join(tmpHome, ".claude", "settings.json")
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		t.Fatalf("mkdir settings dir: %v", err)
	}

	settings := mustClaudeSettings(t, defaultFuseHookEntries())
	if err := mergeClaudeSecureSettings(settings); err != nil {
		t.Fatalf("mergeClaudeSecureSettings: %v", err)
	}
	writeJSONForTest(t, settingsPath, settings)

	stdout, stderr, err := captureDoctorOutput(t, func() error {
		return runDoctor(false, true)
	})
	if err != nil {
		t.Fatalf("unexpected doctor --security error: %v\nstdout:\n%s", err, stdout)
	}
	if stderr != "" {
		t.Fatalf("expected no stderr, got %q", stderr)
	}
	if !strings.Contains(stdout, "Claude security posture") || !strings.Contains(stdout, "secure Claude settings present") {
		t.Fatalf("expected secure Claude PASS output, got:\n%s", stdout)
	}
}

func TestCheckCodexSecurityPosture_WarnsWhenConfigMissing(t *testing.T) {
	t.Setenv("CODEX_HOME", filepath.Join(t.TempDir(), ".codex"))

	got := checkCodexSecurityPosture()
	if got.status != "WARN" {
		t.Fatalf("checkCodexSecurityPosture() status = %q, want WARN", got.status)
	}
	if !strings.Contains(got.detail, "not found") && !strings.Contains(got.detail, "skipping") {
		t.Fatalf("expected warning detail about missing config, got %q", got.detail)
	}
}

func TestRunDoctorSecurity_WarnsWhenCodexShellToolEnabledOrFuseShellMissing(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	codexHome := t.TempDir()
	t.Setenv("CODEX_HOME", codexHome)
	t.Setenv("FUSE_HOME", t.TempDir())

	configPath := filepath.Join(codexHome, "config.toml")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("mkdir codex dir: %v", err)
	}
	configText := `[features]
shell_tool = true

[mcp_servers.other]
command = "other"
args = ["serve"]
`
	if err := os.WriteFile(configPath, []byte(configText), 0o644); err != nil {
		t.Fatalf("write codex config: %v", err)
	}

	stdout, stderr, err := captureDoctorOutput(t, func() error {
		return runDoctor(false, true)
	})
	if err != nil {
		t.Fatalf("unexpected doctor --security error: %v\nstdout:\n%s", err, stdout)
	}
	if stderr != "" {
		t.Fatalf("expected no stderr, got %q", stderr)
	}
	for _, want := range []string{"Codex security posture", "shell_tool", "fuse-shell"} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("expected doctor --security output to include %q, got:\n%s", want, stdout)
		}
	}
}

func TestRunDoctorSecurity_WarnsAboutMCPRiskWhenClaudeHookExistsWithoutProxies(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("CODEX_HOME", filepath.Join(t.TempDir(), ".codex"))
	fuseHome := t.TempDir()
	t.Setenv("FUSE_HOME", fuseHome)

	settingsPath := filepath.Join(tmpHome, ".claude", "settings.json")
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		t.Fatalf("mkdir settings dir: %v", err)
	}
	settings := mustClaudeSettings(t, defaultFuseHookEntries())
	if err := mergeClaudeSecureSettings(settings); err != nil {
		t.Fatalf("mergeClaudeSecureSettings: %v", err)
	}
	writeJSONForTest(t, settingsPath, settings)

	if err := os.MkdirAll(filepath.Dir(configPathForTest(t)), 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	if err := os.WriteFile(configPathForTest(t), []byte("log_level: warn\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	stdout, stderr, err := captureDoctorOutput(t, func() error {
		return runDoctor(false, true)
	})
	if err != nil {
		t.Fatalf("unexpected doctor --security error: %v\nstdout:\n%s", err, stdout)
	}
	if stderr != "" {
		t.Fatalf("expected no stderr, got %q", stderr)
	}
	for _, want := range []string{"MCP mediation posture", "no MCP proxies configured", "direct MCP"} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("expected doctor --security output to include %q, got:\n%s", want, stdout)
		}
	}
}

func TestCheckMCPMediationPosture_WarnsWhenNotAssessed(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("FUSE_HOME", t.TempDir())

	if err := os.MkdirAll(filepath.Dir(configPathForTest(t)), 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	if err := os.WriteFile(configPathForTest(t), []byte("log_level: warn\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	got := checkMCPMediationPosture()
	if got.status != "WARN" {
		t.Fatalf("checkMCPMediationPosture() status = %q, want WARN", got.status)
	}
	if !strings.Contains(got.detail, "not assessed") {
		t.Fatalf("expected warning detail about unassessed posture, got %q", got.detail)
	}
}

func TestRunDoctorSecurity_WarnsWhenClaudeSettingsCannotBeReadForMCPAssessment(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("CODEX_HOME", filepath.Join(t.TempDir(), ".codex"))
	fuseHome := t.TempDir()
	t.Setenv("FUSE_HOME", fuseHome)

	settingsPath := filepath.Join(tmpHome, ".claude", "settings.json")
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		t.Fatalf("mkdir settings dir: %v", err)
	}
	if err := os.WriteFile(settingsPath, []byte("{not valid json\n"), 0o644); err != nil {
		t.Fatalf("write malformed settings: %v", err)
	}

	if err := os.MkdirAll(filepath.Dir(configPathForTest(t)), 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	if err := os.WriteFile(configPathForTest(t), []byte("log_level: warn\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	stdout, stderr, err := captureDoctorOutput(t, func() error {
		return runDoctor(false, true)
	})
	if err == nil {
		t.Fatalf("expected doctor --security to still report the malformed Claude settings failure\nstdout:\n%s", stdout)
	}
	if stderr != "" {
		t.Fatalf("expected no stderr, got %q", stderr)
	}
	for _, want := range []string{"MCP mediation posture", "cannot assess MCP mediation posture safely", "settings"} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("expected doctor --security output to include %q, got:\n%s", want, stdout)
		}
	}
}

func TestRunDoctorSecurity_WarnsOnUnmediatedClaudeMCPServers(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("CODEX_HOME", filepath.Join(t.TempDir(), ".codex"))
	fuseHome := t.TempDir()
	t.Setenv("FUSE_HOME", fuseHome)

	settingsPath := filepath.Join(tmpHome, ".claude", "settings.json")
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		t.Fatalf("mkdir settings dir: %v", err)
	}
	settings := mustClaudeSettings(t, defaultFuseHookEntries())
	settings["mcpServers"] = map[string]interface{}{
		"aws-direct": map[string]interface{}{
			"command": "npx",
			"args":    []interface{}{"-y", "@aws/mcp-server"},
		},
	}
	if err := mergeClaudeSecureSettings(settings); err != nil {
		t.Fatalf("mergeClaudeSecureSettings: %v", err)
	}
	writeJSONForTest(t, settingsPath, settings)

	if err := os.MkdirAll(filepath.Dir(configPathForTest(t)), 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	configYAML := "mcp_proxies:\n  - name: aws-mcp\n    command: echo\n    args: [\"-y\", \"@aws/mcp-server\"]\n    env: {}\n"
	if err := os.WriteFile(configPathForTest(t), []byte(configYAML), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	stdout, stderr, err := captureDoctorOutput(t, func() error {
		return runDoctor(false, true)
	})
	if err != nil {
		t.Fatalf("unexpected doctor --security error: %v\nstdout:\n%s", err, stdout)
	}
	if stderr != "" {
		t.Fatalf("expected no stderr, got %q", stderr)
	}
	for _, want := range []string{"MCP mediation posture", "aws-direct", "not mediated through fuse"} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("expected doctor --security output to include %q, got:\n%s", want, stdout)
		}
	}
}

func TestRunDoctorSecurity_PassesForMediatedClaudeMCPServers(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("CODEX_HOME", filepath.Join(t.TempDir(), ".codex"))
	fuseHome := t.TempDir()
	t.Setenv("FUSE_HOME", fuseHome)

	settingsPath := filepath.Join(tmpHome, ".claude", "settings.json")
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		t.Fatalf("mkdir settings dir: %v", err)
	}
	settings := mustClaudeSettings(t, defaultFuseHookEntries())
	settings["mcpServers"] = map[string]interface{}{
		"aws-mcp": map[string]interface{}{
			"command": "fuse",
			"args":    []interface{}{"proxy", "mcp", "--downstream-name", "aws-mcp"},
		},
	}
	if err := mergeClaudeSecureSettings(settings); err != nil {
		t.Fatalf("mergeClaudeSecureSettings: %v", err)
	}
	writeJSONForTest(t, settingsPath, settings)

	if err := os.MkdirAll(filepath.Dir(configPathForTest(t)), 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	configYAML := "mcp_proxies:\n  - name: aws-mcp\n    command: echo\n    args: [\"-y\", \"@aws/mcp-server\"]\n    env: {}\n"
	if err := os.WriteFile(configPathForTest(t), []byte(configYAML), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	stdout, stderr, err := captureDoctorOutput(t, func() error {
		return runDoctor(false, true)
	})
	if err != nil {
		t.Fatalf("unexpected doctor --security error: %v\nstdout:\n%s", err, stdout)
	}
	if stderr != "" {
		t.Fatalf("expected no stderr, got %q", stderr)
	}
	if !strings.Contains(stdout, "MCP mediation posture") || !strings.Contains(stdout, "looks mediated") {
		t.Fatalf("expected mediated MCP posture PASS output, got:\n%s", stdout)
	}
	if strings.Contains(stdout, "not mediated through fuse") {
		t.Fatalf("expected no unmediated MCP warning, got:\n%s", stdout)
	}
}

func TestRunDoctorSecurity_WarnsWhenClaudeMCPDownstreamNameIsMissingOrUnknown(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("CODEX_HOME", filepath.Join(t.TempDir(), ".codex"))
	fuseHome := t.TempDir()
	t.Setenv("FUSE_HOME", fuseHome)

	settingsPath := filepath.Join(tmpHome, ".claude", "settings.json")
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		t.Fatalf("mkdir settings dir: %v", err)
	}
	settings := mustClaudeSettings(t, defaultFuseHookEntries())
	settings["mcpServers"] = map[string]interface{}{
		"missing-name": map[string]interface{}{
			"command": "fuse",
			"args":    []interface{}{"proxy", "mcp"},
		},
		"unknown-name": map[string]interface{}{
			"command": "fuse",
			"args":    []interface{}{"proxy", "mcp", "--downstream-name", "missing"},
		},
	}
	if err := mergeClaudeSecureSettings(settings); err != nil {
		t.Fatalf("mergeClaudeSecureSettings: %v", err)
	}
	writeJSONForTest(t, settingsPath, settings)

	if err := os.MkdirAll(filepath.Dir(configPathForTest(t)), 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	configYAML := "mcp_proxies:\n  - name: aws-mcp\n    command: echo\n    args: [\"-y\", \"@aws/mcp-server\"]\n    env: {}\n"
	if err := os.WriteFile(configPathForTest(t), []byte(configYAML), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	stdout, stderr, err := captureDoctorOutput(t, func() error {
		return runDoctor(false, true)
	})
	if err != nil {
		t.Fatalf("unexpected doctor --security error: %v\nstdout:\n%s", err, stdout)
	}
	if stderr != "" {
		t.Fatalf("expected no stderr, got %q", stderr)
	}
	for _, want := range []string{"MCP mediation posture", "missing configured --downstream-name", "unknown downstream"} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("expected doctor --security output to include %q, got:\n%s", want, stdout)
		}
	}
}

func TestRunDoctorLive_ReportsTerminalCapabilityChecks(t *testing.T) {
	if runtime.GOOS == "windows" {
		// Windows console checks require a real console (CONIN$).
		// CI runners typically don't have one — skip if unavailable.
		t.Skip("Windows terminal checks require interactive console (not available in CI)")
	}
	tmpDir := t.TempDir()
	t.Setenv("FUSE_HOME", tmpDir)

	stdout, _, err := captureDoctorOutput(t, func() error {
		return runDoctor(true, false)
	})
	if err != nil {
		t.Fatalf("unexpected doctor --live error: %v", err)
	}
	for _, want := range []string{"/dev/tty", "foreground process-group"} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("expected doctor --live output to include %q, got:\n%s", want, stdout)
		}
	}
}

func TestHasFuseHook_RequiresExpectedMatchersAndTimeout(t *testing.T) {
	settings := mustClaudeSettings(t, []map[string]interface{}{
		{
			"matcher": "Bash",
			"hooks": []map[string]interface{}{
				{
					"type":    "command",
					"command": "fuse hook evaluate",
					"timeout": float64(5),
				},
			},
		},
		{
			"matcher": "Read",
			"hooks": []map[string]interface{}{
				{
					"type":    "command",
					"command": "fuse hook evaluate",
					"timeout": float64(30),
				},
			},
		},
	})

	if hasFuseHook(settings) {
		t.Fatal("expected malformed hook schema to fail doctor validation")
	}
}

func captureDoctorOutput(t *testing.T, fn func() error) (string, string, error) {
	t.Helper()

	origStdout := os.Stdout
	origStderr := os.Stderr

	stdoutR, stdoutW, err := os.Pipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	stderrR, stderrW, err := os.Pipe()
	if err != nil {
		t.Fatalf("stderr pipe: %v", err)
	}

	os.Stdout = stdoutW
	os.Stderr = stderrW
	defer func() {
		os.Stdout = origStdout
		os.Stderr = origStderr
	}()

	runErr := fn()
	_ = stdoutW.Close()
	_ = stderrW.Close()

	var stdoutBuf bytes.Buffer
	var stderrBuf bytes.Buffer
	if _, err := stdoutBuf.ReadFrom(stdoutR); err != nil {
		t.Fatalf("read stdout: %v", err)
	}
	if _, err := stderrBuf.ReadFrom(stderrR); err != nil {
		t.Fatalf("read stderr: %v", err)
	}
	return stdoutBuf.String(), stderrBuf.String(), runErr
}

func configPathForTest(t *testing.T) string {
	t.Helper()
	fuseHome := os.Getenv("FUSE_HOME")
	if fuseHome == "" {
		t.Fatal("FUSE_HOME not set")
	}
	return filepath.Join(fuseHome, "config", "config.yaml")
}

func mustClaudeSettings(t *testing.T, entries []map[string]interface{}) map[string]interface{} {
	t.Helper()
	data := map[string]interface{}{
		"hooks": map[string]interface{}{
			"PreToolUse": entries,
		},
	}

	blob, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("marshal settings: %v", err)
	}
	var settings map[string]interface{}
	if err := json.Unmarshal(blob, &settings); err != nil {
		t.Fatalf("unmarshal settings: %v", err)
	}
	return settings
}

func mustWriteExecutable(t *testing.T, dir, name string) string {
	t.Helper()

	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write executable %s: %v", path, err)
	}
	return path
}

// defaultFuseHookEntries returns the standard Bash + mcp__ hook entries
// used across most doctor tests.
func defaultFuseHookEntries() []map[string]interface{} {
	return []map[string]interface{}{
		{
			"matcher": "Bash",
			"hooks": []map[string]interface{}{
				{"type": "command", "command": "fuse hook evaluate", "timeout": float64(30)},
			},
		},
		{
			"matcher": "mcp__.*",
			"hooks": []map[string]interface{}{
				{"type": "command", "command": "fuse hook evaluate", "timeout": float64(30)},
			},
		},
	}
}

func TestMergeClaudeSecureSettings_UpgradesExplicitFalse(t *testing.T) {
	settings := mustClaudeSettings(t, defaultFuseHookEntries())

	// Pre-seed sandbox.enabled=false — the secure upgrade should flip it to true.
	settings["sandbox"] = map[string]interface{}{
		"enabled": false,
	}

	if err := mergeClaudeSecureSettings(settings); err != nil {
		t.Fatalf("mergeClaudeSecureSettings: %v", err)
	}

	sandbox, ok := settings["sandbox"].(map[string]interface{})
	if !ok {
		t.Fatal("expected sandbox map")
	}
	enabled, ok := sandbox["enabled"]
	if !ok {
		t.Fatal("expected sandbox.enabled key")
	}
	if enabled != true {
		t.Fatalf("mergeClaudeSecureSettings did not upgrade sandbox.enabled from false to true, got %v", enabled)
	}
}

func TestCheckTagOverrides_ReportsConfiguredOverrides(t *testing.T) {
	fuseHome := t.TempDir()
	t.Setenv("FUSE_HOME", fuseHome)

	policyPath := filepath.Join(fuseHome, "config", "policy.yaml")
	if err := os.MkdirAll(filepath.Dir(policyPath), 0o755); err != nil {
		t.Fatalf("mkdir policy dir: %v", err)
	}
	policyYAML := "version: \"1\"\ntag_overrides:\n  terraform: enabled\n  cdk: dryrun\n"
	if err := os.WriteFile(policyPath, []byte(policyYAML), 0o644); err != nil {
		t.Fatalf("write policy: %v", err)
	}

	got := checkTagOverrides()
	if got.status != "PASS" {
		t.Fatalf("checkTagOverrides() status = %q, want PASS", got.status)
	}
	if !strings.Contains(got.detail, "2 tag override") {
		t.Fatalf("expected 2 tag overrides in detail, got %q", got.detail)
	}
	for _, want := range []string{"terraform=enabled", "cdk=dryrun"} {
		if !strings.Contains(got.detail, want) {
			t.Fatalf("expected detail to contain %q, got %q", want, got.detail)
		}
	}
}

func TestCheckTagOverrides_PassesWithNoPolicyFile(t *testing.T) {
	t.Setenv("FUSE_HOME", t.TempDir())

	got := checkTagOverrides()
	if got.status != "PASS" {
		t.Fatalf("checkTagOverrides() status = %q, want PASS", got.status)
	}
	if !strings.Contains(got.detail, "no policy file") {
		t.Fatalf("expected 'no policy file' in detail, got %q", got.detail)
	}
}

func TestCheckTagOverrides_PassesWithNoOverrides(t *testing.T) {
	fuseHome := t.TempDir()
	t.Setenv("FUSE_HOME", fuseHome)

	policyPath := filepath.Join(fuseHome, "config", "policy.yaml")
	if err := os.MkdirAll(filepath.Dir(policyPath), 0o755); err != nil {
		t.Fatalf("mkdir policy dir: %v", err)
	}
	if err := os.WriteFile(policyPath, []byte("version: \"1\"\n"), 0o644); err != nil {
		t.Fatalf("write policy: %v", err)
	}

	got := checkTagOverrides()
	if got.status != "PASS" {
		t.Fatalf("checkTagOverrides() status = %q, want PASS", got.status)
	}
	if !strings.Contains(got.detail, "no tag overrides configured") {
		t.Fatalf("expected 'no tag overrides configured' in detail, got %q", got.detail)
	}
}

func TestCheckTagOverrides_FailsOnInvalidOverride(t *testing.T) {
	fuseHome := t.TempDir()
	t.Setenv("FUSE_HOME", fuseHome)

	policyPath := filepath.Join(fuseHome, "config", "policy.yaml")
	if err := os.MkdirAll(filepath.Dir(policyPath), 0o755); err != nil {
		t.Fatalf("mkdir policy dir: %v", err)
	}
	policyYAML := "version: \"1\"\ntag_overrides:\n  terraform: invalid_mode\n"
	if err := os.WriteFile(policyPath, []byte(policyYAML), 0o644); err != nil {
		t.Fatalf("write policy: %v", err)
	}

	got := checkTagOverrides()
	if got.status != "FAIL" {
		t.Fatalf("checkTagOverrides() status = %q, want FAIL", got.status)
	}
	if !strings.Contains(got.detail, "invalid") {
		t.Fatalf("expected 'invalid' in detail, got %q", got.detail)
	}
}

func TestCheckSQLiteDB_PassesWhenDBNotCreated(t *testing.T) {
	t.Setenv("FUSE_HOME", t.TempDir())

	got := checkSQLiteDB()
	if got.status != "PASS" {
		t.Fatalf("checkSQLiteDB() status = %q, want PASS", got.status)
	}
	if !strings.Contains(got.detail, "not yet created") {
		t.Fatalf("expected 'not yet created' in detail, got %q", got.detail)
	}
}

func TestCheckSQLiteDB_PassesWhenDBExists(t *testing.T) {
	fuseHome := t.TempDir()
	t.Setenv("FUSE_HOME", fuseHome)

	stateDir := filepath.Join(fuseHome, "state")
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		t.Fatalf("mkdir state dir: %v", err)
	}
	dbPath := filepath.Join(stateDir, "fuse.db")
	database, err := db.OpenDB(dbPath)
	if err != nil {
		t.Fatalf("create test DB: %v", err)
	}
	_ = database.Close()

	got := checkSQLiteDB()
	if got.status != "PASS" {
		t.Fatalf("checkSQLiteDB() status = %q, want PASS", got.status)
	}
	if !strings.Contains(got.detail, dbPath) {
		t.Fatalf("expected DB path in detail, got %q", got.detail)
	}
}

func TestCheckSQLiteDB_FailsWhenDBCorrupt(t *testing.T) {
	fuseHome := t.TempDir()
	t.Setenv("FUSE_HOME", fuseHome)

	stateDir := filepath.Join(fuseHome, "state")
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		t.Fatalf("mkdir state dir: %v", err)
	}
	dbPath := filepath.Join(stateDir, "fuse.db")
	if err := os.WriteFile(dbPath, []byte("not a sqlite database"), 0o644); err != nil {
		t.Fatalf("write corrupt DB: %v", err)
	}

	got := checkSQLiteDB()
	if got.status != "FAIL" {
		t.Fatalf("checkSQLiteDB() status = %q, want FAIL", got.status)
	}
}

func TestCheckFuseInPath_WarnWhenNotInPath(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	got := checkFuseInPath()
	if got.status != "WARN" {
		t.Fatalf("checkFuseInPath() status = %q, want WARN", got.status)
	}
	if !strings.Contains(got.detail, "not found in PATH") {
		t.Fatalf("expected 'not found in PATH' in detail, got %q", got.detail)
	}
}

func TestCheckApprovalTerminalTrust_WarnsInCI(t *testing.T) {
	t.Setenv("CI", "true")
	t.Setenv("SSH_CONNECTION", "")
	t.Setenv("SSH_TTY", "")
	t.Setenv("TERM", "xterm")

	got := checkApprovalTerminalTrust()
	if got.status != "WARN" {
		t.Fatalf("checkApprovalTerminalTrust() status = %q, want WARN", got.status)
	}
	if !strings.Contains(got.detail, "CI") {
		t.Fatalf("expected 'CI' in detail, got %q", got.detail)
	}
}

func TestCheckApprovalTerminalTrust_WarnsOnSSH(t *testing.T) {
	t.Setenv("CI", "")
	t.Setenv("SSH_CONNECTION", "10.0.0.1 12345 10.0.0.2 22")
	t.Setenv("TERM", "xterm")

	got := checkApprovalTerminalTrust()
	if got.status != "WARN" {
		t.Fatalf("checkApprovalTerminalTrust() status = %q, want WARN", got.status)
	}
	if !strings.Contains(got.detail, "SSH") {
		t.Fatalf("expected 'SSH' in detail, got %q", got.detail)
	}
}

func TestCheckApprovalTerminalTrust_WarnsOnDumbTerminal(t *testing.T) {
	t.Setenv("CI", "")
	t.Setenv("SSH_CONNECTION", "")
	t.Setenv("SSH_TTY", "")
	t.Setenv("TERM", "dumb")

	got := checkApprovalTerminalTrust()
	if got.status != "WARN" {
		t.Fatalf("checkApprovalTerminalTrust() status = %q, want WARN", got.status)
	}
	if !strings.Contains(got.detail, "dumb") {
		t.Fatalf("expected 'dumb' in detail, got %q", got.detail)
	}
}

func TestCheckApprovalTerminalTrust_PassesNormally(t *testing.T) {
	t.Setenv("CI", "")
	t.Setenv("SSH_CONNECTION", "")
	t.Setenv("SSH_TTY", "")
	t.Setenv("TERM", "xterm-256color")

	got := checkApprovalTerminalTrust()
	if got.status != "PASS" {
		t.Fatalf("checkApprovalTerminalTrust() status = %q, want PASS", got.status)
	}
}

func TestCheckPolicyYAML_PassesWithValidPolicy(t *testing.T) {
	fuseHome := t.TempDir()
	t.Setenv("FUSE_HOME", fuseHome)

	policyPath := filepath.Join(fuseHome, "config", "policy.yaml")
	if err := os.MkdirAll(filepath.Dir(policyPath), 0o755); err != nil {
		t.Fatalf("mkdir policy dir: %v", err)
	}
	policyYAML := `version: "1"
rules:
  - pattern: "echo hello"
    action: allow
    reason: "test"
`
	if err := os.WriteFile(policyPath, []byte(policyYAML), 0o644); err != nil {
		t.Fatalf("write policy: %v", err)
	}

	got := checkPolicyYAML()
	if got.status != "PASS" {
		t.Fatalf("checkPolicyYAML() status = %q, want PASS", got.status)
	}
	if !strings.Contains(got.detail, "1 rules loaded") {
		t.Fatalf("expected '1 rules loaded' in detail, got %q", got.detail)
	}
}

func TestCheckPolicyYAML_PassesWhenMissing(t *testing.T) {
	t.Setenv("FUSE_HOME", t.TempDir())

	got := checkPolicyYAML()
	if got.status != "PASS" {
		t.Fatalf("checkPolicyYAML() status = %q, want PASS", got.status)
	}
	if !strings.Contains(got.detail, "not present") {
		t.Fatalf("expected 'not present' in detail, got %q", got.detail)
	}
}

func TestCheckDirectoryStructure_WarnsWhenMissing(t *testing.T) {
	t.Setenv("FUSE_HOME", filepath.Join(t.TempDir(), "nonexistent"))

	got := checkDirectoryStructure()
	if got.status != "WARN" {
		t.Fatalf("checkDirectoryStructure() status = %q, want WARN", got.status)
	}
	if !strings.Contains(got.detail, "does not exist") {
		t.Fatalf("expected 'does not exist' in detail, got %q", got.detail)
	}
}

func TestCheckDirectoryStructure_FailsWhenNotADirectory(t *testing.T) {
	fuseHome := t.TempDir()
	t.Setenv("FUSE_HOME", fuseHome)

	// Create config as a file instead of a directory.
	configDir := filepath.Join(fuseHome, "config")
	if err := os.WriteFile(configDir, []byte("not a dir"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	got := checkDirectoryStructure()
	if got.status != "FAIL" {
		t.Fatalf("checkDirectoryStructure() status = %q, want FAIL", got.status)
	}
	if !strings.Contains(got.detail, "not a directory") {
		t.Fatalf("expected 'not a directory' in detail, got %q", got.detail)
	}
}

func TestRunDoctor_VerboseShowsFullDetail(t *testing.T) {
	fuseHome := t.TempDir()
	t.Setenv("FUSE_HOME", fuseHome)

	// Create a policy with many tag overrides to produce a long detail string.
	policyPath := filepath.Join(fuseHome, "config", "policy.yaml")
	if err := os.MkdirAll(filepath.Dir(policyPath), 0o755); err != nil {
		t.Fatalf("mkdir policy dir: %v", err)
	}
	policyYAML := "version: \"1\"\ntag_overrides:\n  git: enabled\n  aws: enabled\n  gcp: dryrun\n  azure: disabled\n  terraform: enabled\n  kubernetes: dryrun\n"
	if err := os.WriteFile(policyPath, []byte(policyYAML), 0o644); err != nil {
		t.Fatalf("write policy: %v", err)
	}

	// Run with --verbose to capture full detail.
	origVerbose := doctorVerbose
	doctorVerbose = true
	defer func() { doctorVerbose = origVerbose }()

	stdout, _, err := captureDoctorOutput(t, func() error {
		return runDoctor(false, true)
	})
	_ = err // don't care about pass/fail, just checking output

	// With verbose, all overrides should be visible.
	for _, tag := range []string{"git=enabled", "aws=enabled", "gcp=dryrun", "azure=disabled"} {
		if !strings.Contains(stdout, tag) {
			t.Fatalf("expected verbose output to contain %q, got:\n%s", tag, stdout)
		}
	}
}

func TestRunDoctor_FixAppliesAutoFixes(t *testing.T) {
	fuseHome := t.TempDir()
	t.Setenv("FUSE_HOME", fuseHome)
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("CODEX_HOME", filepath.Join(t.TempDir(), ".codex"))

	// Create Claude settings with the hook but without secure settings
	// so --fix has something to fix.
	settingsPath := filepath.Join(tmpHome, ".claude", "settings.json")
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		t.Fatalf("mkdir settings dir: %v", err)
	}
	settings := mustClaudeSettings(t, defaultFuseHookEntries())
	writeJSONForTest(t, settingsPath, settings)

	origFix := doctorFix
	doctorFix = true
	defer func() { doctorFix = origFix }()

	stdout, _, err := captureDoctorOutput(t, func() error {
		return runDoctor(false, true)
	})
	_ = err

	if !strings.Contains(stdout, "fixing:") || !strings.Contains(stdout, "fixed.") {
		t.Fatalf("expected --fix output to show 'fixing:' and 'fixed.', got:\n%s", stdout)
	}
	if !strings.Contains(stdout, "auto-fixed") {
		t.Fatalf("expected summary to mention 'auto-fixed', got:\n%s", stdout)
	}
}

func writeJSONForTest(t *testing.T, path string, data map[string]interface{}) {
	t.Helper()

	if dir := filepath.Dir(path); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}
	blob, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("marshal JSON: %v", err)
	}
	if err := os.WriteFile(path, append(blob, '\n'), 0o644); err != nil {
		t.Fatalf("write JSON file: %v", err)
	}
}

func TestEscapePattern_NoDashDoubleEscaping(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"foo-bar", `foo\-bar`},
		{`foo\bar`, `foo\\bar`},
		{"a-b.c", `a\-b\.c`},
		{"git push --force", `git push \-\-force`},
		{"rm -rf /", `rm \-rf /`},
		{"no-special", `no\-special`},
	}
	for _, tc := range tests {
		got := escapePattern(tc.input)
		if got != tc.want {
			t.Errorf("escapePattern(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}
