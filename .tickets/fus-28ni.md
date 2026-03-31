---
id: fus-28ni
status: open
deps: [fus-6qhn, fus-5tax, fus-llzr, fus-jlcm]
links: []
created: 2026-03-31T15:12:41Z
type: feature
priority: 2
assignee: Ronny Unger
tags: [windows, security, phase-5, wave-2]
---
# CAUTION-level rules: general LOLBins, registry, service management

Issue 5.6. Append CAUTION-level rules to builtins_windows_security.go. These are operations that may be legitimate but warrant logging: general reg operations, general LOLBin usage without exploit args, Stop-Service, Set-ExecutionPolicy, Get-Credential, vaultcmd, general ComObject, Compress-Archive, pcalua -a, hh.exe http://, Invoke-Mimikatz, wevtutil sl, New-ItemProperty/Set-ItemProperty on registry. See plan.

## Acceptance Criteria

1. ~15 CAUTION rules appended to builtins_windows_security.go
2. General reg add/delete/import with specific keywords (not just 'reg')
3. General certutil without exploit args = CAUTION (excluding safe ops via predicate)
4. General New-Object -ComObject = CAUTION
5. pcalua, hh.exe, Invoke-Mimikatz, wevtutil sl included
6. Set-ExecutionPolicy, Stop-Service, Restart-Computer, Compress-Archive included
7. Tags registered in rule_tags.go
8. Test fixtures cover all rules
9. GOOS=windows go vet passes

