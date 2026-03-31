---
id: fus-llzr
status: open
deps: []
links: []
created: 2026-03-31T15:11:49Z
type: feature
priority: 1
assignee: Ronny Unger
tags: [windows, security, phase-5, wave-1]
---
# Persistence and registry rules + sc/reg safe command exclusions

Issue 5.3. Create builtins_windows_persistence.go. APPROVAL rules for: schtasks /create, sc create/config, reg add with Run/RunOnce keys, New-Service, New-ScheduledTask/Register-ScheduledTask, startup folder writes (Start Menu\Programs\Startup and shell:startup), Register-WmiEvent/Set-WmiInstance __EventFilter (WMI event persistence), logman stop/delete (ETW tampering). Add sc query/queryex/qc and reg query/export to safe CMD builtins. See plan.

## Acceptance Criteria

1. ~15 rules in builtins_windows_persistence.go detect schtasks /create, sc create/config, reg add Run/RunOnce, New-Service, startup folder, WMI events, logman
2. sc query/queryex/qc and reg query/export added as safe CMD builtins in safecmds.go
3. Registry Run key regex anchored: \\Run(Once)?(\s|$|\\) to avoid matching RunDiagnostics
4. sc regex anchored: \bsc\s+(create|config)\b to exclude sc query
5. Tags registered with windows:persistence tag
6. Test fixtures cover all rules including safe sc/reg operations
7. GOOS=windows go vet passes

