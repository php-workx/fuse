---
id: plan-2026-03-10-spec-audit-fixes
type: plan
date: 2026-03-10
source: "spec-vs-implementation audit of specs/technical_v1.1.md"
pre-mortem: "[[.agents/council/2026-03-10-pre-mortem-spec-audit-fixes]]"
---

# Plan: Fix Spec Audit Findings

## Context

A 5-agent parallel audit of `specs/technical_v1.1.md` against the implementation found 202 PASS, 16 PARTIAL, 11 GAP, and 2 MISSING_TEST across all 16 spec sections. This plan addresses the actionable findings in priority order. Findings in the false-positive-safe direction (more conservative than spec) are deferred unless trivial.

**Pre-mortem applied:** Council verdict WARN. Fixed 6 concerns — v3 migration (not v2), merged Issue 6 into Issue 5 (shared `db_test.go`), selected Option A for Issue 3 (struct flag), specified post-filter for Issue 7, verified Issue 8 safety.

## Boundaries

**Always:** All changes must pass `go test ./... -timeout 120s`. No new dependencies. No behavioral regressions in existing tests.
**Ask First:** Changes to approval consumption atomicity (SELECT+UPDATE → single statement) affect concurrency semantics.
**Never:** Do not touch bubbletea/lipgloss migration (spec-should-update, not code). Do not change rule patterns. Do not modify the decision model.

## Baseline Audit

| Metric | Command | Result |
|--------|---------|--------|
| DYLD_ entries in strippedEnvVars | `grep -c DYLD_ runner.go` | 2 (need prefix match) |
| Empty command exit code | `grep 'Command == ""' hook.go` | returns 0 (spec says 2) |
| WAL checkpoint calls | `grep -rn wal_checkpoint internal/db/` | 0 (spec requires after cleanup) |
| VACUUM calls | `grep -rn VACUUM internal/db/` | 0 (spec requires every 100th) |
| idx_events_ts index | `grep idx_events schema.go` | missing |
| Credential scrub patterns | `grep -c REDACTED events.go` | 2 (missing password flag, Authorization) |
| Credential scrub text | `grep 'REDACTED' events.go` | uses `***REDACTED***` (spec says `[REDACTED]`) |
| BuildChildEnv tests | `grep -c 'func Test' runner_test.go` | 5 (none for BuildChildEnv) |
| Sensitive resource tests | `grep -c 'func Test' mcpproxy_test.go` | 5 (none for resource blocking) |
| db_test.go REDACTED assertions | `grep -c 'REDACTED' db_test.go` | 6 (will break if scrub text changes) |
| Schema version | `grep currentSchemaVersion schema.go` | "2" (v3 needed for new index) |
| Files to modify | count | 10 source + 5 test files |
| Total LOC in scope | `wc -l` on 10 files | 2,780 |

## Files to Modify

| File | Change |
|------|--------|
| `internal/adapters/runner.go` | DYLD_* prefix matching in `strippedEnvVars` / `BuildChildEnv` |
| `internal/adapters/runner_test.go` | **NEW tests** for `BuildChildEnv` |
| `internal/adapters/hook.go` | Empty command → exit 2 |
| `internal/adapters/hook_test.go` | Test for empty command blocking |
| `internal/adapters/mcpproxy_test.go` | Test for sensitive resource blocking |
| `internal/core/normalize.go` | Add `ExtractionFailed` field to `ClassifiedCommand`; set on bash -c failure |
| `internal/core/classify.go` | Check `ExtractionFailed` flag → escalate to APPROVAL |
| `internal/core/normalize_test.go` | Test for extraction failure fail-closed |
| `internal/core/inspect.go` | Default unknown extension → CAUTION; cloud_sdk alone → CAUTION |
| `internal/core/inspect_test.go` | Test for unknown extension decision |
| `internal/db/schema.go` | v3 migration: `idx_events_ts`, WAL checkpoint setup |
| `internal/db/approvals.go` | 1-hour grace period for consumed approvals |
| `internal/db/events.go` | `[REDACTED]` text; add `-p/--password`, `Authorization:` patterns |
| `internal/db/db.go` | WAL checkpoint helper; VACUUM helper |
| `internal/db/db_test.go` | Tests for checkpoint/vacuum/cleanup grace; update 6 REDACTED assertions |
| `internal/inspect/python.go` | Post-filter: scope `import shutil` to rmtree; scope `import os` to os.system/remove/rmdir |

## Implementation

### 1. DYLD_* prefix-based stripping (security fix)

In `internal/adapters/runner.go`:

- **Replace** the two explicit `DYLD_INSERT_LIBRARIES` / `DYLD_LIBRARY_PATH` entries in `strippedEnvVars` with prefix-based matching.
- **Modify `BuildChildEnv`**: Add a `strings.HasPrefix(name, "DYLD_")` check alongside the map lookup:

```go
// In the loop:
if strippedEnvVars[name] || strings.HasPrefix(name, "DYLD_") {
    continue
}
```

- Remove `DYLD_INSERT_LIBRARIES` and `DYLD_LIBRARY_PATH` from the map (now covered by prefix).

In `internal/adapters/runner_test.go` — add:
- `TestBuildChildEnv_StripsDYLDPrefix`: Set `DYLD_FRAMEWORK_PATH=bad` in input, verify absent from output.
- `TestBuildChildEnv_ResetsPathToTrusted`: Set `PATH=/evil` in input, verify output PATH matches `trustedPath()`.
- `TestBuildChildEnv_StripsLDPreload`: Set `LD_PRELOAD=/lib` in input, verify absent.

### 2. Empty hook command → exit 2

In `internal/adapters/hook.go`:

- **Line 178**: Change `return 0, nil` to `return 2, nil` when `input.Command == ""`.

In `internal/adapters/hook_test.go` — add:
- `TestRunHook_EmptyCommandExitsTwo`: Send `{"tool_name":"Bash","tool_input":{"command":""}}`, verify exit code 2.

### 3. bash -c extraction failure → APPROVAL (Option A: struct flag)

**Design decision (pre-mortem):** Use Option A — add `ExtractionFailed bool` field to `ClassifiedCommand` struct and check it in `classify.go`.

In `internal/core/normalize.go`:

- **Add field to `ClassifiedCommand` struct** (line ~111):
  ```go
  type ClassifiedCommand struct {
      Outer                  string
      Inner                  string
      EscalateClassification bool
      ExtractionFailed       bool   // bash -c inner extraction failed
  }
  ```

- **Around line 144**: When `extractBashCInner` returns `ok == false` for a `bash`/`sh` `-c` token, set `result.ExtractionFailed = true`. The current code falls through to line ~176 where `result.Outer = strings.Join(tokens[i:], " ")` — the outer command is still set as `bash ...`. With the new flag, `classify.go` can detect this case.

In `internal/core/classify.go`:

- **After line ~186** (after `ClassificationNormalize` returns): Check `ExtractionFailed` flag:
  ```go
  if classified.ExtractionFailed {
      return DecisionApproval, "bash -c extraction failed (fail-closed per §5.2)"
  }
  ```

In `internal/core/normalize_test.go` — add:
- `TestClassificationNormalize_BashCExtractionFailure`: Input with unbalanced quotes `bash -c 'echo hello` → verify `ExtractionFailed == true` and `Inner == ""`.

### 4. Missing tests for sensitive resource blocking

In `internal/adapters/mcpproxy_test.go` — add:
- `TestProxyAgentToDownstream_SensitiveResourceReadBlocked`: Send `resources/read` with `uri: "~/.fuse/state/fuse.db"`, verify JSON-RPC error returned, downstream receives nothing.
- `TestProxyAgentToDownstream_NonSensitiveResourceForwarded`: Send `resources/read` with `uri: "/tmp/public.txt"`, verify request frame is written to downstream buffer. (Name changed per pre-mortem recommendation — only verify forwarding, not response.)

### 5. SQLite housekeeping + credential scrubbing (merged with former Issue 6)

**Pre-mortem fix:** Issue 6 (credential scrubbing) merged here because both touch `db_test.go`. Must use v3 migration (not v2) — existing v2 databases skip the v2 block entirely (`schema.go:30` returns early when `version == currentSchemaVersion`).

In `internal/db/schema.go`:
- **Set `currentSchemaVersion = "3"`**.
- **Add v3 migration block**: `if version == "2"` → run:
  - `CREATE INDEX IF NOT EXISTS idx_events_ts ON events(timestamp)`
  - WAL checkpoint setup

In `internal/db/db.go`:
- **Add `WalCheckpoint()`**: Execute `PRAGMA wal_checkpoint(TRUNCATE)`.
- **Add `Vacuum()`**: Execute `VACUUM`.
- **Add cleanup cycle counter**: Package-level `var cleanupCycleCount int64` with atomic increment.

In `internal/db/approvals.go`:
- **Modify `CleanupExpired`**: Change consumed cleanup to retain for 1 hour:
  ```sql
  WHERE (consumed_at IS NOT NULL AND consumed_at < ?)
     OR (expires_at IS NOT NULL AND expires_at <= ?)
  ```
  With `consumed_at < now - 1 hour` computed as `time.Now().Add(-time.Hour)`.

In `internal/db/events.go`:
- **Change** `***REDACTED***` to `[REDACTED]` in all replacement strings.
- **Change** `Bearer ***REDACTED***` to `Bearer [REDACTED]`.
- **Add** pattern for password flags: `(?i)(-p\s+|--password[= ]\s*)\S+` → `$1[REDACTED]`
- **Add** pattern for Authorization header: `(?i)Authorization:\s*\S+` → `Authorization: [REDACTED]`

In `internal/db/db_test.go`:
- **Update 6 existing assertions** (lines 290-325): Change `***REDACTED***` → `[REDACTED]` in all `want` strings.
- **Add** `TestCleanupExpired_RetainsRecentlyConsumed`: Create consumed approval 30min ago, verify not deleted.
- **Add** `TestCleanupExpired_DeletesOldConsumed`: Create consumed approval 2h ago, verify deleted.

### 6. Python scanner scoping (post-filter approach)

**Design decision (pre-mortem):** Use post-filter on the returned `signals` slice (simpler than two-pass file reading).

In `internal/inspect/python.go`:
- **After scanning all lines**, add a `scopeImportSignals` post-filter:
  ```go
  // After scanning all lines:
  signals = scopeImportSignals(signals)
  ```
- **`scopeImportSignals`** removes:
  - `import shutil` / `destructive_fs` signal if no `shutil.rmtree` signal is also in the list
  - `import os` / `subprocess` signal if no `os.system`, `os.remove`, `os.unlink`, or `os.rmdir` signal is present

### 7. Unknown file extension → CAUTION and cloud_sdk inference fix

**Pre-mortem verified:** No existing `inspect_test.go` tests create files with unknown extensions. The "unknown invoker" test at line 332 tests `DetectReferencedFile` (returns empty string), never reaching the extension switch. Safe to change default case.

In `internal/core/inspect.go`:
- **Line 121-125**: Change `default` case from `DecisionSafe` / `"no scanner for extension"` to `DecisionCaution` / `"unknown file type, no scanner available"`.

- **Modify `inferDecisionFromSignals`**: Remove `cloud_sdk` from the immediate-return APPROVAL list. Instead, track a `hasCloudSDK` flag. After the loop:
  - If `hasCloudSDK && (hasDestructive || hasSubprocess)` → APPROVAL
  - If `hasCloudSDK` alone → CAUTION (already the default)
  - Keep `subprocess`, `http_control_plane`, `dynamic_exec`, `dynamic_import` as immediate APPROVAL.

## Tests

**`internal/adapters/runner_test.go`** — add:
- `TestBuildChildEnv_StripsDYLDPrefix`: DYLD_FRAMEWORK_PATH stripped
- `TestBuildChildEnv_ResetsPathToTrusted`: PATH overwritten
- `TestBuildChildEnv_StripsLDPreload`: LD_PRELOAD stripped

**`internal/adapters/hook_test.go`** — add:
- `TestRunHook_EmptyCommandExitsTwo`: Empty command blocked

**`internal/adapters/mcpproxy_test.go`** — add:
- `TestProxyAgentToDownstream_SensitiveResourceReadBlocked`: Sensitive URI blocked
- `TestProxyAgentToDownstream_NonSensitiveResourceForwarded`: Safe URI forwarded

**`internal/core/normalize_test.go`** — add:
- `TestClassificationNormalize_BashCExtractionFailure`: Extraction failure detected, `ExtractionFailed == true`

**`internal/core/inspect_test.go`** — add:
- `TestInspectFile_UnknownExtensionReturnsCaution`: .lua → CAUTION
- `TestInferDecisionFromSignals_CloudSDKAloneIsCaution`: cloud_sdk only → CAUTION
- `TestInferDecisionFromSignals_CloudSDKPlusDestructiveIsApproval`: cloud_sdk + destructive_fs → APPROVAL

**`internal/db/db_test.go`** — update + add:
- **Update** 6 existing assertions from `***REDACTED***` to `[REDACTED]`
- `TestCleanupExpired_RetainsRecentlyConsumed`: 30min consumed kept
- `TestCleanupExpired_DeletesOldConsumed`: 2h consumed deleted

## Conformance Checks

| Issue | Check Type | Check |
|-------|-----------|-------|
| Issue 1 | content_check | `{file: "runner.go", pattern: "HasPrefix.*DYLD_"}` |
| Issue 1 | tests | `go test ./internal/adapters/ -run TestBuildChildEnv` |
| Issue 2 | content_check | `{file: "hook.go", pattern: "return 2.*nil"}` near empty command |
| Issue 2 | tests | `go test ./internal/adapters/ -run TestRunHook_EmptyCommand` |
| Issue 3 | content_check | `{file: "normalize.go", pattern: "ExtractionFailed"}` |
| Issue 3 | tests | `go test ./internal/core/ -run TestClassification.*BashC` |
| Issue 4 | tests | `go test ./internal/adapters/ -run TestProxy.*Resource` |
| Issue 5 | content_check | `{file: "schema.go", pattern: "idx_events_ts"}` |
| Issue 5 | content_check | `{file: "schema.go", pattern: "currentSchemaVersion.*3"}` |
| Issue 5 | content_check | `{file: "events.go", pattern: "\\[REDACTED\\]"}` |
| Issue 5 | tests | `go test ./internal/db/ -run TestCleanupExpired` |
| Issue 6 | tests | `go test ./internal/inspect/ -run TestScanPython` |
| Issue 7 | content_check | `{file: "inspect.go", pattern: "DecisionCaution.*unknown"}` |
| Issue 7 | tests | `go test ./internal/core/ -run TestInfer.*CloudSDK` |

## Verification

1. **Unit tests**: `go test ./internal/adapters/ ./internal/core/ ./internal/db/ ./internal/inspect/ -v -timeout 60s`
2. **Full suite**: `go test ./... -timeout 120s`
3. **Build check**: `go build ./...`
4. **Vet check**: `go vet ./...`

## Issues

### Issue 1: Fix DYLD_* prefix stripping and add BuildChildEnv tests
**Dependencies:** None
**Acceptance:** `strings.HasPrefix(name, "DYLD_")` in BuildChildEnv; 3 new tests pass
**Description:** Security fix — switch from explicit DYLD_ map entries to prefix-based stripping. Add missing unit tests for BuildChildEnv covering DYLD_, LD_PRELOAD, and PATH reset.
**Files:** `runner.go`, `runner_test.go`

### Issue 2: Block empty hook command (exit 2)
**Dependencies:** None
**Acceptance:** Empty `tool_input.command` returns exit 2; new test passes
**Description:** Change `hook.go:178` from `return 0` to `return 2` for empty command. Add test.
**Files:** `hook.go`, `hook_test.go`

### Issue 3: bash -c extraction failure → APPROVAL (Option A: struct flag)
**Dependencies:** None
**Acceptance:** Unbalanced `bash -c 'unterminated` classifies as APPROVAL; `ExtractionFailed` field exists on `ClassifiedCommand`; new test passes
**Description:** Add `ExtractionFailed bool` field to `ClassifiedCommand` struct in `normalize.go`. Set it when `extractBashCInner` returns `ok=false` for bash/sh -c tokens. Check the flag in `classify.go` after `ClassificationNormalize` returns — if set, escalate to APPROVAL (fail-closed per spec §5.2). The failure case currently leaves `result.Inner` empty while `result.Outer` starts with `bash`/`sh`.
**Files:** `normalize.go`, `classify.go`, `normalize_test.go`

### Issue 4: Add missing sensitive resource blocking tests
**Dependencies:** None
**Acceptance:** 2 new mcpproxy tests pass
**Description:** Add tests for `isSensitiveResourceRequest` — verify `resources/read` with `~/.fuse` URI is blocked and safe URI is forwarded. Name forwarding test `TestProxyAgentToDownstream_NonSensitiveResourceForwarded` (verify request frame written to downstream buffer only).
**Files:** `mcpproxy_test.go`

### Issue 5: SQLite housekeeping + credential scrubbing alignment
**Dependencies:** None
**Acceptance:** `currentSchemaVersion = "3"`; `idx_events_ts` exists in v3 migration; WAL checkpoint called after cleanup; consumed approvals retained for 1h; replacement text is `[REDACTED]`; password flag and Authorization patterns present; 6 existing test assertions updated; 2 new tests pass
**Description:** Create v3 schema migration (NOT modify v2 — existing v2 databases skip the v2 block). Add events timestamp index, WAL checkpoint setup, VACUUM helpers. Modify CleanupExpired to retain consumed approvals for 1 hour. Change credential scrubbing text from `***REDACTED***` to `[REDACTED]`, add password flag and Authorization header patterns. Update 6 existing `db_test.go` assertions to match new scrub text.
**Files:** `schema.go`, `db.go`, `approvals.go`, `events.go`, `db_test.go`

### Issue 6: Scope Python import signals (post-filter)
**Dependencies:** None
**Acceptance:** `import shutil` only signals if `rmtree` present; `import os` only signals if `os.system`/`os.remove`/`os.rmdir` present
**Description:** Add `scopeImportSignals` post-filter function. After scanning all lines, call it on the returned signals slice. It removes `destructive_fs` signal from `import shutil` if no `shutil.rmtree` signal exists, and removes `subprocess` signal from `import os` if no `os.system`/`os.remove`/`os.unlink`/`os.rmdir` signal exists.
**Files:** `python.go`

### Issue 7: Unknown extension → CAUTION and cloud_sdk inference fix
**Dependencies:** None
**Acceptance:** `.lua` file returns CAUTION; cloud_sdk alone returns CAUTION; 3 new tests pass
**Description:** Change `inspect.go` default extension case to CAUTION. Fix `inferDecisionFromSignals` to only escalate `cloud_sdk` to APPROVAL when combined with destructive signals. Verified: no existing `inspect_test.go` tests depend on default-SAFE behavior (the "unknown invoker" test at line 332 tests `DetectReferencedFile`, not the extension switch).
**Files:** `inspect.go`, `inspect_test.go`

## File-Conflict Matrix

| File | Issues |
|------|--------|
| `runner.go` | Issue 1 |
| `runner_test.go` | Issue 1 |
| `hook.go` | Issue 2 |
| `hook_test.go` | Issue 2 |
| `normalize.go` | Issue 3 |
| `classify.go` | Issue 3 |
| `normalize_test.go` | Issue 3 |
| `mcpproxy_test.go` | Issue 4 |
| `schema.go` | Issue 5 |
| `db.go` | Issue 5 |
| `approvals.go` | Issue 5 |
| `events.go` | Issue 5 |
| `db_test.go` | Issue 5 |
| `python.go` | Issue 6 |
| `inspect.go` | Issue 7 |
| `inspect_test.go` | Issue 7 |

No file conflicts between issues — all can run in parallel.

## Execution Order

**Wave 1** (all parallel — no shared files): Issues 1, 2, 3, 4, 5, 6, 7

All 7 issues touch independent files and can execute simultaneously.

## Post-Merge Cleanup

After merging all wave 1 results:
- Run `go test ./... -timeout 120s` as final gate
- Run `go vet ./...` for static analysis
- Search `grep -rn 'TODO\|FIXME' internal/` for any deferred markers
