package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

const (
	codexConfigDir  = ".codex"
	codexConfigFile = "config.toml"

	// fuseHookCommand is the command string used in PreToolUse hook entries.
	fuseHookCommand = "fuse hook evaluate"

	codexFuseHookCommand = "FUSE_HOOK_AGENT=codex " + fuseHookCommand
)

var detectCodexNativeHooksSupport = defaultDetectCodexNativeHooksSupport

var installCmd = &cobra.Command{
	Use:     "install [claude|codex]",
	Short:   "Install fuse as a hook for an AI coding agent",
	GroupID: groupSetup,
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		target := args[0]
		if installClaudeSecure && target != "claude" {
			return fmt.Errorf("--secure is only supported for the 'claude' target")
		}
		return runInstallWithSelectedProfile(cmd, target, installClaudeSecure)
	},
}

var installClaudeSecure bool

func init() {
	installCmd.Flags().BoolVar(&installClaudeSecure, "secure", false, "merge recommended secure Claude settings during install")
	rootCmd.AddCommand(installCmd)
}

// fuseHookEntry is the hook configuration entry for a single matcher.
type fuseHookEntry struct {
	Type          string `json:"type"`
	Command       string `json:"command"`
	Timeout       int    `json:"timeout"`
	StatusMessage string `json:"statusMessage,omitempty"`
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

func rejectSymlinkedClaudeSettingsPath(settingsPath string) error {
	// Walk ancestors from settingsPath up to (but not including) the user's
	// home directory. System-level symlinks (e.g., /var -> /private/var on
	// macOS) are normal and should not be flagged.
	homeDir, _ := os.UserHomeDir()
	cleanHome := filepath.Clean(homeDir)
	for candidate := filepath.Clean(settingsPath); ; {
		// Stop at home directory — system paths above it may have normal symlinks.
		if homeDir != "" && candidate == cleanHome {
			break
		}
		next, err := checkAncestorSymlink(candidate)
		if err != nil {
			return err
		}
		if next == "" {
			break
		}
		candidate = next
	}
	return nil
}

// checkAncestorSymlink checks whether a path component is a symlink.
// Returns the parent path to continue walking, or "" to stop.
func checkAncestorSymlink(candidate string) (string, error) {
	info, err := os.Lstat(candidate)
	if err != nil {
		if os.IsNotExist(err) {
			parent := filepath.Dir(candidate)
			if parent == candidate {
				return "", nil
			}
			return parent, nil
		}
		return "", fmt.Errorf("inspect %s: %w", candidate, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("refusing to use symlinked Claude settings path %s", candidate)
	}
	parent := filepath.Dir(candidate)
	if parent == candidate {
		return "", nil
	}
	return parent, nil
}

// installClaude installs fuse as a Claude Code PreToolUse hook.
func installClaude(secure bool) error {
	return installClaudeWithProfile(secure, "")
}

func installClaudeWithProfile(secure bool, profile string) error {
	settingsPath := claudeSettingsPath()
	if err := rejectSymlinkedClaudeSettingsPath(settingsPath); err != nil {
		return err
	}

	// Read existing settings or start with empty object.
	settings, err := readJSONFile(settingsPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("reading %s: %w", settingsPath, err)
	}
	if settings == nil {
		settings = make(map[string]interface{})
	}

	if secure {
		if err := mergeClaudeSecureSettings(settings); err != nil {
			return fmt.Errorf("merge secure Claude settings: %w", err)
		}
	}

	// Merge the fuse hook into settings.
	mergeFuseHook(settings)
	if secure {
		mergeFuseSecureNativeFileHooks(settings)
	}

	// Ensure the directory exists.
	dir := filepath.Dir(settingsPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating directory %s: %w", dir, err)
	}

	// Write back.
	if err := writeJSONFile(settingsPath, settings); err != nil {
		return fmt.Errorf("writing %s: %w", settingsPath, err)
	}

	if err := ensureFuseConfigScaffold(profile); err != nil {
		return err
	}

	if err := ensureFuseStateDB(); err != nil {
		return err
	}

	fmt.Printf("fuse hook installed in %s\n", settingsPath)
	printHookBinaryInfo()
	fmt.Println("Claude Code will now use fuse for command safety checks.")
	return nil
}

// mergeFuseHook adds or updates the fuse hook entries in the settings map.
// It preserves all existing settings and non-fuse hooks.
func mergeFuseHook(settings map[string]interface{}) {
	mergeFuseHookMatchers(settings, []string{"Bash", "mcp__.*"})
}

func mergeFuseSecureNativeFileHooks(settings map[string]interface{}) {
	mergeFuseHookMatchers(settings, []string{"Read", "Write", "Edit", "MultiEdit"})
}

func mergeFuseHookMatchers(settings map[string]interface{}, matchers []string) {
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

	wantedMatchers := make([]fuseMatcherEntry, 0, len(matchers))
	for _, matcher := range matchers {
		wantedMatchers = append(wantedMatchers, fuseMatcherEntry{
			Matcher: matcher,
			Hooks: []fuseHookEntry{
				{Type: "command", Command: fuseHookCommand, Timeout: 300},
			},
		})
	}

	for _, wanted := range wantedMatchers {
		preToolUse = upsertMatcherEntry(preToolUse, wanted)
	}

	hooksObj["PreToolUse"] = preToolUse
}

// upsertMatcherEntry updates an existing matcher entry or appends a new one.
func upsertMatcherEntry(preToolUse []interface{}, wanted fuseMatcherEntry) []interface{} {
	for i, entry := range preToolUse {
		entryMap, ok := entry.(map[string]interface{})
		if !ok {
			continue
		}
		matcher, _ := entryMap["matcher"].(string)
		if matcher == wanted.Matcher {
			preToolUse[i] = ensureFuseHookInEntry(entryMap)
			return preToolUse
		}
	}
	// Not found — add new matcher entry.
	newEntry := map[string]interface{}{
		"matcher": wanted.Matcher,
		"hooks":   fuseHooksToInterface(wanted.Hooks),
	}
	return append(preToolUse, newEntry)
}

// ensureFuseHookInEntry ensures the "fuse hook evaluate" command is present
// in the hooks array of a matcher entry. Returns the updated entry.
func ensureFuseHookInEntry(entry map[string]interface{}) map[string]interface{} {
	hooksRaw, ok := entry["hooks"]
	if !ok {
		entry["hooks"] = fuseHooksToInterface([]fuseHookEntry{
			{Type: "command", Command: fuseHookCommand, Timeout: 300},
		})
		return entry
	}

	hooks, ok := hooksRaw.([]interface{})
	if !ok {
		entry["hooks"] = fuseHooksToInterface([]fuseHookEntry{
			{Type: "command", Command: fuseHookCommand, Timeout: 300},
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
		if cmd == fuseHookCommand {
			// Already present — update timeout.
			hMap["timeout"] = float64(300)
			return entry
		}
	}

	// Not present, add it.
	fuseHook := map[string]interface{}{
		"type":    "command",
		"command": fuseHookCommand,
		"timeout": float64(300),
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
		entry := map[string]interface{}{
			"type":    h.Type,
			"command": h.Command,
			"timeout": float64(h.Timeout),
		}
		if h.StatusMessage != "" {
			entry["statusMessage"] = h.StatusMessage
		}
		result[i] = entry
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
	return os.WriteFile(path, out, 0o644)
}

func codexConfigPath() string {
	if cwd, err := os.Getwd(); err == nil {
		localPath := filepath.Join(cwd, codexConfigDir, codexConfigFile)
		if _, statErr := os.Stat(localPath); statErr == nil {
			return localPath
		}
	}
	if home := os.Getenv("CODEX_HOME"); home != "" {
		return filepath.Join(home, codexConfigFile)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), codexConfigDir, codexConfigFile)
	}
	return filepath.Join(home, codexConfigDir, codexConfigFile)
}

func rejectSymlinkedCodexConfigPath(configPath string) error {
	absPath, err := filepath.Abs(configPath)
	if err != nil {
		return fmt.Errorf("resolve config path: %w", err)
	}
	cwd, cwdErr := os.Getwd()
	if cwdErr != nil {
		return nil //nolint:nilerr // non-critical: can't determine cwd, skip symlink check
	}
	absCwd, absCwdErr := filepath.Abs(cwd)
	if absCwdErr != nil {
		return nil //nolint:nilerr // non-critical: can't resolve cwd, skip symlink check
	}
	rel, relErr := filepath.Rel(absCwd, absPath)
	if relErr != nil || strings.HasPrefix(rel, "..") {
		return nil //nolint:nilerr // relErr means path resolution failed, skip check gracefully
	}

	for _, candidate := range []string{filepath.Join(absCwd, codexConfigDir), absPath} {
		info, err := os.Lstat(candidate)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return fmt.Errorf("inspect %s: %w", candidate, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("refusing to use symlinked Codex config path %s", candidate)
		}
	}
	return nil
}

func installCodex() error {
	return installCodexWithProfile("")
}

func installCodexWithProfile(profile string) error {
	configPath := codexConfigPath()
	if err := rejectSymlinkedCodexConfigPath(configPath); err != nil {
		return err
	}
	existing, err := os.ReadFile(configPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("reading %s: %w", configPath, err)
	}

	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		return fmt.Errorf("creating directory %s: %w", filepath.Dir(configPath), err)
	}

	if detectCodexNativeHooksSupport() {
		updated := mergeCodexNativeHooksConfig(string(existing))
		if err := os.WriteFile(configPath, []byte(updated), 0o644); err != nil {
			return fmt.Errorf("writing %s: %w", configPath, err)
		}

		hooksPath := codexHooksPathFromConfig(configPath)
		hooksExisting, err := os.ReadFile(hooksPath)
		if err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("reading %s: %w", hooksPath, err)
		}
		mergedHooks, err := mergeCodexHooksJSON(hooksExisting)
		if err != nil {
			return fmt.Errorf("merging %s: %w", hooksPath, err)
		}
		if err := os.WriteFile(hooksPath, mergedHooks, 0o644); err != nil {
			return fmt.Errorf("writing %s: %w", hooksPath, err)
		}

		if err := ensureFuseConfigScaffold(profile); err != nil {
			return err
		}

		fmt.Printf("fuse Codex native hook installed in %s\n", hooksPath)
		printHookBinaryInfo()
		return nil
	}

	updated := mergeCodexConfig(string(existing))
	if err := os.WriteFile(configPath, []byte(updated), 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", configPath, err)
	}
	if err := removeCodexHooksFile(codexHooksPathFromConfig(configPath)); err != nil {
		return err
	}

	if err := ensureFuseConfigScaffold(profile); err != nil {
		return err
	}

	if err := ensureFuseStateDB(); err != nil {
		return err
	}

	fmt.Printf("fuse Codex MCP server installed in %s\n", configPath)
	printHookBinaryInfo()
	return nil
}

func printHookBinaryInfo() {
	status, err := resolveHookBinaryStatus()
	if err != nil {
		fmt.Println("Hook binary: fuse (not found in PATH; run `go install .` or install a release, then rerun `fuse doctor`)")
		return
	}
	fmt.Println("Hook binary:", describeFuseBinary(status.HookPath))

	// Same file or same version: hooks will run the current build. Nothing
	// more to report.
	if status.PathsMatch || status.VersionsMatch {
		return
	}
	// Without build metadata we cannot claim a mismatch; a `go install`
	// without ldflags also leaves Version="dev" on the installed binary,
	// so a version-string comparison would produce false positives.
	if !status.VersionKnown {
		return
	}
	fmt.Printf("WARNING: hook binary appears stale or mismatched with current build (%s).\n", status.SelfVersion)
	fmt.Println("Reinstall fuse (e.g. `go install ./cmd/fuse` or download the latest release) so the hook uses the current build.")
}

func defaultDetectCodexNativeHooksSupport() bool {
	if runtime.GOOS == "windows" {
		return false
	}
	codexPath, err := exec.LookPath("codex")
	if err != nil {
		return false
	}
	output, err := exec.Command(codexPath, "--version").CombinedOutput()
	if err != nil {
		return false
	}
	return codexVersionSupportsNativeHooks(runtime.GOOS, string(output))
}

func codexVersionSupportsNativeHooks(goos, output string) bool {
	if goos == "windows" {
		return false
	}
	versionRe := regexp.MustCompile(`\b(\d+)\.(\d+)\.(\d+)\b`)
	matches := versionRe.FindStringSubmatch(output)
	if matches == nil {
		return false
	}
	major, _ := strconv.Atoi(matches[1])
	minor, _ := strconv.Atoi(matches[2])
	patch, _ := strconv.Atoi(matches[3])
	if major != 0 {
		return major > 0
	}
	if minor != 115 {
		return minor > 115
	}
	return patch >= 0
}

func codexHooksPathFromConfig(configPath string) string {
	return filepath.Join(filepath.Dir(configPath), "hooks.json")
}

func mergeCodexConfig(existing string) string {
	result := strings.TrimSpace(existing)
	result = upsertTOMLAssignment(result, `(?ms)^\[features\]\n(?:[^\[]*\n)?`, "[features]\n", "shell_tool = false")
	result = upsertTOMLSection(result, "[mcp_servers.fuse-shell]", `command = "fuse"`+"\n"+`args = ["proxy", "codex-shell"]`)
	if !strings.HasSuffix(result, "\n") {
		result += "\n"
	}
	return result
}

func mergeCodexNativeHooksConfig(existing string) string {
	result := strings.TrimSpace(removeCodexIntegration(existing))
	result = upsertTOMLAssignment(result, `(?ms)^\[features\]\n(?:[^\[]*\n)?`, "[features]\n", "codex_hooks = true")
	if !strings.HasSuffix(result, "\n") {
		result += "\n"
	}
	return result
}

func mergeCodexHooksJSON(existing []byte) ([]byte, error) {
	var settings map[string]interface{}
	if strings.TrimSpace(string(existing)) == "" {
		settings = make(map[string]interface{})
	} else if err := json.Unmarshal(existing, &settings); err != nil {
		return nil, err
	}

	hooksObj, _ := settings["hooks"].(map[string]interface{})
	if hooksObj == nil {
		hooksObj = make(map[string]interface{})
		settings["hooks"] = hooksObj
	}

	preToolUse, _ := extractPreToolUse(hooksObj)
	hooksObj["PreToolUse"] = upsertCodexFuseHook(preToolUse)

	out, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(out, '\n'), nil
}

func upsertCodexFuseHook(preToolUse []interface{}) []interface{} {
	wanted := map[string]interface{}{
		"type":          "command",
		"command":       codexFuseHookCommand,
		"timeout":       float64(300),
		"statusMessage": "Fuse checking command",
	}
	for i, raw := range preToolUse {
		entry, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		matcher, _ := entry["matcher"].(string)
		if matcher != "Bash" {
			continue
		}
		hooks, _ := extractHooksArray(entry)
		filtered, _ := filterFuseHooks(hooks)
		entry["hooks"] = append(filtered, wanted)
		preToolUse[i] = entry
		return preToolUse
	}
	return append(preToolUse, map[string]interface{}{
		"matcher": "Bash",
		"hooks":   []interface{}{wanted},
	})
}

func upsertTOMLAssignment(existing, sectionPattern, sectionHeader, assignment string) string {
	sectionRe := regexp.MustCompile(sectionPattern)
	loc := sectionRe.FindStringIndex(existing)
	if loc == nil {
		if existing != "" {
			existing += "\n\n"
		}
		return existing + sectionHeader + assignment + "\n"
	}

	start := loc[0]
	sectionEnd := nextTOMLSectionBoundary(existing, loc[1])
	section := existing[start:sectionEnd]
	key := strings.TrimSpace(strings.SplitN(assignment, "=", 2)[0])
	assignRe := regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(key) + `\s*=.*$`)
	if assignRe.MatchString(section) {
		section = assignRe.ReplaceAllString(section, assignment)
	} else {
		section = strings.TrimRight(section, "\n") + "\n" + assignment + "\n"
	}
	return existing[:start] + section + existing[sectionEnd:]
}

func upsertTOMLSection(existing, header, body string) string {
	replacement := header + "\n" + body + "\n"
	start := strings.Index(existing, header+"\n")
	if start >= 0 {
		end := nextTOMLSectionBoundary(existing, start+len(header)+1)
		return existing[:start] + replacement + existing[end:]
	}
	if existing != "" {
		existing += "\n\n"
	}
	return existing + replacement
}

func nextTOMLSectionBoundary(s string, from int) int {
	if from >= len(s) {
		return len(s)
	}
	re := regexp.MustCompile(`(?m)^\[`)
	if loc := re.FindStringIndex(s[from:]); loc != nil {
		return from + loc[0]
	}
	return len(s)
}
