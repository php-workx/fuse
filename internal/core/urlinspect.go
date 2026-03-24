package core

import (
	"net"
	"net/url"
	"regexp"
	"strings"
)

// BlockedHostnames are always-blocked destination names → BLOCKED.
// Matched after lowercasing and trailing-dot trimming.
var BlockedHostnames = map[string]bool{
	"169.254.169.254":          true, // AWS/GCP metadata (IPv4)
	"metadata.google.internal": true, // GCP metadata (hostname)
	"100.100.100.200":          true, // Alibaba metadata
	"169.254.170.2":            true, // ECS task metadata
	"192.0.0.192":              true, // OCI metadata
	"168.63.129.16":            true, // Azure WireServer / IMDS
	"localhost":                true, // loopback
	"0.0.0.0":                  true, // all interfaces
}

// BlockedIPRanges are always-blocked IP ranges → BLOCKED.
var BlockedIPRanges []net.IPNet

// CautionIPRanges are private/internal IP ranges → CAUTION.
var CautionIPRanges []net.IPNet

// BlockedSchemes are always-blocked URL schemes → BLOCKED.
var BlockedSchemes = map[string]bool{
	"file": true, "gopher": true, "dict": true,
	"ftp": true, "ftps": true, "scp": true,
	"sftp": true, "tftp": true, "ldap": true,
	"ldaps": true, "smb": true,
}

func init() {
	blockedCIDRs := []string{
		"127.0.0.0/8",            // loopback (IPv4)
		"169.254.0.0/16",         // link-local (IPv4)
		"::1/128",                // loopback (IPv6)
		"fe80::/10",              // link-local (IPv6)
		"::ffff:169.254.0.0/112", // IPv4-mapped link-local
		"fd00:ec2::254/128",      // AWS IMDS IPv6
		"fd20:ce::254/128",       // GCP metadata IPv6
	}
	for _, cidr := range blockedCIDRs {
		_, ipNet, err := net.ParseCIDR(cidr)
		if err != nil {
			panic("invalid blocked CIDR constant: " + cidr + ": " + err.Error())
		}
		BlockedIPRanges = append(BlockedIPRanges, *ipNet)
	}

	cautionCIDRs := []string{
		"10.0.0.0/8",     // RFC1918
		"172.16.0.0/12",  // RFC1918
		"192.168.0.0/16", // RFC1918
		"100.64.0.0/10",  // carrier-grade NAT (RFC6598)
		"198.18.0.0/15",  // benchmarking (RFC2544)
		"fc00::/7",       // IPv6 unique-local (private)
	}
	for _, cidr := range cautionCIDRs {
		_, ipNet, err := net.ParseCIDR(cidr)
		if err != nil {
			panic("invalid caution CIDR constant: " + cidr + ": " + err.Error())
		}
		CautionIPRanges = append(CautionIPRanges, *ipNet)
	}
}

// networkCommandBasenames are commands that make network requests.
var networkCommandBasenames = map[string]bool{
	"curl": true, "wget": true, "http": true, "httpie": true,
	"fetch": true, "aria2c": true,
}

// reURLPattern matches http/https/ftp/file URLs in command text.
var reURLPattern = regexp.MustCompile(`[a-zA-Z][a-zA-Z0-9+.-]*://[^\s'"` + "`" + `]+`)

// reNonCanonicalNumeric detects hex (0x), octal (0-prefixed), or decimal IPs.
var reNonCanonicalNumeric = regexp.MustCompile(`^(0x[0-9a-fA-F]+|0[0-7]+\d|[0-9]{10,})$`)

// reInsecureCertFlags detects flags that disable TLS verification.
var reInsecureCertFlag = regexp.MustCompile(`\b(-k|--insecure|--no-check-certificate|--no-verify-ssl)\b`)

// InspectCommandURLs extracts URLs from a command string and classifies them.
// Runs on any command text, not gated by basename (SEC-006).
// Returns the most restrictive (decision, reason) from all URLs found.
func InspectCommandURLs(cmd string) (Decision, string) {
	urls := reURLPattern.FindAllString(cmd, -1)
	if len(urls) == 0 {
		return "", ""
	}

	bestDecision := Decision("")
	bestReason := ""

	for _, rawURL := range urls {
		d, reason := inspectSingleURL(rawURL, cmd)
		if d != "" && (bestDecision == "" || DecisionSeverity(d) > DecisionSeverity(bestDecision)) {
			bestDecision = d
			bestReason = reason
		}
	}

	// Check insecure cert flags
	if reInsecureCertFlag.MatchString(cmd) {
		d := DecisionCaution
		if bestDecision == "" || DecisionSeverity(d) > DecisionSeverity(bestDecision) {
			bestDecision = d
			bestReason = "insecure TLS flag detected"
		}
	}

	// Check redirect flags (SEC-003) — always contributes CAUTION regardless of other decisions.
	if hasRedirectFlags(cmd) {
		d := DecisionCaution
		if bestDecision == "" || DecisionSeverity(d) > DecisionSeverity(bestDecision) {
			bestDecision = d
			bestReason = "HTTP redirect following enabled"
		}
	}

	return bestDecision, bestReason
}

// inspectSingleURL classifies a single URL through the inspection pipeline.
func inspectSingleURL(rawURL, cmd string) (Decision, string) {
	// Check for shell expansion tokens BEFORE url.Parse (SEC-001).
	// url.Parse fails on $() and backtick syntax, so this must come first.
	if isShellExpansion(rawURL) {
		return DecisionApproval, "URL contains shell variable expansion"
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		// Fail-closed: unparseable URLs get APPROVAL, not silent pass-through.
		return DecisionApproval, "unparseable URL (fail-closed)"
	}

	// Normalize host: lowercase, trim trailing dots.
	host := strings.ToLower(parsed.Hostname())
	host = strings.TrimRight(host, ".")

	// Check scheme against BlockedSchemes (SEC-011).
	scheme := strings.ToLower(parsed.Scheme)
	if BlockedSchemes[scheme] {
		return DecisionBlocked, "blocked URL scheme: " + scheme
	}

	// Non-canonical numeric host → CAUTION (SEC-002).
	if isNonCanonicalNumericHost(host) {
		return DecisionCaution, "non-canonical numeric IP in URL"
	}

	// Check against BlockedHostnames.
	if BlockedHostnames[host] {
		return DecisionBlocked, "blocked hostname: " + host
	}

	// Check IP ranges (direct host and bracket notation).
	if d, reason := classifyIPHost(host, parsed.Host); d != "" {
		return d, reason
	}

	// Non-allowlisted hostname in network commands → CAUTION (SEC-004).
	basename := extractCmdBasename(cmd)
	if networkCommandBasenames[basename] && host != "" && net.ParseIP(host) == nil {
		return DecisionCaution, "non-allowlisted hostname in network command: " + host
	}

	return "", ""
}

// classifyIPHost checks a host against blocked and caution IP ranges.
// Note: url.Hostname() already strips IPv6 brackets, so no bracket handling needed.
func classifyIPHost(host, _ string) (Decision, string) {
	ip := net.ParseIP(host)
	if ip == nil {
		return "", ""
	}
	return matchIPRanges(ip, host)
}

// matchIPRanges checks an IP against blocked and caution ranges.
func matchIPRanges(ip net.IP, label string) (Decision, string) {
	for _, cidr := range BlockedIPRanges {
		if cidr.Contains(ip) {
			return DecisionBlocked, "blocked IP range: " + label
		}
	}
	for _, cidr := range CautionIPRanges {
		if cidr.Contains(ip) {
			return DecisionCaution, "private/internal IP: " + label
		}
	}
	return "", ""
}

// InspectURLsInArgs scans MCP tool argument strings for suspicious URLs.
// Walks nested JSON to find string values containing URLs.
func InspectURLsInArgs(args map[string]interface{}) (Decision, string) {
	if args == nil {
		return "", ""
	}
	values := flattenStringValues(args)
	bestDecision := Decision("")
	bestReason := ""

	for _, v := range values {
		urls := reURLPattern.FindAllString(v, -1)
		for _, rawURL := range urls {
			d, reason := inspectSingleURL(rawURL, v)
			if d != "" && (bestDecision == "" || DecisionSeverity(d) > DecisionSeverity(bestDecision)) {
				bestDecision = d
				bestReason = reason
			}
		}
	}

	return bestDecision, bestReason
}

// isShellExpansion returns true if a string contains shell variable syntax.
func isShellExpansion(s string) bool {
	return strings.ContainsAny(s, "$`")
}

// isNonCanonicalNumericHost detects hex/octal/decimal IP encodings (SEC-002).
func isNonCanonicalNumericHost(host string) bool {
	return reNonCanonicalNumeric.MatchString(host)
}

// reRedirectFlag detects curl -L (standalone or in combined short flags like -kL, -Lv).
var reRedirectFlag = regexp.MustCompile(`(^|\s)-[a-zA-Z]*L`)

// hasRedirectFlags returns true if the command enables HTTP redirects (SEC-003).
func hasRedirectFlags(cmd string) bool {
	// curl -L / -kL / -Lv / --location
	if reRedirectFlag.MatchString(cmd) || strings.Contains(cmd, "--location") {
		return true
	}
	// wget follows redirects by default — check for wget presence
	basename := extractCmdBasename(cmd)
	if basename == "wget" {
		return true
	}
	// httpie --follow
	if strings.Contains(cmd, "--follow") {
		return true
	}
	return false
}

// extractCmdBasename extracts the basename of the first token in a command.
func extractCmdBasename(cmd string) string {
	fields := strings.Fields(cmd)
	if len(fields) == 0 {
		return ""
	}
	parts := strings.Split(fields[0], "/")
	return parts[len(parts)-1]
}
