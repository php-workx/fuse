package core

import (
	"path/filepath"
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
	"rg": true, "colgrep": true, "ag": true, "awk": true, "sed": true, "cut": true, "tr": true,
	"sort": true, "uniq": true, "tee": true, "paste": true, "join": true,
	"comm": true, "fold": true, "fmt": true, "column": true, "nl": true,
	"jq": true, "yq": true, "xq": true,
	// Search / navigation
	"which": true, "whereis": true, "type": true, "pwd": true, "cd": true,
	"tree": true, "realpath": true, "dirname": true, "basename": true,
	// Diff / compare
	"diff": true, "colordiff": true, "vimdiff": true, "cmp": true,
	// Environment
	"cal": true, "uname": true, "hostname": true, "whoami": true,
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
	"where": true, "help": true,
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
	"go vet", "go test", "go fmt", "go help", "go build",
	"npm test", "npm run lint", "npm run test", "npx jest",
	"yarn test", "pnpm test", "bun test",
	"pytest", "python -m pytest", "python -m unittest",
	"tsc --noEmit", "tsc --version",
	"make check", "make test", "make lint",
	"just check",
	"codex --help", "codex exec --help", "codex features",
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

	// Check Windows-specific multi-word prefixes (case-insensitive).
	lowerNormalized := strings.ToLower(normalized)
	for _, prefix := range windowsSafePrefixes {
		lowerPrefix := strings.ToLower(prefix)
		if lowerNormalized == lowerPrefix || strings.HasPrefix(lowerNormalized, lowerPrefix+" ") {
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
	if checker := conditionalSafeCheckers[basename]; checker != nil {
		return checker(fields)
	}
	// Windows command names and PowerShell cmdlets are case-insensitive.
	if checker := windowsConditionalSafeCheckers[strings.ToLower(basename)]; checker != nil {
		return checker(fields)
	}
	return false
}

var conditionalSafeCheckers = map[string]func([]string) bool{
	"find":      isFindSafe,
	"git":       isGitSafe,
	"go":        isGoSafe,
	"sed":       isSedSafe,
	"base64":    isBase64Safe,
	"xargs":     isXargsSafe,
	"docker":    isDockerSafe,
	"kubectl":   isKubectlSafe,
	"terraform": isTerraformSafe,
	"tofu":      isTerraformSafe,
	"pulumi":    isPulumiSafe,
	"aws":       isAwsSafe,
	"gcloud":    isGcloudSafe,
	"az":        isAzSafe,
	"sqlite3":   isSqliteSafe,
	"nc":        isNcSafe,
	"ncat":      isNcSafe,
	"netcat":    isNcSafe,
	"pip":       isPipSafe,
	"pip3":      isPipSafe,
	"fuse":      isFuseSafe,
	"codex":     isCodexSafe,
	"gh":        isGhSafe,
	"tk":        isTkSafe,
	"gofumpt":   isGofumptSafe,
	"just":      isJustSafe,
}

var windowsConditionalSafeCheckers = map[string]func([]string) bool{
	"certutil":    isCertutilSafe,
	"sc":          isSCSafe,
	"reg":         isRegSafe,
	"remove-item": isRemoveItemSafe,
	"set":         isReadOnlySetOrPath,
	"path":        isReadOnlySetOrPath,
	"time":        isReadOnlyTimeOrDate,
	"date":        isReadOnlyTimeOrDate,
}

func KnownUnsafeInspectionVariant(basename, fullCmd string) (string, bool) {
	fields := strings.Fields(fullCmd)
	switch basename {
	case "sqlite3":
		if len(fields) > 0 && !isSqliteSafe(fields) {
			return "sqlite3 command is not read-only", true
		}
	case "gh":
		if len(fields) > 0 && !isGhSafe(fields) {
			return "gh command is not read-only", true
		}
	case "tk":
		if len(fields) > 0 && !isTkSafe(fields) {
			return "tk command is not read-only", true
		}
	case "gofumpt":
		if len(fields) > 0 && !isGofumptSafe(fields) {
			return "gofumpt command may write files", true
		}
	case "just":
		if len(fields) > 0 && !isJustSafe(fields) {
			return "just recipe is not allowlisted as read-only", true
		}
	case "codex":
		if len(fields) > 0 && !isCodexSafe(fields) {
			return "codex command is not allowlisted as read-only", true
		}
	case "fuse":
		if len(fields) > 0 && !isFuseSafe(fields) {
			return "fuse command is not allowlisted as read-only", true
		}
	}
	return "", false
}

func isReadOnlySetOrPath(fields []string) bool {
	return len(fields) == 1
}

func isReadOnlyTimeOrDate(fields []string) bool {
	return len(fields) == 1 || (len(fields) == 2 && strings.EqualFold(fields[1], "/t"))
}

func isCertutilSafe(fields []string) bool {
	safeSwitches := map[string]bool{
		"hashfile":  true,
		"verify":    true,
		"dump":      true,
		"store":     true,
		"viewstore": true,
	}
	sawSafeSwitch := false

	for _, field := range fields[1:] {
		if !strings.HasPrefix(field, "-") && !strings.HasPrefix(field, "/") {
			continue
		}

		name := strings.TrimPrefix(strings.TrimPrefix(strings.ToLower(field), "-"), "/")
		if !safeSwitches[name] {
			return false
		}

		sawSafeSwitch = true
	}

	return sawSafeSwitch
}

func isSCSafe(fields []string) bool {
	if len(fields) < 2 {
		return false
	}
	switch strings.ToLower(fields[1]) {
	case "query", "queryex", "qc":
		return true
	default:
		return false
	}
}

func isRegSafe(fields []string) bool {
	if len(fields) < 2 {
		return false
	}
	switch strings.ToLower(fields[1]) {
	case "query":
		return true
	default:
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
	// `git grep` is a read-only content search across tracked files; it never mutates the repo.
	"grep": true,
	// `git blame` is a read-only annotation of line-level history.
	"blame": true,
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

var safeGeneratedCleanupFiles = map[string]bool{
	"fuse.exe": true,
}

var safeTempCleanupPrefixes = []string{
	"/tmp/fuse-",
	"/tmp/codereview-",
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
		if !isSafeCleanupTarget(arg) {
			return false
		}
	}
	return true
}

func isSafeCleanupTarget(arg string) bool {
	target := strings.TrimRight(arg, "/")
	if target == "" || strings.HasPrefix(target, "~") || strings.Contains(target, "..") {
		return false
	}
	for _, prefix := range safeTempCleanupPrefixes {
		if strings.HasPrefix(target, prefix) {
			return true
		}
	}
	if strings.HasPrefix(target, "/") {
		return false
	}
	parts := strings.Split(target, "/")
	base := parts[len(parts)-1]
	return safeBuildDirs[base] || safeGeneratedCleanupFiles[base]
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

func isFuseSafe(fields []string) bool {
	if len(fields) < 2 || !commandTokenMatches(fields[0], "fuse") {
		return false
	}
	if isHelpOrVersion(fields[1:]) {
		return true
	}
	switch fields[1] {
	case "test":
		return len(fields) >= 3 && fields[2] == "classify"
	case "events":
		return len(fields) == 3 && fields[2] == "--help"
	case "profile":
		return true
	default:
		return false
	}
}

func IsFuseTestClassify(cmd string) bool {
	fields := strings.Fields(cmd)
	return len(fields) >= 3 && commandTokenMatches(fields[0], "fuse") && fields[1] == "test" && fields[2] == "classify"
}

func isCodexSafe(fields []string) bool {
	if len(fields) < 2 || !commandTokenMatches(fields[0], "codex") {
		return false
	}
	if isHelpOrVersion(fields[1:]) {
		return true
	}
	switch fields[1] {
	case "features":
		return len(fields) >= 3 && fields[2] == "list"
	case "exec":
		return len(fields) == 3 && fields[2] == "--help"
	default:
		return false
	}
}

func isGhSafe(fields []string) bool {
	if len(fields) < 2 || !commandTokenMatches(fields[0], "gh") {
		return false
	}
	if isHelpOrVersion(fields[1:]) {
		return true
	}
	if fields[1] == "api" {
		return isGhAPISafe(fields[2:])
	}
	if len(fields) >= 3 {
		switch fields[1] {
		case "pr":
			return map[string]bool{"view": true, "list": true, "status": true, "checks": true, "diff": true}[fields[2]]
		case "issue", "repo", "run":
			return map[string]bool{"view": true, "list": true}[fields[2]]
		case "secret":
			return fields[2] == "list"
		}
	}
	return false
}

func isGhAPISafe(args []string) bool {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "-X", "--method":
			if i+1 >= len(args) || !strings.EqualFold(args[i+1], "GET") {
				return false
			}
			i++
		case "-f", "-F", "--field", "--raw-field", "--input":
			return false
		default:
			if strings.HasPrefix(arg, "-X") && len(arg) > 2 && !strings.EqualFold(strings.TrimPrefix(arg, "-X"), "GET") {
				return false
			}
			if strings.HasPrefix(arg, "--method=") && !strings.EqualFold(strings.TrimPrefix(arg, "--method="), "GET") {
				return false
			}
			if strings.HasPrefix(arg, "-f") && arg != "--paginate" {
				return false
			}
			if strings.HasPrefix(arg, "-F") ||
				strings.HasPrefix(arg, "--field=") ||
				strings.HasPrefix(arg, "--raw-field=") ||
				strings.HasPrefix(arg, "--input=") {
				return false
			}
		}
	}
	return true
}

func isTkSafe(fields []string) bool {
	if len(fields) < 2 || !commandTokenMatches(fields[0], "tk") {
		return false
	}
	if isHelpOrVersion(fields[1:]) {
		return true
	}
	safeSubs := map[string]bool{
		"show": true, "ready": true, "list": true, "ls": true,
		"blocked": true, "closed": true, "next": true,
	}
	return safeSubs[fields[1]]
}

func isGofumptSafe(fields []string) bool {
	if len(fields) == 0 || !commandTokenMatches(fields[0], "gofumpt") {
		return false
	}
	for _, field := range fields[1:] {
		if field == "-w" || strings.Contains(field, "w") && strings.HasPrefix(field, "-") && !strings.HasPrefix(field, "--") {
			return false
		}
	}
	return true
}

func isJustSafe(fields []string) bool {
	return len(fields) >= 2 && commandTokenMatches(fields[0], "just") && fields[1] == "check"
}

// unconditionalSafeGoSubs lists `go` subcommands whose entire invocation is
// read-only regardless of arguments. Covers version/help reporters and the
// standard lint/format/test workflow commands already allow-listed as safe
// prefixes — listing them here lets the explicit conditional safe rule match
// instead of the unknown-command fallback.
var unconditionalSafeGoSubs = map[string]bool{
	"version": true,
	"help":    true,
	"env":     true,
	"list":    true,
	"doc":     true,
	"vet":     true,
	"test":    true,
	"fmt":     true,
	"build":   true,
}

// isGoSafe: `go` is safe for read-only subcommands (version, env, list, doc,
// vet, test, fmt, build) and for `go tool … <version|--version|--help>`.
// Other `go tool` invocations can execute project tools and remain unclassified
// here so they fall through to the default SAFE fallback — the ticket accepts
// that behavior but wants an explicit rule for the common version/help form.
func isGoSafe(fields []string) bool {
	if len(fields) < 2 || !commandTokenMatches(fields[0], "go") {
		return false
	}
	sub := fields[1]
	if unconditionalSafeGoSubs[sub] {
		return true
	}
	if sub == "tool" {
		return IsGoToolReportSafe(fields[2:])
	}
	return false
}

// IsGoToolReportSafe returns true when `go tool` is invoked purely as a
// version/help reporter: the final positional token must be "version",
// "--version", "-version", "--help", "-h", or "help". Flags before it are
// allowed because wrappers like `-modfile=tools.mod` are common.
//
// Exported so tests and external callers can verify read-only `go tool` usage
// without re-implementing the token-shape check that the classifier relies on.
func IsGoToolReportSafe(args []string) bool {
	if len(args) == 0 {
		return false
	}
	last := args[len(args)-1]
	switch last {
	case "version", "--version", "-version", "--help", "-h", "help":
		return true
	default:
		return false
	}
}

// IsGitSubcommandReadOnly reports whether a bare `git <sub> …` invocation is
// a read-only operation according to the built-in safe-command tables. It is
// the public counterpart of isGitSafe, used by tests (and potential external
// callers) to confirm that subcommands like `grep`, `log`, and `status` match
// an explicit allowlist rather than the default-safe fallback.
//
// args contains the tokens after the subcommand (the equivalent of the `rest`
// slice in isGitSafe). Passing nil/empty is valid for bare subcommand forms
// like `git status`.
func IsGitSubcommandReadOnly(sub string, args []string) bool {
	if sub == "" {
		return false
	}
	if unconditionalSafeGitSubs[sub] {
		return true
	}
	if checker, ok := conditionalGitCheckers[sub]; ok {
		return checker(args)
	}
	return false
}

// IsTimeoutWrapped reports whether the command begins with the `timeout`
// wrapper invocation (`timeout <duration> <inner cmd …>`). It only inspects
// the raw token structure — it does not itself strip the wrapper. Use
// ClassificationNormalize for the full wrapper-stripping pipeline; this helper
// exists so tests can assert that a command is intentionally timeout-wrapped
// before classifying its inner command.
func IsTimeoutWrapped(cmd string) bool {
	fields := strings.Fields(cmd)
	if len(fields) < 3 {
		return false
	}
	if filepath.Base(fields[0]) != "timeout" {
		return false
	}
	// Skip any leading timeout flags (e.g. -s TERM, -k 5s) to find the
	// duration argument. The duration must be the first positional token.
	i := 1
	for i < len(fields) {
		t := fields[i]
		if t == "--" {
			i++
			break
		}
		if timeoutFlagsWithArg[t] {
			i += 2
			continue
		}
		if timeoutFlagsNoArg[t] {
			i++
			continue
		}
		if isTimeoutCombinedFlag(t) {
			i++
			continue
		}
		break
	}
	// Need a duration token plus at least one command token after it.
	if i >= len(fields) || strings.HasPrefix(fields[i], "-") {
		return false
	}
	return i+1 < len(fields)
}

func isHelpOrVersion(args []string) bool {
	return len(args) == 1 && (args[0] == "--help" || args[0] == "-h" || args[0] == "help" || args[0] == "--version" || args[0] == "version")
}

func commandTokenMatches(token, want string) bool {
	token = strings.Trim(token, `"'`)
	token = strings.ReplaceAll(token, `\`, "/")
	return filepath.Base(token) == want
}

func IsProvableMktempCleanup(cmd string) bool {
	subCmds, err := SplitCompoundCommand(cmd)
	if err != nil || len(subCmds) < 2 {
		return false
	}
	tempVars := map[string]bool{}
	sawCleanup := false
	for _, subCmd := range subCmds {
		fields := strings.Fields(subCmd)
		if len(fields) == 0 {
			return false
		}
		if name, ok := mktempAssignment(fields); ok {
			tempVars[name] = true
			continue
		}
		if isTempVarReassignment(fields, tempVars) {
			return false
		}
		if isMktempSetupCommand(fields, tempVars) {
			continue
		}
		if isMktempCleanupCommand(fields, tempVars) {
			sawCleanup = true
			continue
		}
		return false
	}
	return sawCleanup
}

func mktempAssignment(fields []string) (string, bool) {
	if len(fields) == 2 && strings.HasSuffix(fields[0], "=$(mktemp") && fields[1] == "-d)" {
		name := strings.TrimSuffix(fields[0], "=$(mktemp")
		if isShellIdentifier(name) {
			return name, true
		}
		return "", false
	}
	if len(fields) != 1 {
		return "", false
	}

	field := fields[0]
	const suffix = "=$(mktemp -d)"
	if strings.HasSuffix(field, suffix) {
		name := strings.TrimSuffix(field, suffix)
		if isShellIdentifier(name) {
			return name, true
		}
	}
	return "", false
}

func isTempVarReassignment(fields []string, tempVars map[string]bool) bool {
	if len(fields) != 1 || !strings.Contains(fields[0], "=") {
		return false
	}
	parts := strings.SplitN(fields[0], "=", 2)
	return tempVars[parts[0]]
}

func isMktempSetupCommand(fields []string, tempVars map[string]bool) bool {
	if len(fields) < 3 {
		return false
	}
	switch fields[0] {
	case "mkdir":
		return fields[1] == "-p" && allTargetsUseTempVar(fields[2:], tempVars)
	case "chmod":
		return allTargetsUseTempVar(fields[2:], tempVars)
	default:
		return false
	}
}

func isMktempCleanupCommand(fields []string, tempVars map[string]bool) bool {
	if len(fields) < 3 || fields[0] != "rm" {
		return false
	}
	hasRecursive := false
	targetStart := 1
	for targetStart < len(fields) && strings.HasPrefix(fields[targetStart], "-") {
		flag := fields[targetStart]
		if strings.ContainsAny(flag, "rR") || flag == "--recursive" {
			hasRecursive = true
		}
		targetStart++
	}
	if !hasRecursive || targetStart >= len(fields) {
		return false
	}
	return allTargetsUseTempVar(fields[targetStart:], tempVars)
}

func allTargetsUseTempVar(targets []string, tempVars map[string]bool) bool {
	if len(targets) == 0 {
		return false
	}
	for _, target := range targets {
		if !targetUsesTempVar(target, tempVars) {
			return false
		}
	}
	return true
}

func targetUsesTempVar(target string, tempVars map[string]bool) bool {
	target = strings.Trim(target, `"'`)
	for name := range tempVars {
		if target == "$"+name || target == "${"+name+"}" ||
			strings.HasPrefix(target, "$"+name+"/") ||
			strings.HasPrefix(target, "${"+name+"}/") {
			return true
		}
	}
	return false
}

func isShellIdentifier(value string) bool {
	if value == "" {
		return false
	}
	for i, r := range value {
		if i == 0 {
			if (r < 'A' || r > 'Z') && (r < 'a' || r > 'z') && r != '_' {
				return false
			}
			continue
		}
		if (r < 'A' || r > 'Z') && (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '_' {
			return false
		}
	}
	return true
}
