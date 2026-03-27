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
	var out strings.Builder
	out.Grow(len(line))

	inSingle := false
	inDouble := false
	inTemplate := false
	templateExprDepth := 0
	escaped := false

	for i := 0; i < len(line); i++ {
		ch := line[i]

		if inBlock {
			if ch == '*' && i+1 < len(line) && line[i+1] == '/' {
				inBlock = false
				i++
			}
			continue
		}

		if inSingle || inDouble {
			out.WriteByte(ch)
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
				continue
			}
			if inSingle && ch == '\'' {
				inSingle = false
			}
			if inDouble && ch == '"' {
				inDouble = false
			}
			continue
		}

		if inTemplate {
			out.WriteByte(ch)
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
				continue
			}
			if templateExprDepth == 0 {
				if ch == '`' {
					inTemplate = false
					continue
				}
				if ch == '$' && i+1 < len(line) && line[i+1] == '{' {
					templateExprDepth = 1
					out.WriteByte(line[i+1])
					i++
				}
				continue
			}
			switch ch {
			case '{':
				templateExprDepth++
			case '}':
				templateExprDepth--
			case '\'':
				inSingle = true
			case '"':
				inDouble = true
			}
			continue
		}

		switch ch {
		case '\'':
			inSingle = true
			out.WriteByte(ch)
		case '"':
			inDouble = true
			out.WriteByte(ch)
		case '`':
			inTemplate = true
			out.WriteByte(ch)
		case '/':
			if i+1 < len(line) && line[i+1] == '*' {
				inBlock = true
				i++
				continue
			}
			out.WriteByte(ch)
		default:
			out.WriteByte(ch)
		}
	}

	return out.String(), inBlock
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

		// Skip empty lines and pure single-line comments.
		// Note: check trimmed content BEFORE inBlockComment — if stripBlockComment
		// returned code before a block comment start, we must scan it even though
		// inBlockComment is now true for the NEXT line.
		if trimmed == "" || strings.HasPrefix(trimmed, "//") {
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
