package policy

import (
	"regexp"

	"github.com/php-workx/fuse/internal/core"
)

func init() {
	rules := []BuiltinRule{
		{
			ID:      "builtin:windows:iex-downloadstring",
			Pattern: regexp.MustCompile(`(?i)\b(Invoke-Expression|iex)\b.*(DownloadString|DownloadFile)\b`),
			Action:  core.DecisionBlocked,
			Reason:  "PowerShell download cradle: download and execute",
		},
		{
			ID:      "builtin:windows:iex-webclient",
			Pattern: regexp.MustCompile(`(?i)((New-Object|Net\.WebClient).*\b(Invoke-Expression|iex)\b|\b(Invoke-Expression|iex)\b.*(New-Object|Net\.WebClient))`),
			Action:  core.DecisionBlocked,
			Reason:  "PowerShell WebClient with Invoke-Expression",
		},
		{
			ID:      "builtin:windows:pipe-to-iex",
			Pattern: regexp.MustCompile(`(?i)\b(Invoke-WebRequest|iwr|Invoke-RestMethod|irm)\b.*\|\s*(Invoke-Expression|iex)\b`),
			Action:  core.DecisionBlocked,
			Reason:  "PowerShell download piped to Invoke-Expression",
		},
		{
			ID:      "builtin:windows:downloadstring-type",
			Pattern: regexp.MustCompile(`(?i)System\.Net\.WebClient.*Download(String|File)`),
			Action:  core.DecisionBlocked,
			Reason:  ".NET WebClient download primitive",
		},
		{
			ID:      "builtin:windows:start-bitstransfer-url",
			Pattern: regexp.MustCompile(`(?i)\bStart-BitsTransfer\b.*\s-Source\b.*https?://`),
			Action:  core.DecisionApproval,
			Reason:  "Downloads content via BITS",
		},
		{
			ID:      "builtin:windows:invoke-webrequest-outfile",
			Pattern: regexp.MustCompile(`(?i)\b(Invoke-WebRequest|iwr)\b.*\s-OutFile\b`),
			Action:  core.DecisionApproval,
			Reason:  "Downloads remote content to disk via Invoke-WebRequest",
		},
		{
			ID:      "builtin:windows:invoke-restmethod-mutating",
			Pattern: regexp.MustCompile(`(?i)\b(Invoke-RestMethod|irm)\b.*\s-Method\s+(POST|PUT|PATCH|DELETE)\b`),
			Action:  core.DecisionApproval,
			Reason:  "Mutating HTTP request via Invoke-RestMethod",
		},
	}

	// Prepend the Windows download rules so they win over broader existing
	// heuristics such as generic archive-creation or exfiltration matches.
	BuiltinRules = append(rules, BuiltinRules...)
}
