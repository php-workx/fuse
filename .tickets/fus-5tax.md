---
id: fus-5tax
status: open
deps: []
links: []
created: 2026-03-31T15:11:32Z
type: feature
priority: 1
assignee: Ronny Unger
tags: [windows, security, phase-5, wave-1]
---
# LOLBin detection rules + certutil safe command exclusions

Issue 5.2. Create builtins_windows_lolbin.go. APPROVAL rules for exploit-specific args (certutil -decode/-urlcache, bitsadmin /transfer, mshta http:/vbscript:, regsvr32 /s /i:http, rundll32 javascript:, cmstp /s .inf, msiexec /i http:, wscript/cscript //e:, forfiles /c). CAUTION rules for general LOLBin usage without exploit args. Add certutil safe operations to safecmds.go IsConditionallySafe. See plan.

## Acceptance Criteria

1. ~15 rules in builtins_windows_lolbin.go detect certutil, bitsadmin, mshta, regsvr32, rundll32, cmstp, msiexec, wscript/cscript, forfiles
2. certutil -hashfile/-verify/-dump/-store/-viewstore added as conditionally safe in safecmds.go
3. General certutil CAUTION rule has predicate excluding safe operations
4. wscript/cscript with //e: flag = APPROVAL, without = CAUTION
5. Tags registered with windows:lolbin tag
6. Test fixtures cover all rules including safe certutil operations
7. GOOS=windows go vet passes

