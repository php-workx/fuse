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
		{`(?i)\b(Start-Process|saps|start)\b`, "process_spawn"},
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
		{`(?i)\b(Clear-EventLog|wevtutil(?:\.exe)?\s+cl)\b`, "blocked_behavior"},
		{`(?i)\breg(?:\.exe)?\s+save\s+.*\\(SAM|SYSTEM|SECURITY)\b`, "blocked_behavior"},
		{`(?i)\bprocdump(?:\.exe)?\b.*\blsass(?:\.exe)?\b|\blsass(?:\.exe)?\b.*\bprocdump(?:\.exe)?\b`, "blocked_behavior"},
		{`(?i)\bRemove-Item\b.*-Recurse\b.*-Force\b|\bRemove-Item\b.*-Force\b.*-Recurse\b`, "destructive_block"},
		{`(?i)\bFormat-Volume\b`, "destructive_block"},
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
// tracking <# ... #> block comments. Like the Unix scanners, it does not track
// here-strings, splatted arguments, or commands split across multiple lines.
func ScanPowerShell(content []byte) []Signal {
	var signals []Signal
	lines := bytes.Split(content, []byte("\n"))
	blockCommentDepth := 0
	inSingleQuote := false
	inDoubleQuote := false

	for i, line := range lines {
		lineStartedInQuote := inSingleQuote || inDoubleQuote
		lineStr := stripPowerShellBlockComments(
			string(line),
			&blockCommentDepth,
			&inSingleQuote,
			&inDoubleQuote,
		)
		trimmed := strings.TrimSpace(lineStr)

		if trimmed == "" {
			continue
		}

		if !lineStartedInQuote && strings.HasPrefix(trimmed, "#") {
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
// tracking nested <# ... #> depth across lines.
func stripPowerShellBlockComments(
	line string,
	blockCommentDepth *int,
	inSingleQuote *bool,
	inDoubleQuote *bool,
) string {
	var b strings.Builder

	for i := 0; i < len(line); {
		if *blockCommentDepth > 0 {
			if i+1 < len(line) {
				if line[i] == '<' && line[i+1] == '#' {
					(*blockCommentDepth)++
					i += 2
					continue
				}
				if line[i] == '#' && line[i+1] == '>' {
					(*blockCommentDepth)--
					i += 2
					continue
				}
			}
			i++
			continue
		}

		if i+1 < len(line) && !*inSingleQuote && !*inDoubleQuote && line[i] == '<' && line[i+1] == '#' {
			(*blockCommentDepth)++
			i += 2
			continue
		}

		ch := line[i]
		switch ch {
		case '\'':
			if !*inDoubleQuote {
				if *inSingleQuote && i+1 < len(line) && line[i+1] == '\'' {
					b.WriteString("''")
					i += 2
					continue
				}
				*inSingleQuote = !*inSingleQuote
			}
		case '"':
			if !*inSingleQuote && !isEscapedPowerShellDoubleQuote(line, i) {
				*inDoubleQuote = !*inDoubleQuote
			}
		}
		b.WriteByte(ch)
		i++
	}

	return b.String()
}

func isEscapedPowerShellDoubleQuote(line string, idx int) bool {
	backticks := 0
	for i := idx - 1; i >= 0 && line[i] == '`'; i-- {
		backticks++
	}
	return backticks%2 == 1
}
