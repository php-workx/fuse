---
id: pre-mortem-2026-04-02-rule-landscape
type: pre-mortem
date: 2026-04-02
source: .agents/plans/2026-04-02-rule-landscape.md
---

# Pre-Mortem: Rule Landscape Gap Closure

## Council Verdict: PASS

| Judge | Verdict | Key Finding |
|-------|---------|-------------|
| Missing-requirements | PASS | The scope is intentionally narrowed to the two real open Priority 1 gaps instead of the whole stale matrix. |
| Feasibility | PASS | CI/CD builtins are straightforward and indirect extraction can reuse existing normalization recursion. |
| Scope | WARN | The main risk is overreaching into full shell interpretation instead of keeping extraction to narrow, high-confidence wrapper shapes. |

## Shared Findings

- Do not attempt full general-purpose parsing for `xargs`, `parallel`, or `watch` in this pass.
- Keep CI/CD rules narrow and operationally destructive; read-only/list commands must remain SAFE.
- Golden fixtures are required in the same change set or the new family will drift immediately.

## Concerns Raised

- Regex overmatch on generic `gh api` commands could create noise.
- Wrapper extraction must fail closed on ambiguous shell-bearing forms rather than guessing.
- The spec file itself is stale, so the implementation must follow codebase reality, not every outdated row in the matrix.

## Recommendation

Proceed with the focused slice in the plan. Validate with unit tests plus fixture coverage before claiming the spec slice is closed.

## Decision Gate

[x] PROCEED - Council passed, ready to implement
[ ] ADDRESS - Fix concerns before implementing
[ ] RETHINK - Fundamental issues, needs redesign
