package cli

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"

	"github.com/spf13/cobra"

	"github.com/php-workx/fuse/internal/adapters"
	"github.com/php-workx/fuse/internal/config"
	"github.com/php-workx/fuse/internal/db"
	"github.com/php-workx/fuse/internal/policy"
)

var (
	doctorLive     bool
	doctorSecurity bool
)

var doctorCmd = &cobra.Command{
	Use:     "doctor",
	Short:   "Run diagnostic checks on fuse setup",
	Long:    "Checks configuration, directory structure, hook installation, and optionally runs a live test.",
	GroupID: groupObserve,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runDoctor(doctorLive, doctorSecurity)
	},
}

var (
	doctorVerbose bool
	doctorFix     bool
)

func init() {
	doctorCmd.Flags().BoolVar(&doctorLive, "live", false, "Run a live classification test")
	doctorCmd.Flags().BoolVar(&doctorSecurity, "security", false, "Run additional security posture diagnostics")
	doctorCmd.Flags().BoolVar(&doctorVerbose, "verbose", false, "Show full detail for all checks")
	doctorCmd.Flags().BoolVar(&doctorFix, "fix", false, "Automatically fix warnings where possible")
	rootCmd.AddCommand(doctorCmd)
}

type checkResult struct {
	name    string
	status  string // "PASS", "FAIL", "WARN"
	detail  string
	fixHint string       // human-readable fix suggestion (e.g., "run: fuse install claude --secure")
	fixFunc func() error // auto-fix function; nil if not auto-fixable
}

func runDoctor(live, security bool) error {
	var results []checkResult

	results = append(results, checkGoVersion())
	results = append(results, checkDirectoryStructure())
	results = append(results, checkConfigYAML())
	results = append(results, checkPolicyYAML())
	results = append(results, checkClaudeSettings())
	results = append(results, checkSQLiteDB())
	results = append(results, checkMCPProxyConfiguration())
	results = append(results, checkFuseInPath())

	if security {
		results = append(results, checkClaudeSecurityPosture())
		results = append(results, checkCodexSecurityPosture())
		results = append(results, checkMCPMediationPosture())
		results = append(results, checkApprovalTerminalTrust())
		results = append(results, checkTagOverrides())
	}

	if live {
		results = append(results, checkLiveTTYAccess())
		results = append(results, checkLiveRawMode())
		results = append(results, checkLiveForegroundProcessGroup())
	}

	// Print results.
	fmt.Println("fuse doctor")
	fmt.Println("===========")
	fmt.Println()

	passes := 0
	fails := 0
	warns := 0

	var fixed int
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
			detail := r.detail
			if !doctorVerbose && len(detail) > 120 {
				// Count semicolons as a proxy for number of items.
				items := strings.Count(detail, ";") + 1
				if items > 2 {
					// Truncate: show first item + count.
					first := strings.SplitN(detail, ";", 2)[0]
					detail = fmt.Sprintf("%s (and %d more — use --verbose for full list)", strings.TrimSpace(first), items-1)
				}
			}
			fmt.Printf("           %s\n", detail)
		}
		if r.fixHint != "" && r.status == "WARN" {
			if doctorFix && r.fixFunc != nil {
				fmt.Printf("           fixing: %s\n", r.fixHint)
				if err := r.fixFunc(); err != nil {
					fmt.Printf("           fix failed: %v\n", err)
				} else {
					fmt.Printf("           fixed.\n")
					fixed++
				}
			} else if !doctorFix {
				fmt.Printf("           fix: %s\n", r.fixHint)
			}
		}
	}

	fmt.Println()
	summary := fmt.Sprintf("Results: %d passed, %d warnings, %d failed", passes, warns, fails)
	if fixed > 0 {
		summary += fmt.Sprintf(", %d auto-fixed", fixed)
	}
	fmt.Println(summary)

	// Policy recommendations from approval history.
	printPolicyRecommendations()

	if fails > 0 {
		return fmt.Errorf("%d check(s) failed", fails)
	}
	return nil
}

// printPolicyRecommendations queries the event database for frequently-approved
// commands and suggests policy rules.
func printPolicyRecommendations() {
	database, err := db.OpenDB(config.DBPath())
	if err != nil {
		return // best-effort
	}
	defer func() { _ = database.Close() }()

	recs, err := database.FrequentApprovals(3)
	if err != nil || len(recs) == 0 {
		return
	}

	fmt.Println()
	fmt.Println("Policy Recommendations")
	fmt.Println("----------------------")
	fmt.Printf("Based on your approval history, consider adding these rules to policy.yaml:\n\n")
	for _, r := range recs {
		fmt.Printf("  # Approved %d times: %s\n", r.Count, truncateCmd(r.Command, 60))
		fmt.Printf("  - pattern: \"^%s$\"\n", escapePattern(r.Command))
		fmt.Printf("    action: \"allow\"\n")
		fmt.Printf("    reason: \"approved %d times\"\n\n", r.Count)
	}
}

func truncateCmd(cmd string, maxLen int) string {
	if len(cmd) <= maxLen {
		return cmd
	}
	return cmd[:maxLen-3] + "..."
}

func escapePattern(cmd string) string {
	// Escape regex metacharacters for use in a pattern.
	replacer := strings.NewReplacer(
		`\`, `\\`, `.`, `\.`, `*`, `\*`, `+`, `\+`,
		`?`, `\?`, `(`, `\(`, `)`, `\)`, `[`, `\[`,
		`]`, `\]`, `{`, `\{`, `}`, `\}`, `^`, `\^`,
		`$`, `\$`, `|`, `\|`,
	)
	return replacer.Replace(cmd)
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
		{config.ConfigDir(), 0o755},
		{config.StateDir(), 0o700},
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

// checkPolicyYAML checks that policy.yaml is valid if present. Uses LKG fallback
// and reports when the fallback is active (ECO-009: loud LKG warning).
func checkPolicyYAML() checkResult {
	policyPath := config.PolicyPath()

	if _, err := os.Stat(policyPath); os.IsNotExist(err) {
		return checkResult{
			name:   "Policy (policy.yaml)",
			status: "PASS",
			detail: "not present (using built-in rules only)",
		}
	}

	// Try primary policy first.
	pol, err := policy.LoadPolicy(policyPath)
	if err == nil {
		policyHash := computePolicyHash(policyPath)
		return checkResult{
			name:   "Policy (policy.yaml)",
			status: "PASS",
			detail: fmt.Sprintf("%d rules loaded (version: %s, hash: %s)", len(pol.Rules), pol.Version, policyHash),
		}
	}

	// Primary failed — try loading LKG to verify it's actually usable.
	lkgPath := policyPath + ".lkg"
	lkgCfg, lkgErr := policy.LoadPolicy(lkgPath)
	if lkgErr != nil {
		return checkResult{
			name:   "Policy (policy.yaml)",
			status: "FAIL",
			detail: fmt.Sprintf("error loading policy: %v (LKG fallback also unusable)", err),
		}
	}

	// LKG is valid and loadable — report warning with active policy hash.
	lkgHash := computePolicyHash(lkgPath)
	return checkResult{
		name:   "Policy (policy.yaml)",
		status: "WARN",
		detail: fmt.Sprintf("WARNING: using fallback policy (policy.yaml has errors: %v). Active LKG hash: %s, %d rules", err, lkgHash, len(lkgCfg.Rules)),
	}
}

// computePolicyHash returns a short SHA-256 hash of a policy file for identification.
func computePolicyHash(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return "unknown"
	}
	h := sha256.Sum256(data)
	return fmt.Sprintf("%.8x", h)
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

func checkTagOverrides() checkResult {
	policyPath := config.PolicyPath()
	if _, statErr := os.Stat(policyPath); os.IsNotExist(statErr) {
		return checkResult{
			name:   "Tag overrides",
			status: "PASS",
			detail: "no policy file (no tag overrides configured)",
		}
	}
	policyCfg, err := policy.LoadPolicy(policyPath)
	if err != nil {
		return checkResult{
			name:   "Tag overrides",
			status: "FAIL",
			detail: fmt.Sprintf("cannot load policy.yaml: %v", err),
		}
	}

	overrides, parseErr := policy.ParseTagOverrides(policyCfg)
	if parseErr != nil {
		return checkResult{
			name:   "Tag overrides",
			status: "FAIL",
			detail: fmt.Sprintf("invalid tag_overrides in policy.yaml: %v", parseErr),
		}
	}

	if len(overrides) == 0 {
		return checkResult{
			name:   "Tag overrides",
			status: "PASS",
			detail: "no tag overrides configured (all rules follow global mode)",
		}
	}

	var details []string
	for tag, mode := range policyCfg.TagOverrides {
		details = append(details, fmt.Sprintf("%s=%s", tag, mode))
	}
	sort.Strings(details)
	return checkResult{
		name:   "Tag overrides",
		status: "PASS",
		detail: fmt.Sprintf("%d tag override(s): %s", len(details), strings.Join(details, ", ")),
	}
}

func checkClaudeSecurityPosture() checkResult {
	settingsPath := claudeSettingsPath()
	settings, err := readJSONFile(settingsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return checkResult{
				name:   "Claude security posture",
				status: "WARN",
				detail: "Claude settings not found; security posture not evaluated",
			}
		}
		return checkResult{
			name:   "Claude security posture",
			status: "WARN",
			detail: fmt.Sprintf("cannot inspect Claude settings: %v", err),
		}
	}

	if !hasFuseHook(settings) {
		return checkResult{
			name:   "Claude security posture",
			status: "WARN",
			detail: "fuse hook missing; secure Claude settings not evaluated",
		}
	}

	warnings, err := claudeSecurityWarnings(settings)
	if err != nil {
		return checkResult{
			name:   "Claude security posture",
			status: "WARN",
			detail: fmt.Sprintf("cannot validate secure Claude settings safely: %v", err),
		}
	}
	if len(warnings) > 0 {
		return checkResult{
			name:    "Claude security posture",
			status:  "WARN",
			detail:  "missing or weaker secure settings: " + strings.Join(warnings, "; "),
			fixHint: "fuse install claude --secure",
			fixFunc: func() error { return installClaude(true) },
		}
	}
	return checkResult{
		name:   "Claude security posture",
		status: "PASS",
		detail: "secure Claude settings present",
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

	requiredMatchers := map[string]bool{
		"Bash":    false,
		"mcp__.*": false,
	}

	for _, entry := range preToolUse {
		entryMap, ok := entry.(map[string]interface{})
		if !ok {
			continue
		}
		matcher, _ := entryMap["matcher"].(string)
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
			hookType, _ := hMap["type"].(string)
			cmd, _ := hMap["command"].(string)
			timeout, _ := hMap["timeout"].(float64)
			if cmd == "fuse hook evaluate" && hookType == "command" && timeout == 30 {
				if _, wanted := requiredMatchers[matcher]; wanted {
					requiredMatchers[matcher] = true
				}
			}
		}
	}

	for _, found := range requiredMatchers {
		if !found {
			return false
		}
	}
	return true
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
func checkMCPProxyConfiguration() checkResult {
	cfg, err := config.LoadConfig(config.ConfigPath())
	if err != nil {
		return checkResult{
			name:   "MCP proxy configuration",
			status: "FAIL",
			detail: fmt.Sprintf("error loading config: %v", err),
		}
	}
	if cfg == nil || len(cfg.MCPProxies) == 0 {
		return checkResult{
			name:   "MCP proxy configuration",
			status: "PASS",
			detail: "no MCP proxies configured",
		}
	}

	var missing []string
	for _, proxy := range cfg.MCPProxies {
		if _, err := exec.LookPath(proxy.Command); err != nil {
			missing = append(missing, fmt.Sprintf("%s (%s)", proxy.Name, proxy.Command))
		}
	}
	if len(missing) > 0 {
		return checkResult{
			name:   "MCP proxy configuration",
			status: "FAIL",
			detail: "downstream command not found: " + strings.Join(missing, ", "),
		}
	}
	return checkResult{
		name:   "MCP proxy configuration",
		status: "PASS",
		detail: fmt.Sprintf("%d proxy command(s) available", len(cfg.MCPProxies)),
	}
}

func checkCodexSecurityPosture() checkResult {
	configPath := codexConfigPath()
	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return checkResult{
				name:   "Codex security posture",
				status: "WARN",
				detail: "Codex config not found; skipping Codex security checks",
			}
		}
		return checkResult{
			name:   "Codex security posture",
			status: "WARN",
			detail: fmt.Sprintf("cannot inspect Codex config: %v", err),
		}
	}

	warnings := codexSecurityWarnings(string(data))
	if len(warnings) > 0 {
		return checkResult{
			name:    "Codex security posture",
			status:  "WARN",
			detail:  strings.Join(warnings, "; "),
			fixHint: "fuse install codex",
			fixFunc: installCodex,
		}
	}
	return checkResult{
		name:   "Codex security posture",
		status: "PASS",
		detail: fmt.Sprintf("Codex shell mediation looks correct in %s", configPath),
	}
}

func checkMCPMediationPosture() checkResult {
	cfg, err := config.LoadConfig(config.ConfigPath())
	if err != nil {
		return checkResult{
			name:   "MCP mediation posture",
			status: "WARN",
			detail: fmt.Sprintf("cannot assess MCP mediation safely: %v", err),
		}
	}

	settings, err := readJSONFile(claudeSettingsPath())
	if err != nil {
		if os.IsNotExist(err) {
			return checkResult{
				name:   "MCP mediation posture",
				status: "WARN",
				detail: "no Claude fuse hook detected; MCP mediation risk not assessed",
			}
		}
		return checkResult{
			name:   "MCP mediation posture",
			status: "WARN",
			detail: fmt.Sprintf("cannot assess MCP mediation posture safely because Claude settings could not be read: %v", err),
		}
	}

	hookInstalled := hasFuseHook(settings)
	configuredProxies := configuredMCPProxyNames(cfg)
	claudeMCPWarnings, mediatedServers := claudeMCPServerWarnings(settings, configuredProxies)
	var warnings []string
	warnings = append(warnings, claudeMCPWarnings...)

	if (hookInstalled || mediatedServers > 0) && (cfg == nil || len(cfg.MCPProxies) == 0) {
		warnings = append(warnings, "no MCP proxies configured while fuse-mediated Claude MCP paths are active; direct MCP servers may bypass fuse proxy mediation")
	}
	if len(warnings) > 0 {
		return checkResult{
			name:   "MCP mediation posture",
			status: "WARN",
			detail: strings.Join(warnings, "; "),
		}
	}

	if !hookInstalled && mediatedServers == 0 {
		return checkResult{
			name:   "MCP mediation posture",
			status: "WARN",
			detail: "no Claude fuse hook detected; MCP mediation risk not assessed",
		}
	}

	if mediatedServers > 0 {
		return checkResult{
			name:   "MCP mediation posture",
			status: "PASS",
			detail: fmt.Sprintf("%d Claude MCP server(s) looks mediated through fuse; %d MCP proxy configuration(s) present", mediatedServers, len(cfg.MCPProxies)),
		}
	}
	return checkResult{
		name:   "MCP mediation posture",
		status: "PASS",
		detail: fmt.Sprintf("%d MCP proxy configuration(s) present", len(cfg.MCPProxies)),
	}
}

func configuredMCPProxyNames(cfg *config.Config) map[string]struct{} {
	if cfg == nil {
		return map[string]struct{}{}
	}
	names := make(map[string]struct{}, len(cfg.MCPProxies))
	for _, proxy := range cfg.MCPProxies {
		if proxy.Name == "" {
			continue
		}
		names[proxy.Name] = struct{}{}
	}
	return names
}

func checkApprovalTerminalTrust() checkResult {
	switch {
	case os.Getenv("CI") == "true":
		return checkResult{
			name:   "Approval terminal trust",
			status: "WARN",
			detail: "CI environment detected; terminal-based approval trust is lower in non-interactive sessions",
		}
	case os.Getenv("SSH_CONNECTION") != "" || os.Getenv("SSH_TTY") != "":
		return checkResult{
			name:   "Approval terminal trust",
			status: "WARN",
			detail: "remote SSH terminal detected; verify approval prompts are trusted in this session",
		}
	case os.Getenv("TERM") == "dumb":
		return checkResult{
			name:   "Approval terminal trust",
			status: "WARN",
			detail: "TERM=dumb; approval prompt rendering or trust may be degraded",
		}
	default:
		return checkResult{
			name:   "Approval terminal trust",
			status: "PASS",
			detail: "no obvious terminal trust warnings detected",
		}
	}
}

func checkLiveTTYAccess() checkResult {
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return checkResult{
			name:   "Live terminal /dev/tty access",
			status: "WARN",
			detail: fmt.Sprintf("cannot open /dev/tty: %v", err),
		}
	}
	_ = tty.Close()
	return checkResult{
		name:   "Live terminal /dev/tty access",
		status: "PASS",
		detail: "/dev/tty opened successfully",
	}
}

func checkLiveRawMode() checkResult {
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return checkResult{
			name:   "Live terminal raw mode",
			status: "WARN",
			detail: fmt.Sprintf("cannot open /dev/tty: %v", err),
		}
	}
	defer func() { _ = tty.Close() }()

	fd := int(tty.Fd())
	orig, err := unix.IoctlGetTermios(fd, doctorIoctlGetTermios)
	if err != nil {
		return checkResult{
			name:   "Live terminal raw mode",
			status: "WARN",
			detail: fmt.Sprintf("raw mode not available: %v", err),
		}
	}
	raw := *orig
	raw.Lflag &^= unix.ICANON | unix.ECHO
	if len(raw.Cc) > unix.VMIN {
		raw.Cc[unix.VMIN] = 1
	}
	if len(raw.Cc) > unix.VTIME {
		raw.Cc[unix.VTIME] = 0
	}
	if err := unix.IoctlSetTermios(fd, doctorIoctlSetTermios, &raw); err != nil {
		return checkResult{
			name:   "Live terminal raw mode",
			status: "FAIL",
			detail: fmt.Sprintf("enter raw mode: %v", err),
		}
	}
	if err := unix.IoctlSetTermios(fd, doctorIoctlSetTermios, orig); err != nil {
		return checkResult{
			name:   "Live terminal raw mode",
			status: "FAIL",
			detail: fmt.Sprintf("restore terminal state: %v", err),
		}
	}
	return checkResult{
		name:   "Live terminal raw mode",
		status: "PASS",
		detail: "entered and restored raw mode on /dev/tty",
	}
}

func checkLiveForegroundProcessGroup() checkResult {
	fd := int(os.Stdin.Fd())
	if _, err := unix.IoctlGetInt(fd, unix.TIOCGPGRP); err != nil {
		return checkResult{
			name:   "Live foreground process-group handoff",
			status: "WARN",
			detail: "stdin is not a terminal; foreground process-group handoff not probed",
		}
	}

	cmd, err := startForegroundProbeProcess(os.Stdin, io.Discard, io.Discard)
	if err != nil {
		return checkResult{
			name:   "Live foreground process-group handoff",
			status: "FAIL",
			detail: fmt.Sprintf("start probe child: %v", err),
		}
	}
	defer func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()

	restore, err := adapters.ForegroundChildProcessGroupIfTTY(cmd.Process.Pid)
	if err != nil {
		return checkResult{
			name:   "Live foreground process-group handoff",
			status: "FAIL",
			detail: fmt.Sprintf("handoff probe failed: %v", err),
		}
	}
	if restore != nil {
		restore()
	}
	return checkResult{
		name:   "Live foreground process-group handoff",
		status: "PASS",
		detail: "foreground handoff to a child process group succeeded",
	}
}

func startForegroundProbeProcess(stdin io.Reader, stdout, stderr io.Writer) (*exec.Cmd, error) {
	cmd := exec.Command("/bin/sh", "-c", "trap 'exit 0' TERM INT HUP; while :; do sleep 1; done")
	cmd.Stdin = stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return cmd, nil
}
