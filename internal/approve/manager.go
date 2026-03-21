package approve

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/runger/fuse/internal/core"
	"github.com/runger/fuse/internal/db"
)

// Manager handles approval lifecycle: creating, consuming, and prompting.
type Manager struct {
	db     *db.DB
	secret []byte
}

// NewManager creates a new approval Manager.
// Secret must be exactly 32 bytes (the HMAC-SHA256 key size).
func NewManager(database *db.DB, secret []byte) (*Manager, error) {
	if len(secret) != 32 {
		return nil, fmt.Errorf("secret must be exactly 32 bytes, got %d", len(secret))
	}
	return &Manager{
		db:     database,
		secret: secret,
	}, nil
}

// scopeExpiry returns the expiration time for a given scope, or nil for
// scopes that do not expire.
func scopeExpiry(scope string) *time.Time {
	switch scope {
	case "command", "session":
		// Command- and session-scoped approvals last 24 hours.
		t := time.Now().Add(24 * time.Hour)
		return &t
	case "forever":
		return nil
	default:
		// "once" and unknown scopes expire after 1 hour as a safety net.
		t := time.Now().Add(1 * time.Hour)
		return &t
	}
}

// RequestApproval checks for an existing valid approval, or resolves via
// parallel TTY prompt + database polling. The TUI (fuse monitor) can create
// approval records that the DB poll picks up, enabling centralized approval.
//
// Flow:
//  1. Check for cached approval (ConsumeApproval)
//  2. Write a pending request so the TUI can see it
//  3. Run TTY prompt and DB poll in parallel — first to resolve wins
//  4. Clean up pending request on exit
func (m *Manager) RequestApproval(
	ctx context.Context,
	decisionKey, command, reason, sessionID, source string,
	hookMode, nonInteractive bool,
) (core.Decision, error) {
	// Step 1: Check for an existing valid approval.
	existing, err := m.ConsumeApproval(decisionKey, sessionID)
	if err != nil {
		return core.DecisionBlocked, fmt.Errorf("check existing approval: %w", err)
	}
	if existing != "" {
		return existing, nil
	}

	// Step 2: Write pending request for TUI visibility.
	pendingID := uuid.New().String()
	if source == "" {
		source = "hook"
	}
	if insertErr := m.db.InsertPendingRequest(db.PendingRequest{
		ID:          pendingID,
		DecisionKey: decisionKey,
		Command:     db.ScrubCredentials(command),
		Reason:      reason,
		Source:      source,
		SessionID:   sessionID,
	}); insertErr != nil {
		slog.Warn("failed to insert pending request for TUI visibility", "error", insertErr)
	}
	// Don't delete the pending request on timeout — let it persist so the TUI
	// can show it even after the hook exits. The user may approve minutes later;
	// the agent's retry will find the cached approval via ConsumeApproval.
	// Only delete on successful resolution (approved or denied).
	deletePending := func() { _ = m.db.DeletePendingRequest(pendingID) }

	// Step 3: Run TTY prompt and DB poll in parallel.
	subCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	type result struct {
		decision core.Decision
		scope    string
		fromPoll bool
		err      error
	}
	ch := make(chan result, 2)

	// Goroutine A: TTY prompt (skipped if non-interactive).
	go func() {
		approved, scope, promptErr := PromptUser(subCtx, command, reason, hookMode, nonInteractive)
		if promptErr != nil {
			ch <- result{err: promptErr}
			return
		}
		if !approved {
			ch <- result{decision: core.DecisionBlocked}
			return
		}
		ch <- result{decision: core.DecisionApproval, scope: scope}
	}()

	// Goroutine B: Poll database for externally-created approval (from TUI).
	go func() {
		ticker := time.NewTicker(200 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-subCtx.Done():
				return
			case <-ticker.C:
				decision, pollErr := m.ConsumeApproval(decisionKey, sessionID)
				if pollErr != nil {
					continue // transient DB error, retry
				}
				if decision != "" {
					ch <- result{decision: decision, fromPoll: true}
					return
				}
			}
		}
	}()

	// Step 4: Wait for first result.
	r := <-ch

	if r.err != nil {
		// Prompt failed (e.g., non-interactive terminal). The TUI (fuse monitor)
		// can still resolve this — keep polling until the context expires.
		// The hook caller provides a deadline (hookTimeout - 2s). If no
		// deadline (e.g., run mode), fall back to 2 minutes.
		if _, hasDeadline := ctx.Deadline(); !hasDeadline {
			var deadlineCancel context.CancelFunc
			ctx, deadlineCancel = context.WithTimeout(ctx, 2*time.Minute)
			defer deadlineCancel()
		}
		select {
		case r2 := <-ch:
			cancel()
			if r2.err == nil && r2.decision != "" {
				deletePending() // resolved via TUI poll
				return r2.decision, nil
			}
		case <-ctx.Done():
			cancel()
		}
		// Timeout — DON'T delete pending request. It persists so the TUI
		// can still show it and the user can pre-approve for the next retry.
		return core.DecisionBlocked, fmt.Errorf("prompt user: %w", r.err)
	}

	cancel()        // stop the poll goroutine
	deletePending() // resolved via TTY prompt

	// If TTY denied but the TUI approved concurrently, prefer the approval.
	// The channel is buffered (size 2), so if goroutine B already sent an
	// APPROVAL result (after consuming the approval from DB), it's sitting
	// in the buffer. Draining it prevents losing consumed approvals.
	if r.decision == core.DecisionBlocked && !r.fromPoll {
		select {
		case r2 := <-ch:
			if r2.decision == core.DecisionApproval {
				return r2.decision, nil
			}
		default:
			// no second result — TTY denial stands
		}
	}

	// If approved via TTY prompt, store the approval.
	if !r.fromPoll && r.decision == core.DecisionApproval {
		if storeErr := m.CreateApproval(decisionKey, string(core.DecisionApproval), r.scope, sessionID); storeErr != nil {
			return core.DecisionBlocked, fmt.Errorf("store approval: %w", storeErr)
		}
	}

	return r.decision, nil
}

// CreateApproval stores a new signed approval record.
func (m *Manager) CreateApproval(decisionKey, decision, scope, sessionID string) error {
	id := uuid.New().String()
	mac := SignApproval(m.secret, id, decisionKey)
	expiresAt := scopeExpiry(scope)

	return m.db.CreateApproval(id, decisionKey, decision, scope, sessionID, mac, expiresAt)
}

// ConsumeApproval checks for and consumes an existing approval.
// Returns the approval decision if valid, or empty Decision if none found.
func (m *Manager) ConsumeApproval(decisionKey, sessionID string) (core.Decision, error) {
	approval, err := m.db.ConsumeApproval(decisionKey, sessionID)
	if err != nil {
		return "", fmt.Errorf("consume approval: %w", err)
	}
	if approval == nil {
		return "", nil
	}

	// Verify HMAC integrity before trusting the record.
	if !VerifyApproval(m.secret, approval.ID, approval.DecisionKey, approval.HMAC) {
		return "", fmt.Errorf("approval HMAC verification failed (possible tampering)")
	}

	switch core.Decision(approval.Decision) {
	case core.DecisionApproval:
		return core.DecisionApproval, nil
	case core.DecisionBlocked:
		return core.DecisionBlocked, nil
	case core.DecisionSafe, core.DecisionCaution:
		return core.Decision(approval.Decision), nil
	default:
		return "", fmt.Errorf("unknown approval decision: %s", approval.Decision)
	}
}
