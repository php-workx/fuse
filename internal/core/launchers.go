package core

import (
	"slices"
	"strings"
)

// Launchers run another command passed as trailing arguments (mise exec --
// cmd, stdbuf -oL cmd, uv run cmd). Rules anchored at command start never see
// that command unless normalization extracts it, so it is added as an inner
// command while the launcher itself stays the outer command.

// launcherSpec describes how a launcher lays out its arguments.
type launcherSpec struct {
	subcommands        []string        // required subcommand, e.g. "exec" or "run"; empty when none
	valueFlags         map[string]bool // options that consume the following token
	positionals        int             // operands before the command, e.g. flock FILE, chrt PRIORITY
	untilDashDash      bool            // every token before "--" belongs to the launcher (mise exec TOOLS --)
	requireCommandFlag bool            // the command only follows a commandFlags option (nix develop -c)
	commandFlags       map[string]bool // option after which the remaining tokens are the command
	shellFlags         map[string]bool // option whose value is a shell command string (su -c 'cmd')
	shellOnly          bool            // only shellFlags carry a command; bare operands do not
	noCommandFlags     map[string]bool // options meaning no command is launched (chrt -p PID)
	escalate           bool            // runs the command as another user, like sudo
}

func flagSet(flags ...string) map[string]bool {
	m := make(map[string]bool, len(flags))
	for _, f := range flags {
		m[f] = true
	}
	return m
}

var condaRunSpec = launcherSpec{
	subcommands: []string{"run"},
	valueFlags:  flagSet("-n", "--name", "-p", "--prefix", "--cwd"),
}

var launcherSpecs = map[string]launcherSpec{
	// Process attribute and scheduling wrappers.
	"stdbuf": {valueFlags: flagSet("-i", "-o", "-e", "--input", "--output", "--error")},
	"chrt": {
		positionals:    1,
		valueFlags:     flagSet("-T", "-P", "-D", "--sched-runtime", "--sched-period", "--sched-deadline"),
		noCommandFlags: flagSet("-p", "--pid", "-m", "--max"),
	},
	"unbuffer":   {},
	"caffeinate": {valueFlags: flagSet("-t", "-w")},
	"flock":      {positionals: 1, valueFlags: flagSet("-w", "--timeout", "-E", "--conflict-exit-code"), shellFlags: flagSet("-c", "--command")},
	"numactl":    {valueFlags: flagSet("-N", "-m", "-C", "-i", "-p", "-P")},
	"cpulimit":   {valueFlags: flagSet("-l", "--limit"), noCommandFlags: flagSet("-p", "--pid", "-e", "--exe")},
	"trickle":    {valueFlags: flagSet("-d", "-u", "-w", "-t", "-l", "-n")},
	"arch":       {valueFlags: flagSet("-arch", "-e")},
	"xargs": {valueFlags: flagSet(
		"-n", "--max-args", "-I", "-L", "--max-lines", "-P", "--max-procs",
		"-d", "--delimiter", "-E", "-s", "--max-chars", "-a", "--arg-file",
	)},

	// Supervisors, sandboxes, and tracers.
	"systemd-run": {valueFlags: flagSet(
		"-u", "--unit", "-p", "--property", "-E", "--setenv", "-M", "--machine", "-H", "--host",
		"--slice", "--uid", "--gid", "--nice", "--description", "--working-directory", "--service-type",
	)},
	"tini":      {valueFlags: flagSet("-p", "-e")},
	"dumb-init": {valueFlags: flagSet("-r", "--rewrite")},
	"fakeroot":  {valueFlags: flagSet("-l", "--lib", "--faked", "-s", "-i", "-b")},
	"firejail":  {},
	"valgrind":  {},
	"perf": {
		subcommands:    []string{"stat", "record", "trace"},
		valueFlags:     flagSet("-e", "--event", "-o", "--output", "-r", "--repeat", "-C", "--cpu", "-G", "--cgroup", "-x", "-F", "-c"),
		noCommandFlags: flagSet("-p", "--pid", "-t", "--tid"),
	},
	"torsocks":     {valueFlags: flagSet("-u", "-p", "-a", "-P")},
	"proxychains":  {valueFlags: flagSet("-f")},
	"proxychains4": {valueFlags: flagSet("-f")},
	"xcrun":        {valueFlags: flagSet("--sdk", "-sdk", "--toolchain", "-toolchain")},

	// Toolchain, environment, and secret injectors.
	"mise":   {subcommands: []string{"exec", "x"}, untilDashDash: true, shellFlags: flagSet("-c", "--command")},
	"asdf":   {subcommands: []string{"exec"}},
	"direnv": {subcommands: []string{"exec"}, positionals: 1},
	"nix":    {subcommands: []string{"develop", "shell"}, requireCommandFlag: true, commandFlags: flagSet("-c", "--command")},
	"devbox": {subcommands: []string{"run"}, valueFlags: flagSet("-c", "--config", "-e", "--env", "--env-file")},
	"uv": {subcommands: []string{"run"}, valueFlags: flagSet(
		"--with", "--with-requirements", "--python", "-p", "--project", "--directory", "--env-file",
		"--group", "--extra", "--package", "--index", "--index-url", "--from", "-w",
	)},
	"uvx":        {valueFlags: flagSet("--with", "--python", "-p", "--from")},
	"poetry":     {subcommands: []string{"run"}, valueFlags: flagSet("-C", "--directory", "-P", "--project")},
	"pipenv":     {subcommands: []string{"run"}},
	"pixi":       {subcommands: []string{"run", "exec"}, valueFlags: flagSet("-e", "--environment", "--manifest-path", "-s", "--spec")},
	"conda":      condaRunSpec,
	"mamba":      condaRunSpec,
	"micromamba": condaRunSpec,
	"npx":        {valueFlags: flagSet("-p", "--package"), shellFlags: flagSet("-c", "--call")},
	"bunx":       {valueFlags: flagSet("-p", "--package")},
	"pnpm":       {subcommands: []string{"exec", "dlx"}, valueFlags: flagSet("-C", "--dir", "--filter", "-F")},
	"yarn":       {subcommands: []string{"exec", "dlx"}},
	"dotenv":     {valueFlags: flagSet("-e", "-v", "-c")},
	"dotenvx":    {subcommands: []string{"run"}, valueFlags: flagSet("-f", "--env-file", "-e", "--env", "--convention")},
	"doppler": {
		subcommands: []string{"run"},
		valueFlags:  flagSet("-p", "--project", "-c", "--config", "--fallback", "--mount", "--mount-format", "--token"),
		shellFlags:  flagSet("--command"),
	},
	"op":        {subcommands: []string{"run"}, valueFlags: flagSet("--env-file", "--account")},
	"aws-vault": {subcommands: []string{"exec"}, positionals: 1, valueFlags: flagSet("-d", "--duration", "-t", "--mfa-token", "--region", "--backend")},

	// User and group switching.
	"su": {
		shellOnly:  true,
		escalate:   true,
		valueFlags: flagSet("-s", "--shell", "-g", "--group", "-G", "--supp-group", "-w", "--whitelist-environment"),
		shellFlags: flagSet("-c", "--command"),
	},
	"runuser": {
		escalate:   true,
		valueFlags: flagSet("-u", "--user", "-g", "--group", "-G", "--supp-group", "-s", "--shell", "-w"),
		shellFlags: flagSet("-c", "--command"),
	},
	"pkexec": {escalate: true, valueFlags: flagSet("--user", "-u")},
	"sg":     {positionals: 1, shellFlags: flagSet("-c")},
}

// handleLauncher extracts the command run by a known launcher, or the git
// subcommand when global options precede it. Returns true if the result was
// fully resolved (caller should return immediately).
func handleLauncher(tokens []string, i, depth int, firstToken string, result *ClassifiedCommand) bool {
	var innerCmd string
	var ok bool
	spec, isLauncher := launcherSpecs[firstToken]
	switch {
	case isLauncher:
		innerCmd, ok = extractLauncherCommand(tokens, i, spec)
	case firstToken == "git":
		innerCmd, ok = gitCommandWithoutGlobalOptions(tokens, i)
	}
	if !ok {
		return false
	}
	result.Outer = strings.Join(append([]string{firstToken}, tokens[i+1:]...), " ")
	appendInnerCommand(result, innerCmd, depth, true)
	if spec.escalate {
		result.EscalateClassification = true
	}
	return true
}

// extractLauncherCommand returns the command the launcher at tokens[i] runs.
func extractLauncherCommand(tokens []string, i int, spec launcherSpec) (string, bool) {
	j := i + 1
	if len(spec.subcommands) > 0 {
		if j >= len(tokens) || !slices.Contains(spec.subcommands, tokens[j]) {
			return "", false
		}
		j++
	}
	if cmd, ok := launcherShellCommand(tokens[j:], spec); ok || spec.shellOnly {
		return cmd, ok
	}

	positionals := spec.positionals
	sawCommandFlag := false
	for j < len(tokens) {
		t := tokens[j]
		if t == "--" {
			j++
			break
		}
		if spec.untilDashDash {
			j++
			continue
		}
		if spec.noCommandFlags[t] {
			return "", false
		}
		if spec.commandFlags[t] {
			j++
			sawCommandFlag = true
			break
		}
		if strings.HasPrefix(t, "-") && len(t) > 1 {
			j++
			if spec.valueFlags[t] {
				j++
			}
			continue
		}
		if spec.requireCommandFlag {
			j++
			continue
		}
		if positionals > 0 {
			positionals--
			j++
			continue
		}
		break
	}
	if spec.untilDashDash && (j == 0 || tokens[j-1] != "--") {
		return "", false
	}
	if spec.requireCommandFlag && !sawCommandFlag {
		return "", false
	}
	if j >= len(tokens) {
		return "", false
	}
	return strings.Join(tokens[j:], " "), true
}

// launcherShellCommand finds a shell-string option (-c 'cmd', --command=cmd)
// before any "--" terminator.
func launcherShellCommand(args []string, spec launcherSpec) (string, bool) {
	for k, t := range args {
		if t == "--" {
			break
		}
		if spec.shellFlags[t] && k+1 < len(args) {
			return args[k+1], true
		}
		if name, value, found := strings.Cut(t, "="); found && strings.HasPrefix(name, "--") && spec.shellFlags[name] {
			return value, true
		}
	}
	return "", false
}

// gitCommandWithoutGlobalOptions rewrites `git -C dir -c k=v push ...` to
// `git push ...` so rules anchored on the subcommand still apply.
func gitCommandWithoutGlobalOptions(tokens []string, i int) (string, bool) {
	j := i + 1
	for j < len(tokens) && strings.HasPrefix(tokens[j], "-") && tokens[j] != "--" {
		flag := tokens[j]
		j++
		if gitGlobalFlagsWithValue[flag] {
			j++
		}
	}
	if j == i+1 || j >= len(tokens) {
		return "", false
	}
	return "git " + strings.Join(tokens[j:], " "), true
}

// appendInnerCommand records innerCmd as an inner command and, within the
// recursion limit, the commands it contains: its compound parts (a; b, (a)),
// each part's normalized form, and the commands nested inside each part.
func appendInnerCommand(result *ClassifiedCommand, innerCmd string, depth int, propagateFailure bool) {
	result.Inner = append(result.Inner, innerCmd)
	if depth >= maxRecursionDepth {
		return
	}
	parts := []string{innerCmd}
	if split, err := SplitCompoundCommand(innerCmd); err == nil && len(split) > 1 {
		parts = split
		result.Inner = append(result.Inner, split...)
	}
	for _, part := range parts {
		inner := classificationNormalizeRecursive(part, depth+1)
		// Inner commands are classified as written, so also add the normalized
		// form (wrappers stripped, quotes removed) that a top-level command
		// would be classified as: nice git push -> git push, eval "rm -rf ~" ->
		// eval rm -rf ~.
		if inner.Outer != "" && inner.Outer != part {
			result.Inner = append(result.Inner, inner.Outer)
		}
		result.Inner = append(result.Inner, inner.Inner...)
		if inner.EscalateClassification {
			result.EscalateClassification = true
		}
		if propagateFailure && inner.ExtractionFailed {
			result.ExtractionFailed = true
		}
	}
}

// --- Flag skipping for prefix wrappers stripped by stripWrappers ---

// skipTimeArgs skips flags of the time wrapper (GNU time and `time -p`).
func skipTimeArgs(tokens []string, i int) int {
	for i < len(tokens) {
		t := tokens[i]
		switch {
		case t == "-p" || t == "-v" || t == "-q" || t == "-a" ||
			t == "--portability" || t == "--verbose" || t == "--quiet" || t == "--append":
			i++
		case t == "-o" || t == "-f" || t == "--output" || t == "--format":
			i += 2
		case strings.HasPrefix(t, "--output=") || strings.HasPrefix(t, "--format="):
			i++
		default:
			return i
		}
	}
	return i
}

// skipCommandArgs skips `command -p`. The -v and -V forms only look a name up
// and are left in place.
func skipCommandArgs(tokens []string, i int) int {
	for i < len(tokens) && tokens[i] == "-p" {
		i++
	}
	return i
}

// skipExecArgs skips flags of the exec builtin (-c, -l, -a NAME).
func skipExecArgs(tokens []string, i int) int {
	for i < len(tokens) {
		switch tokens[i] {
		case "-c", "-l", "-cl", "-lc":
			i++
		case "-a":
			i += 2
		default:
			return i
		}
	}
	return i
}
