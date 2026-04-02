package policy

import (
	"regexp"

	"github.com/php-workx/fuse/internal/core"
)

func init() {
	rules := []BuiltinRule{
		// Download cradles and retrieval primitives.
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
			ID:      "builtin:windows:iex-webrequest-content",
			Pattern: regexp.MustCompile(`(?i)\b(Invoke-Expression|iex)\b.*\b(Invoke-WebRequest|iwr)\b.*\.Content\b`),
			Action:  core.DecisionBlocked,
			Reason:  "PowerShell Invoke-WebRequest content executed via Invoke-Expression",
		},
		{
			ID:      "builtin:windows:downloadstring-type",
			Pattern: regexp.MustCompile(`(?i)System\.Net\.WebClient.*Download(String|File)`),
			Action:  core.DecisionBlocked,
			Reason:  ".NET WebClient download primitive",
		},
		{
			ID:      "builtin:windows:start-bitstransfer-url",
			Pattern: regexp.MustCompile(`(?i)\bStart-BitsTransfer\b(?:.*\s-Source\b.*https?://|\s+https?://)`),
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

		// Persistence creation and ETW tampering primitives.
		{
			ID:      "builtin:windows:schtasks-create",
			Pattern: regexp.MustCompile(`(?i)\bschtasks\b.*\s/create\b`),
			Action:  core.DecisionApproval,
			Reason:  "scheduled task creation",
		},
		{
			ID:      "builtin:windows:sc-create-config",
			Pattern: regexp.MustCompile(`(?i)\bsc\s+(create|config)\b`),
			Action:  core.DecisionApproval,
			Reason:  "service creation or reconfiguration",
		},
		{
			ID:      "builtin:windows:reg-run-key",
			Pattern: regexp.MustCompile(`(?i)\breg\s+add\b.*\\Run(Once)?(\s|$|\\)`),
			Action:  core.DecisionApproval,
			Reason:  "registry Run/RunOnce persistence",
		},
		{
			ID:      "builtin:windows:new-service",
			Pattern: regexp.MustCompile(`(?i)\bNew-Service\b`),
			Action:  core.DecisionApproval,
			Reason:  "PowerShell service creation",
		},
		{
			ID:      "builtin:windows:scheduledtask-register",
			Pattern: regexp.MustCompile(`(?i)\b(New-ScheduledTask|Register-ScheduledTask)\b`),
			Action:  core.DecisionApproval,
			Reason:  "PowerShell scheduled task persistence",
		},
		{
			ID:      "builtin:windows:startup-folder",
			Pattern: regexp.MustCompile(`(?i)(Start Menu\\Programs\\Startup|shell:startup)`),
			Action:  core.DecisionApproval,
			Reason:  "startup folder persistence",
		},
		{
			ID:      "builtin:windows:wmi-event-persistence",
			Pattern: regexp.MustCompile(`(?i)\b(Register-WmiEvent|Set-WmiInstance)\b.*(__EventFilter|CommandLineEventConsumer|ActiveScriptEventConsumer)?`),
			Action:  core.DecisionApproval,
			Reason:  "WMI event subscription persistence",
		},
		{
			ID:      "builtin:windows:logman-tamper",
			Pattern: regexp.MustCompile(`(?i)\blogman\b.*\s(stop|delete)\b`),
			Action:  core.DecisionApproval,
			Reason:  "ETW trace session tampering",
		},
	}

	// Prepend the Windows download and persistence rules so they win over
	// broader existing heuristics.
	BuiltinRules = append(rules, BuiltinRules...)
}
