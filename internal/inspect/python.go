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

		// Dangerous call detection
		{`\bsubprocess\.(run|call|Popen|check_call|check_output)\b`, "subprocess"},
		{`\bos\.system\b`, "subprocess"},
		{`\bos\.remove\b`, "destructive_fs"},
		{`\bos\.unlink\b`, "destructive_fs"},
		{`\bos\.rmdir\b`, "destructive_fs"},
		{`\bshutil\.rmtree\b`, "destructive_fs"},
		{`\bshutil\.move\b`, "destructive_fs"},
		{`\b(delete_stack|terminate_instances|delete_bucket|delete_object)\b`, "cloud_sdk"},
		{`\b(delete_db_instance|delete_table|delete_function)\b`, "cloud_sdk"},
		{`\b(delete_cluster|delete_service|delete_secret)\b`, "cloud_sdk"},
		{`\brequests\.(delete|put|post)\b.*\b(iam|cloudformation|ec2|s3|rds)\b`, "http_control_plane"},

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
				})
			}
		}
	}

	return signals
}
