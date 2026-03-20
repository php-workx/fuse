---
id: plan-2026-03-20-concurrent-codex-shell
type: plan
date: 2026-03-20
source: "[[docs/plans/2026-03-20-concurrent-codex-shell-plan.md]]"
---

# Plan: Concurrent MCP Request Handling for Codex Shell Server

## Context

The Codex shell MCP server (`fuse proxy codex-shell`) processes requests synchronously in a single for-loop. When a command requires user approval, `PromptUser` blocks for up to 5 minutes on `/dev/tty`. During this time the server cannot read stdin, the Codex client's 120s timeout fires, and the protocol state becomes permanently desynchronized. Both processes remain alive but can no longer communicate.

Confirmed by live debugging: fuse blocked on `read()`, codex all threads parked, stdout pipe at 65536 bytes (max buffer), zero events logged after the APPROVAL command.

Applied findings: none (no compiled planning rules or findings registry).

## Files to Modify

| File | Change |
|------|--------|
| `internal/adapters/codexshell.go` (309 LOC) | Rewrite `RunCodexShellServer` for concurrent dispatch; add `codexShellWriter`, `processRequest`, `waitGroupWithTimeout`; thread `context.Context` through handlers |
| `internal/approve/prompt.go` (232 LOC) | Add `ttyMu sync.Mutex`; add `ctx context.Context` param to `PromptUser`; check `ctx.Err()` in polling loop |
| `internal/approve/manager.go` (113 LOC) | Add `ctx context.Context` param to `RequestApproval`; thread to `PromptUser` |
| `internal/adapters/hook.go` | Update `RequestApproval` call to pass `context.Background()` (1 line) |
| `internal/adapters/runner.go` | Add `ctx context.Context` param to `executeCapturedShellCommand` and `executeCapturedShellCommandWithStdin`; update `RequestApproval` call (3 changes) |
| `internal/adapters/mcpproxy.go` | Update `RequestApproval` call to pass `context.Background()` (1 line) |
| `internal/adapters/codexshell_test.go` (617 LOC) | Update existing 15 tests for new signatures; add 6 new concurrency tests |

## Boundaries

**Always:**
- Reader goroutine must NEVER block on processing
- Stdout writes must be fully serialized (no frame interleaving)
- TTY prompts must be serialized (one prompt at a time)
- Existing 30-min command timeout and 5-min prompt timeout preserved
- All 15 existing codexshell tests pass
- Race detector clean (`-race`)

**Ask First:**
- Whether to add structured logging (slog.Debug) for MCP request lifecycle

**Never:**
- Per-request timeout that kills long-running commands (breaks modal deploy, etc.)
- Semaphore that blocks the reader goroutine
- Changes to `mcpio.go` (stateless, goroutine-safe)
- Changes to `db/db.go` (open-per-request pattern is correct)

## Baseline Audit

| Metric | Command | Result |
|--------|---------|--------|
| codexshell.go LOC | `wc -l internal/adapters/codexshell.go` | 309 |
| RunCodexShellServer LOC | `sed -n` range count | 45 |
| PromptUser callers | `grep -rn 'PromptUser(' --include='*.go'` | 1 (manager.go:61) |
| RequestApproval callers | `grep -rn 'RequestApproval(' --include='*.go'` | 4 (runner.go:187, codexshell.go:278, hook.go:270, mcpproxy.go:293) |
| Existing codexshell tests | `grep -c 'func Test' codexshell_test.go` | 15 |
| Existing approve tests | `grep -c 'func Test' approve/*_test.go` | 9 (4 hmac + 5 manager) |

## Implementation

### 1. Mutex-Protected Writer (`codexshell.go`)

Add `codexShellWriter` type:

```go
type codexShellWriter struct {
    mu        sync.Mutex
    writer    *bufio.Writer
    transport codexShellTransport
}

func (w *codexShellWriter) writeResponse(msg jsonRPCMessage) error {
    w.mu.Lock()
    defer w.mu.Unlock()
    data, err := encodeJSONRPC(msg)
    if err != nil {
        return err
    }
    if err := writeCodexShellPayload(w.writer, data, w.transport); err != nil {
        return err
    }
    return nil
}
```

**Key:** encode + frame + flush all under one mutex hold. Reuses existing `encodeJSONRPC` and `writeCodexShellPayload`.

### 2. TTY Mutex + Context in PromptUser (`approve/prompt.go`)

Add package-level mutex at `prompt.go:14`:
```go
var ttyMu sync.Mutex
```

Modify `PromptUser` signature at `prompt.go:31`:
- From: `func PromptUser(command, reason string, hookMode, nonInteractive bool) (bool, string, error)`
- To: `func PromptUser(ctx context.Context, command, reason string, hookMode, nonInteractive bool) (bool, string, error)`

Add at start of function (before `openTTY`):
```go
if nonInteractive || os.Getenv("FUSE_NON_INTERACTIVE") != "" {
    return false, "", errNonInteractive
}
ttyMu.Lock()
defer ttyMu.Unlock()
```

Add in polling loop at `prompt.go:90`, alongside the existing deadline check:
```go
if ctx.Err() != nil {
    fmt.Fprintf(tty, "\n  Denied (shutdown).\n\n")
    return false, "", nil
}
```

**Modify `readScope` signature** at `prompt.go:145`:
- From: `func readScope(tty *os.File, fd int, deadline time.Time, sigCh <-chan os.Signal) (string, bool)`
- To: `func readScope(ctx context.Context, tty *os.File, fd int, deadline time.Time, sigCh <-chan os.Signal) (string, bool)`

Add `ctx.Err()` check in `readScope` polling loop at `prompt.go:147`, alongside the existing deadline check.

Update the call site in `PromptUser` at `prompt.go:126`:
- From: `scopeResult, denied := readScope(tty, fd, deadline, sigCh)`
- To: `scopeResult, denied := readScope(ctx, tty, fd, deadline, sigCh)`

### 3. Context in RequestApproval (`approve/manager.go`)

Modify `RequestApproval` signature at `manager.go:50`:
- From: `func (m *Manager) RequestApproval(decisionKey, command, reason, sessionID string, hookMode, nonInteractive bool) (core.Decision, error)`
- To: `func (m *Manager) RequestApproval(ctx context.Context, decisionKey, command, reason, sessionID string, hookMode, nonInteractive bool) (core.Decision, error)`

Thread `ctx` to `PromptUser` call at `manager.go:61`:
- From: `approved, scope, err := PromptUser(command, reason, hookMode, nonInteractive)`
- To: `approved, scope, err := PromptUser(ctx, command, reason, hookMode, nonInteractive)`

### 4. Update All RequestApproval Callers (4 sites)

| File | Line | Change |
|------|------|--------|
| `codexshell.go` | 278 | `mgr.RequestApproval(ctx, ...)` — uses shutdown context |
| `hook.go` | 270 | `mgr.RequestApproval(context.Background(), ...)` |
| `runner.go` | 187 | `mgr.RequestApproval(context.Background(), ...)` |
| `mcpproxy.go` | 293 | `mgr.RequestApproval(context.Background(), ...)` |

### 5. Context Through Runner Execution Functions (`runner.go`)

The shutdown context must reach `exec.CommandContext` so child processes are killed on shutdown. Currently `executeCapturedShellCommandWithStdin` (runner.go:332) creates its own `context.Background()`, ignoring any parent context.

Modify signatures:
- From: `func executeCapturedShellCommand(command, cwd string, timeout time.Duration) (commandExecution, error)`
- To: `func executeCapturedShellCommand(ctx context.Context, command, cwd string, timeout time.Duration) (commandExecution, error)`

- From: `func executeCapturedShellCommandWithStdin(command, cwd string, timeout time.Duration, stdin io.Reader) (commandExecution, error)`
- To: `func executeCapturedShellCommandWithStdin(ctx context.Context, command, cwd string, timeout time.Duration, stdin io.Reader) (commandExecution, error)`

In `executeCapturedShellCommandWithStdin` at runner.go:333:
- From: `ctx := context.Background()`
- To: use the passed `ctx` as parent: `ctx, cancel := context.WithTimeout(ctx, timeout)` (replacing the existing `context.Background()` + `context.WithTimeout` block)

Update the single internal caller at runner.go:329:
- From: `return executeCapturedShellCommandWithStdin(command, cwd, timeout, nil)`
- To: `return executeCapturedShellCommandWithStdin(ctx, command, cwd, timeout, nil)`

Also update the caller in `executeCodexShellCommand` at codexshell.go:301:
- From: `execResult, err := executeCapturedShellCommand(command, cwd, timeout)`
- To: `execResult, err := executeCapturedShellCommand(ctx, command, cwd, timeout)`

### 6. Context Through Codex Shell Handlers (`codexshell.go`)

Modify signatures:
- `handleCodexShellMessage(ctx context.Context, msg jsonRPCMessage, sessionID string)`
- `handleCodexShellToolCall(ctx context.Context, msg jsonRPCMessage, sessionID string)`
- `executeCodexShellCommand(ctx context.Context, command, cwd, sessionID string, timeout time.Duration)`

In `executeCodexShellCommand`:
- Check `ctx.Err()` before classify, approve, and execute steps
- Pass `ctx` to `executeCapturedShellCommand` (which now accepts it per section 5)

**Key reuse points:**
- `encodeJSONRPC()` at `mcpio.go:74` — stateless, goroutine-safe
- `writeCodexShellPayload()` at `codexshell.go:102` — stateless, goroutine-safe
- `jsonRPCErrorResponse()` at `mcpio.go:82` — stateless, goroutine-safe
- `executeCapturedShellCommand()` at `runner.go:328` — goroutine-safe (own cmd, buffers, context)

### 7. WaitGroup Helper (`codexshell.go`)

```go
func waitGroupWithTimeout(wg *sync.WaitGroup, timeout time.Duration) {
    done := make(chan struct{})
    go func() { wg.Wait(); close(done) }()
    select {
    case <-done:
    case <-time.After(timeout):
    }
}
```

### 8. Rewrite RunCodexShellServer (`codexshell.go`)

Replace the synchronous for-loop (lines 44-73) with concurrent dispatch:

```go
func RunCodexShellServer(stdin io.Reader, stdout io.Writer) error {
    reader := bufio.NewReader(stdin)
    sessionID := generateSessionID()
    transport, err := detectCodexShellTransport(reader)
    if err != nil {
        if errors.Is(err, io.EOF) {
            return nil
        }
        return err
    }

    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()
    writer := &codexShellWriter{writer: bufio.NewWriter(stdout), transport: transport}
    var wg sync.WaitGroup

    for {
        payload, err := readCodexShellPayload(reader, transport)
        if err != nil {
            if errors.Is(err, io.EOF) {
                break
            }
            break
        }

        msg, err := decodeJSONRPC(payload)
        if err != nil {
            _ = writer.writeResponse(jsonRPCErrorResponse(nil, -32700, err.Error()))
            continue
        }

        wg.Add(1)
        go func(m jsonRPCMessage) {
            defer wg.Done()
            processRequest(ctx, m, sessionID, writer)
        }(msg)
    }

    cancel()
    waitGroupWithTimeout(&wg, 5*time.Second)
    return nil
}
```

### 9. processRequest (`codexshell.go`)

```go
func processRequest(ctx context.Context, msg jsonRPCMessage, sessionID string, writer *codexShellWriter) {
    defer func() {
        if r := recover(); r != nil {
            if id, ok := msg["id"]; ok {
                _ = writer.writeResponse(jsonRPCErrorResponse(id, -32603, fmt.Sprintf("internal error: %v", r)))
            }
        }
    }()

    response, respond, err := handleCodexShellMessage(ctx, msg, sessionID)
    if err != nil {
        if id, ok := msg["id"]; ok {
            _ = writer.writeResponse(jsonRPCErrorResponse(id, -32603, err.Error()))
        }
        return
    }
    if !respond || response == nil {
        return
    }
    if ctx.Err() != nil {
        return // shutting down, skip write
    }
    _ = writer.writeResponse(response)
}
```

## Tests

**`internal/adapters/codexshell_test.go`** — update existing 15 tests for new function signatures, then add:
- `TestRunCodexShellServer_ConcurrentToolCalls`: 3 tool calls in one buffer, all 3 responses arrive with correct IDs
- `TestRunCodexShellServer_SlowRequestDoesNotBlockFast`: `sleep 3` + `printf fast` via `io.Pipe`, fast response arrives first (wall-clock)
- `TestRunCodexShellServer_ResponseIDsMatchRequests`: init + list + tool call, each response carries correct `id`
- `TestRunCodexShellServer_ParseErrorContinuesProcessing`: malformed JSON then valid init, both get responses
- `TestRunCodexShellServer_GracefulShutdownOnEOF`: one request, close stdin, verify response + nil return
- `TestCodexShellWriter_ConcurrentWrites`: N goroutines write simultaneously, verify no frame corruption
- `TestRunCodexShellServer_ShutdownKillsInFlight`: send `sleep 30` command, close stdin immediately, verify function returns within ~6s (5s drain + margin) not 30s — proves shutdown context cancels child processes

**`internal/approve/manager_test.go`** — update existing 5 tests to pass `context.Background()` to `RequestApproval`.

| Test | Level | Rationale |
|------|-------|-----------|
| `TestRunCodexShellServer_ConcurrentToolCalls` | L2 (Integration) | Multiple requests through full MCP pipeline |
| `TestRunCodexShellServer_SlowRequestDoesNotBlockFast` | L2 (Integration) | Timing-dependent concurrent behavior |
| `TestRunCodexShellServer_ResponseIDsMatchRequests` | L1 (Unit) | ID matching correctness |
| `TestRunCodexShellServer_ParseErrorContinuesProcessing` | L1 (Unit) | Error resilience |
| `TestRunCodexShellServer_GracefulShutdownOnEOF` | L1 (Unit) | Lifecycle correctness |
| `TestCodexShellWriter_ConcurrentWrites` | L1 (Unit) | Mutex correctness |
| `TestRunCodexShellServer_ShutdownKillsInFlight` | L2 (Integration) | Shutdown context kills child processes |

## Conformance Checks

| Issue | Check Type | Check |
|-------|-----------|-------|
| Issue 1 | content_check | `{file: "internal/adapters/codexshell.go", pattern: "type codexShellWriter struct"}` |
| Issue 2 | content_check | `{file: "internal/approve/prompt.go", pattern: "ttyMu.Lock"}` |
| Issue 2 | content_check | `{file: "internal/approve/prompt.go", pattern: "ctx context.Context"}` |
| Issue 3 | content_check | `{file: "internal/approve/manager.go", pattern: "ctx context.Context"}` |
| Issue 4 | content_check | `{file: "internal/adapters/codexshell.go", pattern: "go func"}` |
| Issue 4 | content_check | `{file: "internal/adapters/codexshell.go", pattern: "waitGroupWithTimeout"}` |
| Issue 5 | tests | `go test -race ./internal/adapters/ -run TestCodexShell -v` |
| Issue 5 | tests | `go test -race ./internal/approve/ -v` |

## Verification

1. `go test -race ./internal/adapters/ -run TestCodexShell -v` — all 22 tests pass
2. `go test -race ./internal/approve/ -v` — all 9 tests pass
3. `go test -race ./...` — full suite green
4. Manual: `fuse proxy codex-shell` with `sleep 3` then `echo ok` — fast response arrives without waiting
5. Manual: APPROVAL prompt active + second SAFE request — second completes while prompt waits
6. `just check-local` — full quality gate

## File-Conflict Matrix

| File | Issues |
|------|--------|
| `internal/adapters/codexshell.go` | Issue 1, Issue 3, Issue 4 | CONFLICT: serialize (Issue 1 → Issue 3 → Issue 4) |
| `internal/approve/prompt.go` | Issue 2 only |
| `internal/approve/manager.go` | Issue 3 only |
| `internal/adapters/hook.go` | Issue 3 only |
| `internal/adapters/runner.go` | Issue 3 only |
| `internal/adapters/mcpproxy.go` | Issue 3 only |
| `internal/adapters/codexshell_test.go` | Issue 3, Issue 5 | CONFLICT: serialize (Issue 3 → Issue 5) |

## Cross-Wave Shared Files

| File | Wave 1 Issues | Wave 2+ Issues | Mitigation |
|------|---------------|----------------|------------|
| `internal/adapters/codexshell.go` | Issue 1 (W1) | Issue 3 (W2), Issue 4 (W3) | Serial: dependency chain 1 → 3 → 4 |
| `internal/adapters/codexshell_test.go` | — | Issue 3 (W2), Issue 5 (W4) | Serial: dependency chain 3 → 4 → 5 |

## Issues

### Issue 1: Add codexShellWriter and waitGroupWithTimeout
**Dependencies:** None
**Wave:** 1
**Files:** `internal/adapters/codexshell.go`
**Acceptance:** `codexShellWriter` type exists with `writeResponse` method; `waitGroupWithTimeout` function exists; existing tests still pass (no behavior change yet)
**Description:** Pure additions to codexshell.go. Add the `codexShellWriter` type (mutex + bufio.Writer + transport) with `writeResponse` method, and the `waitGroupWithTimeout` helper. Do NOT rewrite RunCodexShellServer yet — that's Issue 4.

### Issue 2: Add TTY mutex and context to PromptUser
**Dependencies:** None
**Wave:** 1
**Files:** `internal/approve/prompt.go`
**Acceptance:** `ttyMu` mutex exists; `PromptUser` accepts `ctx context.Context`; `ctx.Err()` checked in polling loop; non-interactive fast path before mutex acquire
**Description:** Add `ttyMu sync.Mutex` at package level. Add `ctx context.Context` as first param to `PromptUser`. Check non-interactive before locking. Check `ctx.Err()` in both the main polling loop and `readScope`. See Implementation section 2 for exact code locations.

### Issue 3: Thread context through RequestApproval, runner, and all callers
**Dependencies:** Issue 2
**Wave:** 2
**Files:** `internal/approve/manager.go`, `internal/adapters/hook.go`, `internal/adapters/runner.go`, `internal/adapters/mcpproxy.go`, `internal/adapters/codexshell.go`, `internal/adapters/codexshell_test.go`
**Acceptance:** `RequestApproval` accepts `ctx context.Context`; all 4 RequestApproval call sites updated; `executeCapturedShellCommand(WithStdin)` accept `ctx context.Context`; `handleCodexShellMessage`, `handleCodexShellToolCall`, `executeCodexShellCommand` accept context; existing tests updated with `context.Background()` and all pass
**Description:** Thread `context.Context` through the entire call chain:
1. `RequestApproval` signature + all 4 callers (hook.go, runner.go, mcpproxy.go pass `context.Background()`; codexshell.go passes `context.Background()` temporarily). See Implementation sections 3-4.
2. `executeCapturedShellCommand` and `executeCapturedShellCommandWithStdin` in runner.go — add `ctx` param, use as parent context instead of `context.Background()`. See Implementation section 5.
3. Codexshell handler chain: `handleCodexShellMessage`, `handleCodexShellToolCall`, `executeCodexShellCommand` — add `ctx` param, check `ctx.Err()` before major steps. See Implementation section 6.
4. Update existing tests in `codexshell_test.go` that call `executeCodexShellCommand` directly — add `context.Background()` as first arg. This is required for compilation after the signature change.

### Issue 4: Rewrite RunCodexShellServer with concurrent dispatch
**Dependencies:** Issue 1, Issue 3
**Wave:** 3
**Files:** `internal/adapters/codexshell.go`
**Acceptance:** Reader loop spawns goroutines per request; `codexShellWriter` used for all writes; shutdown context cancels on EOF; `waitGroupWithTimeout` used for drain; existing tests pass
**Description:** Replace the synchronous for-loop with: shutdown context + concurrent goroutine dispatch + `processRequest` function + bounded drain. Wire the real context (from `context.WithCancel`) instead of `context.Background()`. See Implementation sections 7-8 for exact code.

### Issue 5: Add concurrency tests
**Dependencies:** Issue 4
**Wave:** 4
**Files:** `internal/adapters/codexshell_test.go`, `internal/approve/manager_test.go`
**Acceptance:** 7 new tests pass; all 15 existing codexshell tests pass; all 5 manager tests pass; `go test -race` clean
**Description:** Add 7 new tests covering concurrent tool calls, slow-doesn't-block-fast, ID matching, parse error resilience, graceful shutdown, writer concurrency, and shutdown-kills-in-flight. Update manager_test.go to pass `context.Background()`. See Tests section for exact test names and descriptions.

## Execution Order

**Wave 1** (parallel): Issue 1, Issue 2
**Wave 2** (after Wave 1): Issue 3
**Wave 3** (after Wave 2): Issue 4
**Wave 4** (after Wave 3): Issue 5

## Post-Merge Cleanup

After implementation:
- `grep -rn 'TODO\|FIXME' internal/adapters/codexshell.go internal/approve/prompt.go internal/approve/manager.go`
- Verify no `context.Background()` remains in codexshell.go handlers (should all use the shutdown context)

## Next Steps
- Run `/pre-mortem` to validate plan
- Run `/crank` for autonomous execution
- Or `/implement <issue>` for single issue
