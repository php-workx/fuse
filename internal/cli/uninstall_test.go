package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRemoveFuseHook_RemovesFuseKeepsOthers(t *testing.T) {
	settings := map[string]interface{}{
		"hooks": map[string]interface{}{
			"PreToolUse": []interface{}{
				map[string]interface{}{
					"matcher": "Bash",
					"hooks": []interface{}{
						map[string]interface{}{
							"type":    "command",
							"command": "fuse hook evaluate",
							"timeout": float64(300),
						},
						map[string]interface{}{
							"type":    "command",
							"command": "other-tool hook",
							"timeout": float64(100),
						},
					},
				},
			},
		},
	}

	modified := removeFuseHook(settings)
	if !modified {
		t.Fatal("expected modification")
	}

	hooksObj := settings["hooks"].(map[string]interface{})
	preToolUse := hooksObj["PreToolUse"].([]interface{})
	if len(preToolUse) != 1 {
		t.Fatalf("expected 1 matcher entry, got %d", len(preToolUse))
	}
	entry := preToolUse[0].(map[string]interface{})
	hooks := entry["hooks"].([]interface{})
	if len(hooks) != 1 {
		t.Fatalf("expected 1 remaining hook, got %d", len(hooks))
	}
	cmd := hooks[0].(map[string]interface{})["command"].(string)
	if cmd != "other-tool hook" {
		t.Errorf("expected other-tool hook preserved, got %q", cmd)
	}
}

func TestRemoveFuseHook_DropsEmptyMatcherEntry(t *testing.T) {
	settings := map[string]interface{}{
		"hooks": map[string]interface{}{
			"PreToolUse": []interface{}{
				map[string]interface{}{
					"matcher": "Bash",
					"hooks": []interface{}{
						map[string]interface{}{
							"type":    "command",
							"command": "fuse hook evaluate",
						},
					},
				},
			},
		},
	}

	modified := removeFuseHook(settings)
	if !modified {
		t.Fatal("expected modification")
	}

	hooksObj := settings["hooks"].(map[string]interface{})
	if _, ok := hooksObj["PreToolUse"]; ok {
		t.Fatal("expected PreToolUse removed when empty")
	}
}

func TestRemoveFuseHook_NoHooksKey(t *testing.T) {
	settings := map[string]interface{}{
		"theme": "dark",
	}
	if removeFuseHook(settings) {
		t.Error("expected no modification when hooks key missing")
	}
}

func TestRemoveFuseHook_NoPreToolUse(t *testing.T) {
	settings := map[string]interface{}{
		"hooks": map[string]interface{}{
			"PostToolUse": []interface{}{},
		},
	}
	if removeFuseHook(settings) {
		t.Error("expected no modification when PreToolUse missing")
	}
}

func TestRemoveFuseHook_NoFuseHooksPresent(t *testing.T) {
	settings := map[string]interface{}{
		"hooks": map[string]interface{}{
			"PreToolUse": []interface{}{
				map[string]interface{}{
					"matcher": "Bash",
					"hooks": []interface{}{
						map[string]interface{}{
							"type":    "command",
							"command": "some-other-hook",
						},
					},
				},
			},
		},
	}

	if removeFuseHook(settings) {
		t.Error("expected no modification when no fuse hooks present")
	}
}

func TestRemoveFuseHook_MultipleMatchers(t *testing.T) {
	settings := map[string]interface{}{
		"hooks": map[string]interface{}{
			"PreToolUse": []interface{}{
				map[string]interface{}{
					"matcher": "Bash",
					"hooks": []interface{}{
						map[string]interface{}{"type": "command", "command": "fuse hook evaluate"},
					},
				},
				map[string]interface{}{
					"matcher": "mcp__.*",
					"hooks": []interface{}{
						map[string]interface{}{"type": "command", "command": "fuse hook evaluate"},
						map[string]interface{}{"type": "command", "command": "other-hook"},
					},
				},
				map[string]interface{}{
					"matcher": "Read",
					"hooks": []interface{}{
						map[string]interface{}{"type": "command", "command": "fuse hook evaluate"},
					},
				},
			},
		},
	}

	modified := removeFuseHook(settings)
	if !modified {
		t.Fatal("expected modification")
	}

	hooksObj := settings["hooks"].(map[string]interface{})
	preToolUse := hooksObj["PreToolUse"].([]interface{})
	// Bash and Read should be dropped (only fuse hook); mcp__.* should remain with other-hook.
	if len(preToolUse) != 1 {
		t.Fatalf("expected 1 remaining matcher entry, got %d", len(preToolUse))
	}
	entry := preToolUse[0].(map[string]interface{})
	if entry["matcher"] != "mcp__.*" {
		t.Errorf("expected mcp__.* matcher preserved, got %q", entry["matcher"])
	}
}

func TestFilterFuseHooks_NonMapEntries(t *testing.T) {
	hooks := []interface{}{
		"not-a-map",
		map[string]interface{}{"command": "fuse hook evaluate"},
		map[string]interface{}{"command": "keep-me"},
	}
	remaining, removed := filterFuseHooks(hooks)
	if !removed {
		t.Error("expected removed=true")
	}
	if len(remaining) != 2 {
		t.Fatalf("expected 2 remaining (string + keep-me), got %d", len(remaining))
	}
}

func TestCleanMatcherEntry_NonMapEntry(t *testing.T) {
	kept, modified := cleanMatcherEntry("not-a-map")
	if modified {
		t.Error("expected no modification for non-map entry")
	}
	if kept != "not-a-map" {
		t.Errorf("expected entry passed through, got %v", kept)
	}
}

func TestCleanMatcherEntry_NoHooksKey(t *testing.T) {
	entry := map[string]interface{}{
		"matcher": "Bash",
	}
	kept, modified := cleanMatcherEntry(entry)
	if modified {
		t.Error("expected no modification when hooks key missing")
	}
	if kept == nil {
		t.Error("expected entry preserved")
	}
}

func TestUninstallClaude_NoSettingsFile(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	// No settings.json exists — should succeed silently.
	if err := uninstallClaude(); err != nil {
		t.Fatalf("uninstallClaude with no settings: %v", err)
	}
}

func TestUninstallClaude_MalformedJSON(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	settingsPath := filepath.Join(tmpHome, ".claude", "settings.json")
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(settingsPath, []byte("{invalid json"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	err := uninstallClaude()
	if err == nil {
		t.Fatal("expected error for malformed JSON")
	}
}

func TestUninstallClaude_RoundTrip(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	// Install first.
	if err := installClaude(false); err != nil {
		t.Fatalf("installClaude: %v", err)
	}

	settingsPath := filepath.Join(tmpHome, ".claude", "settings.json")
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("read settings: %v", err)
	}
	if !strings.Contains(string(data), "fuse hook evaluate") {
		t.Fatal("install did not add fuse hook")
	}

	// Now uninstall.
	if err := uninstallClaude(); err != nil {
		t.Fatalf("uninstallClaude: %v", err)
	}

	data, err = os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("read settings after uninstall: %v", err)
	}
	if strings.Contains(string(data), "fuse hook evaluate") {
		t.Fatalf("uninstall did not remove fuse hook:\n%s", data)
	}

	// Verify JSON is still valid.
	var settings map[string]interface{}
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("settings invalid JSON after uninstall: %v", err)
	}
}

func TestUninstallCodex_NoConfigFile(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("CODEX_HOME", tmpHome)
	// No config.toml exists — should succeed silently.
	if err := uninstallCodex(); err != nil {
		t.Fatalf("uninstallCodex with no config: %v", err)
	}
}

func TestRemoveCodexIntegration_PreservesUnrelatedSections(t *testing.T) {
	input := `[features]
shell_tool = false

[mcp_servers.fuse-shell]
command = "fuse"
args = ["proxy", "codex-shell"]

[other_section]
key = "value"
`
	got := removeCodexIntegration(input)
	if strings.Contains(got, "fuse-shell") {
		t.Errorf("expected fuse-shell removed:\n%s", got)
	}
	if strings.Contains(got, "shell_tool") {
		t.Errorf("expected shell_tool removed:\n%s", got)
	}
	if !strings.Contains(got, "[other_section]") {
		t.Errorf("expected other_section preserved:\n%s", got)
	}
	if !strings.Contains(got, `key = "value"`) {
		t.Errorf("expected key=value preserved:\n%s", got)
	}
}

func TestRemoveCodexIntegration_NothingToRemove(t *testing.T) {
	input := `[other]
value = "keep"
`
	got := removeCodexIntegration(input)
	if !strings.Contains(got, "[other]") {
		t.Errorf("expected content preserved:\n%s", got)
	}
}

func TestRemoveCodexIntegration_EmptyInput(t *testing.T) {
	got := removeCodexIntegration("")
	if got != "\n" {
		t.Errorf("expected single newline for empty input, got %q", got)
	}
}
