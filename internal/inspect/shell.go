package inspect

import (
	"bytes"
	"regexp"
	"strings"
)

// shellPattern pairs a compiled regex with its signal category and raw pattern string.
type shellPattern struct {
	re       *regexp.Regexp
	category string
	raw      string
}

// shellPatterns are compiled at package init time.
var shellPatterns []shellPattern

func init() {
	defs := []struct {
		pattern  string
		category string
	}{
		{`\brm\s+(-[a-zA-Z]*r[a-zA-Z]*f|-[a-zA-Z]*f[a-zA-Z]*r)\b`, "destructive_fs"},
		{`\bmkfs\b`, "destructive_fs"},
		{`\bdd\b.*\bof=`, "destructive_fs"},
		{`\b(aws|gcloud|az|oci)\b`, "cloud_cli"},
		{`\bcurl\b.*\b(delete|DELETE)\b`, "http_control_plane"},
		{`\bkubectl\s+delete\b`, "destructive_verb"},
		{`\bterraform\s+(destroy|apply)\b`, "destructive_verb"},
		{`\beval\b`, "subprocess"},
		{`\$\(.*\)`, "subprocess"},
	}

	shellPatterns = make([]shellPattern, len(defs))
	for i, d := range defs {
		shellPatterns[i] = shellPattern{
			re:       regexp.MustCompile(d.pattern),
			category: d.category,
			raw:      d.pattern,
		}
	}
}

// ScanShell scans shell script content for dangerous patterns.
// It performs a line-by-line regex scan, skipping comment lines (starting with #).
func ScanShell(content []byte) []Signal {
	var signals []Signal
	lines := bytes.Split(content, []byte("\n"))

	for i, line := range lines {
		lineStr := string(line)
		trimmed := strings.TrimSpace(lineStr)

		// Skip comment lines (but not shebangs on line 1, though shebangs
		// typically won't match any dangerous patterns anyway).
		if strings.HasPrefix(trimmed, "#") {
			continue
		}

		// Skip empty lines.
		if trimmed == "" {
			continue
		}

		for _, p := range shellPatterns {
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
