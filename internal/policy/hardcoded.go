package policy

import (
	"regexp"
	"strings"
)

// HardcodedBlocked contains the non-overridable BLOCKED rules compiled into
// the binary. These protect against catastrophic system destruction and fuse
// self-protection. They cannot be disabled via disabled_builtins or policy.yaml.
// unsafeDevRedirect matches redirects to actual block/char devices.
var unsafeDevRedirect = regexp.MustCompile(`>\s*/dev/(?:sd|vd|hd|xvd|nvme|disk|loop|dm-|md|nbd|sr|mapper|tcp|udp)[a-z0-9/]`)

// hasUnsafeDevRedirect returns true if the command contains a redirect to a raw
// device that isn't /dev/null, /dev/stderr, /dev/stdout, or /dev/fd.
func hasUnsafeDevRedirect(cmd string) bool {
	return unsafeDevRedirect.MatchString(cmd)
}

// catastrophicPaths are top-level directories where rm -rf would be catastrophic.
// Paths like /tmp/mydir, /var/folders/xxx, or /Users/dev/project are NOT catastrophic.
var catastrophicPaths = map[string]bool{
	"/":     true,
	"/etc":  true,
	"/usr":  true,
	"/var":  true,
	"/bin":  true,
	"/sbin": true,
	"/lib":  true,
	"/boot": true,
	"/root": true,
	"/home": true,
	"/opt":  true,
	"/srv":  true,
	"/sys":  true,
	"/proc": true,
}

// isCatastrophicRmTarget returns true if any rm target path is catastrophic.
// Matches: /*, ~, ~/, $VAR, or a top-level system directory.
// Does NOT match specific subdirectories like /tmp/mydir or /var/folders/xxx.
func isCatastrophicRmTarget(cmd string) bool {
	// Always block: /* and variable-expanded targets.
	if strings.Contains(cmd, " /*") || strings.Contains(cmd, " $") {
		return true
	}
	// Check each argument after flags for catastrophic paths.
	fields := strings.Fields(cmd)
	for _, f := range fields {
		if strings.HasPrefix(f, "-") {
			continue
		}
		if f == "rm" {
			continue
		}
		// Treat tilde expansion as targeting the user's home directory.
		if f == "~" || strings.HasPrefix(f, "~/") {
			return true
		}
		// Clean the path and check against catastrophic list.
		clean := strings.TrimRight(f, "/")
		if clean == "" {
			clean = "/" // root
		}
		if catastrophicPaths[clean] {
			return true
		}
	}
	return false
}

var HardcodedBlocked = []HardcodedRule{
	// === Catastrophic filesystem destruction ===

	// rm -rf targeting catastrophic paths (/, /etc, /usr, /home, ~, $VAR, /*).
	// The regex matches any rm with recursive+force flags. The predicate checks
	// whether ANY argument targets a catastrophic path — including cases like
	// `rm -rf ./safe /etc` where the catastrophic path is not the first argument.
	{
		Pattern:   regexp.MustCompile(`\brm\s+(-[a-zA-Z]*r[a-zA-Z]*f|f[a-zA-Z]*r)\b`),
		Reason:    "Recursive force-remove of system directory, home, or variable path",
		Predicate: isCatastrophicRmTarget,
	},

	// rm with split short flags (e.g., -r -f, -f -r)
	{
		Pattern:   regexp.MustCompile(`\brm\s+.*-r\b.*-f\b`),
		Reason:    "Recursive force-remove (split flags) of system directory",
		Predicate: isCatastrophicRmTarget,
	},
	{
		Pattern:   regexp.MustCompile(`\brm\s+.*-f\b.*-r\b`),
		Reason:    "Recursive force-remove (split flags) of system directory",
		Predicate: isCatastrophicRmTarget,
	},

	// rm with long flags (--recursive --force in any order)
	{
		Pattern:   regexp.MustCompile(`\brm\s+.*--recursive\b.*--force\b`),
		Reason:    "Recursive force-remove (long flags) of system directory",
		Predicate: isCatastrophicRmTarget,
	},
	{
		Pattern:   regexp.MustCompile(`\brm\s+.*--force\b.*--recursive\b`),
		Reason:    "Recursive force-remove (long flags) of system directory",
		Predicate: isCatastrophicRmTarget,
	},

	// rm with mixed short/long flags
	{
		Pattern:   regexp.MustCompile(`\brm\s+.*-r\b.*--force\b`),
		Reason:    "Recursive force-remove (mixed flags) of system directory",
		Predicate: isCatastrophicRmTarget,
	},
	{
		Pattern:   regexp.MustCompile(`\brm\s+.*--recursive\b.*-f\b`),
		Reason:    "Recursive force-remove (mixed flags) of system directory",
		Predicate: isCatastrophicRmTarget,
	},

	// Filesystem formatting
	{
		Pattern: regexp.MustCompile(`\bmkfs\b`),
		Reason:  "Filesystem formatting",
	},
	{
		Pattern: regexp.MustCompile(`\bmkswap\s+/dev/`),
		Reason:  "Swap formatting on device",
	},

	// Raw disk overwrite
	{
		Pattern: regexp.MustCompile(`\bdd\b.*\bof=/dev/[a-z]`),
		Reason:  "Raw disk write via dd",
	},
	{
		// Block redirects to raw devices like /dev/sda, /dev/vda, /dev/disk0.
		// Exclude /dev/null, /dev/stderr, /dev/stdout, /dev/fd (safe standard redirects).
		Pattern:   regexp.MustCompile(`>\s*/dev/[a-z]`),
		Reason:    "Redirect to raw device",
		Predicate: hasUnsafeDevRedirect,
	},

	// Fork bomb
	{
		Pattern: regexp.MustCompile(`:\(\)\s*\{\s*:\|:\s*&\s*\}\s*;?\s*:`),
		Reason:  "Fork bomb",
	},

	// Catastrophic permission changes
	{
		Pattern: regexp.MustCompile(`\bchmod\s+(-[a-zA-Z]*R[a-zA-Z]*\s+)?777\s+/\s*$`),
		Reason:  "chmod 777 on root",
	},
	{
		Pattern: regexp.MustCompile(`\bchown\s+(-[a-zA-Z]*R[a-zA-Z]*\s+)\S+\s+/\s*$`),
		Reason:  "chown on root",
	},

	// === Self-protection: fuse runtime integrity ===

	// Prevent agent from disabling/uninstalling fuse (only at command position)
	{
		Pattern: regexp.MustCompile(`(^|[;&|]\s*)fuse\s+(disable|uninstall|enable|dryrun)\b`),
		Reason:  "Cannot modify fuse state through mediated path",
	},

	// Prevent agent from modifying fuse config/policy files
	{
		Pattern: regexp.MustCompile(`(>|>>|tee|cp|mv|sed\s+-i|cat\s+.*>)\s*.*[~/.]fuse/config/`),
		Reason:  "Cannot modify fuse configuration through mediated path",
	},
	{
		Pattern: regexp.MustCompile(`(>|>>|tee|cp|mv|sed\s+-i|cat\s+.*>)\s*.*\.claude/settings\.json`),
		Reason:  "Cannot modify Claude Code hooks through mediated path",
	},
	{
		Pattern: regexp.MustCompile(`\brm\s+.*[~/.]fuse/`),
		Reason:  "Cannot delete fuse files through mediated path",
	},
	{
		Pattern: regexp.MustCompile(`\brm\s+.*\.claude/settings\.json`),
		Reason:  "Cannot delete Claude Code settings through mediated path",
	},

	// Prevent agent from modifying fuse SQLite database (allow read-only queries).
	// Matches destructive SQL whether it appears before or after the db path.
	{
		Pattern: regexp.MustCompile(
			`\bsqlite3?\b.*\b(DELETE|DROP|INSERT|UPDATE|ALTER|ATTACH|DETACH|VACUUM|REINDEX)\b.*fuse\.db` +
				`|\bsqlite3?\s+.*fuse\.db\b.*\b(DELETE|DROP|INSERT|UPDATE|ALTER|ATTACH|DETACH|VACUUM|REINDEX)\b`),
		Reason: "Cannot modify fuse database through mediated path",
	},

	// Prevent inline interpreter/eval commands from touching fuse-managed files
	{
		Pattern: regexp.MustCompile(`\b(python[23]?|node|perl|ruby|(ba)?sh)\s+(-c|-e|--eval)\b.*(~/\.fuse/|\.fuse/|\.claude/settings\.json|fuse\.db|secret\.key)`),
		Reason:  "Cannot reference fuse-managed files through inline interpreter/eval",
	},
}
