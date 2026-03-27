package inspect

import (
	"bytes"
	"regexp"
	"strings"
)

type jsScanState struct {
	inBlock           bool
	inSingle          bool
	inDouble          bool
	inTemplate        bool
	templateExprDepth int
	escaped           bool
}

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
	state := jsScanState{inBlock: inBlock}

	for i := 0; i < len(line); i++ {
		ch := line[i]

		if state.inBlock {
			if ch == '*' && i+1 < len(line) && line[i+1] == '/' {
				state.inBlock = false
				i++
			}
			continue
		}

		if scanQuotedLiteral(&state, &out, ch) {
			continue
		}

		if scanTemplateLiteral(&state, &out, ch, line, &i) {
			continue
		}

		if beginBlockComment(&state, ch, line, &i) {
			continue
		}
		beginLiteral(&state, ch)
		out.WriteByte(ch)
	}

	return out.String(), state.inBlock
}

func scanQuotedLiteral(state *jsScanState, out *strings.Builder, ch byte) bool {
	if !state.inSingle && !state.inDouble {
		return false
	}
	out.WriteByte(ch)
	if state.escaped {
		state.escaped = false
		return true
	}
	if ch == '\\' {
		state.escaped = true
		return true
	}
	if state.inSingle && ch == '\'' {
		state.inSingle = false
	}
	if state.inDouble && ch == '"' {
		state.inDouble = false
	}
	return true
}

func scanTemplateLiteral(state *jsScanState, out *strings.Builder, ch byte, line string, i *int) bool {
	if !state.inTemplate {
		return false
	}
	out.WriteByte(ch)
	if state.escaped {
		state.escaped = false
		return true
	}
	if ch == '\\' {
		state.escaped = true
		return true
	}
	if state.templateExprDepth == 0 {
		if ch == '`' {
			state.inTemplate = false
			return true
		}
		if ch == '$' && *i+1 < len(line) && line[*i+1] == '{' {
			state.templateExprDepth = 1
			out.WriteByte(line[*i+1])
			*i++
		}
		return true
	}
	switch ch {
	case '{':
		state.templateExprDepth++
	case '}':
		state.templateExprDepth--
	case '\'':
		state.inSingle = true
	case '"':
		state.inDouble = true
	}
	return true
}

func beginBlockComment(state *jsScanState, ch byte, line string, i *int) bool {
	if ch == '/' && *i+1 < len(line) && line[*i+1] == '*' {
		state.inBlock = true
		*i++
		return true
	}
	return false
}

func beginLiteral(state *jsScanState, ch byte) {
	switch ch {
	case '\'':
		state.inSingle = true
	case '"':
		state.inDouble = true
	case '`':
		state.inTemplate = true
	}
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
