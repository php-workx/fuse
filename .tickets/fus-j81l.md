---
id: fus-j81l
status: open
deps: [fus-6qhn, fus-5tax, fus-llzr, fus-jlcm]
links: []
created: 2026-03-31T15:12:27Z
type: feature
priority: 1
assignee: Ronny Unger
tags: [windows, security, phase-5, wave-2]
---
# Credential, WMI, firewall, user management, network rules + alias fixes

Issue 5.5. Create builtins_windows_security.go with APPROVAL rules for: cmdkey /add, net user /add, net localgroup administrators /add, ntdsutil, specific dangerous COM ProgIDs, Start-Process -Verb RunAs (with saps alias), Invoke-WmiMethod Create, wmic process call create, Invoke-Command -ComputerName (with icm alias), New-PSSession (with nsn alias), Enter-PSSession (with etsn alias), wmic /node:, netsh advfirewall add/delete rule, New-NetFirewallRule, auditpol /set disable. Add icm/nsn/etsn aliases to normalize.go. See plan.

## Acceptance Criteria

1. ~20 rules in builtins_windows_security.go for credential, network, WMI, user management
2. icm, nsn, etsn aliases added to normalize.go powerShellAliases map
3. Remote execution rules include aliases in regex (Invoke-Command|icm, New-PSSession|nsn, Enter-PSSession|etsn)
4. New-Object -ComObject with dangerous ProgIDs (WScript.Shell, Shell.Application, MMC20.Application) = APPROVAL
5. General New-Object -ComObject = CAUTION (not APPROVAL)
6. Tags registered with windows:credential and windows:network
7. Test fixtures cover all rules
8. GOOS=windows go vet passes

