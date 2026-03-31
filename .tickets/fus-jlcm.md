---
id: fus-jlcm
status: open
deps: []
links: []
created: 2026-03-31T15:12:12Z
type: feature
priority: 0
assignee: Ronny Unger
tags: [windows, security, phase-5, wave-1]
---
# Hardcoded BLOCKED rules: Defender, AMSI, credential theft, event logs

Issue 5.4. Add non-overridable BLOCKED rules to hardcoded.go. These represent attacks that are NEVER legitimate in a developer context from an AI agent. AMSI bypass (AmsiUtils, amsiInitFailed, [Ref].Assembly.GetType) was flagged as BLOCKER by security reviewer — AI agents prompted to 'disable security' will generate these. procdump lsass and reg save SAM/SYSTEM are credential theft that should never pass. Defender tampering and event log clearing are anti-forensics. See plan.

## Acceptance Criteria

1. ~8 hardcoded BLOCKED rules in hardcoded.go for: Add-MpPreference exclusions, Set-MpPreference disable monitoring, AmsiUtils/amsiInitFailed/[Ref].Assembly.GetType (AMSI bypass), Clear-EventLog, wevtutil cl (anchored to exclude wevtutil el/gl), procdump lsass, reg save SAM/SYSTEM/SECURITY
2. Tests in hardcoded_test.go verify all rules fire and safe variants don't match
3. Test fixtures in commands.yaml
4. wevtutil cl anchored with (?i)\bwevtutil\s+cl\b
5. GOOS=windows go vet passes

