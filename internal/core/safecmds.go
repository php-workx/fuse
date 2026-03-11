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
	// Read-only help/docs
	"man": true, "info": true, "tldr": true, "help": true,
	// Linters / formatters / test runners (single-word basenames)
	"eslint": true, "prettier": true, "black": true, "ruff": true,
	"mypy": true, "pylint": true, "flake8": true, "gofmt": true,
	"golint": true, "rustfmt": true, "goimports": true, "pytest": true,
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

// IsUnconditionalSafe returns true if the command basename (no path) is in the
// unconditionally safe set. For multi-word safe commands (e.g. "cargo test"),
// callers should use IsUnconditionalSafeCmd which checks the full command.
func IsUnconditionalSafe(basename string) bool {
	return unconditionalSafe[basename]
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

	// Check single-word safe set.
	if unconditionalSafe[basename] {
		return true
	}

	// Check multi-word prefixes against the normalized (Fields-joined) command.
	normalized := strings.Join(fields, " ")
	for _, prefix := range unconditionalSafePrefixes {
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

// isGcloudSafe: gcloud is safe with describe, list, config list, info, auth list.
func isGcloudSafe(fields []string) bool {
	if len(fields) < 2 {
		return false
	}

	// Find the first non-flag token after "gcloud".
	idx := 1
	for idx < len(fields) && strings.HasPrefix(fields[idx], "-") {
		idx++
		if idx < len(fields) && !strings.HasPrefix(fields[idx], "-") {
			idx++
		}
	}
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
