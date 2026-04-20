package inspect

import (
	"bytes"
	"regexp"
	"strings"
)

// pythonPattern pairs a compiled regex with its signal category and raw pattern string.
type pythonPattern struct {
	re       *regexp.Regexp
	category string
	raw      string
}

// pythonPatterns are compiled at package init time.
var pythonPatterns []pythonPattern

func init() {
	defs := []struct {
		pattern  string
		category string
	}{
		// Import detection
		{`^\s*(import|from)\s+(boto3|botocore)\b`, "cloud_sdk"},
		{`^\s*(import|from)\s+(google\.cloud|googleapiclient)\b`, "cloud_sdk"},
		{`^\s*(import|from)\s+(azure\.|msrestazure)\b`, "cloud_sdk"},
		{`^\s*(import|from)\s+oci\b`, "cloud_sdk"},
		{`^\s*(import|from)\s+subprocess\b`, "subprocess"},
		{`^\s*(import|from)\s+shutil\b`, "destructive_fs"},
		{`^\s*(import|from)\s+os\b`, "subprocess"},
		{`^\s*(import|from)\s+(urllib|http\.client|socket|requests|httpx)\b`, "network_io"},

		// Dangerous call detection
		{`\bsubprocess\.(run|call|Popen|check_call|check_output)\b`, "subprocess"},
		{`\bos\.system\b`, "subprocess"},
		{`\bos\.remove\b`, "destructive_fs"},
		{`\bos\.unlink\b`, "destructive_fs"},
		{`\bos\.rmdir\b`, "destructive_fs"},
		{`\bos\.makedirs\b`, "destructive_fs"},
		{`\bos\.rename\b`, "destructive_fs"},
		{`\bshutil\.rmtree\b`, "destructive_fs"},
		{`\bshutil\.move\b`, "destructive_fs"},
		{`\bshutil\.copy(?:tree|file|2)?\b`, "destructive_fs"},

		// Write-mode opens and pathlib.Path write/delete operations
		{`\bopen\s*\([^)]*,\s*['"][wax]`, "destructive_fs"},
		{`\b(?:Path|pathlib\.Path)\s*\([^)]*\)\s*\.\s*(?:unlink|rmdir|rename|replace|write_text|write_bytes|mkdir|touch|chmod)\b`, "destructive_fs"},
		{`\b(?:Path|pathlib\.Path)\s*\([^)]*\)\s*\.\s*open\s*\(\s*['"]\s*[wax]`, "destructive_fs"},

		// Cloud control-plane destructive verbs
		{`\b(delete_stack|terminate_instances|delete_bucket|delete_object)\b`, "cloud_sdk"},
		{`\b(delete_db_instance|delete_table|delete_function)\b`, "cloud_sdk"},
		{`\b(delete_cluster|delete_service|delete_secret)\b`, "cloud_sdk"},
		{`\brequests\.(delete|put|post)\b.*\b(iam|cloudformation|ec2|s3|rds)\b`, "http_control_plane"},

		// General network I/O — flagged as CAUTION (non-approval) network activity.
		{`\burllib\.request\.(?:urlopen|Request)\s*\(`, "network_io"},
		{`\burlopen\s*\(`, "network_io"},
		{`\brequests\.(?:get|post|put|patch|delete|head|options|request)\s*\(`, "network_io"},
		{`\bhttpx\.(?:get|post|put|patch|delete|head|options|request|stream|AsyncClient|Client)\s*\(`, "network_io"},
		{`\bsocket\s*\.\s*socket\s*\(`, "network_io"},
		{`\bhttp\.client\.(?:HTTPConnection|HTTPSConnection)\s*\(`, "network_io"},

		// Secret-like file reads
		{`\b(?:open|Path|pathlib\.Path)\s*\(\s*['"][^'"]*(?:\.env|id_rsa|id_ed25519|private[_-]?key|secret|token|credential)[^'"]*['"]`, "secret_read"},

		// Dynamic code execution detection
		{`\bexec\s*\(`, "dynamic_exec"},
		{`\beval\s*\(`, "dynamic_exec"},
		{`\b__import__\s*\(`, "dynamic_import"},
		{`\bimportlib\.import_module\s*\(`, "dynamic_import"},
		{`\bcompile\s*\(.*\bexec\b`, "dynamic_exec"},
	}

	pythonPatterns = make([]pythonPattern, len(defs))
	for i, d := range defs {
		pythonPatterns[i] = pythonPattern{
			re:       regexp.MustCompile(d.pattern),
			category: d.category,
			raw:      d.pattern,
		}
	}
}

// ScanPython scans Python source content for dangerous patterns.
// It performs a line-by-line regex scan, skipping comment lines (starting with #).
func ScanPython(content []byte) []Signal {
	var signals []Signal
	lines := bytes.Split(content, []byte("\n"))

	for i, line := range lines {
		lineStr := string(line)
		trimmed := strings.TrimSpace(lineStr)

		// Skip comment lines.
		if strings.HasPrefix(trimmed, "#") {
			continue
		}

		// Skip empty lines.
		if trimmed == "" {
			continue
		}

		for _, p := range pythonPatterns {
			match := p.re.FindString(lineStr)
			if match != "" {
				signals = append(signals, Signal{
					Category: p.category,
					Pattern:  p.raw,
					Line:     i + 1, // 1-indexed
					Match:    match,
					FullLine: lineStr,
				})
			}
		}
	}

	return scopeImportSignals(signals)
}

// importShutilPattern and importOsPattern identify import-level signals that
// should be removed when no corresponding dangerous call is present.
//
// importOsDangerousFuncPattern and importShutilDangerousFuncPattern detect
// "from os import system" / "from shutil import rmtree" style imports that
// alias in a specific dangerous function.  These are treated as equivalent to
// finding the corresponding call: the programmer explicitly imported the
// dangerous symbol, so filtering the signal would hide genuine risk.
var (
	importShutilPattern              = regexp.MustCompile(`^\s*(import|from)\s+shutil\b`)
	importOsPattern                  = regexp.MustCompile(`^\s*(import|from)\s+os\b`)
	importOsDangerousFuncPattern     = regexp.MustCompile(`^\s*from\s+os\s+import\s+.*\b(system|remove|unlink|rmdir|makedirs|rename)\b`)
	importShutilDangerousFuncPattern = regexp.MustCompile(`^\s*from\s+shutil\s+import\s+.*\b(rmtree|move|copytree|copyfile|copy2?)\b`)
)

// scopeImportSignals removes import-only signals when no corresponding
// dangerous call was found in the same file. Specifically:
//   - destructive_fs from "import shutil" is removed unless a shutil.rmtree,
//     shutil.move call signal exists OR the import is "from shutil import
//     <dangerous-fn>" (alias import of a dangerous symbol).
//   - subprocess from "import os" is removed unless an os.system, os.remove,
//     os.unlink, or os.rmdir call signal exists OR the import is "from os
//     import <dangerous-fn>".
func scopeImportSignals(signals []Signal) []Signal {
	hasShutilCall := false
	hasOsCall := false

	for _, s := range signals {
		m := s.Match
		// fullLine is the complete source line; prefer it over the (potentially
		// shorter) regex match when looking for alias import patterns that
		// require the full "from X import <fn>" text.
		fullLine := s.FullLine
		if fullLine == "" {
			fullLine = m
		}
		if strings.Contains(m, "shutil.rmtree") || strings.Contains(m, "shutil.move") ||
			importShutilDangerousFuncPattern.MatchString(fullLine) {
			hasShutilCall = true
		}
		if strings.Contains(m, "os.system") || strings.Contains(m, "os.remove") ||
			strings.Contains(m, "os.unlink") || strings.Contains(m, "os.rmdir") ||
			importOsDangerousFuncPattern.MatchString(fullLine) {
			hasOsCall = true
		}
	}

	filtered := make([]Signal, 0, len(signals))
	for _, s := range signals {
		// Drop destructive_fs from bare "import shutil" when no shutil call found.
		if s.Category == "destructive_fs" && importShutilPattern.MatchString(s.Match) && !hasShutilCall {
			continue
		}
		// Drop subprocess from bare "import os" when no os dangerous call found.
		if s.Category == "subprocess" && importOsPattern.MatchString(s.Match) && !hasOsCall {
			continue
		}
		filtered = append(filtered, s)
	}
	return filtered
}
