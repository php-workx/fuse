package cli

import (
	"fmt"
	"path/filepath"
	"strings"
)

var managedClaudePermissionDeny = []string{
	"Read(./.env)",
	"Read(./.env.*)",
	"Read(./secrets/**)",
	"Edit(./.env)",
	"Edit(./.env.*)",
	"Edit(./secrets/**)",
}

var managedClaudeSandboxDenyRead = []string{
	"~/.fuse",
	"~/.ssh",
	"~/.aws",
	"~/.config/gcloud",
}

var managedClaudeSandboxDenyWrite = []string{
	"~/.fuse",
	"~/.claude",
	"~/.codex",
	"~/.ssh",
}

func mergeClaudeSecureSettings(settings map[string]interface{}) error {
	permissions, err := ensureOptionalObject(settings, "permissions")
	if err != nil {
		return err
	}
	permissions["defaultMode"] = upgradedDefaultMode(permissions["defaultMode"])
	permissions["disableBypassPermissionsMode"] = upgradedDisableBypassMode(permissions["disableBypassPermissionsMode"])
	permissions["deny"], err = mergeManagedStringList("permissions.deny", permissions["deny"], managedClaudePermissionDeny)
	if err != nil {
		return err
	}

	sandbox, err := ensureOptionalObject(settings, "sandbox")
	if err != nil {
		return err
	}
	sandbox["enabled"], err = upgradedBool("sandbox.enabled", sandbox["enabled"], true)
	if err != nil {
		return err
	}
	sandbox["autoAllowBashIfSandboxed"], err = upgradedBool("sandbox.autoAllowBashIfSandboxed", sandbox["autoAllowBashIfSandboxed"], false)
	if err != nil {
		return err
	}
	sandbox["allowUnsandboxedCommands"], err = upgradedBool("sandbox.allowUnsandboxedCommands", sandbox["allowUnsandboxedCommands"], false)
	if err != nil {
		return err
	}

	filesystem, err := ensureOptionalObject(sandbox, "filesystem")
	if err != nil {
		return fmt.Errorf("sandbox.filesystem: %w", err)
	}
	filesystem["denyRead"], err = mergeManagedStringList("sandbox.filesystem.denyRead", filesystem["denyRead"], managedClaudeSandboxDenyRead)
	if err != nil {
		return err
	}
	filesystem["denyWrite"], err = mergeManagedStringList("sandbox.filesystem.denyWrite", filesystem["denyWrite"], managedClaudeSandboxDenyWrite)
	if err != nil {
		return err
	}
	return nil
}

func claudeSecurityWarnings(settings map[string]interface{}) ([]string, error) {
	permissions, permissionsPresent, err := optionalObjectForValidation(settings, "permissions")
	if err != nil {
		return nil, err
	}
	sandbox, sandboxPresent, err := optionalObjectForValidation(settings, "sandbox")
	if err != nil {
		return nil, err
	}
	filesystem, filesystemPresent, err := optionalObjectForValidation(sandbox, "filesystem")
	if err != nil {
		return nil, fmt.Errorf("sandbox.filesystem: %w", err)
	}

	var warnings []string

	if !permissionsPresent {
		warnings = append(warnings, "permissions block is missing")
	}
	if warning := defaultModeWarning(permissions["defaultMode"]); warning != "" {
		warnings = append(warnings, warning)
	}
	if warning := disableBypassWarning(permissions["disableBypassPermissionsMode"]); warning != "" {
		warnings = append(warnings, warning)
	}

	denyWarnings, err := missingManagedEntriesWarning("permissions.deny", permissions["deny"], managedClaudePermissionDeny)
	if err != nil {
		return nil, err
	}
	warnings = append(warnings, denyWarnings...)

	if !sandboxPresent {
		warnings = append(warnings, "sandbox block is missing")
	}
	sandboxEnabled, err := readBool("sandbox.enabled", sandbox["enabled"])
	if err != nil {
		warnings = append(warnings, err.Error())
	} else if !sandboxEnabled {
		warnings = append(warnings, "sandbox.enabled should be true")
	}

	autoAllow, err := readBool("sandbox.autoAllowBashIfSandboxed", sandbox["autoAllowBashIfSandboxed"])
	if err != nil {
		warnings = append(warnings, err.Error())
	} else if autoAllow {
		warnings = append(warnings, "sandbox.autoAllowBashIfSandboxed should be false")
	}

	allowUnsandboxed, err := readBool("sandbox.allowUnsandboxedCommands", sandbox["allowUnsandboxedCommands"])
	if err != nil {
		warnings = append(warnings, err.Error())
	} else if allowUnsandboxed {
		warnings = append(warnings, "sandbox.allowUnsandboxedCommands should be false")
	}

	if !filesystemPresent {
		warnings = append(warnings, "sandbox.filesystem block is missing")
	}
	denyReadWarnings, err := missingManagedEntriesWarning("sandbox.filesystem.denyRead", filesystem["denyRead"], managedClaudeSandboxDenyRead)
	if err != nil {
		return nil, err
	}
	warnings = append(warnings, denyReadWarnings...)

	denyWriteWarnings, err := missingManagedEntriesWarning("sandbox.filesystem.denyWrite", filesystem["denyWrite"], managedClaudeSandboxDenyWrite)
	if err != nil {
		return nil, err
	}
	warnings = append(warnings, denyWriteWarnings...)

	return warnings, nil
}

func codexSecurityWarnings(configText string) []string {
	var warnings []string

	featuresSection, hasFeatures := tomlSection(configText, "[features]")
	if !hasFeatures {
		warnings = append(warnings, "features.shell_tool is not explicitly disabled")
	} else {
		shellToolValue := tomlAssignment(featuresSection, "shell_tool")
		if shellToolValue == "" || shellToolValue != "false" {
			warnings = append(warnings, "features.shell_tool should be false to disable the built-in Codex shell")
		}
	}

	fuseShellSection, hasFuseShell := tomlSection(configText, "[mcp_servers.fuse-shell]")
	if !hasFuseShell {
		warnings = append(warnings, "mcp_servers.fuse-shell is missing")
		return warnings
	}

	commandValue := strings.Trim(tomlAssignment(fuseShellSection, "command"), `"`)
	if commandValue != "fuse" {
		warnings = append(warnings, `mcp_servers.fuse-shell.command should be "fuse"`)
	}

	argsValue := tomlAssignment(fuseShellSection, "args")
	if !isExpectedCodexArgs(argsValue) {
		warnings = append(warnings, `mcp_servers.fuse-shell.args should be ["proxy", "codex-shell"]`)
	}

	return warnings
}

func claudeMCPServerWarnings(settings map[string]interface{}, configured map[string]struct{}) ([]string, int) {
	raw, ok := settings["mcpServers"]
	if !ok || raw == nil {
		return nil, 0
	}

	servers, ok := raw.(map[string]interface{})
	if !ok {
		return []string{"mcpServers must be an object"}, 0
	}

	var warnings []string
	mediatedCount := 0
	for name, entryRaw := range servers {
		entry, ok := entryRaw.(map[string]interface{})
		if !ok {
			warnings = append(warnings, fmt.Sprintf("mcpServers.%s must be an object", name))
			continue
		}

		command, _ := entry["command"].(string)
		args, err := toStringSetInOrder("mcpServers."+name+".args", entry["args"])
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("mcpServers.%s has invalid args: %v", name, err))
			continue
		}
		downstreamName, isMediated := mediatedClaudeMCPDownstreamName(command, args)
		if isMediated && downstreamName == "" {
			warnings = append(warnings, fmt.Sprintf("mcpServers.%s is missing configured --downstream-name", name))
			continue
		}
		if isMediated && !configuredMCPProxyExists(configured, downstreamName) {
			warnings = append(warnings, fmt.Sprintf("mcpServers.%s references unknown downstream MCP proxy %q", name, downstreamName))
			continue
		}
		if isMediated {
			mediatedCount++
			continue
		}
		warnings = append(warnings, fmt.Sprintf("mcpServers.%s is not mediated through fuse", name))
	}

	return warnings, mediatedCount
}

func mediatedClaudeMCPDownstreamName(command string, args []string) (string, bool) {
	if filepath.Base(command) != "fuse" {
		return "", false
	}
	if len(args) < 2 || args[0] != "proxy" || args[1] != "mcp" {
		return "", false
	}
	for i := 2; i < len(args); i++ {
		arg := args[i]
		if arg == "--downstream-name" {
			if i+1 >= len(args) {
				return "", true
			}
			return args[i+1], true
		}
		if strings.HasPrefix(arg, "--downstream-name=") {
			return strings.TrimPrefix(arg, "--downstream-name="), true
		}
	}
	return "", true
}

func configuredMCPProxyExists(configured map[string]struct{}, name string) bool {
	if name == "" {
		return false
	}
	if len(configured) == 0 {
		return false
	}
	_, ok := configured[name]
	return ok
}

func ensureOptionalObject(parent map[string]interface{}, key string) (map[string]interface{}, error) {
	if existing, ok := parent[key].(map[string]interface{}); ok && existing != nil {
		return existing, nil
	}
	if existing, ok := parent[key]; ok && existing != nil {
		return nil, fmt.Errorf("%s must be an object", key)
	}
	created := make(map[string]interface{})
	parent[key] = created
	return created, nil
}

func optionalObjectForValidation(parent map[string]interface{}, key string) (map[string]interface{}, bool, error) {
	if parent == nil {
		return map[string]interface{}{}, false, nil
	}
	existing, ok := parent[key]
	if !ok || existing == nil {
		return map[string]interface{}{}, false, nil
	}
	obj, ok := existing.(map[string]interface{})
	if !ok {
		return nil, false, fmt.Errorf("%s must be an object", key)
	}
	return obj, true, nil
}

func mergeManagedStringList(path string, existing interface{}, managed []string) ([]interface{}, error) {
	values, err := toStringSetInOrder(path, existing)
	if err != nil {
		return nil, err
	}
	for _, want := range managed {
		if !containsOrdered(values, want) {
			values = append(values, want)
		}
	}

	result := make([]interface{}, len(values))
	for i, value := range values {
		result[i] = value
	}
	return result, nil
}

func toStringSetInOrder(path string, existing interface{}) ([]string, error) {
	switch typed := existing.(type) {
	case nil:
		return nil, nil
	case []interface{}:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			str, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("%s must contain only strings", path)
			}
			if !containsOrdered(out, str) {
				out = append(out, str)
			}
		}
		return out, nil
	case []string:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if !containsOrdered(out, item) {
				out = append(out, item)
			}
		}
		return out, nil
	default:
		return nil, fmt.Errorf("%s must be a list of strings", path)
	}
}

func containsOrdered(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func missingManagedEntriesWarning(path string, value interface{}, managed []string) ([]string, error) {
	values, err := toStringSetInOrder(path, value)
	if err != nil {
		return nil, err
	}

	var warnings []string
	for _, want := range managed {
		if !containsOrdered(values, want) {
			warnings = append(warnings, fmt.Sprintf("%s missing %s", path, want))
		}
	}
	return warnings, nil
}

func upgradedDefaultMode(existing interface{}) interface{} {
	switch typed := existing.(type) {
	case string:
		switch typed {
		case "bypassPermissions":
			return "acceptEdits"
		case "acceptEdits", "askUser":
			return typed
		default:
			return typed
		}
	case nil:
		return "acceptEdits"
	default:
		return existing
	}
}

func upgradedDisableBypassMode(existing interface{}) interface{} {
	switch typed := existing.(type) {
	case string:
		if typed == "allow" {
			return "disable"
		}
		return typed
	case nil:
		return "disable"
	default:
		return existing
	}
}

func upgradedBool(path string, existing interface{}, recommended bool) (bool, error) {
	switch typed := existing.(type) {
	case nil:
		return recommended, nil
	case bool:
		if recommended {
			return typed || recommended, nil
		}
		return false, nil
	default:
		return false, fmt.Errorf("%s must be a boolean", path)
	}
}

func readBool(path string, existing interface{}) (bool, error) {
	switch typed := existing.(type) {
	case bool:
		return typed, nil
	case nil:
		return false, fmt.Errorf("%s is missing", path)
	default:
		return false, fmt.Errorf("%s must be a boolean", path)
	}
}

func defaultModeWarning(existing interface{}) string {
	switch typed := existing.(type) {
	case nil:
		return "permissions.defaultMode is missing or weaker than the recommended secure posture"
	case string:
		switch typed {
		case "acceptEdits", "askUser":
			return ""
		default:
			return fmt.Sprintf("permissions.defaultMode=%q is missing or weaker than the recommended secure posture", typed)
		}
	default:
		return "permissions.defaultMode has an invalid type"
	}
}

func disableBypassWarning(existing interface{}) string {
	switch typed := existing.(type) {
	case nil:
		return "permissions.disableBypassPermissionsMode is missing or weaker than the recommended secure posture"
	case string:
		if typed == "disable" {
			return ""
		}
		return fmt.Sprintf("permissions.disableBypassPermissionsMode=%q should be \"disable\"", typed)
	default:
		return "permissions.disableBypassPermissionsMode has an invalid type"
	}
}

func tomlSection(content, header string) (string, bool) {
	start := strings.Index(content, header+"\n")
	if start < 0 {
		if strings.HasSuffix(content, header) {
			return header, true
		}
		return "", false
	}
	end := nextTOMLSectionBoundary(content, start+len(header)+1)
	return content[start:end], true
}

func tomlAssignment(section, key string) string {
	lines := strings.Split(section, "\n")
	for i := 0; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if !strings.HasPrefix(line, key) {
			continue
		}

		remainder := strings.TrimSpace(strings.TrimPrefix(line, key))
		if !strings.HasPrefix(remainder, "=") {
			continue
		}

		value := strings.TrimSpace(strings.TrimPrefix(remainder, "="))
		if value == "" {
			return ""
		}
		if strings.HasPrefix(value, "[") && !hasBalancedTOMLBrackets(value) {
			var builder strings.Builder
			builder.WriteString(value)
			for j := i + 1; j < len(lines); j++ {
				next := strings.TrimSpace(lines[j])
				if next == "" || strings.HasPrefix(next, "#") {
					continue
				}
				builder.WriteString("\n")
				builder.WriteString(next)
				if hasBalancedTOMLBrackets(builder.String()) {
					return builder.String()
				}
			}
		}
		return value
	}
	return ""
}

func hasBalancedTOMLBrackets(value string) bool {
	depth := 0
	inString := false
	escaped := false

	for _, r := range value {
		switch {
		case escaped:
			escaped = false
		case r == '\\' && inString:
			escaped = true
		case r == '"':
			inString = !inString
		case !inString && r == '[':
			depth++
		case !inString && r == ']':
			depth--
			if depth == 0 {
				return true
			}
		default:
			// Other characters: no action needed.
		}
	}

	return false
}

func isExpectedCodexArgs(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || !strings.HasPrefix(value, "[") || !strings.HasSuffix(value, "]") {
		return false
	}

	inner := strings.TrimSpace(value[1 : len(value)-1])
	if inner == "" {
		return false
	}

	parts := strings.Split(inner, ",")
	var args []string
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		if len(trimmed) < 2 || trimmed[0] != '"' || trimmed[len(trimmed)-1] != '"' {
			return false
		}
		args = append(args, trimmed[1:len(trimmed)-1])
	}

	return len(args) == 2 && args[0] == "proxy" && args[1] == "codex-shell"
}
