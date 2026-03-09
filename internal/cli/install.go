package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

var installCmd = &cobra.Command{
	Use:   "install [claude|codex]",
	Short: "Install fuse as a hook for an AI coding agent",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		target := args[0]
		switch target {
		case "claude":
			return installClaude()
		case "codex":
			fmt.Println("Codex integration is not yet implemented.")
			fmt.Println("This will be available in a future release.")
			return nil
		default:
			return fmt.Errorf("unknown install target %q (supported: claude, codex)", target)
		}
	},
}

func init() {
	rootCmd.AddCommand(installCmd)
}

// fuseHookEntry is the hook configuration entry for a single matcher.
type fuseHookEntry struct {
	Type    string `json:"type"`
	Command string `json:"command"`
	Timeout int    `json:"timeout"`
}

// fuseMatcherEntry is a PreToolUse matcher entry.
type fuseMatcherEntry struct {
	Matcher string          `json:"matcher"`
	Hooks   []fuseHookEntry `json:"hooks"`
}

// claudeSettingsPath returns the path to ~/.claude/settings.json.
func claudeSettingsPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), ".claude", "settings.json")
	}
	return filepath.Join(home, ".claude", "settings.json")
}

// installClaude installs fuse as a Claude Code PreToolUse hook.
func installClaude() error {
	settingsPath := claudeSettingsPath()

	// Read existing settings or start with empty object.
	settings, err := readJSONFile(settingsPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("reading %s: %w", settingsPath, err)
	}
	if settings == nil {
		settings = make(map[string]interface{})
	}

	// Merge the fuse hook into settings.
	mergeFuseHook(settings)

	// Ensure the directory exists.
	dir := filepath.Dir(settingsPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating directory %s: %w", dir, err)
	}

	// Write back.
	if err := writeJSONFile(settingsPath, settings); err != nil {
		return fmt.Errorf("writing %s: %w", settingsPath, err)
	}

	fmt.Printf("fuse hook installed in %s\n", settingsPath)
	fmt.Println("Claude Code will now use fuse for command safety checks.")
	return nil
}

// mergeFuseHook adds or updates the fuse hook entries in the settings map.
// It preserves all existing settings and non-fuse hooks.
func mergeFuseHook(settings map[string]interface{}) {
	// Ensure hooks object exists.
	hooksObj, _ := settings["hooks"].(map[string]interface{})
	if hooksObj == nil {
		hooksObj = make(map[string]interface{})
		settings["hooks"] = hooksObj
	}

	// Get or create the PreToolUse array.
	var preToolUse []interface{}
	if existing, ok := hooksObj["PreToolUse"]; ok {
		if arr, ok := existing.([]interface{}); ok {
			preToolUse = arr
		}
	}

	// The matchers we want to install.
	wantedMatchers := []fuseMatcherEntry{
		{
			Matcher: "Bash",
			Hooks: []fuseHookEntry{
				{Type: "command", Command: "fuse hook evaluate", Timeout: 30},
			},
		},
	}

	for _, wanted := range wantedMatchers {
		found := false
		for i, entry := range preToolUse {
			entryMap, ok := entry.(map[string]interface{})
			if !ok {
				continue
			}
			matcher, _ := entryMap["matcher"].(string)
			if matcher == wanted.Matcher {
				// Update existing entry: ensure fuse hook is present.
				preToolUse[i] = ensureFuseHookInEntry(entryMap)
				found = true
				break
			}
		}
		if !found {
			// Add new matcher entry.
			newEntry := map[string]interface{}{
				"matcher": wanted.Matcher,
				"hooks":   fuseHooksToInterface(wanted.Hooks),
			}
			preToolUse = append(preToolUse, newEntry)
		}
	}

	hooksObj["PreToolUse"] = preToolUse
}

// ensureFuseHookInEntry ensures the "fuse hook evaluate" command is present
// in the hooks array of a matcher entry. Returns the updated entry.
func ensureFuseHookInEntry(entry map[string]interface{}) map[string]interface{} {
	hooksRaw, ok := entry["hooks"]
	if !ok {
		entry["hooks"] = fuseHooksToInterface([]fuseHookEntry{
			{Type: "command", Command: "fuse hook evaluate", Timeout: 30},
		})
		return entry
	}

	hooks, ok := hooksRaw.([]interface{})
	if !ok {
		entry["hooks"] = fuseHooksToInterface([]fuseHookEntry{
			{Type: "command", Command: "fuse hook evaluate", Timeout: 30},
		})
		return entry
	}

	// Check if fuse hook already exists.
	for _, h := range hooks {
		hMap, ok := h.(map[string]interface{})
		if !ok {
			continue
		}
		cmd, _ := hMap["command"].(string)
		if cmd == "fuse hook evaluate" {
			// Already present — update timeout.
			hMap["timeout"] = float64(30)
			return entry
		}
	}

	// Not present, add it.
	fuseHook := map[string]interface{}{
		"type":    "command",
		"command": "fuse hook evaluate",
		"timeout": float64(30),
	}
	hooks = append(hooks, fuseHook)
	entry["hooks"] = hooks
	return entry
}

// fuseHooksToInterface converts typed hook entries to generic interface slices
// for JSON marshalling.
func fuseHooksToInterface(hooks []fuseHookEntry) []interface{} {
	result := make([]interface{}, len(hooks))
	for i, h := range hooks {
		result[i] = map[string]interface{}{
			"type":    h.Type,
			"command": h.Command,
			"timeout": float64(h.Timeout),
		}
	}
	return result
}

// readJSONFile reads and parses a JSON file into a map.
func readJSONFile(path string) (map[string]interface{}, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("parsing JSON: %w", err)
	}
	return result, nil
}

// writeJSONFile writes a map as indented JSON to a file.
func writeJSONFile(path string, data map[string]interface{}) error {
	out, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("marshalling JSON: %w", err)
	}
	out = append(out, '\n')
	return os.WriteFile(path, out, 0644)
}
