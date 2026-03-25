package inspect

import (
	"bytes"
	"regexp"
	"strings"
)

// jsPattern pairs a compiled regex with its signal category and raw pattern string.
type jsPattern struct {
	re       *regexp.Regexp
	category string
	raw      string
}

// jsPatterns are compiled at package init time.
var jsPatterns []jsPattern

func init() {
	defs := []struct {
		pattern  string
		category string
	}{
		// child_process imports
		{`require\s*\(\s*['"]child_process['"]\s*\)`, "subprocess"},
		{`from\s+['"]child_process['"]`, "subprocess"},

		// Dangerous subprocess calls
		{`\b(exec|execSync|spawn|spawnSync|fork)\s*\(`, "subprocess"},

		// Destructive filesystem operations
		{`\bfs\.(rmSync|unlinkSync|rmdirSync|rm)\b`, "destructive_fs"},
		{`\bfs\.promises\.(rm|unlink|rmdir)\b`, "destructive_fs"},

		// Cloud SDK imports
		{`require\s*\(\s*['"]@aws-sdk/`, "cloud_sdk"},
		{`from\s+['"]@aws-sdk/`, "cloud_sdk"},
		{`require\s*\(\s*['"]@google-cloud/`, "cloud_sdk"},
		{`from\s+['"]@google-cloud/`, "cloud_sdk"},
		{`require\s*\(\s*['"]@azure/`, "cloud_sdk"},

		// Cloud SDK destructive commands
		{`\b(DeleteCommand|TerminateCommand|DestroyCommand)\b`, "cloud_sdk"},
	}

	jsPatterns = make([]jsPattern, len(defs))
	for i, d := range defs {
		jsPatterns[i] = jsPattern{
			re:       regexp.MustCompile(d.pattern),
			category: d.category,
			raw:      d.pattern,
		}
	}
}

// stripBlockComment handles block comment state for a single line.
// If inBlock is true, we are inside a /* ... */ block comment from a previous line.
// It returns the effective line content after stripping commented portions,
// and whether we are still inside a block comment.
func stripBlockComment(line string, inBlock bool) (string, bool) {
	if inBlock {
		idx := strings.Index(line, "*/")
		if idx < 0 {
			return "", true
		}
		// Found closing — process the remainder for further comment markers.
		return stripBlockComment(line[idx+2:], false)
	}

	start := strings.Index(line, "/*")
	if start < 0 {
		return line, false
	}

	// Check for closing on the same line (inline block comment).
	rest := line[start+2:]
	end := strings.Index(rest, "*/")
	if end >= 0 {
		// Strip the inline comment and process the remainder for more comments.
		stripped := line[:start] + rest[end+2:]
		return stripBlockComment(stripped, false)
	}

	// Multi-line block comment starts here; keep only content before it.
	return line[:start], true
}

// ScanJavaScript scans JavaScript/TypeScript source content for dangerous patterns.
// It performs a line-by-line regex scan, skipping single-line comment lines
// (starting with //) and best-effort skipping of /* */ block comments.
func ScanJavaScript(content []byte) []Signal {
	var signals []Signal
	inBlockComment := false

	for i, rawLine := range bytes.Split(content, []byte("\n")) {
		trimmed := strings.TrimSpace(string(rawLine))

		trimmed, inBlockComment = stripBlockComment(trimmed, inBlockComment)
		trimmed = strings.TrimSpace(trimmed)

		if inBlockComment || trimmed == "" || strings.HasPrefix(trimmed, "//") {
			continue
		}

		for _, p := range jsPatterns {
			match := p.re.FindString(trimmed)
			if match != "" {
				signals = append(signals, Signal{
					Category: p.category,
					Pattern:  p.raw,
					Line:     i + 1, // 1-indexed
					Match:    match,
				})
			}
		}
	}

	return signals
}
