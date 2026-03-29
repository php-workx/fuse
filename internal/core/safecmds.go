package core

import (
	"strings"
)

// unconditionalSafe contains commands that are SAFE regardless of arguments.
// Source: spec §6.5 — Codex safe command list + standard developer workflow commands.
var unconditionalSafe = map[string]bool{
	// File reading / inspection
	"ls": true, "cat": true, "head": true, "tail": true, "less": true,
	"more": true, "file": true, "stat": true, "wc": true,
	"md5sum": true, "sha256sum": true, "sha1sum": true, "cksum": true,
	"du": true, "df": true,
	// Text processing
	"echo": true, "printf": true, "grep": true, "egrep": true, "fgrep": true,
	"rg": true, "ag": true, "awk": true, "sed": true, "cut": true, "tr": true,
	"sort": true, "uniq": true, "tee": true, "paste": true, "join": true,
	"comm": true, "fold": true, "fmt": true, "column": true,
	"jq": true, "yq": true, "xq": true,
	// Search / navigation
	"which": true, "whereis": true, "type": true, "pwd": true, "cd": true,
	"tree": true, "realpath": true, "dirname": true, "basename": true,
	// Diff / compare
	"diff": true, "colordiff": true, "vimdiff": true, "cmp": true,
	// Environment
	"date": true, "cal": true, "uname": true, "hostname": true, "whoami": true,
	"id": true, "groups": true, "uptime": true, "free": true, "top": true,
	"htop": true, "ps": true, "pgrep": true, "lsof": true, "lsblk": true,
	"mount": true,
	// Network diagnostics (read-only)
	"ping": true, "traceroute": true, "tracepath": true, "mtr": true,
	"host": true, "nslookup": true, "dig": true, "whois": true,
	// Read-only help/docs
	"man": true, "info": true, "tldr": true, "help": true,
	// Linters / formatters / test runners (single-word basenames)
	"eslint": true, "prettier": true, "black": true, "ruff": true,
	"mypy": true, "pylint": true, "flake8": true, "gofmt": true,
	"golint": true, "rustfmt": true, "goimports": true, "pytest": true,
}

// windowsSafeCmdlets contains PowerShell cmdlets that are SAFE regardless of arguments.
// These are read-only or display-only cmdlets.
var windowsSafeCmdlets = map[string]bool{
	"Get-ChildItem": true, "Get-Content": true, "Get-Item": true,
	"Get-ItemProperty": true, "Get-Location": true, "Get-Process": true,
	"Get-Service": true, "Get-Date": true, "Get-Help": true,
	"Get-Command": true, "Get-Alias": true, "Get-Variable": true,
	"Get-Member": true, "Get-Host": true, "Get-History": true,
	"Get-Culture": true, "Get-ComputerInfo": true, "Get-Disk": true,
	"Get-Volume": true, "Get-NetAdapter": true, "Get-NetIPAddress": true,
	"Get-DnsClientCache": true, "Get-EventLog": true,
	"Test-Path": true, "Test-Connection": true, "Test-NetConnection": true,
	"Write-Output": true, "Write-Host": true, "Write-Verbose": true,
	"Format-List": true, "Format-Table": true, "Format-Wide": true,
	"Out-String": true, "Out-Null": true,
	"Select-Object": true, "Where-Object": true, "Sort-Object": true,
	"Group-Object": true, "Measure-Object": true, "ForEach-Object": true,
	"Compare-Object": true, "ConvertTo-Json": true, "ConvertFrom-Json": true,
	"Select-String": true, "Resolve-Path": true,
	"Get-TypeData": true, "Get-FormatData": true,
}

// windowsSafeCMDBuiltins contains CMD.exe builtins and utilities that are SAFE.
var windowsSafeCMDBuiltins = map[string]bool{
	"dir": true, "type": true, "echo": true,
	"ver": true, "vol": true, "cls": true, "title": true,
	"path": true, "where": true, "help": true,
	"hostname": true, "whoami": true, "systeminfo": true,
	"tasklist": true, "findstr": true, "find": true, "more": true,
	"sort": true, "fc": true, "comp": true, "tree": true,
}

// windowsSafePrefixes contains multi-word Windows command prefixes that are
// unconditionally safe (read-only operations).
var windowsSafePrefixes = []string{
	"Get-ChildItem", "Get-Content", "Get-Process", "Get-Service",
	"Test-Path", "Test-Connection", "Test-NetConnection",
	"Select-String", "Measure-Object",
	"dotnet --version", "dotnet --info", "dotnet --list-sdks",
	"winget --version", "winget list", "winget show",
	"choco list", "choco info", "choco --version",
	"scoop list", "scoop info", "scoop --version",
}

// unconditionalSafePrefixes contains multi-word command prefixes that are
// unconditionally safe. These are checked after splitting the full command.
// Source: spec §6.5.
var unconditionalSafePrefixes = []string{
	// Development tools (read-only)
	"cargo check", "cargo test", "cargo clippy", "cargo fmt",
	"go vet", "go test", "go fmt",
	"npm test", "npm run lint", "npm run test", "npx jest",
	"yarn test", "pnpm test", "bun test",
	"pytest", "python -m pytest", "python -m unittest",
	"tsc --noEmit", "tsc --version",
	"make check", "make test", "make lint",
	// Version / info
	"node --version", "python --version", "go version",
	"rustc --version", "cargo --version", "npm --version",
	"git --version", "terraform --version", "aws --version",
	"gcloud --version", "az --version",
}

// windowsSafeCmdletsLower is a case-folded lookup for PowerShell cmdlets.
var windowsSafeCmdletsLower = func() map[string]bool {
	m := make(map[string]bool, len(windowsSafeCmdlets))
	for k := range windowsSafeCmdlets {
		m[strings.ToLower(k)] = true
	}
	return m
}()

// IsUnconditionalSafe returns true if the command basename (no path) is in the
// unconditionally safe set. For multi-word safe commands (e.g. "cargo test"),
// callers should use IsUnconditionalSafeCmd which checks the full command.
func IsUnconditionalSafe(basename string) bool {
	if unconditionalSafe[basename] {
		return true
	}
	// PowerShell cmdlets are case-insensitive
	if windowsSafeCmdletsLower[strings.ToLower(basename)] {
		return true
	}
	// CMD builtins are case-insensitive
	if windowsSafeCMDBuiltins[strings.ToLower(basename)] {
		return true
	}
	return false
}

// IsUnconditionalSafeCmd returns true if the full command string matches either
// a single-word unconditionally safe command or a multi-word safe prefix.
func IsUnconditionalSafeCmd(fullCmd string) bool {
	// Extract the first token as the basename.
	fields := strings.Fields(fullCmd)
	if len(fields) == 0 {
		return false
	}
	basename := fields[0]

	// Check single-word safe set (includes Windows cmdlets and CMD builtins).
	if IsUnconditionalSafe(basename) {
		return true
	}

	// Check multi-word prefixes against the normalized (Fields-joined) command.
	normalized := strings.Join(fields, " ")
	for _, prefix := range unconditionalSafePrefixes {
		if normalized == prefix || strings.HasPrefix(normalized, prefix+" ") {
			return true
		}
	}

	// Check Windows-specific multi-word prefixes.
	for _, prefix := range windowsSafePrefixes {
		if normalized == prefix || strings.HasPrefix(normalized, prefix+" ") {
			return true
		}
	}

	return false
}

// IsConditionallySafe returns true if the command identified by basename is
// safe given the full command string. This implements the conditional safety
// rules from spec §6.5. If the command is not in the conditionally safe
// table at all, it returns false (caller should fall through to other rules).
func IsConditionallySafe(basename, fullCmd string) bool {
	fields := strings.Fields(fullCmd)
	switch basename {
	case "find":
		return isFindSafe(fields)
	case "git":
		return isGitSafe(fields)
	case "sed":
		return isSedSafe(fields)
	case "base64":
		return isBase64Safe(fields)
	case "xargs":
		return isXargsSafe(fields)
	case "docker":
		return isDockerSafe(fields)
	case "kubectl":
		return isKubectlSafe(fields)
	case "terraform", "tofu":
		return isTerraformSafe(fields)
	case "pulumi":
		return isPulumiSafe(fields)
	case "aws":
		return isAwsSafe(fields)
	case "gcloud":
		return isGcloudSafe(fields)
	case "az":
		return isAzSafe(fields)
	case "sqlite3":
		return isSqliteSafe(fields)
	case "nc", "ncat", "netcat":
		return isNcSafe(fields)
	case "pip", "pip3":
		return isPipSafe(fields)
	case "Remove-Item":
		return isRemoveItemSafe(fields)
	case "set":
		// CMD set without args displays env vars (safe); with args modifies them (dangerous).
		return len(fields) == 1
	case "time", "date":
		// CMD time/date without args or with /t displays value (safe); with args modifies (dangerous).
		return len(fields) == 1 || (len(fields) == 2 && strings.EqualFold(fields[1], "/t"))
	default:
		// PowerShell cmdlet case-insensitive match for conditional checks.
		if strings.EqualFold(basename, "Remove-Item") {
			return isRemoveItemSafe(fields)
		}
		return false
	}
}

// --- Per-command conditional safety helpers ---

// isFindSafe: find is safe when it has no -delete, -exec rm, or -exec sh flags.
func isFindSafe(fields []string) bool {
	for i, f := range fields {
		if f == "-delete" {
			return false
		}
		if f == "-exec" || f == "-execdir" {
			// Check the token after -exec for dangerous commands.
			if i+1 < len(fields) {
				next := fields[i+1]
				if next == "rm" || next == "sh" || strings.HasPrefix(next, "rm ") {
					return false
				}
			}
		}
	}
	return true
}

// gitGlobalFlagsWithValue lists git global flags that take a value argument.
var gitGlobalFlagsWithValue = map[string]bool{
	"-C": true, "-c": true, "--git-dir": true, "--work-tree": true,
}

// unconditionalSafeGitSubs are git subcommands safe with any arguments.
var unconditionalSafeGitSubs = map[string]bool{
	"status": true, "log": true, "diff": true, "show": true,
	"fetch": true, "rev-parse": true, "describe": true,
	"shortlog": true, "ls-files": true, "ls-tree": true,
}

// conditionalGitCheckers map git subcommands to argument validators.
// Each function receives the args AFTER the subcommand and returns true if safe.
var conditionalGitCheckers = map[string]func([]string) bool{
	"branch":   gitBranchSafe,
	"stash":    gitStashSafe,
	"remote":   gitRemoteSafe,
	"pull":     gitPullSafe,
	"checkout": gitCheckoutSafe,
	"config":   gitConfigSafe,
	"tag":      gitTagSafe,
	"restore":  gitRestoreSafe,
}

// isGitSafe: git is safe with read-only subcommands.
// Uses data-driven lookup tables instead of a monolithic switch.
func isGitSafe(fields []string) bool {
	if len(fields) < 2 {
		return false
	}

	// Skip global flags (e.g. git -C /path status).
	idx := 1
	for idx < len(fields) && strings.HasPrefix(fields[idx], "-") {
		prev := fields[idx]
		idx++
		if gitGlobalFlagsWithValue[prev] && idx < len(fields) {
			idx++
		}
	}
	if idx >= len(fields) {
		return false
	}
	sub := fields[idx]
	rest := fields[idx+1:]

	// Layer 1: Unconditionally safe subcommands.
	if unconditionalSafeGitSubs[sub] {
		return true
	}

	// Layer 2: Conditionally safe subcommands with argument checks.
	if checker, ok := conditionalGitCheckers[sub]; ok {
		return checker(rest)
	}

	return false
}

// gitBranchSafe: branch is unsafe with -D, -d, or --delete.
func gitBranchSafe(args []string) bool {
	for _, a := range args {
		if a == "-D" || a == "-d" || a == "--delete" {
			return false
		}
	}
	return true
}

// gitStashSafe: only "stash list" is safe.
func gitStashSafe(args []string) bool {
	return len(args) > 0 && args[0] == "list"
}

// gitRemoteSafe: remote (no args), remote -v, remote show are safe.
func gitRemoteSafe(args []string) bool {
	if len(args) == 0 {
		return true
	}
	return args[0] == "-v" || args[0] == "--verbose" || args[0] == "show"
}

// gitPullSafe: pull is unsafe with --force flags.
func gitPullSafe(args []string) bool {
	for _, a := range args {
		if a == "--force" || a == "-f" || a == "--force-with-lease" {
			return false
		}
	}
	return true
}

// gitCheckoutSafe: checkout -b (create branch) is safe; checkout -- . is unsafe.
func gitCheckoutSafe(args []string) bool {
	hasDashB := false
	for i, a := range args {
		if a == "-b" || a == "-B" {
			hasDashB = true
		}
		if a == "--" && i+1 < len(args) && args[i+1] == "." {
			return false
		}
	}
	return hasDashB
}

// gitConfigSafe: only --list, --get, and -l are safe.
func gitConfigSafe(args []string) bool {
	for _, a := range args {
		if a == "--list" || a == "--get" || a == "-l" || strings.HasPrefix(a, "--get") {
			return true
		}
	}
	return false
}

// gitTagSafe: listing is safe; creation (-a, -s) and deletion (-d) are not.
func gitTagSafe(args []string) bool {
	for _, a := range args {
		if a == "-l" || a == "--list" {
			return true
		}
		if a == "-d" || a == "--delete" || a == "-a" || a == "-s" {
			return false
		}
	}
	// Plain "git tag" (list) is safe.
	return true
}

// isSedSafe: sed is safe without -i (in-place edit).
func isSedSafe(fields []string) bool {
	for _, f := range fields {
		if f == "-i" || f == "--in-place" || strings.HasPrefix(f, "-i") {
			// -i can appear as -i'' or -i.bak — any form is in-place.
			return false
		}
	}
	return true
}

// isBase64Safe: base64 is safe without -d/--decode.
func isBase64Safe(fields []string) bool {
	for _, f := range fields {
		if f == "-d" || f == "--decode" || f == "-D" {
			return false
		}
	}
	return true
}

// isXargsSafe: xargs is unsafe when targeting rm, kill, etc.
func isXargsSafe(fields []string) bool {
	for _, f := range fields {
		if f == "rm" || f == "kill" || f == "rmdir" {
			return false
		}
	}
	return true
}

// isDockerSafe: docker is safe with read-only subcommands.
func isDockerSafe(fields []string) bool {
	if len(fields) < 2 {
		return false
	}

	// Skip flags before subcommand (e.g. docker --context foo ps).
	idx := 1
	for idx < len(fields) && strings.HasPrefix(fields[idx], "-") {
		idx++
		// Skip flag values for known value-taking flags.
		if idx < len(fields) && !strings.HasPrefix(fields[idx], "-") {
			idx++
		}
	}
	if idx >= len(fields) {
		return false
	}
	sub := fields[idx]

	safeSubs := map[string]bool{
		"ps": true, "images": true, "logs": true, "inspect": true,
		"stats": true, "top": true, "version": true, "info": true,
		"network": true, "volume": true,
	}

	if !safeSubs[sub] {
		return false
	}

	// network and volume are only safe with "ls".
	if sub == "network" || sub == "volume" {
		if idx+1 >= len(fields) || fields[idx+1] != "ls" {
			return false
		}
	}

	return true
}

// isKubectlSafe: kubectl is safe with read-only subcommands.
func isKubectlSafe(fields []string) bool {
	if len(fields) < 2 {
		return false
	}
	sub := fields[1]

	safeSubs := map[string]bool{
		"get": true, "describe": true, "logs": true, "top": true,
		"version": true, "config": true, "api-resources": true,
		"cluster-info": true, "explain": true, "api-versions": true,
	}

	if !safeSubs[sub] {
		return false
	}

	// "config" is only safe with "view".
	if sub == "config" {
		if len(fields) < 3 || fields[2] != "view" {
			return false
		}
	}

	return true
}

// isTerraformSafe: terraform/tofu is safe with read-only subcommands.
func isTerraformSafe(fields []string) bool {
	if len(fields) < 2 {
		return false
	}
	sub := fields[1]

	safeSubs := map[string]bool{
		"plan": true, "validate": true, "fmt": true, "show": true,
		"output": true, "providers": true, "version": true, "graph": true,
	}

	return safeSubs[sub]
}

// isPulumiSafe: pulumi is safe with read-only subcommands.
func isPulumiSafe(fields []string) bool {
	if len(fields) < 2 {
		return false
	}
	sub := fields[1]

	// Simple safe subcommands.
	simpleSafe := map[string]bool{
		"preview": true, "config": true, "version": true, "about": true,
	}
	if simpleSafe[sub] {
		return true
	}

	// "stack ls" is safe.
	if sub == "stack" && len(fields) >= 3 && fields[2] == "ls" {
		return true
	}

	return false
}

// isAwsSafe: aws is safe with describe-*, list-*, get-* subcommands and s3 ls.
func isAwsSafe(fields []string) bool {
	if len(fields) < 3 {
		return false
	}

	// Skip global flags (e.g. aws --region us-east-1 s3 ls).
	idx := 1
	for idx < len(fields) && strings.HasPrefix(fields[idx], "-") {
		idx++
		// Skip flag values.
		if idx < len(fields) && !strings.HasPrefix(fields[idx], "-") {
			idx++
		}
	}
	if idx+1 >= len(fields) {
		return false
	}

	service := fields[idx]
	action := fields[idx+1]

	// sts get-caller-identity is safe.
	if service == "sts" && action == "get-caller-identity" {
		return true
	}

	// s3 ls is safe.
	if service == "s3" && action == "ls" {
		return true
	}

	// General read-only actions.
	if strings.HasPrefix(action, "describe-") ||
		strings.HasPrefix(action, "list-") ||
		strings.HasPrefix(action, "get-") {
		return true
	}

	return false
}

// skipGcloudFlags advances past leading flags (and their non-flag arguments)
// starting at position start, returning the index of the first positional token.
func skipGcloudFlags(fields []string, start int) int {
	idx := start
	for idx < len(fields) && strings.HasPrefix(fields[idx], "-") {
		idx++
		if idx < len(fields) && !strings.HasPrefix(fields[idx], "-") {
			idx++
		}
	}
	return idx
}

// isGcloudSafe: gcloud is safe with describe, list, config list, info, auth list.
func isGcloudSafe(fields []string) bool {
	if len(fields) < 2 {
		return false
	}

	// Find the first non-flag token after "gcloud".
	idx := skipGcloudFlags(fields, 1)
	if idx >= len(fields) {
		return false
	}

	// Check for safe patterns anywhere in the remaining arguments.
	rest := fields[idx:]

	safeVerbs := map[string]bool{
		"describe": true, "list": true, "info": true,
	}

	// "config list" is safe.
	if len(rest) >= 2 && rest[0] == "config" && rest[1] == "list" {
		return true
	}
	// "auth list" is safe.
	if len(rest) >= 2 && rest[0] == "auth" && rest[1] == "list" {
		return true
	}

	// Look for safe verb as the last positional token (gcloud <group> <verb>).
	for _, tok := range rest {
		if strings.HasPrefix(tok, "-") {
			continue
		}
		if safeVerbs[tok] {
			return true
		}
	}

	return false
}

// isAzSafe: az is safe with show, list, account show.
func isAzSafe(fields []string) bool {
	if len(fields) < 2 {
		return false
	}

	rest := fields[1:]

	safeVerbs := map[string]bool{
		"show": true, "list": true,
	}

	// "account show" is safe.
	if len(rest) >= 2 && rest[0] == "account" && rest[1] == "show" {
		return true
	}

	// Check for safe verb in positional tokens.
	for _, tok := range rest {
		if strings.HasPrefix(tok, "-") {
			continue
		}
		if safeVerbs[tok] {
			return true
		}
	}

	return false
}

// safeBuildDirs are directories that agents commonly clean and are safe to rm -rf.
// Sourced from AgentGuard's default allowlist.
var safeBuildDirs = map[string]bool{
	"node_modules": true, "dist": true, "build": true, ".next": true,
	"out": true, "target": true, ".cache": true, "__pycache__": true,
	".pytest_cache": true, "tmp": true, "temp": true, "coverage": true,
	".nyc_output": true, ".turbo": true, ".parcel-cache": true,
	".vite": true, ".nuxt": true, ".output": true, ".svelte-kit": true,
	"vendor": true, ".tox": true, ".mypy_cache": true, ".ruff_cache": true,
	"bin": true, "obj": true, ".gradle": true, ".angular": true,
}

// IsSafeBuildCleanup returns true if the command is rm -rf targeting
// only known safe build/cache directories.
func IsSafeBuildCleanup(cmd string) bool {
	fields := strings.Fields(cmd)
	if len(fields) < 3 || fields[0] != "rm" {
		return false
	}
	hasRecursive := false
	argStart := 1
	for argStart < len(fields) && strings.HasPrefix(fields[argStart], "-") {
		if strings.ContainsAny(fields[argStart], "rR") || fields[argStart] == "--recursive" {
			hasRecursive = true
		}
		argStart++
	}
	if !hasRecursive || argStart >= len(fields) {
		return false
	}
	for _, arg := range fields[argStart:] {
		dir := strings.TrimRight(arg, "/")
		// Reject absolute paths, home-relative paths, and parent traversal.
		if strings.HasPrefix(dir, "/") || strings.HasPrefix(dir, "~") || strings.Contains(dir, "..") {
			return false
		}
		parts := strings.Split(dir, "/")
		if !safeBuildDirs[parts[len(parts)-1]] {
			return false
		}
	}
	return true
}

// gitRestoreSafe: git restore is safe with --staged (unstages without discarding).
// Without --staged, git restore discards working tree changes.
func gitRestoreSafe(args []string) bool {
	sawStaged := false
	for _, a := range args {
		if a == "--worktree" {
			return false
		}
		if len(a) > 1 && a[0] == '-' && a[1] != '-' && strings.ContainsRune(a[1:], 'W') {
			return false
		}
		if a == "--staged" {
			sawStaged = true
			continue
		}
		if len(a) > 1 && a[0] == '-' && a[1] != '-' && strings.ContainsRune(a[1:], 'S') {
			sawStaged = true
		}
	}
	return sawStaged
}

// isSqliteSafe: sqlite3 is safe with read-only queries (SELECT, PRAGMA, EXPLAIN).
// Blocks destructive SQL keywords and sqlite3 dot-commands that execute code.
func isSqliteSafe(fields []string) bool {
	destructive := []string{"DELETE", "DROP", "INSERT", "UPDATE", "ALTER", "ATTACH", "DETACH", "CREATE"}
	normalized := strings.NewReplacer(
		`""`, "",
		`''`, "",
		"``", "",
		`"`, "",
		`'`, "",
		"`", "",
		" ", "",
		"\t", "",
		"\n", "",
		"\r", "",
	).Replace(strings.Join(fields, " "))
	normalized = strings.ToUpper(normalized)
	for _, kw := range destructive {
		if strings.Contains(normalized, kw) {
			return false
		}
	}
	// Per-field checks for injection markers and dangerous dot-commands.
	safeDotCmds := map[string]bool{
		".tables": true, ".schema": true, ".headers": true, ".mode": true,
		".separator": true, ".width": true, ".help": true, ".show": true,
		".databases": true, ".indices": true, ".explain": true, ".timer": true,
		".nullvalue": true, ".print": true, ".bail": true, ".eqp": true,
		".stats": true, ".dbinfo": true, ".lint": true, ".fullschema": true,
	}
	for _, f := range fields {
		lower := strings.ToLower(f)
		if strings.ContainsAny(f, ";`") {
			return false
		}
		if strings.HasPrefix(lower, ".") && !safeDotCmds[lower] {
			return false
		}
	}
	return true
}

// isNcSafe: nc/ncat/netcat is safe in scan mode (-z) without exec flags.
func isNcSafe(fields []string) bool {
	if hasNcExecFlag(fields) {
		return false
	}
	return hasNcScanFlag(fields)
}

// hasNcExecFlag returns true if any field is an exec flag (-e, -c, --exec, etc.).
func hasNcExecFlag(fields []string) bool {
	for _, f := range fields {
		lower := strings.ToLower(f)
		if lower == "--exec" || lower == "--sh-exec" || lower == "--lua-exec" ||
			strings.HasPrefix(lower, "--exec=") || strings.HasPrefix(lower, "--sh-exec=") || strings.HasPrefix(lower, "--lua-exec=") {
			return true
		}
		if len(f) > 1 && f[0] == '-' && f[1] != '-' {
			if strings.ContainsRune(f[1:], 'e') || strings.ContainsRune(f[1:], 'c') {
				return true
			}
		}
	}
	return false
}

// hasNcScanFlag returns true if any field contains the -z scan flag.
func hasNcScanFlag(fields []string) bool {
	for _, f := range fields {
		if f == "-z" {
			return true
		}
		if len(f) > 1 && f[0] == '-' && f[1] != '-' && strings.ContainsRune(f, 'z') {
			return true
		}
	}
	return false
}

// isRemoveItemSafe: Remove-Item is only safe when -WhatIf is present.
// -WhatIf makes PowerShell show what would happen without actually performing the action.
func isRemoveItemSafe(fields []string) bool {
	for _, f := range fields {
		if strings.EqualFold(f, "-WhatIf") {
			return true
		}
	}
	return false
}

// isPipSafe: pip is safe for truly read-only operations.
// Excludes config (can modify index), cache (can delete), wheel (executes setup.py).
func isPipSafe(fields []string) bool {
	if len(fields) < 2 {
		return false
	}
	safeSubs := map[string]bool{
		"list": true, "show": true, "freeze": true, "check": true,
		"search": true, "debug": true,
	}
	return safeSubs[fields[1]]
}
