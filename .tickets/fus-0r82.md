---
id: fus-0r82
status: closed
deps: []
links: []
created: 2026-03-31T06:10:26Z
type: chore
priority: 3
assignee: Ronny Unger
tags: [documentation, phase-3]
---
# Update errNonInteractive message in specs/technical_v1.1.md

Phase 3 changed the errNonInteractive message from '/dev/tty unavailable' to 'console unavailable' to be platform-agnostic. But specs/technical_v1.1.md was not updated and still references the old message. The machine-parsed NON_INTERACTIVE_MODE token is preserved, so no functional impact — but the spec is stale.

Found by: our api-contract explorer.

Files: specs/technical_v1.1.md

## Acceptance Criteria

1. specs/technical_v1.1.md lines 230 and 1563 say 'console unavailable' (not '/dev/tty unavailable')
2. Message matches actual code in prompt_shared.go

