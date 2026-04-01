package policy

import (
	"regexp"
	"strings"

	"github.com/php-workx/fuse/internal/core"
)

var reWindowsRunKeyBoundary = regexp.MustCompile(`(?i)\\run(once)?(\s|$|\\)`)

func init() {
	rules := []BuiltinRule{
		{
			ID:      "builtin:windows:cmdkey-add",
			Pattern: regexp.MustCompile(`(?i)\bcmdkey\b.*\s/add(:|\b)`),
			Action:  core.DecisionApproval,
			Reason:  "Windows Credential Manager target creation",
		},
		{
			ID:      "builtin:windows:net-user-add",
			Pattern: regexp.MustCompile(`(?i)\bnet\b\s+user\b.*\s/add\b`),
			Action:  core.DecisionApproval,
			Reason:  "Local user creation via net user",
		},
		{
			ID:      "builtin:windows:net-localgroup-admin-add",
			Pattern: regexp.MustCompile(`(?i)\bnet\b\s+localgroup\b.*\badministrators\b.*\s/add\b`),
			Action:  core.DecisionApproval,
			Reason:  "Adds account to local Administrators group",
		},
		{
			ID:      "builtin:windows:ntdsutil",
			Pattern: regexp.MustCompile(`(?i)\bntdsutil\b`),
			Action:  core.DecisionApproval,
			Reason:  "Directory Services database tooling invocation",
		},
		{
			ID:      "builtin:windows:comobject-wscript-shell",
			Pattern: regexp.MustCompile(`(?i)\bNew-Object\b.*\s-ComObject\s+WScript\.Shell\b`),
			Action:  core.DecisionApproval,
			Reason:  "Dangerous COM object creation: WScript.Shell",
		},
		{
			ID:      "builtin:windows:comobject-shell-application",
			Pattern: regexp.MustCompile(`(?i)\bNew-Object\b.*\s-ComObject\s+Shell\.Application\b`),
			Action:  core.DecisionApproval,
			Reason:  "Dangerous COM object creation: Shell.Application",
		},
		{
			ID:      "builtin:windows:comobject-shellbrowserwindow",
			Pattern: regexp.MustCompile(`(?i)\bNew-Object\b.*\s-ComObject\s+ShellBrowserWindow\b`),
			Action:  core.DecisionApproval,
			Reason:  "Dangerous COM object creation: ShellBrowserWindow",
		},
		{
			ID:      "builtin:windows:comobject-mmc20",
			Pattern: regexp.MustCompile(`(?i)\bNew-Object\b.*\s-ComObject\s+MMC20\.Application\b`),
			Action:  core.DecisionApproval,
			Reason:  "Dangerous COM object creation: MMC20.Application",
		},
		{
			ID:      "builtin:windows:start-process-runas",
			Pattern: regexp.MustCompile(`(?i)\b(Start-Process|saps|start)\b.*\s-Verb\s+RunAs\b`),
			Action:  core.DecisionApproval,
			Reason:  "PowerShell process launch with elevation",
		},
		{
			ID:      "builtin:windows:invoke-wmimethod-create",
			Pattern: regexp.MustCompile(`(?i)\bInvoke-WmiMethod\b.*\s-Name\s+Create\b`),
			Action:  core.DecisionApproval,
			Reason:  "WMI method invocation creating a process",
		},
		{
			ID:      "builtin:windows:wmic-process-create",
			Pattern: regexp.MustCompile(`(?i)\bwmic\b.*\bprocess\b.*\bcall\b.*\bcreate\b`),
			Action:  core.DecisionApproval,
			Reason:  "wmic process creation",
		},
		{
			ID:      "builtin:windows:invoke-command-remote",
			Pattern: regexp.MustCompile(`(?i)\b(Invoke-Command|icm)\b.*\s-ComputerName\b`),
			Action:  core.DecisionApproval,
			Reason:  "PowerShell remoting to another host",
		},
		{
			ID:      "builtin:windows:new-pssession",
			Pattern: regexp.MustCompile(`(?i)\b(New-PSSession|nsn)\b.*\s-ComputerName\b`),
			Action:  core.DecisionApproval,
			Reason:  "Creates a remote PowerShell session",
		},
		{
			ID:      "builtin:windows:enter-pssession",
			Pattern: regexp.MustCompile(`(?i)\b(Enter-PSSession|etsn)\b.*\s-ComputerName\b`),
			Action:  core.DecisionApproval,
			Reason:  "Enters a remote PowerShell session",
		},
		{
			ID:      "builtin:windows:wmic-node-remote",
			Pattern: regexp.MustCompile(`(?i)\bwmic\b.*\s/node:\S+`),
			Action:  core.DecisionApproval,
			Reason:  "wmic remote node execution or inspection",
		},
		{
			ID:      "builtin:windows:netsh-advfirewall-rule",
			Pattern: regexp.MustCompile(`(?i)\bnetsh\b.*\badvfirewall\b.*\b(add|delete)\b.*\brule\b`),
			Action:  core.DecisionApproval,
			Reason:  "Adds or deletes firewall rules via netsh",
		},
		{
			ID:      "builtin:windows:new-netfirewallrule",
			Pattern: regexp.MustCompile(`(?i)\bNew-NetFirewallRule\b`),
			Action:  core.DecisionApproval,
			Reason:  "Creates a new firewall rule in PowerShell",
		},
		{
			ID:      "builtin:windows:auditpol-disable",
			Pattern: regexp.MustCompile(`(?i)\bauditpol\b.*\s/set\b.*\bdisable\b`),
			Action:  core.DecisionApproval,
			Reason:  "Disables Windows audit policy coverage",
		},
		{
			ID:      "builtin:windows:reg-add-general",
			Pattern: regexp.MustCompile(`(?i)\breg\s+add\b`),
			Action:  core.DecisionCaution,
			Reason:  "General registry modification via reg add",
			Predicate: func(cmd string) bool {
				return !reWindowsRunKeyBoundary.MatchString(cmd)
			},
		},
		{
			ID:      "builtin:windows:reg-delete-general",
			Pattern: regexp.MustCompile(`(?i)\breg\s+delete\b`),
			Action:  core.DecisionCaution,
			Reason:  "General registry deletion via reg delete",
		},
		{
			ID:      "builtin:windows:reg-import-general",
			Pattern: regexp.MustCompile(`(?i)\breg\s+import\b`),
			Action:  core.DecisionCaution,
			Reason:  "Registry import from .reg content",
		},
		{
			ID:      "builtin:windows:stop-service",
			Pattern: regexp.MustCompile(`(?i)\bStop-Service\b`),
			Action:  core.DecisionCaution,
			Reason:  "Stops a Windows service",
		},
		{
			ID:      "builtin:windows:restart-computer",
			Pattern: regexp.MustCompile(`(?i)\bRestart-Computer\b`),
			Action:  core.DecisionCaution,
			Reason:  "Restarts the computer",
		},
		{
			ID:      "builtin:windows:stop-computer",
			Pattern: regexp.MustCompile(`(?i)\bStop-Computer\b`),
			Action:  core.DecisionCaution,
			Reason:  "Shuts down the computer",
		},
		{
			ID:      "builtin:windows:set-executionpolicy",
			Pattern: regexp.MustCompile(`(?i)\bSet-ExecutionPolicy\b`),
			Action:  core.DecisionCaution,
			Reason:  "Changes PowerShell execution policy",
		},
		{
			ID:      "builtin:windows:get-credential",
			Pattern: regexp.MustCompile(`(?i)\bGet-Credential\b`),
			Action:  core.DecisionCaution,
			Reason:  "Prompts for credentials",
		},
		{
			ID:      "builtin:windows:vaultcmd",
			Pattern: regexp.MustCompile(`(?i)\bvaultcmd\b`),
			Action:  core.DecisionCaution,
			Reason:  "Interacts with Windows Credential Manager",
		},
		{
			ID:      "builtin:windows:compress-archive",
			Pattern: regexp.MustCompile(`(?i)\bCompress-Archive\b`),
			Action:  core.DecisionCaution,
			Reason:  "Creates an archive that may stage data",
		},
		{
			ID:      "builtin:windows:pcalua-launch",
			Pattern: regexp.MustCompile(`(?i)\bpcalua\b.*\s-a\b`),
			Action:  core.DecisionCaution,
			Reason:  "Launches a program through Program Compatibility Assistant",
		},
		{
			ID:      "builtin:windows:hh-remote",
			Pattern: regexp.MustCompile(`(?i)\bhh\.exe\b.*https?://`),
			Action:  core.DecisionCaution,
			Reason:  "HTML Help invoked with a remote URL",
		},
		{
			ID:      "builtin:windows:invoke-mimikatz",
			Pattern: regexp.MustCompile(`(?i)\bInvoke-Mimikatz\b`),
			Action:  core.DecisionApproval,
			Reason:  "Known offensive PowerShell function name",
		},
		{
			ID:      "builtin:windows:wevtutil-set-log",
			Pattern: regexp.MustCompile(`(?i)\bwevtutil(?:\.exe)?\b.*\bsl\b.*\s/e:(false|0)\b`),
			Action:  core.DecisionBlocked,
			Reason:  "Disables a Windows event log channel",
		},
		{
			ID:      "builtin:windows:new-itemproperty-registry",
			Pattern: regexp.MustCompile(`(?i)\bNew-ItemProperty\b.*\b(HKLM:|HKCU:|Registry::)`),
			Action:  core.DecisionCaution,
			Reason:  "Creates a Windows registry property",
		},
		{
			ID:      "builtin:windows:set-itemproperty-registry",
			Pattern: regexp.MustCompile(`(?i)\bSet-ItemProperty\b.*\b(HKLM:|HKCU:|Registry::)`),
			Action:  core.DecisionCaution,
			Reason:  "Modifies a Windows registry property",
		},
		{
			ID:      "builtin:windows:rundll32-general",
			Pattern: regexp.MustCompile(`(?i)\brundll32\b`),
			Action:  core.DecisionCaution,
			Reason:  "General rundll32 usage",
			Predicate: func(cmd string) bool {
				return !strings.Contains(strings.ToLower(cmd), "javascript:")
			},
		},
		{
			ID:      "builtin:windows:regsvr32-general",
			Pattern: regexp.MustCompile(`(?i)\bregsvr32\b`),
			Action:  core.DecisionCaution,
			Reason:  "General regsvr32 usage",
			Predicate: func(cmd string) bool {
				lower := strings.ToLower(cmd)
				return !strings.Contains(lower, "/i:http://") && !strings.Contains(lower, "/i:https://")
			},
		},
		{
			ID:      "builtin:windows:mshta-general",
			Pattern: regexp.MustCompile(`(?i)\bmshta\b`),
			Action:  core.DecisionCaution,
			Reason:  "General mshta usage",
			Predicate: func(cmd string) bool {
				lower := strings.ToLower(cmd)
				return !strings.Contains(lower, "http://") &&
					!strings.Contains(lower, "https://") &&
					!strings.Contains(lower, "vbscript:") &&
					!strings.Contains(lower, "javascript:")
			},
		},
		{
			ID:      "builtin:windows:comobject-general",
			Pattern: regexp.MustCompile(`(?i)\bNew-Object\b.*\s-ComObject\b`),
			Action:  core.DecisionCaution,
			Reason:  "General COM object instantiation",
		},
	}

	BuiltinRules = append(rules, BuiltinRules...)
}
