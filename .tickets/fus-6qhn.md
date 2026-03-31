---
id: fus-6qhn
status: open
deps: []
links: []
created: 2026-03-31T15:11:20Z
type: feature
priority: 1
assignee: Ronny Unger
tags: [windows, security, phase-5, wave-1]
---
# Download cradle and obfuscation rules + irm alias fix

Issue 5.1. Create builtins_windows_download.go with download cradle detection rules. All patterns include aliases (iex, iwr, irm) directly in regex since alias resolution only fires inside powershell -Command wrappers. Add irm -> Invoke-RestMethod to normalize.go alias table. Rules: iex-downloadstring (BLOCKED), iex-webclient (BLOCKED), pipe-to-iex (BLOCKED), downloadstring-type (BLOCKED), irm-pipe-iex (BLOCKED), Start-BitsTransfer with URL (APPROVAL), Invoke-WebRequest -OutFile (APPROVAL), Invoke-RestMethod with mutating verbs (APPROVAL). See plan: .agents/plans/2026-03-31-windows-security-intelligence.md

## Acceptance Criteria

1. ~10 rules in builtins_windows_download.go detect IEX+DownloadString, pipe-to-IEX, .NET WebClient, irm|iex, Start-BitsTransfer
2. irm alias added to normalize.go powerShellAliases map
3. Rules include both canonical names AND aliases in regex (e.g. Invoke-Expression|iex)
4. Tags registered in rule_tags.go with windows:download tag
5. Test fixtures in commands.yaml cover all rules
6. GOOS=windows go vet passes

