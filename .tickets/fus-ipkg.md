---
id: fus-ipkg
status: open
deps: [fus-j81l, fus-28ni]
links: []
created: 2026-03-31T15:12:57Z
type: feature
priority: 1
assignee: Ronny Unger
tags: [windows, security, phase-5, wave-3]
---
# PowerShell file scanner (.ps1)

Issue 5.7. Create internal/inspect/powershell.go implementing ScanPowerShell(content []byte) []Signal. Follow ScanShell() pattern but add block comment tracking (<# ... #>). Patterns include aliases (iex, iwr, irm, saps) since file content is not normalized through the alias resolution pipeline. Tests must verify block comments are skipped and legitimate patterns inside comments don't produce signals. See plan for full pattern list.

## Acceptance Criteria

1. ScanPowerShell() in internal/inspect/powershell.go follows ScanShell() pattern
2. Block comment tracking: <# ... #> toggled with boolean state, lines inside blocks skipped
3. Single-line comments (starting with #) skipped
4. Patterns scan for: dynamic_exec, http_download, process_spawn, persistence, defender_tamper, amsi_bypass, registry_modify, network_object, lolbin
5. Unit tests in powershell_test.go with clean content, commented-out patterns, block-commented patterns, and real attack patterns
6. Known limitation documented in code: multi-line here-strings and split commands not detected

