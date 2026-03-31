---
id: fus-rh1w
status: closed
deps: []
links: []
created: 2026-03-31T06:10:38Z
type: task
priority: 3
assignee: Ronny Unger
tags: [security, phase-3]
---
# Sanitize getContextVars env var values individually

getContextVars() in prompt_shared.go:36 builds the context string as 'K1=V1, K2=V2' where values are raw os.Getenv() output. The composite string is later passed to sanitizePrompt() which strips ANSI/control chars but allows printable ASCII including commas and equals signs.

If an env var value contains ', KUBECONFIG=/attacker/path', the approval prompt displays it as if an additional context variable is set. This could mislead an operator reviewing the approval context about which cloud account or cluster is active.

Low severity: attacker must control env var values (requires prior compromise) and the deception only affects the display context, not the command being approved.

Found by: our security-dataflow explorer.

Files: internal/approve/prompt_shared.go

Test cases:
- Set AWS_PROFILE to 'dev, KUBECONFIG=/evil/path', call getContextVars() — output should show the raw value, not split into two entries
- Sanitization applied per-value, not just to the composite string

## Acceptance Criteria

1. Each env var value is sanitized before concatenation in getContextVars()
2. A value containing ', FAKEKEY=fakeval' does not appear as a separate context entry
3. go test ./internal/approve/ -run TestGetContextVars -v passes

