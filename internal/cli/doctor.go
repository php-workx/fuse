package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/runger/fuse/internal/config"
	"github.com/runger/fuse/internal/core"
	"github.com/runger/fuse/internal/db"
	"github.com/runger/fuse/internal/policy"
	"github.com/spf13/cobra"
)

var doctorLive bool

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Run diagnostic checks on fuse setup",
	Long:  "Checks configuration, directory structure, hook installation, and optionally runs a live test.",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runDoctor(doctorLive)
	},
}

func init() {
	doctorCmd.Flags().BoolVar(&doctorLive, "live", false, "Run a live classification test")
	rootCmd.AddCommand(doctorCmd)
}

type checkResult struct {
	name   string
	status string // "PASS", "FAIL", "WARN"
	detail string
}

func runDoctor(live bool) error {
	var results []checkResult

	results = append(results, checkGoVersion())
	results = append(results, checkDirectoryStructure())
	results = append(results, checkConfigYAML())
	results = append(results, checkPolicyYAML())
	results = append(results, checkClaudeSettings())
	results = append(results, checkSQLiteDB())
	results = append(results, checkFuseInPath())

	if live {
		results = append(results, checkLiveClassification())
	}

	// Print results.
	fmt.Println("fuse doctor")
	fmt.Println("===========")
	fmt.Println()

	passes := 0
	fails := 0
	warns := 0

	for _, r := range results {
		icon := "[ PASS ]"
		switch r.status {
		case "FAIL":
			icon = "[ FAIL ]"
			fails++
		case "WARN":
			icon = "[ WARN ]"
			warns++
		default:
			passes++
		}
		fmt.Printf("  %s  %s\n", icon, r.name)
		if r.detail != "" {
			fmt.Printf("           %s\n", r.detail)
		}
	}

	fmt.Println()
	fmt.Printf("Results: %d passed, %d warnings, %d failed\n", passes, warns, fails)

	if fails > 0 {
		return fmt.Errorf("%d check(s) failed", fails)
	}
	return nil
}

// checkGoVersion checks that Go is available and reports the version.
func checkGoVersion() checkResult {
	goVersion := runtime.Version()
	return checkResult{
		name:   "Go runtime",
		status: "PASS",
		detail: goVersion,
	}
}

// checkDirectoryStructure checks that ~/.fuse/ and subdirectories exist
// with correct permissions.
func checkDirectoryStructure() checkResult {
	baseDir := config.BaseDir()

	dirs := []struct {
		path string
		perm os.FileMode
	}{
		{baseDir, 0},
		{config.ConfigDir(), 0755},
		{config.StateDir(), 0700},
	}

	for _, d := range dirs {
		info, err := os.Stat(d.path)
		if err != nil {
			if os.IsNotExist(err) {
				return checkResult{
					name:   "Directory structure",
					status: "WARN",
					detail: fmt.Sprintf("%s does not exist (will be created on first use)", d.path),
				}
			}
			return checkResult{
				name:   "Directory structure",
				status: "FAIL",
				detail: fmt.Sprintf("cannot stat %s: %v", d.path, err),
			}
		}
		if !info.IsDir() {
			return checkResult{
				name:   "Directory structure",
				status: "FAIL",
				detail: fmt.Sprintf("%s exists but is not a directory", d.path),
			}
		}
		if d.perm != 0 {
			actual := info.Mode().Perm()
			if actual != d.perm {
				return checkResult{
					name:   "Directory structure",
					status: "WARN",
					detail: fmt.Sprintf("%s has permissions %o (expected %o)", d.path, actual, d.perm),
				}
			}
		}
	}

	return checkResult{
		name:   "Directory structure",
		status: "PASS",
		detail: baseDir,
	}
}

// checkConfigYAML checks that config.yaml is readable (or missing = OK with defaults).
func checkConfigYAML() checkResult {
	cfgPath := config.ConfigPath()

	_, err := config.LoadConfig(cfgPath)
	if err != nil {
		return checkResult{
			name:   "Configuration (config.yaml)",
			status: "FAIL",
			detail: fmt.Sprintf("error loading config: %v", err),
		}
	}

	if _, statErr := os.Stat(cfgPath); os.IsNotExist(statErr) {
		return checkResult{
			name:   "Configuration (config.yaml)",
			status: "PASS",
			detail: "not present (using defaults)",
		}
	}

	return checkResult{
		name:   "Configuration (config.yaml)",
		status: "PASS",
		detail: cfgPath,
	}
}

// checkPolicyYAML checks that policy.yaml is valid if present.
func checkPolicyYAML() checkResult {
	policyPath := config.PolicyPath()

	if _, err := os.Stat(policyPath); os.IsNotExist(err) {
		return checkResult{
			name:   "Policy (policy.yaml)",
			status: "PASS",
			detail: "not present (using built-in rules only)",
		}
	}

	pol, err := policy.LoadPolicy(policyPath)
	if err != nil {
		return checkResult{
			name:   "Policy (policy.yaml)",
			status: "FAIL",
			detail: fmt.Sprintf("error loading policy: %v", err),
		}
	}

	return checkResult{
		name:   "Policy (policy.yaml)",
		status: "PASS",
		detail: fmt.Sprintf("%d rules loaded (version: %s)", len(pol.Rules), pol.Version),
	}
}

// checkClaudeSettings checks that Claude Code's settings.json has the fuse hook.
func checkClaudeSettings() checkResult {
	settingsPath := claudeSettingsPath()

	settings, err := readJSONFile(settingsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return checkResult{
				name:   "Claude Code hook",
				status: "WARN",
				detail: fmt.Sprintf("%s not found (run 'fuse install claude')", settingsPath),
			}
		}
		return checkResult{
			name:   "Claude Code hook",
			status: "FAIL",
			detail: fmt.Sprintf("error reading settings: %v", err),
		}
	}

	if hasFuseHook(settings) {
		return checkResult{
			name:   "Claude Code hook",
			status: "PASS",
			detail: "fuse hook found in PreToolUse",
		}
	}

	return checkResult{
		name:   "Claude Code hook",
		status: "WARN",
		detail: fmt.Sprintf("fuse hook not found in %s (run 'fuse install claude')", settingsPath),
	}
}

// hasFuseHook checks if the settings map contains a fuse hook entry.
func hasFuseHook(settings map[string]interface{}) bool {
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

	for _, entry := range preToolUse {
		entryMap, ok := entry.(map[string]interface{})
		if !ok {
			continue
		}
		hooksRaw, ok := entryMap["hooks"]
		if !ok {
			continue
		}
		hooks, ok := hooksRaw.([]interface{})
		if !ok {
			continue
		}
		for _, h := range hooks {
			hMap, ok := h.(map[string]interface{})
			if !ok {
				continue
			}
			cmd, _ := hMap["command"].(string)
			if cmd == "fuse hook evaluate" {
				return true
			}
		}
	}

	return false
}

// checkSQLiteDB checks that the SQLite database is accessible if it exists.
func checkSQLiteDB() checkResult {
	dbPath := config.DBPath()

	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		return checkResult{
			name:   "SQLite database",
			status: "PASS",
			detail: "not yet created (will be created on first use)",
		}
	}

	database, err := db.OpenDB(dbPath)
	if err != nil {
		return checkResult{
			name:   "SQLite database",
			status: "FAIL",
			detail: fmt.Sprintf("error opening database: %v", err),
		}
	}
	_ = database.Close()

	return checkResult{
		name:   "SQLite database",
		status: "PASS",
		detail: dbPath,
	}
}

// checkFuseInPath checks that the fuse binary is in PATH.
func checkFuseInPath() checkResult {
	fusePath, err := exec.LookPath("fuse")
	if err != nil {
		// Also check if the current binary name is in PATH.
		selfPath, selfErr := os.Executable()
		if selfErr == nil {
			base := filepath.Base(selfPath)
			if foundPath, lookErr := exec.LookPath(base); lookErr == nil {
				return checkResult{
					name:   "fuse binary in PATH",
					status: "PASS",
					detail: foundPath,
				}
			}
		}
		return checkResult{
			name:   "fuse binary in PATH",
			status: "WARN",
			detail: "fuse not found in PATH (hooks may not work)",
		}
	}

	return checkResult{
		name:   "fuse binary in PATH",
		status: "PASS",
		detail: fusePath,
	}
}

// checkLiveClassification runs a test classification to verify the pipeline.
func checkLiveClassification() checkResult {
	req := core.ShellRequest{
		RawCommand: "echo hello",
		Cwd:        "/tmp",
		Source:     "doctor",
	}

	// Load policy if available.
	var evaluator core.PolicyEvaluator
	policyPath := config.PolicyPath()
	if _, err := os.Stat(policyPath); err == nil {
		pol, loadErr := policy.LoadPolicy(policyPath)
		if loadErr != nil {
			fmt.Fprintf(os.Stderr, "warning: policy.yaml exists but failed to parse: %v\n", loadErr)
		}
		if pol != nil {
			evaluator = policy.NewEvaluator(pol)
		}
	}
	if evaluator == nil {
		evaluator = policy.NewEvaluator(nil)
	}

	result, err := core.Classify(req, evaluator)
	if err != nil {
		return checkResult{
			name:   "Live classification test",
			status: "FAIL",
			detail: fmt.Sprintf("classification error: %v", err),
		}
	}

	if result.Decision != core.DecisionSafe {
		return checkResult{
			name:   "Live classification test",
			status: "WARN",
			detail: fmt.Sprintf("'echo hello' classified as %s (expected SAFE): %s", result.Decision, result.Reason),
		}
	}

	return checkResult{
		name:   "Live classification test",
		status: "PASS",
		detail: fmt.Sprintf("'echo hello' -> %s", result.Decision),
	}
}
