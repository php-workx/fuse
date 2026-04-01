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

func init() {
	defs := []struct {
		pattern  string
		category string
	}{
		// LOLBins and script host helpers.
		{`(?i)\b(certutil|bitsadmin|mshta|regsvr32|rundll32|wscript|cscript|forfiles)\b`, "lolbin"},
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

// ScanBatch scans Windows batch content for dangerous patterns.
// It performs a line-by-line regex scan, skipping REM comments (with a trailing
// space requirement) and :: comment lines.
func ScanBatch(content []byte) []Signal {
	var signals []Signal
	lines := bytes.Split(content, []byte("\n"))

	for i, line := range lines {
		lineStr := string(line)
		trimmed := strings.TrimSpace(lineStr)
		upper := strings.ToUpper(trimmed)

		// Skip comment lines.
		if strings.HasPrefix(upper, "REM ") || strings.HasPrefix(trimmed, "::") {
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
					Line:     i + 1, // 1-indexed
					Match:    match,
				})
			}
		}
	}

	return signals
}
