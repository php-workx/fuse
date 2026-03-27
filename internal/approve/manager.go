package approve

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/php-workx/fuse/internal/core"
	"github.com/php-workx/fuse/internal/db"
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

// ApprovalRequest contains the parameters for requesting approval.
type ApprovalRequest struct {
	DecisionKey    string // unique key for this command classification
	Command        string // the shell command or tool call
	Reason         string // why approval is needed
	SessionID      string // agent session identifier
	Source         string // adapter: "hook", "codex-shell", "run", "mcp-proxy"
	HookMode       bool   // true = short TTY prompt timeout
	NonInteractive bool   // true = skip TTY prompt entirely
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
//
// approvalResult carries the outcome of a TTY prompt or DB poll goroutine.
type approvalResult struct {
	decision core.Decision
	scope    string
	fromPoll bool
	err      error
}

func (m *Manager) RequestApproval(ctx context.Context, req ApprovalRequest) (core.Decision, error) {
	// Step 1: Check for an existing valid approval.
	existing, err := m.ConsumeApproval(req.DecisionKey, req.SessionID)
	if err != nil {
		return core.DecisionBlocked, fmt.Errorf("check existing approval: %w", err)
	}
	if existing != "" {
		return existing, nil
	}

	// Step 2: Write pending request for TUI visibility.
	pendingID := m.insertPendingRequest(req)
	deletePending := func() { _ = m.db.DeletePendingRequest(pendingID) }

	// Step 3: Run TTY prompt and DB poll in parallel.
	subCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	ch := make(chan approvalResult, 2)
	go m.runTTYPrompt(subCtx, req, ch)
	go m.runDBPoll(subCtx, req, ch)

	// Step 4: Wait for first result.
	r := <-ch

	if r.err != nil {
		return m.handlePromptError(ctx, cancel, ch, deletePending, r.err)
	}

	cancel()        // stop the poll goroutine
	deletePending() // resolved via TTY prompt

	return m.resolveApproval(req, r, ch)
}

// insertPendingRequest writes a pending request record for TUI visibility
// and returns the pending ID.
func (m *Manager) insertPendingRequest(req ApprovalRequest) string {
	pendingID := uuid.New().String()
	source := req.Source
	if source == "" {
		source = "hook"
	}
	if insertErr := m.db.InsertPendingRequest(db.PendingRequest{
		ID:          pendingID,
		DecisionKey: req.DecisionKey,
		Command:     db.ScrubCredentials(req.Command),
		Reason:      req.Reason,
		Source:      source,
		SessionID:   req.SessionID,
	}); insertErr != nil {
		slog.Warn("failed to insert pending request for TUI visibility", "error", insertErr)
	}
	return pendingID
}

// runTTYPrompt runs the interactive TTY approval prompt in a goroutine.
func (m *Manager) runTTYPrompt(ctx context.Context, req ApprovalRequest, ch chan<- approvalResult) {
	approved, scope, promptErr := PromptUser(ctx, req.Command, req.Reason, req.HookMode, req.NonInteractive)
	if promptErr != nil {
		ch <- approvalResult{err: promptErr}
		return
	}
	if !approved {
		ch <- approvalResult{decision: core.DecisionBlocked}
		return
	}
	ch <- approvalResult{decision: core.DecisionApproval, scope: scope}
}

// runDBPoll polls the database for externally-created approvals (from TUI).
func (m *Manager) runDBPoll(ctx context.Context, req ApprovalRequest, ch chan<- approvalResult) {
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			decision, pollErr := m.ConsumeApproval(req.DecisionKey, req.SessionID)
			if pollErr != nil {
				continue // transient DB error, retry
			}
			if decision != "" {
				ch <- approvalResult{decision: decision, fromPoll: true}
				return
			}
		}
	}
}

// handlePromptError handles the case where the TTY prompt failed (e.g.,
// non-interactive terminal). It waits for the DB poll to resolve or times out.
func (m *Manager) handlePromptError(
	ctx context.Context,
	cancel context.CancelFunc,
	ch <-chan approvalResult,
	deletePending func(),
	promptErr error,
) (core.Decision, error) {
	// The TUI (fuse monitor) can still resolve this — keep polling until
	// the context expires. If no deadline, fall back to 2 minutes.
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
	return core.DecisionBlocked, fmt.Errorf("prompt user: %w", promptErr)
}

// resolveApproval handles the final decision after a successful TTY prompt,
// checking for concurrent TUI approvals and storing new approvals.
func (m *Manager) resolveApproval(
	req ApprovalRequest,
	r approvalResult,
	ch <-chan approvalResult,
) (core.Decision, error) {
	// If TTY denied but the TUI approved concurrently, prefer the approval.
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
		if storeErr := m.CreateApproval(req.DecisionKey, string(core.DecisionApproval), r.scope, req.SessionID); storeErr != nil {
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
