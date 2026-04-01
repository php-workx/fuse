package inspect

import (
	"bytes"
	"regexp"
	"strings"
)

// powershellPattern pairs a compiled regex with its signal category and raw pattern string.
type powershellPattern struct {
	re       *regexp.Regexp
	category string
	raw      string
}

// powershellPatterns are compiled at package init time.
var powershellPatterns []powershellPattern

func init() {
	defs := []struct {
		pattern  string
		category string
	}{
		{`(?i)\b(Invoke-Expression|iex)\b`, "dynamic_exec"},
		{`(?i)\b(DownloadString|DownloadFile|Invoke-WebRequest|iwr|Invoke-RestMethod|irm|Start-BitsTransfer)\b`, "http_download"},
		{`(?i)\b(Start-Process|saps)\b`, "process_spawn"},
		{`(?i)\b(Invoke-WmiMethod|Invoke-Command|icm|New-PSSession|nsn|Enter-PSSession|etsn)\b`, "process_spawn"},
		{`(?i)\bwmic\b.*\bprocess\b.*\bcall\b.*\bcreate\b`, "process_spawn"},
		{
			`(?i)\b(` +
				`New-Service|schtasks\b.*\b/create\b|Register-ScheduledTask|New-ScheduledTask|` +
				`Register-WmiEvent|Set-WmiInstance|logman\b.*\b(stop|delete)\b|` +
				`sc\b.*\b(create|config)\b` +
				`)\b`,
			"persistence",
		},
		{`(?i)\breg\s+add\b.*\\Run(Once)?(?:\s|$|\\)`, "persistence"},
		{`(?i)\b(Add-MpPreference|Set-MpPreference)\b`, "defender_tamper"},
		{`(?i)\b(AmsiUtils|amsiInitFailed)\b|\[Ref\]\.Assembly\.GetType`, "amsi_bypass"},
		{`(?i)\breg\s+(add|delete|import|save)\b|\b(New-ItemProperty|Set-ItemProperty)\b`, "registry_modify"},
		{`(?i)\bNew-Object\b.*\b(Net\.WebClient|System\.Net\.Sockets)\b`, "network_object"},
		{`(?i)\b(certutil|bitsadmin|mshta|regsvr32|rundll32|wscript|cscript|cmdkey|ntdsutil|pcalua|hh\.exe|vaultcmd|wevtutil|auditpol|wmic|netsh)\b`, "lolbin"},
	}

	powershellPatterns = make([]powershellPattern, len(defs))
	for i, d := range defs {
		powershellPatterns[i] = powershellPattern{
			re:       regexp.MustCompile(d.pattern),
			category: d.category,
			raw:      d.pattern,
		}
	}
}

// ScanPowerShell scans PowerShell content for dangerous patterns.
// It performs a line-by-line regex scan, skipping single-line comments and
// tracking <# ... #> block comments.
func ScanPowerShell(content []byte) []Signal {
	var signals []Signal
	lines := bytes.Split(content, []byte("\n"))
	inBlockComment := false

	for i, line := range lines {
		lineStr := stripPowerShellBlockComments(string(line), &inBlockComment)
		trimmed := strings.TrimSpace(lineStr)

		if trimmed == "" {
			continue
		}

		if strings.HasPrefix(trimmed, "#") {
			continue
		}

		for _, p := range powershellPatterns {
			match := p.re.FindString(lineStr)
			if match != "" {
				signals = append(signals, Signal{
					Category: p.category,
					Pattern:  p.raw,
					Line:     i + 1,
					Match:    match,
				})
			}
		}
	}

	return signals
}

// stripPowerShellBlockComments removes block comment segments from a line while
// tracking whether the parser is currently inside a <# ... #> comment block.
func stripPowerShellBlockComments(line string, inBlockComment *bool) string {
	var b strings.Builder
	rest := line

	for rest != "" {
		if *inBlockComment {
			end := strings.Index(rest, "#>")
			if end < 0 {
				return b.String()
			}
			rest = rest[end+2:]
			*inBlockComment = false
			continue
		}

		start := strings.Index(rest, "<#")
		if start < 0 {
			b.WriteString(rest)
			break
		}

		b.WriteString(rest[:start])
		rest = rest[start+2:]
		*inBlockComment = true
	}

	return b.String()
}
