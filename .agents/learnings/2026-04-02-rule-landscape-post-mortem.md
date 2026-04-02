---
type: learning
source: rpi
confidence: high
date: 2026-04-02
---

# Learning: Rule-matrix specs drift faster than the code

## Pattern

Cross-project gap matrices are useful for prioritization, but they become misleading if they are treated as current-state inventories after multiple implementation waves. The right first move is to re-audit the matrix against the tree and narrow the implementation to the still-real gaps.

## Findings

1. Two of the most obvious “missing” items in `specs/rule-landscape.md` were already done: safe build-dir cleanup and container-escape coverage.
2. A focused slice was still enough to move the spec forward materially: CI/CD protection plus indirect-wrapper extraction closed the highest-value remaining Priority 1 gaps.
3. For indirect execution, high-confidence extraction beats broad guessing. Ambiguous shell-bearing wrappers should fail closed rather than attempt full shell interpretation.

## Recommendation

When executing a landscape-style spec, start by splitting it into:
- stale rows to update
- real gaps to implement now
- remaining families to track as follow-up work

