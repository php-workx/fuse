package core

import (
	"path/filepath"
	"regexp"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// Compiled regexes for DisplayNormalize.
var (
	reNullBytes  = regexp.MustCompile(`\x00`)
	reANSIEscape = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)
	reInternalWS = regexp.MustCompile(`[ \t]+`)
)

// wrapperBinaries is the set of known wrapper/prefix commands that should be
// stripped during classification normalization.
var wrapperBinaries = map[string]bool{
	"sudo":    true,
	"doas":    true,
	"env":     true,
	"command": true,
	"nohup":   true,
	"time":    true,
	"nice":    true,
	"ionice":  true,
	"timeout": true,
	"strace":  true,
	"ltrace":  true,
	"taskset": true,
	"setsid":  true,
	"chroot":  true,
}

// escalationWrappers trigger the EscalateClassification flag.
var escalationWrappers = map[string]bool{
	"sudo": true,
	"doas": true,
}

// DisplayNormalize sanitizes a raw command string for display and approval hashing.
func DisplayNormalize(raw string) string {
	// 1. Strip null bytes
	s := reNullBytes.ReplaceAllString(raw, "")

	// 2. Strip ANSI escape sequences
	s = reANSIEscape.ReplaceAllString(s, "")

	// 3. NFKC Unicode normalization
	s = norm.NFKC.String(s)

	// 4. Strip Unicode control characters (Cc, Cf) except \n and \t
	s = stripControlChars(s)

	// 5. Strip non-ASCII whitespace (NBSP, em space, zero-width space, etc.)
	s = stripNonASCIIWhitespace(s)

	// 6. Trim leading/trailing whitespace
	s = strings.TrimSpace(s)

	// 7. Collapse internal runs of whitespace to single space
	s = reInternalWS.ReplaceAllString(s, " ")

	return s
}

// stripControlChars removes Unicode control characters (Cc, Cf) except \n and \t.
func stripControlChars(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if r == '\n' || r == '\t' {
			b.WriteRune(r)
			continue
		}
		if unicode.Is(unicode.Cc, r) || unicode.Is(unicode.Cf, r) {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// stripNonASCIIWhitespace removes whitespace characters that are outside the
// ASCII range (U+00A0 NBSP, U+200B zero-width space, U+2003 em space, etc.).
func stripNonASCIIWhitespace(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		// Keep ASCII characters as-is
		if r <= 0x7F {
			b.WriteRune(r)
			continue
		}
		// Drop non-ASCII whitespace
		if unicode.Is(unicode.Zs, r) || r == '\u200B' || r == '\uFEFF' {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// ClassificationNormalize performs classification normalization on a single
// sub-command (already display-normalized). It extracts the outer command
// after stripping wrappers and paths, plus any inner commands from
// bash -c / sh -c or ssh invocations.
func ClassificationNormalize(subCommand string) ClassifiedCommand {
	return classificationNormalizeRecursive(subCommand, 0)
}

const maxRecursionDepth = 3

func classificationNormalizeRecursive(subCommand string, depth int) ClassifiedCommand {
	result := ClassifiedCommand{}

	tokens, unbalancedQuotes := tokenizeQuoteAware(subCommand)
	if len(tokens) == 0 {
		result.Outer = ""
		return result
	}

	i := 0

	// Strip wrapper prefixes and their flags/arguments.
	i = stripWrappers(tokens, i, &result)

	if i >= len(tokens) {
		result.Outer = ""
		return result
	}

	// Extract basename from first token if it contains '/'.
	firstToken := tokens[i]
	if strings.Contains(firstToken, "/") {
		firstToken = filepath.Base(firstToken)
	}

	// Check for bash -c / sh -c pattern.
	if (firstToken == "bash" || firstToken == "sh") && i+1 < len(tokens) {
		innerCmd, ok := extractBashCInner(tokens, i)
		if ok && !unbalancedQuotes {
			// The outer command is the full remaining tokens joined.
			result.Outer = strings.Join(tokens[i:], " ")
			if depth < maxRecursionDepth {
				innerResult := classificationNormalizeRecursive(innerCmd, depth+1)
				result.Inner = append(result.Inner, innerCmd)
				result.Inner = append(result.Inner, innerResult.Inner...)
				if innerResult.EscalateClassification {
					result.EscalateClassification = true
				}
			} else {
				result.Inner = append(result.Inner, innerCmd)
			}
			return result
		}
		// bash/sh with -c detected but extraction failed: either the tokenizer
		// extracted ok but quotes were unbalanced (ok==true, unbalancedQuotes==true),
		// or -c was present but had no argument token (ok==false with -c in tokens).
		if ok || containsDashC(tokens[i+1:]) {
			result.ExtractionFailed = true
		}
	}

	// Check for ssh pattern.
	if firstToken == "ssh" && i+1 < len(tokens) {
		remoteCmd, ok := extractSSHRemoteCommand(tokens, i)
		if ok {
			result.Outer = strings.Join(tokens[i:], " ")
			result.Inner = append(result.Inner, remoteCmd)
			return result
		}
	}

	// Build outer command: replace first token with basename, keep rest.
	remaining := make([]string, 0, len(tokens)-i)
	remaining = append(remaining, firstToken)
	remaining = append(remaining, tokens[i+1:]...)
	result.Outer = strings.Join(remaining, " ")

	return result
}

// tokenizeQuoteAware splits a command string into tokens, respecting single and double quotes. Also
// reports whether the input had unbalanced (unclosed) quotes.
func tokenizeQuoteAware(s string) ([]string, bool) {
	var tokens []string
	var current strings.Builder
	inSingle := false
	inDouble := false
	escaped := false

	for idx := 0; idx < len(s); idx++ {
		ch := s[idx]

		if escaped {
			current.WriteByte(ch)
			escaped = false
			continue
		}

		if ch == '\\' && !inSingle {
			escaped = true
			// In double-quote context, keep the backslash for non-special chars
			// For simplicity in tokenization, we just skip the backslash
			continue
		}

		if ch == '\'' && !inDouble {
			inSingle = !inSingle
			continue
		}

		if ch == '"' && !inSingle {
			inDouble = !inDouble
			continue
		}

		if (ch == ' ' || ch == '\t') && !inSingle && !inDouble {
			if current.Len() > 0 {
				tokens = append(tokens, current.String())
				current.Reset()
			}
			continue
		}

		current.WriteByte(ch)
	}

	if current.Len() > 0 {
		tokens = append(tokens, current.String())
	}

	unbalanced := inSingle || inDouble
	return tokens, unbalanced
}

// stripWrappers removes known wrapper prefixes and their flags/arguments
// from the token list starting at index i. Returns the new index past all wrappers.
func stripWrappers(tokens []string, i int, result *ClassifiedCommand) int {
	for i < len(tokens) {
		tok := tokens[i]
		// Extract basename if wrapper is invoked by path.
		base := tok
		if strings.Contains(tok, "/") {
			base = filepath.Base(tok)
		}

		if !wrapperBinaries[base] {
			break
		}

		if escalationWrappers[base] {
			result.EscalateClassification = true
		}

		i++ // skip the wrapper binary itself

		// Skip wrapper-specific flags and arguments.
		switch base {
		case "sudo":
			i = skipSudoArgs(tokens, i)
		case "doas":
			i = skipDoasArgs(tokens, i)
		case "env":
			i = skipEnvArgs(tokens, i)
		case "nice":
			i = skipNiceArgs(tokens, i)
		case "ionice":
			i = skipIoniceArgs(tokens, i)
		case "timeout":
			i = skipTimeoutArgs(tokens, i)
		case "strace", "ltrace":
			i = skipTraceArgs(tokens, i)
		case "taskset":
			i = skipTasksetArgs(tokens, i)
		case "chroot":
			i = skipChrootArgs(tokens, i)
		case "setsid":
			i = skipSetsidArgs(tokens, i)
		default:
			// command, nohup, time: no special flag handling needed
		}
	}
	return i
}

// skipSudoArgs skips sudo's flags and their arguments.
// Handles: -u user, -i, -s, --preserve-env, -E, and other single-letter flags.
func skipSudoArgs(tokens []string, i int) int {
	for i < len(tokens) {
		t := tokens[i]
		if !strings.HasPrefix(t, "-") {
			break
		}
		if t == "--" {
			i++
			break
		}
		if t == "--preserve-env" || t == "-E" || t == "-i" || t == "-s" ||
			t == "-n" || t == "-k" || t == "-K" || t == "-H" || t == "-P" ||
			t == "-S" || t == "-b" {
			i++
			continue
		}
		if t == "-u" || t == "--user" || t == "-g" || t == "--group" ||
			t == "-C" || t == "-D" || t == "--chdir" {
			i += 2 // skip flag and its argument
			continue
		}
		// Combined short flags like -iu, -su, etc.
		if len(t) > 1 && t[0] == '-' && t[1] != '-' {
			// Check if 'u' is in the combined flags (requires next arg)
			if strings.ContainsRune(t, 'u') {
				i += 2 // skip combined flag + user arg
			} else {
				i++
			}
			continue
		}
		break
	}
	return i
}

// skipDoasArgs skips doas flags.
func skipDoasArgs(tokens []string, i int) int {
	for i < len(tokens) {
		t := tokens[i]
		if !strings.HasPrefix(t, "-") {
			break
		}
		if t == "--" {
			i++
			break
		}
		if t == "-u" {
			i += 2
			continue
		}
		if t == "-n" || t == "-s" || t == "-L" {
			i++
			continue
		}
		// Combined flags
		if len(t) > 1 && t[0] == '-' && t[1] != '-' {
			if strings.ContainsRune(t, 'u') {
				i += 2
			} else {
				i++
			}
			continue
		}
		break
	}
	return i
}

// skipEnvArgs skips env flags and VAR=val assignments.
func skipEnvArgs(tokens []string, i int) int {
	for i < len(tokens) {
		t := tokens[i]
		if t == "--" {
			i++
			break
		}
		if t == "-i" || t == "--ignore-environment" || t == "-0" || t == "--null" {
			i++
			continue
		}
		if t == "-u" || t == "--unset" {
			i += 2
			continue
		}
		// Skip VAR=val assignments (before the command)
		if strings.Contains(t, "=") && !strings.HasPrefix(t, "-") && isEnvAssignment(t) {
			i++
			continue
		}
		if strings.HasPrefix(t, "-") && len(t) > 1 {
			i++
			continue
		}
		break
	}
	return i
}

// isEnvAssignment checks if a token looks like a VAR=value assignment.
func isEnvAssignment(t string) bool {
	idx := strings.Index(t, "=")
	if idx <= 0 {
		return false
	}
	name := t[:idx]
	for i, ch := range name {
		if i == 0 {
			if !unicode.IsLetter(ch) && ch != '_' {
				return false
			}
		} else {
			if !unicode.IsLetter(ch) && !unicode.IsDigit(ch) && ch != '_' {
				return false
			}
		}
	}
	return true
}

// skipNiceArgs skips nice flags: -n <priority>, --adjustment=N
func skipNiceArgs(tokens []string, i int) int {
	for i < len(tokens) {
		t := tokens[i]
		if !strings.HasPrefix(t, "-") {
			break
		}
		if t == "-n" || t == "--adjustment" {
			i += 2
			continue
		}
		if strings.HasPrefix(t, "-n") || strings.HasPrefix(t, "--adjustment=") {
			i++
			continue
		}
		// Numeric argument like -10
		if len(t) > 1 && t[0] == '-' && t[1] >= '0' && t[1] <= '9' {
			i++
			continue
		}
		break
	}
	return i
}

// skipIoniceArgs skips ionice flags.
func skipIoniceArgs(tokens []string, i int) int {
	for i < len(tokens) {
		t := tokens[i]
		if !strings.HasPrefix(t, "-") {
			break
		}
		if t == "-c" || t == "--class" || t == "-n" || t == "--classdata" ||
			t == "-p" || t == "--pid" {
			i += 2
			continue
		}
		if strings.HasPrefix(t, "-c") || strings.HasPrefix(t, "-n") ||
			strings.HasPrefix(t, "-p") {
			i++
			continue
		}
		if t == "-t" || t == "--ignore" {
			i++
			continue
		}
		break
	}
	return i
}

// skipTimeoutArgs skips timeout flags and the duration argument.
func skipTimeoutArgs(tokens []string, i int) int {
	for i < len(tokens) {
		t := tokens[i]
		if t == "--" {
			i++
			break
		}
		if t == "-s" || t == "--signal" {
			i += 2
			continue
		}
		if t == "-k" || t == "--kill-after" {
			i += 2
			continue
		}
		if t == "--foreground" || t == "--preserve-status" || t == "-v" || t == "--verbose" {
			i++
			continue
		}
		if strings.HasPrefix(t, "-s") || strings.HasPrefix(t, "-k") ||
			strings.HasPrefix(t, "--signal=") || strings.HasPrefix(t, "--kill-after=") {
			i++
			continue
		}
		// The duration argument (e.g., "10", "5s", "1m")
		if !strings.HasPrefix(t, "-") {
			i++ // skip the duration
			break
		}
		break
	}
	return i
}

// skipTraceArgs skips strace/ltrace flags.
func skipTraceArgs(tokens []string, i int) int {
	for i < len(tokens) {
		t := tokens[i]
		if !strings.HasPrefix(t, "-") {
			break
		}
		if t == "--" {
			i++
			break
		}
		// Flags that take an argument
		if t == "-e" || t == "-o" || t == "-p" || t == "-s" || t == "-a" ||
			t == "-S" || t == "-P" || t == "-I" || t == "-b" || t == "-X" {
			i += 2
			continue
		}
		// Other flags (no argument)
		i++
		continue
	}
	return i
}

// skipTasksetArgs skips taskset flags.
func skipTasksetArgs(tokens []string, i int) int {
	for i < len(tokens) {
		t := tokens[i]
		if t == "-p" || t == "--pid" || t == "-a" || t == "--all-tasks" ||
			t == "-c" || t == "--cpu-list" {
			i++
			continue
		}
		// The mask/cpu-list argument
		if !strings.HasPrefix(t, "-") {
			i++ // skip the mask
			break
		}
		break
	}
	return i
}

// skipChrootArgs skips chroot arguments (the directory).
func skipChrootArgs(tokens []string, i int) int {
	// chroot <directory> <command...>
	// Skip options first
	for i < len(tokens) {
		t := tokens[i]
		if t == "--userspec" || t == "--groups" {
			i += 2
			continue
		}
		if strings.HasPrefix(t, "--userspec=") || strings.HasPrefix(t, "--groups=") {
			i++
			continue
		}
		if t == "--skip-chdir" {
			i++
			continue
		}
		break
	}
	// Skip the directory argument
	if i < len(tokens) {
		i++
	}
	return i
}

// skipSetsidArgs skips setsid flags.
func skipSetsidArgs(tokens []string, i int) int {
	for i < len(tokens) {
		t := tokens[i]
		if t == "-c" || t == "--ctty" || t == "-f" || t == "--fork" ||
			t == "-w" || t == "--wait" {
			i++
			continue
		}
		break
	}
	return i
}

// containsDashC checks if any token in the slice is "-c" or a combined short flag
// ending in "c" (like "-lc", "-xc"). Used to distinguish "bash script.sh" (no -c)
// from "bash -c ..." where extraction failed.
func containsDashC(tokens []string) bool {
	for _, t := range tokens {
		if t == "-c" {
			return true
		}
		if len(t) > 2 && t[0] == '-' && t[1] != '-' && strings.HasSuffix(t, "c") {
			return true
		}
	}
	return false
}

// extractBashCInner extracts the inner command from "bash -c 'cmd'" or "sh -c 'cmd'" patterns.
// tokens[i] should be "bash" or "sh". Returns the inner command string and true if found.
func extractBashCInner(tokens []string, i int) (string, bool) {
	j := i + 1
	// Skip short option flags before -c (e.g., -l, -x, -e, etc.)
	for j < len(tokens) {
		t := tokens[j]
		if t == "-c" {
			// Found -c; the next token is the inner command.
			if j+1 < len(tokens) {
				return tokens[j+1], true
			}
			return "", false
		}
		// Accept combined flags like -lc, -xc
		if len(t) > 1 && t[0] == '-' && t[1] != '-' && strings.HasSuffix(t, "c") {
			// This is a combined flag ending in 'c', next token is the inner command.
			if j+1 < len(tokens) {
				return tokens[j+1], true
			}
			return "", false
		}
		// Skip other short flags (like -l, -x, -e, etc.)
		if len(t) > 1 && t[0] == '-' && t[1] != '-' {
			j++
			continue
		}
		break
	}
	return "", false
}

// extractSSHRemoteCommand extracts the remote command from an ssh invocation.
// tokens[i] should be "ssh". Skips ssh flags and user@host to find the remote command.
func extractSSHRemoteCommand(tokens []string, i int) (string, bool) {
	j := i + 1

	// SSH flags that take an argument.
	sshFlagsWithArg := map[string]bool{
		"-b": true, "-c": true, "-D": true, "-E": true, "-e": true,
		"-F": true, "-I": true, "-i": true, "-J": true, "-L": true,
		"-l": true, "-m": true, "-O": true, "-o": true, "-p": true,
		"-Q": true, "-R": true, "-S": true, "-W": true, "-w": true,
	}

	// SSH flags that don't take an argument.
	sshFlagsNoArg := map[string]bool{
		"-4": true, "-6": true, "-A": true, "-a": true, "-C": true,
		"-f": true, "-G": true, "-g": true, "-K": true, "-k": true,
		"-M": true, "-N": true, "-n": true, "-q": true, "-s": true,
		"-T": true, "-t": true, "-V": true, "-v": true, "-X": true,
		"-x": true, "-Y": true, "-y": true,
	}

	// Skip flags and find the host, then the remote command.
	hostFound := false
	for j < len(tokens) {
		t := tokens[j]

		if sshFlagsWithArg[t] {
			if j+1 >= len(tokens) {
				break
			}
			j += 2
			continue
		}
		if sshFlagsNoArg[t] {
			j++
			continue
		}
		// Long options
		if strings.HasPrefix(t, "-") && len(t) > 1 {
			j++
			continue
		}

		if !hostFound {
			// This token is the host (or user@host)
			hostFound = true
			j++
			continue
		}

		// Everything after the host is the remote command.
		remoteCmd := strings.Join(tokens[j:], " ")
		if remoteCmd != "" {
			return remoteCmd, true
		}
		return "", false
	}

	return "", false
}
