package core

import (
	"net"
	"net/url"
	"regexp"
	"strings"
	"sync"
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
	"httpx": true, "xh": true, "curlie": true, "grpcurl": true,
	"nc": true, "ncat": true, "netcat": true, "socat": true,
	"lynx": true, "links": true, "w3m": true,
}

// trustedDomains is the set of user-configured trusted domains that are exempt
// from SEC-004 non-allowlisted hostname CAUTION. Set via SetTrustedDomains.
// Protected by trustedDomainsMu for concurrent codex-shell access.
var (
	trustedDomains   map[string]bool
	trustedDomainsMu sync.RWMutex
)

// SetTrustedDomains configures the set of trusted domains for URL inspection.
// Domains are matched after lowercasing and trailing-dot trimming.
// Safe for concurrent use.
func SetTrustedDomains(domains []string) {
	trustedDomainsMu.Lock()
	defer trustedDomainsMu.Unlock()
	if len(domains) == 0 {
		trustedDomains = nil
		return
	}
	m := make(map[string]bool, len(domains))
	for _, d := range domains {
		m[strings.ToLower(strings.TrimRight(d, "."))] = true
	}
	trustedDomains = m
}

func isTrustedDomain(host string) bool {
	trustedDomainsMu.RLock()
	defer trustedDomainsMu.RUnlock()
	return trustedDomains[host]
}

// reURLPattern matches http/https/ftp/file URLs in command text.
var reURLPattern = regexp.MustCompile(`[a-zA-Z][a-zA-Z0-9+.-]*://[^\s'"` + "`" + `]+`)

// reNonCanonicalNumeric detects hex (0x), octal (0-prefixed), or decimal IPs.
var reNonCanonicalNumeric = regexp.MustCompile(`^(0x[0-9a-fA-F]+|0[0-7]+\d|[0-9]{10,})$`)

// reInsecureCertFlag detects flags that disable TLS verification.
// Uses regex to catch combined short flags (e.g., -kL, -kvs).
var reInsecureCertFlag = regexp.MustCompile(`(^|\s)-[a-zA-Z]*k`)

// insecureCertLongFlags are long-form flags that disable TLS verification.
var insecureCertLongFlags = []string{"--insecure", "--no-check-certificate", "--no-verify-ssl"}

// --- L7 progressive enforcement ---

// reDestructiveHTTPMethod detects HTTP methods that modify/delete resources.
// Matches: -X DELETE, -X PUT, -X PATCH, --request DELETE, etc.
var reDestructiveHTTPMethod = regexp.MustCompile(`(?i)(^|\s)(-X|--request)\s+(DELETE|PUT|PATCH)\b`)

// reDataUploadFlags detects flags that send data payloads (exfiltration risk).
var reDataUploadFlags = []string{
	" -d ", " -d@", "--data ", "--data=", "--data-raw ", "--data-binary ",
	"--upload-file ", "-T ", "--json ",
}

// reFileUploadFlag detects -d @file or --data @file patterns (file exfiltration).
var reFileUploadFlag = regexp.MustCompile(`(^|\s)(-d\s*@|--data[= ]\s*@|--upload-file\s+|-T\s+)\S`)

// InspectCommandURLs extracts URLs from a command string and classifies them.
// Runs on any command text, not gated by basename (SEC-006).
// Returns the most restrictive (decision, reason) from all URLs found.
func InspectCommandURLs(cmd string) (Decision, string) {
	bestDecision := Decision("")
	bestReason := ""

	// Check insecure/redirect flags BEFORE URL extraction so they fire even when
	// URLs are variable-substituted (e.g., curl -k "$URL"). Only for network commands.
	basename := extractCmdBasename(cmd)
	isNetCmd := networkCommandBasenames[basename]

	if isNetCmd && hasInsecureCertFlag(cmd) {
		d := DecisionCaution
		if bestDecision == "" || DecisionSeverity(d) > DecisionSeverity(bestDecision) {
			bestDecision = d
			bestReason = "insecure TLS flag detected"
		}
	}

	// L7: Destructive HTTP methods → APPROVAL (DELETE, PUT, PATCH on remote resources).
	if isNetCmd && reDestructiveHTTPMethod.MatchString(cmd) {
		d := DecisionApproval
		if bestDecision == "" || DecisionSeverity(d) > DecisionSeverity(bestDecision) {
			bestDecision = d
			bestReason = "destructive HTTP method detected"
		}
	}

	// L7: Data/file upload flags → CAUTION (exfiltration risk).
	if isNetCmd && hasDataUploadFlag(cmd) {
		d := DecisionCaution
		if bestDecision == "" || DecisionSeverity(d) > DecisionSeverity(bestDecision) {
			bestDecision = d
			bestReason = "data upload/exfiltration flag detected"
		}
	}

	// L7: File upload specifically (@file) → APPROVAL (direct file exfiltration).
	if isNetCmd && reFileUploadFlag.MatchString(cmd) {
		d := DecisionApproval
		if bestDecision == "" || DecisionSeverity(d) > DecisionSeverity(bestDecision) {
			bestDecision = d
			bestReason = "file upload flag detected (exfiltration risk)"
		}
	}

	// Check redirect flags (SEC-003).
	if isNetCmd && hasRedirectFlags(cmd) {
		d := DecisionCaution
		if bestDecision == "" || DecisionSeverity(d) > DecisionSeverity(bestDecision) {
			bestDecision = d
			bestReason = "HTTP redirect following enabled"
		}
	}

	// Extract and inspect literal URLs.
	urls := reURLPattern.FindAllString(cmd, -1)
	for _, rawURL := range urls {
		d, reason := inspectSingleURL(rawURL, cmd, false)
		if d != "" && (bestDecision == "" || DecisionSeverity(d) > DecisionSeverity(bestDecision)) {
			bestDecision = d
			bestReason = reason
		}
	}

	return bestDecision, bestReason
}

// inspectSingleURL classifies a single URL through the inspection pipeline.
// When networkContext is true, SEC-004 (non-allowlisted hostname) applies
// regardless of the command basename (used for MCP arg scanning).
func inspectSingleURL(rawURL, cmd string, networkContext bool) (Decision, string) {
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
	// Exempt user-configured trusted domains.
	basename := extractCmdBasename(cmd)
	if (networkContext || networkCommandBasenames[basename]) && host != "" && net.ParseIP(host) == nil {
		if !isTrustedDomain(host) {
			return DecisionCaution, "non-allowlisted hostname in network command: " + host
		}
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
			d, reason := inspectSingleURL(rawURL, v, true) // MCP args are always network context
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

// hasInsecureCertFlag checks if the command contains TLS verification bypass flags.
// Detects -k as standalone or in combined short flags (-kL, -kvs).
func hasInsecureCertFlag(cmd string) bool {
	if reInsecureCertFlag.MatchString(cmd) {
		return true
	}
	for _, flag := range insecureCertLongFlags {
		if strings.Contains(cmd, flag) {
			return true
		}
	}
	return false
}

// hasDataUploadFlag checks if the command contains data upload flags.
func hasDataUploadFlag(cmd string) bool {
	padded := " " + cmd + " "
	for _, flag := range reDataUploadFlags {
		if strings.Contains(padded, flag) {
			return true
		}
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
