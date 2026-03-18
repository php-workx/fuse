package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
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

func TestRunDoctorSecurity_WarnsWhenClaudeHookExistsWithoutSecureSettings(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("CODEX_HOME", filepath.Join(t.TempDir(), ".codex"))
	t.Setenv("FUSE_HOME", t.TempDir())

	settingsPath := filepath.Join(tmpHome, ".claude", "settings.json")
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		t.Fatalf("mkdir settings dir: %v", err)
	}

	settings := mustClaudeSettings(t, []map[string]interface{}{
		{
			"matcher": "Bash",
			"hooks": []map[string]interface{}{
				{
					"type":    "command",
					"command": "fuse hook evaluate",
					"timeout": float64(30),
				},
			},
		},
		{
			"matcher": "mcp__.*",
			"hooks": []map[string]interface{}{
				{
					"type":    "command",
					"command": "fuse hook evaluate",
					"timeout": float64(30),
				},
			},
		},
	})
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
	for _, want := range []string{"Claude security posture", "missing or weaker", "permissions.defaultMode"} {
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

	settings := mustClaudeSettings(t, []map[string]interface{}{
		{
			"matcher": "Bash",
			"hooks": []map[string]interface{}{
				{
					"type":    "command",
					"command": "fuse hook evaluate",
					"timeout": float64(30),
				},
			},
		},
		{
			"matcher": "mcp__.*",
			"hooks": []map[string]interface{}{
				{
					"type":    "command",
					"command": "fuse hook evaluate",
					"timeout": float64(30),
				},
			},
		},
	})
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
	settings := mustClaudeSettings(t, []map[string]interface{}{
		{
			"matcher": "Bash",
			"hooks": []map[string]interface{}{
				{
					"type":    "command",
					"command": "fuse hook evaluate",
					"timeout": float64(30),
				},
			},
		},
		{
			"matcher": "mcp__.*",
			"hooks": []map[string]interface{}{
				{
					"type":    "command",
					"command": "fuse hook evaluate",
					"timeout": float64(30),
				},
			},
		},
	})
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
	settings := mustClaudeSettings(t, []map[string]interface{}{
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
	})
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
	settings := mustClaudeSettings(t, []map[string]interface{}{
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
	})
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
	settings := mustClaudeSettings(t, []map[string]interface{}{
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
	})
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

func TestStartForegroundProbeProcess_StaysAliveUntilKilled(t *testing.T) {
	cmd, err := startForegroundProbeProcess(nil, io.Discard, io.Discard)
	if err != nil {
		t.Fatalf("startForegroundProbeProcess: %v", err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()

	time.Sleep(200 * time.Millisecond)
	if err := syscall.Kill(cmd.Process.Pid, 0); err != nil {
		t.Fatalf("expected probe child to still be alive, got %v", err)
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

func writeJSONForTest(t *testing.T, path string, data map[string]interface{}) {
	t.Helper()

	blob, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("marshal JSON: %v", err)
	}
	if err := os.WriteFile(path, append(blob, '\n'), 0o644); err != nil {
		t.Fatalf("write JSON file: %v", err)
	}
}
