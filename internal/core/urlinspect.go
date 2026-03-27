package core

import (
	"net"
	"net/url"
	"regexp"
	"strconv"
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
	"0":                        true, // all interfaces (short form)
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

// reURLPatternExpanded keeps backticks in the matched URL so shell expansion
// markers survive extraction and the URL is classified conservatively.
var reURLPatternExpanded = regexp.MustCompile(`[a-zA-Z][a-zA-Z0-9+.-]*://[^\s'"]+`)

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

// reNonCanonicalNumeric detects hex (0x), octal (0-prefixed), decimal integer, or
// short-form IPs that Go's net.ParseIP won't handle but curl/wget resolve natively.
var reNonCanonicalNumeric = regexp.MustCompile(
	`^(0x[0-9a-fA-F]+|0[0-7]+\d|[0-9]{10,})$`,
)

// reNonCanonicalDotted detects dotted IPs with any non-standard octet encoding:
// leading-zero octets (octal), 0x-prefix octets (hex), or fewer than 4 octets (short-form).
var reNonCanonicalDotted = regexp.MustCompile(
	`^[0-9a-fA-Fx.]+$`,
)

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

	escalate := func(d Decision, reason string) {
		if bestDecision == "" || DecisionSeverity(d) > DecisionSeverity(bestDecision) {
			bestDecision = d
			bestReason = reason
		}
	}

	// Check insecure/redirect flags BEFORE URL extraction so they fire even when
	// URLs are variable-substituted (e.g., curl -k "$URL"). Only for network commands.
	basename := extractCmdBasename(cmd)
	isNetCmd := networkCommandBasenames[basename]

	if isNetCmd {
		inspectNetworkCommandFlags(cmd, escalate)
	}

	// Extract and inspect literal URLs.
	urls := reURLPatternExpanded.FindAllString(cmd, -1)
	for _, rawURL := range urls {
		if d, reason := inspectSingleURL(rawURL, cmd, false); d != "" {
			escalate(d, reason)
		}
	}

	// Variable destination detection: network command with no literal URLs
	// but shell variables in arguments suggests an unresolvable destination.
	// Only flag when there are ZERO literal :// strings in the entire command
	// to avoid false positives on header variables (e.g., curl -H "Bearer $TOKEN" https://...).
	if isNetCmd && len(urls) == 0 && !strings.Contains(cmd, "://") && containsShellVariable(cmd) {
		escalate(DecisionCaution, "network command with variable/unresolvable destination")
	}

	return bestDecision, bestReason
}

// inspectNetworkCommandFlags checks L7 flags on network commands (TLS bypass,
// destructive HTTP methods, data upload, redirects) and calls escalate for each match.
func inspectNetworkCommandFlags(cmd string, escalate func(Decision, string)) {
	if hasInsecureCertFlag(cmd) {
		escalate(DecisionCaution, "insecure TLS flag detected")
	}
	if reDestructiveHTTPMethod.MatchString(cmd) {
		escalate(DecisionApproval, "destructive HTTP method detected")
	}
	if hasDataUploadFlag(cmd) {
		escalate(DecisionCaution, "data upload/exfiltration flag detected")
	}
	if reFileUploadFlag.MatchString(cmd) {
		escalate(DecisionApproval, "file upload flag detected (exfiltration risk)")
	}
	if hasRedirectFlags(cmd) {
		escalate(DecisionCaution, "HTTP redirect following enabled")
	}
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

	// Non-canonical numeric host: decode and check against blocked ranges (SEC-002).
	if isNonCanonicalNumericHost(host) {
		return classifyNonCanonicalHost(host)
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

// classifyNonCanonicalHost decodes a non-canonical numeric IP and checks it
// against blocked ranges and hostnames. Returns CAUTION if decoding fails or
// the IP is not in any known range (still suspicious).
func classifyNonCanonicalHost(host string) (Decision, string) {
	decoded := decodeNonCanonicalIP(host)
	if decoded == nil {
		return DecisionCaution, "non-canonical numeric IP in URL"
	}
	// Check decoded IP against blocked/caution ranges.
	if d, reason := matchIPRanges(decoded, host); d != "" {
		return d, reason + " (decoded from non-canonical form)"
	}
	// Also check the decoded canonical form against BlockedHostnames.
	canonical := decoded.String()
	if BlockedHostnames[canonical] {
		return DecisionBlocked, "blocked hostname: " + canonical + " (decoded from " + host + ")"
	}
	return DecisionCaution, "non-canonical numeric IP in URL"
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
	values, _ := flattenStringValues(args)
	bestDecision := Decision("")
	bestReason := ""

	for _, v := range values {
		urls := reURLPatternExpanded.FindAllString(v, -1)
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

// containsShellVariable checks for shell variable references ($VAR, ${VAR}, $())
// in the argument portion of a command (skips the command name itself).
// Does not match shell specials like $?, $!, $$.
func containsShellVariable(cmd string) bool {
	fields := strings.Fields(cmd)
	for _, f := range fields[1:] { // skip command basename
		if strings.Contains(f, "${") || strings.Contains(f, "$(") || strings.Contains(f, "`") {
			return true
		}
		idx := strings.Index(f, "$")
		if idx >= 0 && idx+1 < len(f) {
			next := f[idx+1]
			if (next >= 'A' && next <= 'Z') || (next >= 'a' && next <= 'z') || next == '_' {
				return true
			}
		}
	}
	return false
}

// isNonCanonicalNumericHost detects hex/octal/decimal/short-form IP encodings (SEC-002).
func isNonCanonicalNumericHost(host string) bool {
	if reNonCanonicalNumeric.MatchString(host) {
		return true
	}
	// Detect dotted non-canonical forms: octal octets (leading zero), hex octets (0x),
	// or short-form (fewer than 4 octets). Only trigger if the host looks fully numeric
	// AND actually decodes to a valid IP. This avoids false positives on hex-looking
	// hostnames like "dead.beef" or "cafe.com".
	if reNonCanonicalDotted.MatchString(host) && strings.Contains(host, ".") {
		return decodeNonCanonicalIP(host) != nil
	}
	return false
}

// decodeNonCanonicalIP attempts to decode a non-canonical IP host string
// (hex, octal, decimal integer, dotted-octal, short-form, mixed) into a
// canonical net.IP. Returns nil if decoding fails.
//
// Follows BSD inet_aton rules used by curl:
// - Single number: entire 32-bit address
// - Two parts: a.b → a.0.0.b (last part fills remaining bytes)
// - Three parts: a.b.c → a.b.0.c
// - Four parts: standard dotted notation
// - Each part: 0x = hex, leading 0 = octal, else decimal
func decodeNonCanonicalIP(host string) net.IP {
	host = strings.TrimRight(host, ".")

	// Single number (no dots): hex, octal, or decimal integer for full 32-bit addr.
	if !strings.Contains(host, ".") {
		return decodeSingleNumberIP(host)
	}

	parts := strings.Split(host, ".")
	if len(parts) < 2 || len(parts) > 4 {
		return nil
	}

	// Parse each part, last part gets a wider range per inet_aton rules.
	var octets [4]byte
	var ok bool
	switch len(parts) {
	case 4:
		octets, ok = decodeFourPartIP(parts)
	case 3:
		octets, ok = decodeThreePartIP(parts)
	case 2:
		octets, ok = decodeTwoPartIP(parts)
	}
	if !ok {
		return nil
	}
	return net.IPv4(octets[0], octets[1], octets[2], octets[3])
}

// decodeSingleNumberIP decodes a single numeric host (no dots) into an IPv4 address.
func decodeSingleNumberIP(host string) net.IP {
	val, ok := parseOctet(host, 0xFFFFFFFF)
	if !ok {
		return nil
	}
	return net.IPv4(byte(val>>24), byte(val>>16), byte(val>>8), byte(val))
}

// decodeFourPartIP decodes a standard 4-octet dotted IP with non-canonical encoding.
func decodeFourPartIP(parts []string) ([4]byte, bool) {
	var octets [4]byte
	for i, p := range parts {
		val, ok := parseOctet(p, 0xFF)
		if !ok || val > 0xFF {
			return octets, false
		}
		octets[i] = byte(val)
	}
	return octets, true
}

// decodeThreePartIP decodes a.b.c → a.b.0.c (c fills last 2 bytes if > 255).
func decodeThreePartIP(parts []string) ([4]byte, bool) {
	var octets [4]byte
	for i := 0; i < 2; i++ {
		val, ok := parseOctet(parts[i], 0xFF)
		if !ok || val > 0xFF {
			return octets, false
		}
		octets[i] = byte(val)
	}
	val, ok := parseOctet(parts[2], 0xFFFF)
	if !ok || val > 0xFFFF {
		return octets, false
	}
	octets[2] = byte(val >> 8)
	octets[3] = byte(val)
	return octets, true
}

// decodeTwoPartIP decodes a.b → a.0.0.b (b fills last 3 bytes if > 255).
func decodeTwoPartIP(parts []string) ([4]byte, bool) {
	var octets [4]byte
	val0, ok := parseOctet(parts[0], 0xFF)
	if !ok || val0 > 0xFF {
		return octets, false
	}
	octets[0] = byte(val0)
	val1, ok := parseOctet(parts[1], 0xFFFFFF)
	if !ok || val1 > 0xFFFFFF {
		return octets, false
	}
	octets[1] = byte(val1 >> 16)
	octets[2] = byte(val1 >> 8)
	octets[3] = byte(val1)
	return octets, true
}

// parseOctet parses a single numeric part: 0x = hex, leading 0 = octal, else decimal.
// Returns the value and true if valid and within maxVal.
func parseOctet(s string, maxVal uint64) (uint64, bool) {
	if s == "" {
		return 0, false
	}
	var val uint64
	var err error
	switch {
	case strings.HasPrefix(s, "0x") || strings.HasPrefix(s, "0X"):
		if len(s) <= 2 {
			return 0, false
		}
		val, err = strconv.ParseUint(s[2:], 16, 64)
	case len(s) > 1 && s[0] == '0':
		val, err = strconv.ParseUint(s, 8, 64)
	default:
		val, err = strconv.ParseUint(s, 10, 64)
	}
	if err != nil || val > maxVal {
		return 0, false
	}
	return val, true
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
	classified := ClassificationNormalize(cmd)
	fields := strings.Fields(classified.Outer)
	if len(fields) == 0 {
		return ""
	}
	parts := strings.Split(fields[0], "/")
	return parts[len(parts)-1]
}
