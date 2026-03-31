---
id: fus-jq6z
status: open
deps: [fus-ipkg, fus-3n90]
links: []
created: 2026-03-31T15:13:28Z
type: feature
priority: 1
assignee: Ronny Unger
tags: [windows, security, phase-5, wave-3]
---
# Wire scanners into inspect pipeline + update inferDecisionFromSignals

Issue 5.9. Wire the PowerShell and batch scanners (from 5.7 and 5.8) into the inspection pipeline. Three changes to internal/core/inspect.go: (1) Add .ps1/.bat/.cmd cases to dispatchScannerAndInfer switch. (2) Add powershell.exe/pwsh.exe to DetectReferencedFile invoker list so 'powershell.exe script.ps1' triggers file inspection. (3) Update inferDecisionFromSignals with new signal category -> decision mappings. Critical: without the inferDecisionFromSignals update, defender_tamper signals from .ps1 files would only get CAUTION instead of BLOCKED. See plan for full mapping table.

## Acceptance Criteria

1. dispatchScannerAndInfer handles .ps1, .bat, .cmd extensions
2. DetectReferencedFile recognizes .ps1, .bat, .cmd extensions
3. powershell.exe and pwsh.exe added to DetectReferencedFile invoker list
4. inferDecisionFromSignals updated: defender_tamper/amsi_bypass -> BLOCKED, lolbin/http_download/process_spawn/persistence/firewall_modify/user_modify -> APPROVAL, registry_modify/network_object -> CAUTION
5. go test ./internal/core/ -run TestFixtureCoverage passes
6. go test ./internal/inspect/ passes

