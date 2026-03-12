package cli

import "fmt"

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
