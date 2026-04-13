package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/spf13/cobra"

	"github.com/php-workx/fuse/internal/config"
)

var uninstallPurge bool

var uninstallCmd = &cobra.Command{
	Use:     "uninstall",
	Short:   "Remove fuse hooks and optionally purge all data",
	GroupID: groupSetup,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Step 1: Remove fuse hook entries from Claude Code settings.json.
		if err := uninstallClaude(); err != nil {
			fmt.Fprintf(os.Stderr, "warning: could not clean Claude Code settings: %v\n", err)
		}
		if err := uninstallCodex(); err != nil {
			fmt.Fprintf(os.Stderr, "warning: could not clean Codex config: %v\n", err)
		}

		// Step 2: With --purge, remove ~/.fuse/ entirely.
		if uninstallPurge {
			baseDir := config.BaseDir()
			if err := os.RemoveAll(baseDir); err != nil {
				return fmt.Errorf("removing %s: %w", baseDir, err)
			}
			fmt.Printf("Removed %s\n", baseDir)
		}

		fmt.Println("fuse has been uninstalled.")
		return nil
	},
}

func init() {
	uninstallCmd.Flags().BoolVar(&uninstallPurge, "purge", false, "Also remove ~/.fuse/ directory entirely")
	rootCmd.AddCommand(uninstallCmd)
}

// uninstallClaude removes fuse hook entries from Claude Code's settings.json.
func uninstallClaude() error {
	settingsPath := claudeSettingsPath()

	settings, err := readJSONFile(settingsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // Nothing to clean.
		}
		return fmt.Errorf("reading %s: %w", settingsPath, err)
	}

	if removeFuseHook(settings) {
		if err := writeJSONFile(settingsPath, settings); err != nil {
			return fmt.Errorf("writing %s: %w", settingsPath, err)
		}
		fmt.Printf("Removed fuse hook from %s\n", settingsPath)
	}

	return nil
}

// removeFuseHook removes fuse hook entries from a settings map.
// Returns true if any modifications were made.
func removeFuseHook(settings map[string]interface{}) bool {
	hooksObj, ok := settings["hooks"].(map[string]interface{})
	if !ok {
		return false
	}

	preToolUse, ok := extractPreToolUse(hooksObj)
	if !ok {
		return false
	}

	modified := false
	var cleaned []interface{}

	for _, entry := range preToolUse {
		kept, wasModified := cleanMatcherEntry(entry)
		if wasModified {
			modified = true
		}
		if kept != nil {
			cleaned = append(cleaned, kept)
		}
	}

	if modified {
		if len(cleaned) > 0 {
			hooksObj["PreToolUse"] = cleaned
		} else {
			delete(hooksObj, "PreToolUse")
		}
	}

	return modified
}

// extractPreToolUse retrieves the PreToolUse array from the hooks object.
func extractPreToolUse(hooksObj map[string]interface{}) ([]interface{}, bool) {
	preToolUseRaw, ok := hooksObj["PreToolUse"]
	if !ok {
		return nil, false
	}
	preToolUse, ok := preToolUseRaw.([]interface{})
	return preToolUse, ok
}

// cleanMatcherEntry processes a single PreToolUse matcher entry, removing
// fuse hooks. Returns the cleaned entry (nil if dropped) and whether it was modified.
func cleanMatcherEntry(entry interface{}) (interface{}, bool) {
	entryMap, ok := entry.(map[string]interface{})
	if !ok {
		return entry, false
	}

	hooks, ok := extractHooksArray(entryMap)
	if !ok {
		return entry, false
	}

	remainingHooks, removed := filterFuseHooks(hooks)

	if len(remainingHooks) > 0 {
		entryMap["hooks"] = remainingHooks
		return entryMap, removed
	}
	// Drop the entire matcher entry since it has no hooks left.
	return nil, true
}

// extractHooksArray retrieves the hooks array from a matcher entry map.
func extractHooksArray(entryMap map[string]interface{}) ([]interface{}, bool) {
	hooksRaw, ok := entryMap["hooks"]
	if !ok {
		return nil, false
	}
	hooks, ok := hooksRaw.([]interface{})
	return hooks, ok
}

// filterFuseHooks filters out fuse hook commands from a hooks array.
// Returns the remaining hooks and whether any were removed.
func filterFuseHooks(hooks []interface{}) ([]interface{}, bool) {
	var remaining []interface{}
	removed := false
	for _, h := range hooks {
		hMap, ok := h.(map[string]interface{})
		if !ok {
			remaining = append(remaining, h)
			continue
		}
		cmd, _ := hMap["command"].(string)
		if isFuseHookCommand(cmd) {
			removed = true
			continue
		}
		remaining = append(remaining, h)
	}
	return remaining, removed
}

func isFuseHookCommand(cmd string) bool {
	return strings.Contains(cmd, fuseHookCommand)
}

func uninstallCodex() error {
	configPath := codexConfigPath()
	if err := rejectSymlinkedCodexConfigPath(configPath); err != nil {
		return err
	}
	data, err := os.ReadFile(configPath)
	if err != nil {
		if !os.IsNotExist(err) {
			return err
		}
	} else {
		cleaned := removeCodexIntegration(string(data))
		if cleaned != string(data) {
			if err := os.WriteFile(configPath, []byte(cleaned), 0o644); err != nil {
				return err
			}
		}
	}

	return removeCodexHooksFile(codexHooksPathFromConfig(configPath))
}

func removeCodexHooksFile(hooksPath string) error {
	hooksData, err := os.ReadFile(hooksPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	cleanedHooks, modified, err := removeCodexHooksJSON(hooksData)
	if err != nil {
		return fmt.Errorf("cleaning %s: %w", hooksPath, err)
	}
	if modified {
		return os.WriteFile(hooksPath, cleanedHooks, 0o644)
	}
	return nil
}

func removeCodexHooksJSON(existing []byte) ([]byte, bool, error) {
	var settings map[string]interface{}
	if err := json.Unmarshal(existing, &settings); err != nil {
		return nil, false, err
	}
	modified := removeFuseHook(settings)
	if !modified {
		return existing, false, nil
	}
	out, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return nil, false, err
	}
	return append(out, '\n'), true, nil
}

func removeCodexIntegration(existing string) string {
	fuseShellSectionRe := regexp.MustCompile(`(?m)^\[mcp_servers\.fuse-shell(?:\]|\.)`)
	for {
		loc := fuseShellSectionRe.FindStringIndex(existing)
		if loc == nil {
			break
		}
		end := nextTOMLSectionBoundary(existing, loc[1])
		existing = existing[:loc[0]] + existing[end:]
	}

	featuresRe := regexp.MustCompile(`(?m)^shell_tool\s*=\s*false\s*$`)
	existing = featuresRe.ReplaceAllString(existing, "")

	blankLines := regexp.MustCompile(`\n{3,}`)
	existing = blankLines.ReplaceAllString(existing, "\n\n")
	return strings.TrimSpace(existing) + "\n"
}
