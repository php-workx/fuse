package cli

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/spf13/cobra"

	"github.com/runger/fuse/internal/config"
)

var uninstallPurge bool

var uninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Remove fuse hooks and optionally purge all data",
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

	preToolUseRaw, ok := hooksObj["PreToolUse"]
	if !ok {
		return false
	}

	preToolUse, ok := preToolUseRaw.([]interface{})
	if !ok {
		return false
	}

	modified := false
	var cleaned []interface{}

	for _, entry := range preToolUse {
		entryMap, ok := entry.(map[string]interface{})
		if !ok {
			cleaned = append(cleaned, entry)
			continue
		}

		hooksRaw, ok := entryMap["hooks"]
		if !ok {
			cleaned = append(cleaned, entry)
			continue
		}

		hooks, ok := hooksRaw.([]interface{})
		if !ok {
			cleaned = append(cleaned, entry)
			continue
		}

		// Filter out fuse hook commands.
		var remainingHooks []interface{}
		for _, h := range hooks {
			hMap, ok := h.(map[string]interface{})
			if !ok {
				remainingHooks = append(remainingHooks, h)
				continue
			}
			cmd, _ := hMap["command"].(string)
			if cmd == "fuse hook evaluate" {
				modified = true
				continue // Skip fuse hook.
			}
			remainingHooks = append(remainingHooks, h)
		}

		// If hooks remain, keep the matcher entry; otherwise drop it.
		if len(remainingHooks) > 0 {
			entryMap["hooks"] = remainingHooks
			cleaned = append(cleaned, entryMap)
		} else {
			modified = true
			// Drop the entire matcher entry since it has no hooks left.
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

func uninstallCodex() error {
	configPath := codexConfigPath()
	if err := rejectSymlinkedCodexConfigPath(configPath); err != nil {
		return err
	}
	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	cleaned := removeCodexIntegration(string(data))
	if cleaned == string(data) {
		return nil
	}
	return os.WriteFile(configPath, []byte(cleaned), 0o644)
}

func removeCodexIntegration(existing string) string {
	header := "[mcp_servers.fuse-shell]\n"
	start := strings.Index(existing, header)
	if start >= 0 {
		end := nextTOMLSectionBoundary(existing, start+len(header))
		existing = existing[:start] + existing[end:]
	}

	featuresRe := regexp.MustCompile(`(?m)^shell_tool\s*=\s*false\s*$`)
	existing = featuresRe.ReplaceAllString(existing, "")

	blankLines := regexp.MustCompile(`\n{3,}`)
	existing = blankLines.ReplaceAllString(existing, "\n\n")
	return strings.TrimSpace(existing) + "\n"
}
