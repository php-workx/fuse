package core

import (
	"path/filepath"
	"regexp"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
	"mvdan.cc/sh/v3/syntax"
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
	// Windows wrapper
	"runas": true,
}

// escalationWrappers trigger the EscalateClassification flag.
var escalationWrappers = map[string]bool{
	"sudo":  true,
	"doas":  true,
	"runas": true,
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

	// Skip leading bare env var assignments (VAR=value before the command).
	i = skipLeadingEnvAssignments(tokens, i, &result)
	if i >= len(tokens) {
		result.Outer = ""
		return result
	}

	// Extract basename from first token if it contains '/' or '\'.
	firstToken := tokens[i]
	if strings.Contains(firstToken, "/") || strings.Contains(firstToken, `\`) {
		firstToken = filepath.Base(firstToken)
		// Handle Windows backslash paths on non-Windows platforms
		if j := strings.LastIndex(firstToken, `\`); j >= 0 {
			firstToken = firstToken[j+1:]
		}
	}

	// Check for bash -c / sh -c pattern.
	if (firstToken == "bash" || firstToken == "sh") && i+1 < len(tokens) {
		if handleBashC(tokens, i, depth, unbalancedQuotes, &result) {
			return result
		}
	}

	// Check for powershell / pwsh pattern.
	lowerFirst := strings.ToLower(firstToken)
	if (lowerFirst == "powershell.exe" || lowerFirst == "powershell" ||
		lowerFirst == "pwsh.exe" || lowerFirst == "pwsh") && i+1 < len(tokens) {
		if handlePowerShellCommand(tokens, i, depth, unbalancedQuotes, &result) {
			return result
		}
	}

	// Check for cmd.exe /c pattern.
	if (lowerFirst == "cmd.exe" || lowerFirst == "cmd") && i+1 < len(tokens) {
		if handleCmdC(tokens, i, depth, unbalancedQuotes, &result) {
			return result
		}
	}

	// Check for wsl wrapper pattern.
	if (lowerFirst == "wsl.exe" || lowerFirst == "wsl") && i+1 < len(tokens) {
		if handleWslCommand(tokens, i, depth, unbalancedQuotes, &result) {
			return result
		}
	}

	// Check for ssh pattern.
	if firstToken == "ssh" && i+1 < len(tokens) {
		if handleSSH(tokens, i, &result) {
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

// skipLeadingEnvAssignments skips bare env var assignments (VAR=value) before the command.
// Must happen before filepath.Base to avoid mangling VAR=/path/value into a basename.
func skipLeadingEnvAssignments(tokens []string, i int, result *ClassifiedCommand) int {
	for i < len(tokens) {
		tok := tokens[i]
		if !isEnvAssignment(tok) || strings.HasPrefix(tok, "-") {
			break
		}
		if isSensitiveEnvAssignment(tok) {
			result.SensitiveEnvAssignment = true
		}
		i++
	}
	return i
}

// handleBashC processes bash/sh -c inner command extraction. Returns true if
// the result was fully resolved (caller should return immediately).
func handleBashC(tokens []string, i, depth int, unbalancedQuotes bool, result *ClassifiedCommand) bool {
	innerCmd, ok := extractBashCInner(tokens, i)
	if ok && !unbalancedQuotes {
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
		return true
	}
	// bash/sh with -c detected but extraction failed: either the tokenizer
	// extracted ok but quotes were unbalanced (ok==true, unbalancedQuotes==true),
	// or -c was present but had no argument token (ok==false with -c in tokens).
	if ok || containsDashC(tokens[i+1:]) {
		result.ExtractionFailed = true
	}
	return false
}

// handleSSH processes ssh remote command extraction. Returns true if
// the result was fully resolved (caller should return immediately).
func handleSSH(tokens []string, i int, result *ClassifiedCommand) bool {
	remoteCmd, ok := extractSSHRemoteCommand(tokens, i)
	if !ok {
		return false
	}
	result.Outer = strings.Join(tokens[i:], " ")
	result.Inner = append(result.Inner, remoteCmd)
	return true
}

// tokenizeQuoteAware splits a command string into tokens, respecting single and double quotes. Also
// reports whether the input had unbalanced (unclosed) quotes.
func tokenizeQuoteAware(s string) ([]string, bool) {
	var tokens []string
	var current strings.Builder
	state := &quoteState{}

	for idx := 0; idx < len(s); idx++ {
		ch := s[idx]
		action := state.processChar(ch)
		switch action {
		case charConsumed:
			current.WriteByte(ch)
		case charToggle:
			// Quote toggled; character itself is not written.
		case charWhitespace:
			if current.Len() > 0 {
				tokens = append(tokens, current.String())
				current.Reset()
			}
		case charEscaped:
			// Backslash consumed; nothing written yet.
		}
	}

	if current.Len() > 0 {
		tokens = append(tokens, current.String())
	}

	return tokens, state.unbalanced()
}

// charAction describes what the tokenizer should do after processing a character.
type charAction int

const (
	charConsumed   charAction = iota // write the character to the current token
	charToggle                       // quote toggled; skip the character
	charWhitespace                   // whitespace outside quotes; flush token
	charEscaped                      // backslash consumed; skip the character
)

// quoteState tracks the quoting context during tokenization.
type quoteState struct {
	inSingle bool
	inDouble bool
	escaped  bool
}

func (q *quoteState) unbalanced() bool {
	return q.inSingle || q.inDouble
}

// processChar evaluates a single character in the current quoting context and
// returns the action the tokenizer should take.
func (q *quoteState) processChar(ch byte) charAction {
	if q.escaped {
		q.escaped = false
		return charConsumed
	}
	if ch == '\\' && !q.inSingle {
		q.escaped = true
		return charEscaped
	}
	if ch == '\'' && !q.inDouble {
		q.inSingle = !q.inSingle
		return charToggle
	}
	if ch == '"' && !q.inSingle {
		q.inDouble = !q.inDouble
		return charToggle
	}
	if (ch == ' ' || ch == '\t') && !q.inSingle && !q.inDouble {
		return charWhitespace
	}
	return charConsumed
}

// stripWrappers removes known wrapper prefixes and their flags/arguments
// from the token list starting at index i. Returns the new index past all wrappers.
func stripWrappers(tokens []string, i int, result *ClassifiedCommand) int {
	for i < len(tokens) {
		tok := tokens[i]
		// Extract basename if wrapper is invoked by path.
		base := tok
		if strings.Contains(tok, "/") || strings.Contains(tok, `\`) {
			base = filepath.Base(tok)
			// Handle Windows backslash paths on non-Windows platforms
			if j := strings.LastIndex(base, `\`); j >= 0 {
				base = base[j+1:]
			}
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
			i = skipEnvArgs(tokens, i, result)
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
		case "runas":
			i = skipRunasArgs(tokens, i)
		default:
			// command, nohup, time: no special flag handling needed
		}
	}
	return i
}

// skipCombinedFlag checks if a token is a combined short flag (e.g., -iu, -su).
// If it contains argFlag, the flag takes an argument so we skip 2 tokens.
// Otherwise we skip 1 token for the flag alone.
// Returns 0 if the token is not a combined short flag.
func skipCombinedFlag(token string, argFlag rune) int {
	if len(token) > 1 && token[0] == '-' && token[1] != '-' {
		if strings.ContainsRune(token, argFlag) {
			return 2 // skip flag + argument
		}
		return 1 // skip flag only
	}
	return 0 // not a combined flag
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
		if skip := skipCombinedFlag(t, 'u'); skip > 0 {
			i += skip
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
		if skip := skipCombinedFlag(t, 'u'); skip > 0 {
			i += skip
			continue
		}
		break
	}
	return i
}

// envFlagSkipCount maps env flags to how many tokens to skip (flag + arguments).
var envFlagSkipCount = map[string]int{
	"-i":                   1,
	"--ignore-environment": 1,
	"-0":                   1,
	"--null":               1,
	"-u":                   2,
	"--unset":              2,
}

// skipEnvArgs skips env flags and VAR=val assignments.
// Sets result.SensitiveEnvAssignment if any skipped assignment is security-sensitive.
func skipEnvArgs(tokens []string, i int, result *ClassifiedCommand) int {
	for i < len(tokens) {
		t := tokens[i]
		if t == "--" {
			i++
			break
		}
		if skip, ok := envFlagSkipCount[t]; ok {
			i += skip
			continue
		}
		if skipEnvVarAssignment(t, result) {
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

// skipEnvVarAssignment checks if a token is a VAR=val assignment and records
// sensitivity. Returns true if the token was consumed.
func skipEnvVarAssignment(t string, result *ClassifiedCommand) bool {
	if !strings.Contains(t, "=") || strings.HasPrefix(t, "-") || !isEnvAssignment(t) {
		return false
	}
	if isSensitiveEnvAssignment(t) {
		result.SensitiveEnvAssignment = true
	}
	return true
}

// isSensitiveEnvAssignment checks if a VAR=value token sets a security-sensitive variable.
func isSensitiveEnvAssignment(token string) bool {
	for _, prefix := range sensitiveEnvPrefixes {
		if strings.HasPrefix(token, prefix) {
			return true
		}
	}
	return false
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

// timeoutFlagsWithArg are timeout flags that consume the next token as an argument.
var timeoutFlagsWithArg = map[string]bool{
	"-s": true, "--signal": true,
	"-k": true, "--kill-after": true,
}

// timeoutFlagsNoArg are timeout flags that don't consume an additional token.
var timeoutFlagsNoArg = map[string]bool{
	"--foreground": true, "--preserve-status": true,
	"-v": true, "--verbose": true,
}

// timeoutCombinedPrefixes are prefix patterns for combined/inline timeout flags.
var timeoutCombinedPrefixes = []string{"-s", "-k", "--signal=", "--kill-after="}

// skipTimeoutArgs skips timeout flags and the duration argument.
func skipTimeoutArgs(tokens []string, i int) int {
	for i < len(tokens) {
		t := tokens[i]
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
		// The duration argument (e.g., "10", "5s", "1m")
		if !strings.HasPrefix(t, "-") {
			i++ // skip the duration
			break
		}
		break
	}
	return i
}

// isTimeoutCombinedFlag checks if a token is a combined/inline timeout flag.
func isTimeoutCombinedFlag(t string) bool {
	for _, prefix := range timeoutCombinedPrefixes {
		if strings.HasPrefix(t, prefix) {
			return true
		}
	}
	return false
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

// --- Inline script extraction (v2 classification pipeline) ---

// maxInlineBodyBytes is the maximum size for extracted inline script bodies (50KB).
const maxInlineBodyBytes = 50 * 1024

// heredocCollector accumulates heredoc body parts during a syntax.Walk,
// truncating when the total size exceeds maxInlineBodyBytes.
type heredocCollector struct {
	parts     []string
	totalSize int
	truncated bool
}

// visit is the syntax.Walk callback for heredoc extraction.
func (c *heredocCollector) visit(node syntax.Node) bool {
	stmt, ok := node.(*syntax.Stmt)
	if !ok {
		return true
	}
	for _, redir := range stmt.Redirs {
		c.collectRedir(redir)
	}
	return true
}

// collectRedir processes a single redirect, extracting heredoc content if applicable.
func (c *heredocCollector) collectRedir(redir *syntax.Redirect) {
	if redir.Op != syntax.Hdoc && redir.Op != syntax.DashHdoc {
		return
	}
	if redir.Hdoc == nil {
		return
	}
	part := printNode(redir.Hdoc)
	c.totalSize += len(part)
	if c.totalSize > maxInlineBodyBytes {
		excess := c.totalSize - maxInlineBodyBytes
		if len(part) > excess {
			part = part[:len(part)-excess]
		} else {
			part = ""
		}
		c.truncated = true
	}
	if part != "" {
		c.parts = append(c.parts, part)
	}
}

// extractHeredocBody uses the mvdan.cc/sh parser to extract heredoc bodies.
// Returns (body, complete). When body > maxInlineBodyBytes, truncates and returns
// complete=false. Skips heredocs attached to cat commands (string quoting, not
// code execution). On parse error or panic, returns ("", false) — fail-closed (SEC-008).
func extractHeredocBody(cmd string) (body string, complete bool) {
	complete = true
	if len(cmd) > maxInputSize {
		return "", false
	}

	defer func() {
		if r := recover(); r != nil {
			body = ""
			complete = false
		}
	}()

	parser := syntax.NewParser(syntax.Variant(syntax.LangBash))
	prog, err := parser.Parse(strings.NewReader(cmd), "")
	if err != nil {
		return "", false
	}

	collector := &heredocCollector{}
	syntax.Walk(prog, collector.visit)

	body = strings.Join(collector.parts, "\n")
	complete = !collector.truncated
	return body, complete
}

// extractCommandSubstitutions uses the mvdan.cc/sh parser to extract $() contents.
// Skips $(cat <<...) patterns (string quoting, not code execution).
// Returns (results, complete). On parse error or panic, returns (nil, false) — fail-closed (SEC-008).
// maxCmdSubstitutions limits extracted $() count for defense-in-depth.
const maxCmdSubstitutions = 50

func extractCommandSubstitutions(cmd string) (results []string, complete bool) {
	complete = true
	if len(cmd) > maxInputSize {
		return nil, false
	}

	defer func() {
		if r := recover(); r != nil {
			results = nil
			complete = false
		}
	}()

	parser := syntax.NewParser(syntax.Variant(syntax.LangBash))
	prog, err := parser.Parse(strings.NewReader(cmd), "")
	if err != nil {
		return nil, false
	}

	syntax.Walk(prog, func(node syntax.Node) bool {
		if len(results) >= maxCmdSubstitutions {
			complete = false
			return false // stop walking
		}
		cs, ok := node.(*syntax.CmdSubst)
		if !ok {
			return true
		}
		content := extractCmdSubstContent(cs)
		if content != "" {
			results = append(results, content)
		}
		return false // don't recurse into nested CmdSubst
	})

	return results, complete
}

// isStmtCatCommand returns true if the statement's command is "cat".
func isStmtCatCommand(stmt *syntax.Stmt) bool {
	if stmt == nil || stmt.Cmd == nil {
		return false
	}
	call, ok := stmt.Cmd.(*syntax.CallExpr)
	if !ok || len(call.Args) == 0 {
		return false
	}
	name := printNode(call.Args[0])
	return filepath.Base(name) == "cat"
}

// extractCmdSubstContent extracts the textual content from a command substitution.
// Returns "" for $(cat <<...) patterns (string quoting, not code execution).
func extractCmdSubstContent(cs *syntax.CmdSubst) string {
	if isCmdSubstCat(cs) {
		return ""
	}
	var parts []string
	for _, stmt := range cs.Stmts {
		if part := printNode(stmt); part != "" {
			parts = append(parts, part)
		}
	}
	return strings.Join(parts, "; ")
}

// isCmdSubstCat returns true if the command substitution is a $(cat <<...) pattern.
func isCmdSubstCat(cs *syntax.CmdSubst) bool {
	if len(cs.Stmts) != 1 {
		return false
	}
	stmt := cs.Stmts[0]
	if !isStmtCatCommand(stmt) {
		return false
	}
	for _, redir := range stmt.Redirs {
		if redir.Op == syntax.Hdoc || redir.Op == syntax.DashHdoc {
			return true
		}
	}
	return false
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

// --- PowerShell alias resolution ---

// powerShellAliases maps common PowerShell aliases and shorthand names to their
// full cmdlet names. Used only when resolving inner commands of an explicit
// powershell/pwsh invocation.
var powerShellAliases = map[string]string{
	"ls": "Get-ChildItem", "dir": "Get-ChildItem", "gci": "Get-ChildItem",
	"cat": "Get-Content", "gc": "Get-Content", "type": "Get-Content",
	"cd": "Set-Location", "chdir": "Set-Location", "sl": "Set-Location",
	"cp": "Copy-Item", "copy": "Copy-Item", "cpi": "Copy-Item",
	"mv": "Move-Item", "move": "Move-Item", "mi": "Move-Item",
	"rm": "Remove-Item", "del": "Remove-Item", "rmdir": "Remove-Item", "rd": "Remove-Item", "ri": "Remove-Item",
	"echo": "Write-Output", "write": "Write-Output",
	"ps": "Get-Process", "gps": "Get-Process",
	"kill": "Stop-Process", "spps": "Stop-Process",
	"cls": "Clear-Host", "clear": "Clear-Host",
	"man": "Get-Help", "help": "Get-Help",
	"where":   "Where-Object",
	"select":  "Select-Object",
	"sort":    "Sort-Object",
	"measure": "Measure-Object",
	"iwr":     "Invoke-WebRequest", "wget": "Invoke-WebRequest", "curl": "Invoke-WebRequest",
	"iex":  "Invoke-Expression",
	"sal":  "Set-Alias",
	"saps": "Start-Process", "start": "Start-Process",
}

// resolvePowerShellAlias returns the canonical PowerShell cmdlet name for
// a known alias, or the original token unchanged if it is not an alias.
func resolvePowerShellAlias(token string) string {
	// PowerShell aliases are case-insensitive.
	if cmdlet, ok := powerShellAliases[strings.ToLower(token)]; ok {
		return cmdlet
	}
	return token
}

// --- Runas flag skipping ---

// skipRunasArgs skips runas flags. runas /user:admin cmd.exe ...
func skipRunasArgs(tokens []string, i int) int {
	for i < len(tokens) {
		t := tokens[i]
		if !strings.HasPrefix(t, "/") {
			break
		}
		// /user:X, /profile, /env, /netonly, /savecred, /smartcard, /noprofile
		// All start with / and are runas flags.
		i++
	}
	return i
}

// --- PowerShell command handling ---

// skipPowerShellArgs skips PowerShell invocation flags.
// Returns the index of the first token that is part of the inner command.
// If -EncodedCommand is found, sets result.ExtractionFailed = true and returns
// past the end of tokens (fail-closed, pm-20260328-107).
// If -Command is found, returns the index of the next token (start of inner command).
func skipPowerShellArgs(tokens []string, i int, result *ClassifiedCommand) int {
	// PowerShell flags that take a value argument (case-insensitive).
	psValueFlags := []string{
		"-ExecutionPolicy", "-OutputFormat", "-WindowStyle",
	}

	// PowerShell flags that are standalone (no value).
	psStandaloneFlags := []string{
		"-NoProfile", "-NonInteractive", "-NoLogo", "-NoExit",
		"-Sta", "-Mta",
	}

	for i < len(tokens) {
		t := tokens[i]

		// -Command: remaining tokens are the inner command.
		if strings.EqualFold(t, "-Command") {
			i++ // skip the -Command flag itself
			return i
		}

		// -EncodedCommand: opaque Base64 — fail-closed.
		if strings.EqualFold(t, "-EncodedCommand") {
			result.ExtractionFailed = true
			return len(tokens) // stop processing
		}

		// -File: script content is in the file, not the command line — fail-closed.
		if strings.EqualFold(t, "-File") {
			result.ExtractionFailed = true
			return len(tokens) // stop processing
		}

		// Check standalone flags.
		matched := false
		for _, flag := range psStandaloneFlags {
			if strings.EqualFold(t, flag) {
				i++
				matched = true
				break
			}
		}
		if matched {
			continue
		}

		// Check value flags (consume flag + next token).
		for _, flag := range psValueFlags {
			if strings.EqualFold(t, flag) {
				i += 2
				matched = true
				break
			}
		}
		if matched {
			continue
		}

		// Unknown flag or start of positional arguments — stop.
		break
	}
	return i
}

// handlePowerShellCommand processes powershell/pwsh -Command inner command extraction.
// Returns true if the result was fully resolved.
func handlePowerShellCommand(tokens []string, i, depth int, unbalancedQuotes bool, result *ClassifiedCommand) bool {
	if i+1 >= len(tokens) {
		return false
	}

	j := i + 1 // skip the powershell/pwsh token
	innerStart := skipPowerShellArgs(tokens, j, result)

	// If ExtractionFailed was set by -EncodedCommand, record outer and return.
	if result.ExtractionFailed {
		result.Outer = strings.Join(tokens[i:], " ")
		return true
	}

	if innerStart >= len(tokens) {
		return false
	}

	innerCmd := strings.Join(tokens[innerStart:], " ")
	if innerCmd == "" {
		return false
	}

	// Resolve PowerShell aliases in the inner command.
	innerTokens := strings.Fields(innerCmd)
	if len(innerTokens) > 0 {
		innerTokens[0] = resolvePowerShellAlias(innerTokens[0])
		innerCmd = strings.Join(innerTokens, " ")
	}

	result.Outer = strings.Join(tokens[i:], " ")

	if depth < maxRecursionDepth && !unbalancedQuotes {
		innerResult := classificationNormalizeRecursive(innerCmd, depth+1)
		result.Inner = append(result.Inner, innerCmd)
		result.Inner = append(result.Inner, innerResult.Inner...)
		if innerResult.EscalateClassification {
			result.EscalateClassification = true
		}
	} else {
		result.Inner = append(result.Inner, innerCmd)
	}
	return true
}

// --- CMD command handling ---

// skipCmdArgs skips CMD flags.
// Returns the index of the first token that is part of the inner command.
// When /c or /k is found, returns the index of the next token.
func skipCmdArgs(tokens []string, i int) int {
	for i < len(tokens) {
		t := strings.ToLower(tokens[i])
		switch t {
		case "/c", "/k":
			i++ // skip the flag; remaining tokens are the inner command
			return i
		case "/s", "/q", "/d", "/a", "/u",
			"/e:on", "/e:off", "/v:on", "/v:off":
			i++
			continue
		}
		// Unknown flag or start of inner command — stop.
		break
	}
	return i
}

// handleCmdC processes cmd /c inner command extraction.
// Returns true if the result was fully resolved.
func handleCmdC(tokens []string, i, depth int, unbalancedQuotes bool, result *ClassifiedCommand) bool {
	if i+1 >= len(tokens) {
		return false
	}

	j := i + 1 // skip the cmd/cmd.exe token
	innerStart := skipCmdArgs(tokens, j)

	if innerStart >= len(tokens) {
		return false
	}

	innerCmd := strings.Join(tokens[innerStart:], " ")
	if innerCmd == "" {
		return false
	}

	result.Outer = strings.Join(tokens[i:], " ")

	if depth < maxRecursionDepth && !unbalancedQuotes {
		innerResult := classificationNormalizeRecursive(innerCmd, depth+1)
		result.Inner = append(result.Inner, innerCmd)
		result.Inner = append(result.Inner, innerResult.Inner...)
		if innerResult.EscalateClassification {
			result.EscalateClassification = true
		}
	} else {
		result.Inner = append(result.Inner, innerCmd)
	}
	return true
}

// --- WSL command handling ---

// handleWslCommand processes wsl wrapper command extraction.
// Patterns: wsl -e bash -c "...", wsl -- command, wsl command.
// Returns true if the result was fully resolved.
func handleWslCommand(tokens []string, i, depth int, unbalancedQuotes bool, result *ClassifiedCommand) bool {
	if i+1 >= len(tokens) {
		return false
	}

	j := i + 1 // skip the wsl/wsl.exe token

	// Skip WSL flags.
	for j < len(tokens) {
		t := tokens[j]
		if t == "--" {
			j++ // skip the --
			break
		}
		// -d <distro>, -u <user>: flags that take a value.
		if t == "-d" || t == "-u" {
			j += 2
			continue
		}
		// -e: execute command (remaining tokens are the command).
		if t == "-e" {
			j++
			break
		}
		// Other flags starting with -.
		if strings.HasPrefix(t, "-") {
			j++
			continue
		}
		// First non-flag token: start of the inner command.
		break
	}

	if j >= len(tokens) {
		return false
	}

	innerCmd := strings.Join(tokens[j:], " ")
	if innerCmd == "" {
		return false
	}

	result.Outer = strings.Join(tokens[i:], " ")

	if depth < maxRecursionDepth && !unbalancedQuotes {
		innerResult := classificationNormalizeRecursive(innerCmd, depth+1)
		result.Inner = append(result.Inner, innerCmd)
		result.Inner = append(result.Inner, innerResult.Inner...)
		if innerResult.EscalateClassification {
			result.EscalateClassification = true
		}
	} else {
		result.Inner = append(result.Inner, innerCmd)
	}
	return true
}
