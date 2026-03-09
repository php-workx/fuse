package policy

import "regexp"

// HardcodedBlocked contains the 22 non-overridable BLOCKED rules compiled into
// the binary. These protect against catastrophic system destruction and fuse
// self-protection. They cannot be disabled via disabled_builtins or policy.yaml.
var HardcodedBlocked = []HardcodedRule{
	// === Catastrophic filesystem destruction ===

	// rm -rf with combined short flags (e.g., -rf, -rfi, -fir)
	{
		Pattern: regexp.MustCompile(`\brm\s+(-[a-zA-Z]*r[a-zA-Z]*f|f[a-zA-Z]*r)\s+[/~$]`),
		Reason:  "Recursive force-remove of root, home, or variable path",
	},
	{
		Pattern: regexp.MustCompile(`\brm\s+(-[a-zA-Z]*r[a-zA-Z]*f|f[a-zA-Z]*r)\s+/\*`),
		Reason:  "Recursive force-remove of /*",
	},

	// rm with split short flags (e.g., -r -f, -f -r)
	{
		Pattern: regexp.MustCompile(`\brm\s+.*-r\b.*-f\b.*[/~$]`),
		Reason:  "Recursive force-remove (split flags) of root/home",
	},
	{
		Pattern: regexp.MustCompile(`\brm\s+.*-f\b.*-r\b.*[/~$]`),
		Reason:  "Recursive force-remove (split flags) of root/home",
	},

	// rm with long flags (--recursive --force in any order)
	{
		Pattern: regexp.MustCompile(`\brm\s+.*--recursive\b.*--force\b.*[/~$]`),
		Reason:  "Recursive force-remove (long flags) of root/home",
	},
	{
		Pattern: regexp.MustCompile(`\brm\s+.*--force\b.*--recursive\b.*[/~$]`),
		Reason:  "Recursive force-remove (long flags) of root/home",
	},

	// rm with mixed short/long flags
	{
		Pattern: regexp.MustCompile(`\brm\s+.*-r\b.*--force\b.*[/~$]`),
		Reason:  "Recursive force-remove (mixed flags) of root/home",
	},
	{
		Pattern: regexp.MustCompile(`\brm\s+.*--recursive\b.*-f\b.*[/~$]`),
		Reason:  "Recursive force-remove (mixed flags) of root/home",
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
		Pattern: regexp.MustCompile(`>\s*/dev/[a-z]`),
		Reason:  "Redirect to raw device",
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

	// Prevent agent from disabling/uninstalling fuse
	{
		Pattern: regexp.MustCompile(`\bfuse\s+(disable|uninstall|enable)\b`),
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

	// Prevent agent from directly manipulating fuse SQLite database
	{
		Pattern: regexp.MustCompile(`\bsqlite3?\s+.*fuse\.db\b`),
		Reason:  "Cannot directly access fuse database",
	},

	// Prevent inline interpreter/eval commands from touching fuse-managed files
	{
		Pattern: regexp.MustCompile(`\b(python[23]?|node|perl|ruby|(ba)?sh)\s+(-c|-e|--eval)\b.*(~/\.fuse/|\.fuse/|\.claude/settings\.json|fuse\.db|secret\.key)`),
		Reason:  "Cannot reference fuse-managed files through inline interpreter/eval",
	},
}
