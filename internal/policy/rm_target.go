package policy

import "strings"

// isRecursiveRmOfCatastrophicTarget reports whether cmd starts with rm, asks
// for recursive removal in any spelling (-r, -R, a cluster containing either,
// or --recursive), and targets a catastrophic path.
func isRecursiveRmOfCatastrophicTarget(cmd string) bool {
	fields := strings.Fields(cmd)
	if len(fields) < 2 {
		return false
	}
	executable := fields[0]
	if idx := strings.LastIndexAny(executable, `/\`); idx >= 0 {
		executable = executable[idx+1:]
	}
	if executable != "rm" {
		return false
	}
	return rmArgsRecursive(fields[1:]) && isCatastrophicRmTarget(cmd)
}

// rmArgsRecursive reports whether rm arguments request recursive removal.
// GNU rm accepts options after operands, so scanning continues past them
// until a "--" terminator.
func rmArgsRecursive(args []string) bool {
	for _, arg := range args {
		switch {
		case arg == "--":
			return false
		case arg == "--recursive":
			return true
		case len(arg) > 1 && arg[0] == '-' && arg[1] != '-' && strings.ContainsAny(arg[1:], "rR"):
			return true
		}
	}
	return false
}
