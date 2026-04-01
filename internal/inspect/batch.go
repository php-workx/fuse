package inspect

import (
	"bytes"
	"regexp"
	"strings"
)

// batchPattern pairs a compiled regex with its signal category and raw pattern string.
type batchPattern struct {
	re       *regexp.Regexp
	category string
	raw      string
}

// batchPatterns are compiled at package init time.
var batchPatterns []batchPattern

var batchREMCommentPattern = regexp.MustCompile(`(?i)^@?rem(?:\s|$)`)

func init() {
	defs := []struct {
		pattern  string
		category string
	}{
		// LOLBins and script host helpers.
		{`(?i)\b(bitsadmin|mshta|regsvr32|rundll32|wscript|cscript|forfiles)\b`, "lolbin"},
		{`(?i)\bcertutil\b.*\s-(decode|urlcache)\b`, "lolbin"},
		{`(?i)\bbitsadmin\b.*\s/transfer\b`, "lolbin"},
		{`(?i)\bforfiles\b.*\s/c\b`, "lolbin"},

		// Registry modification.
		{`(?i)\breg\s+(add|delete|import)\b`, "registry_modify"},
		{`(?i)\breg\s+add\b.*\\Run(Once)?(\s|$|\\)`, "persistence"},

		// Persistence and service management.
		{`(?i)\bschtasks\b.*\s/create\b`, "persistence"},
		{`(?i)\bsc\s+(create|config)\b`, "persistence"},
		{`(?i)\bnet\s+user\b.*\s/(add|delete)\b`, "user_modify"},
		{`(?i)\bnet\s+localgroup\b.*\badministrators\b.*\s/(add|delete)\b`, "user_modify"},
		{`(?i)\bnetsh\b.*\badvfirewall\b.*\b(add|delete)\b.*\brule\b`, "firewall_modify"},

		// Destructive filesystem operations.
		{`(?i)\bdel\b.*\s/[sq]+(\s|$)`, "destructive_fs"},
		{`(?i)\b(rd|rmdir)\b.*\s/[sq]+(\s|$)`, "destructive_fs"},
	}

	batchPatterns = make([]batchPattern, len(defs))
	for i, d := range defs {
		batchPatterns[i] = batchPattern{
			re:       regexp.MustCompile(d.pattern),
			category: d.category,
			raw:      d.pattern,
		}
	}
}

type logicalBatchLine struct {
	line int
	text string
}

// reconstructBatchLogicalLines joins physical lines continued with a trailing
// caret (^) into logical commands before regex matching.
func reconstructBatchLogicalLines(content []byte) []logicalBatchLine {
	physical := bytes.Split(content, []byte("\n"))
	logical := make([]logicalBatchLine, 0, len(physical))

	var current strings.Builder
	startLine := 0

	flush := func() {
		if current.Len() == 0 {
			return
		}
		logical = append(logical, logicalBatchLine{
			line: startLine,
			text: current.String(),
		})
		current.Reset()
		startLine = 0
	}

	appendSegment := func(segment string) {
		segment = strings.TrimSpace(segment)
		if segment == "" {
			return
		}
		if current.Len() > 0 {
			current.WriteByte(' ')
		}
		current.WriteString(segment)
	}

	for i, raw := range physical {
		lineNo := i + 1
		line := strings.TrimRight(string(raw), "\r")
		trimmedRight := strings.TrimRight(line, " \t")
		continues := strings.HasSuffix(trimmedRight, "^")
		if continues {
			trimmedRight = strings.TrimRight(trimmedRight[:len(trimmedRight)-1], " \t")
		}

		if current.Len() == 0 {
			startLine = lineNo
		}
		appendSegment(trimmedRight)

		if continues {
			continue
		}
		flush()
	}

	flush()
	return logical
}

// ScanBatch scans Windows batch content for dangerous patterns.
// It reconstructs caret-continued logical lines first, then scans line-by-line,
// skipping REM/:: comment lines.
func ScanBatch(content []byte) []Signal {
	var signals []Signal
	lines := reconstructBatchLogicalLines(content)

	for _, line := range lines {
		lineStr := line.text
		trimmed := strings.TrimSpace(lineStr)

		// Skip comment lines.
		if batchREMCommentPattern.MatchString(trimmed) || strings.HasPrefix(trimmed, "::") {
			continue
		}

		// Skip empty lines.
		if trimmed == "" {
			continue
		}

		for _, p := range batchPatterns {
			match := p.re.FindString(lineStr)
			if match != "" {
				signals = append(signals, Signal{
					Category: p.category,
					Pattern:  p.raw,
					Line:     line.line, // logical line start (1-indexed physical line)
					Match:    match,
				})
			}
		}
	}

	return signals
}
