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

// ScanJavaScript scans JavaScript/TypeScript source content for dangerous patterns.
// It performs a line-by-line regex scan, skipping single-line comment lines
// (starting with //) and best-effort skipping of /* */ block comments.
func ScanJavaScript(content []byte) []Signal {
	var signals []Signal
	lines := bytes.Split(content, []byte("\n"))

	inBlockComment := false

	for i, line := range lines {
		lineStr := string(line)
		trimmed := strings.TrimSpace(lineStr)

		// Handle block comments (best-effort).
		if inBlockComment {
			if idx := strings.Index(trimmed, "*/"); idx >= 0 {
				inBlockComment = false
				// Keep the remainder after the block comment close for scanning.
				lineStr = trimmed[idx+2:]
				trimmed = strings.TrimSpace(lineStr)
			} else {
				continue
			}
		}

		// Check for block comment start.
		if strings.Contains(trimmed, "/*") {
			if strings.Contains(trimmed, "*/") {
				// Single-line block comment: remove the commented portion
				// and scan what remains.
				start := strings.Index(trimmed, "/*")
				end := strings.Index(trimmed, "*/")
				if end > start {
					lineStr = trimmed[:start] + trimmed[end+2:]
					trimmed = strings.TrimSpace(lineStr)
				}
			} else {
				// Multi-line block comment starts here.
				// Scan only the part before the comment start.
				start := strings.Index(trimmed, "/*")
				lineStr = trimmed[:start]
				trimmed = strings.TrimSpace(lineStr)
				inBlockComment = true
			}
		}

		// Skip single-line comment lines.
		if strings.HasPrefix(trimmed, "//") {
			continue
		}

		// Skip empty lines.
		if trimmed == "" {
			continue
		}

		for _, p := range jsPatterns {
			match := p.re.FindString(lineStr)
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
