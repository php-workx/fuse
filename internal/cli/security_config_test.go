package cli

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestMergeClaudeSecureSettingsMergesWithoutClobberingUnrelatedValues(t *testing.T) {
	settings := map[string]interface{}{
		"hooks": map[string]interface{}{
			"PreToolUse": []interface{}{
				map[string]interface{}{
					"matcher": "Read",
					"hooks": []interface{}{
						map[string]interface{}{
							"type":    "command",
							"command": "echo keep",
							"timeout": float64(5),
						},
					},
				},
			},
		},
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
				"denyWrite": []interface{}{
					"~/scratch",
				},
			},
		},
		"theme": "dark",
	}

	if err := mergeClaudeSecureSettings(settings); err != nil {
		t.Fatalf("mergeClaudeSecureSettings: %v", err)
	}
	mergeFuseHook(settings)

	assertClaudeSecureDefaults(t, settings, "askUser")

	if settings["theme"] != "dark" {
		t.Fatalf("expected unrelated top-level setting preserved, got %#v", settings["theme"])
	}

	hooksObj := settings["hooks"].(map[string]interface{})
	preToolUse := hooksObj["PreToolUse"].([]interface{})
	foundReadMatcher := false
	for _, raw := range preToolUse {
		entry := raw.(map[string]interface{})
		if entry["matcher"] == "Read" {
			foundReadMatcher = true
		}
	}
	if !foundReadMatcher {
		t.Fatal("expected unrelated Read matcher preserved")
	}

	permissions := settings["permissions"].(map[string]interface{})
	deny := stringsFromValue(t, permissions["deny"])
	if !containsString(deny, "Read(./customer-secrets/**)") {
		t.Fatalf("expected unrelated deny rule preserved, got %v", deny)
	}

	sandbox := settings["sandbox"].(map[string]interface{})
	filesystem := sandbox["filesystem"].(map[string]interface{})
	denyRead := stringsFromValue(t, filesystem["denyRead"])
	denyWrite := stringsFromValue(t, filesystem["denyWrite"])
	if !containsString(denyRead, "~/private") {
		t.Fatalf("expected unrelated denyRead preserved, got %v", denyRead)
	}
	if !containsString(denyWrite, "~/scratch") {
		t.Fatalf("expected unrelated denyWrite preserved, got %v", denyWrite)
	}
}

func TestMergeClaudeSecureSettingsIsIdempotent(t *testing.T) {
	settings := map[string]interface{}{}

	if err := mergeClaudeSecureSettings(settings); err != nil {
		t.Fatalf("mergeClaudeSecureSettings: %v", err)
	}
	first := canonicalJSON(t, settings)

	if err := mergeClaudeSecureSettings(settings); err != nil {
		t.Fatalf("mergeClaudeSecureSettings: %v", err)
	}
	second := canonicalJSON(t, settings)

	if first != second {
		t.Fatalf("expected idempotent merge\nfirst:  %s\nsecond: %s", first, second)
	}
}

func TestMergeClaudeSecureSettingsUpgradesInsecureManagedValues(t *testing.T) {
	settings := map[string]interface{}{
		"permissions": map[string]interface{}{
			"defaultMode":                  "bypassPermissions",
			"disableBypassPermissionsMode": "allow",
			"deny": []interface{}{
				"Read(./.env)",
			},
		},
		"sandbox": map[string]interface{}{
			"enabled":                  false,
			"autoAllowBashIfSandboxed": true,
			"allowUnsandboxedCommands": true,
			"filesystem":               map[string]interface{}{},
		},
	}

	if err := mergeClaudeSecureSettings(settings); err != nil {
		t.Fatalf("mergeClaudeSecureSettings: %v", err)
	}

	assertClaudeSecureDefaults(t, settings, "acceptEdits")
}

func TestMergeClaudeSecureSettingsPreservesStricterDefaultMode(t *testing.T) {
	settings := map[string]interface{}{
		"permissions": map[string]interface{}{
			"defaultMode":                  "askUser",
			"disableBypassPermissionsMode": "disable",
		},
	}

	if err := mergeClaudeSecureSettings(settings); err != nil {
		t.Fatalf("mergeClaudeSecureSettings: %v", err)
	}

	permissions := settings["permissions"].(map[string]interface{})
	if got := permissions["defaultMode"]; got != "askUser" {
		t.Fatalf("permissions.defaultMode = %#v, want %q", got, "askUser")
	}
}

func TestMergeClaudeSecureSettingsPreservesExistingDenyListsAlongsideManagedEntries(t *testing.T) {
	settings := map[string]interface{}{
		"permissions": map[string]interface{}{
			"deny": []interface{}{
				"Edit(./customer/**)",
			},
		},
		"sandbox": map[string]interface{}{
			"filesystem": map[string]interface{}{
				"denyRead": []interface{}{
					"~/finance",
				},
				"denyWrite": []interface{}{
					"~/backups",
				},
			},
		},
	}

	if err := mergeClaudeSecureSettings(settings); err != nil {
		t.Fatalf("mergeClaudeSecureSettings: %v", err)
	}

	permissions := settings["permissions"].(map[string]interface{})
	deny := stringsFromValue(t, permissions["deny"])
	if !containsString(deny, "Edit(./customer/**)") {
		t.Fatalf("expected unrelated deny entry preserved, got %v", deny)
	}
	for _, want := range managedClaudePermissionDeny {
		if !containsString(deny, want) {
			t.Fatalf("expected managed deny entry %q present, got %v", want, deny)
		}
	}

	sandbox := settings["sandbox"].(map[string]interface{})
	filesystem := sandbox["filesystem"].(map[string]interface{})
	denyRead := stringsFromValue(t, filesystem["denyRead"])
	denyWrite := stringsFromValue(t, filesystem["denyWrite"])
	if !containsString(denyRead, "~/finance") {
		t.Fatalf("expected unrelated denyRead entry preserved, got %v", denyRead)
	}
	if !containsString(denyWrite, "~/backups") {
		t.Fatalf("expected unrelated denyWrite entry preserved, got %v", denyWrite)
	}
	for _, want := range managedClaudeSandboxDenyRead {
		if !containsString(denyRead, want) {
			t.Fatalf("expected managed denyRead entry %q present, got %v", want, denyRead)
		}
	}
	for _, want := range managedClaudeSandboxDenyWrite {
		if !containsString(denyWrite, want) {
			t.Fatalf("expected managed denyWrite entry %q present, got %v", want, denyWrite)
		}
	}
}

func TestMergeClaudeSecureSettingsRejectsUnexpectedShapes(t *testing.T) {
	tests := []struct {
		name     string
		settings map[string]interface{}
		wantErr  string
	}{
		{
			name: "permissions is not an object",
			settings: map[string]interface{}{
				"permissions": "askUser",
			},
			wantErr: "permissions",
		},
		{
			name: "sandbox is not an object",
			settings: map[string]interface{}{
				"sandbox": true,
			},
			wantErr: "sandbox",
		},
		{
			name: "filesystem is not an object",
			settings: map[string]interface{}{
				"sandbox": map[string]interface{}{
					"filesystem": []interface{}{},
				},
			},
			wantErr: "sandbox.filesystem",
		},
		{
			name: "permissions deny has wrong shape",
			settings: map[string]interface{}{
				"permissions": map[string]interface{}{
					"deny": "Read(./.env)",
				},
			},
			wantErr: "permissions.deny",
		},
		{
			name: "denyRead has wrong shape",
			settings: map[string]interface{}{
				"sandbox": map[string]interface{}{
					"filesystem": map[string]interface{}{
						"denyRead": map[string]interface{}{},
					},
				},
			},
			wantErr: "sandbox.filesystem.denyRead",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := mergeClaudeSecureSettings(tt.settings)
			if err == nil {
				t.Fatal("expected error for unexpected existing shape")
			}
			if !containsString([]string{err.Error()}, tt.wantErr) && !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected error to mention %q, got %v", tt.wantErr, err)
			}
		})
	}
}

func TestCodexSecurityWarnings_AcceptsMultilineArgsArray(t *testing.T) {
	configText := `[features]
shell_tool = false

[mcp_servers.fuse-shell]
command = "fuse"
args = [
  "proxy",
  "codex-shell",
]
`

	warnings := codexSecurityWarnings(configText)
	for _, warning := range warnings {
		if strings.Contains(warning, "mcp_servers.fuse-shell.args") {
			t.Fatalf("expected multiline args array to be accepted, got warnings %v", warnings)
		}
	}
}

func TestClaudeMCPServerWarnings_DistinguishesMediatedAndDirectEntries(t *testing.T) {
	settings := map[string]interface{}{
		"mcpServers": map[string]interface{}{
			"aws-direct": map[string]interface{}{
				"command": "npx",
				"args":    []interface{}{"-y", "@aws/mcp-server"},
			},
			"aws-fuse": map[string]interface{}{
				"command": "/usr/local/bin/fuse",
				"args":    []interface{}{"proxy", "mcp", "--downstream-name", "aws-mcp"},
			},
		},
	}

	configured := map[string]struct{}{
		"aws-mcp": {},
	}
	warnings, mediated := claudeMCPServerWarnings(settings, configured)
	if mediated != 1 {
		t.Fatalf("mediated count = %d, want 1", mediated)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "aws-direct") {
		t.Fatalf("expected one warning for direct MCP entry, got %v", warnings)
	}
}

func TestClaudeMCPServerWarnings_RequiresConfiguredDownstreamName(t *testing.T) {
	settings := map[string]interface{}{
		"mcpServers": map[string]interface{}{
			"missing-name": map[string]interface{}{
				"command": "fuse",
				"args":    []interface{}{"proxy", "mcp"},
			},
			"unknown-name": map[string]interface{}{
				"command": "fuse",
				"args":    []interface{}{"proxy", "mcp", "--downstream-name", "missing"},
			},
		},
	}

	configured := map[string]struct{}{
		"aws-mcp": {},
	}
	warnings, mediated := claudeMCPServerWarnings(settings, configured)
	if mediated != 0 {
		t.Fatalf("mediated count = %d, want 0", mediated)
	}
	for _, want := range []string{"missing-name", "missing configured --downstream-name", "unknown-name", "unknown downstream"} {
		if !containsWarning(warnings, want) {
			t.Fatalf("expected warnings to include %q, got %v", want, warnings)
		}
	}
}

func containsWarning(warnings []string, want string) bool {
	for _, warning := range warnings {
		if strings.Contains(warning, want) {
			return true
		}
	}
	return false
}

func assertClaudeSecureDefaults(t *testing.T, settings map[string]interface{}, expectedDefaultMode string) {
	t.Helper()

	permissions, ok := settings["permissions"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected permissions map, got %#v", settings["permissions"])
	}
	if permissions["defaultMode"] != expectedDefaultMode {
		t.Fatalf("permissions.defaultMode = %#v, want %q", permissions["defaultMode"], expectedDefaultMode)
	}
	if permissions["disableBypassPermissionsMode"] != "disable" {
		t.Fatalf("permissions.disableBypassPermissionsMode = %#v, want %q", permissions["disableBypassPermissionsMode"], "disable")
	}

	deny := stringsFromValue(t, permissions["deny"])
	for _, want := range managedClaudePermissionDeny {
		if !containsString(deny, want) {
			t.Fatalf("expected managed deny entry %q present, got %v", want, deny)
		}
	}

	sandbox, ok := settings["sandbox"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected sandbox map, got %#v", settings["sandbox"])
	}
	if sandbox["enabled"] != true {
		t.Fatalf("sandbox.enabled = %#v, want true", sandbox["enabled"])
	}
	if sandbox["autoAllowBashIfSandboxed"] != false {
		t.Fatalf("sandbox.autoAllowBashIfSandboxed = %#v, want false", sandbox["autoAllowBashIfSandboxed"])
	}
	if sandbox["allowUnsandboxedCommands"] != false {
		t.Fatalf("sandbox.allowUnsandboxedCommands = %#v, want false", sandbox["allowUnsandboxedCommands"])
	}

	filesystem, ok := sandbox["filesystem"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected sandbox.filesystem map, got %#v", sandbox["filesystem"])
	}
	denyRead := stringsFromValue(t, filesystem["denyRead"])
	denyWrite := stringsFromValue(t, filesystem["denyWrite"])
	for _, want := range managedClaudeSandboxDenyRead {
		if !containsString(denyRead, want) {
			t.Fatalf("expected managed denyRead entry %q present, got %v", want, denyRead)
		}
	}
	for _, want := range managedClaudeSandboxDenyWrite {
		if !containsString(denyWrite, want) {
			t.Fatalf("expected managed denyWrite entry %q present, got %v", want, denyWrite)
		}
	}
}

func stringsFromValue(t *testing.T, value interface{}) []string {
	t.Helper()

	switch v := value.(type) {
	case []string:
		return append([]string(nil), v...)
	case []interface{}:
		out := make([]string, 0, len(v))
		for _, item := range v {
			s, ok := item.(string)
			if !ok {
				t.Fatalf("expected string item, got %#v", item)
			}
			out = append(out, s)
		}
		return out
	default:
		t.Fatalf("expected string slice, got %#v", value)
		return nil
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func canonicalJSON(t *testing.T, value interface{}) string {
	t.Helper()

	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal JSON: %v", err)
	}
	return string(data)
}
