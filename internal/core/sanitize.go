package core

import "regexp"

// KnownSafeVerbs lists command verbs whose quoted arguments are data, not commands.
// When a command starts with one of these verbs, double-quoted strings are also masked
// to prevent false positives during classification.
var KnownSafeVerbs = map[string]bool{
	"echo":   true,
	"printf": true,
	"grep":   true,
	"awk":    true,
	"sed":    true,
	"cat":    true,
	"log":    true,
}

var (
	singleQuoteRe = regexp.MustCompile(`'[^']*'`)
	doubleQuoteRe = regexp.MustCompile(`"[^"]*"`)
	trailingComment = regexp.MustCompile(`\s+#\s.*$`)
)

// SanitizeForClassification masks quoted content and strips trailing comments
// to prevent false positives during rule matching.
//
// Single-quoted strings are always replaced with __SQ__. If knownSafeVerb is
// true, double-quoted strings are also replaced with __DQ__. Trailing comments
// (# ...) are stripped last, after quote masking has already hidden any # chars
// that appeared inside quotes.
func SanitizeForClassification(cmd string, knownSafeVerb bool) string {
	// Step 1: Mask single-quoted strings.
	result := singleQuoteRe.ReplaceAllString(cmd, "__SQ__")

	// Step 2: If known safe verb, also mask double-quoted strings.
	if knownSafeVerb {
		result = doubleQuoteRe.ReplaceAllString(result, "__DQ__")
	}

	// Step 3: Strip trailing comments (# ...) — safe because quotes are already masked.
	result = trailingComment.ReplaceAllString(result, "")

	return result
}
