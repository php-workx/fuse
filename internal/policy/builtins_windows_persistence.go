package policy

import (
	"regexp"

	"github.com/php-workx/fuse/internal/core"
)

func init() {
	rules := []BuiltinRule{
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

	BuiltinRules = append(rules, BuiltinRules...)
}
