package approve

import (
	"fmt"
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
	case "once":
		// "once" approvals expire after 1 hour as a safety net,
		// but are typically consumed immediately.
		t := time.Now().Add(1 * time.Hour)
		return &t
	case "command":
		// Command-scoped approvals last 24 hours.
		t := time.Now().Add(24 * time.Hour)
		return &t
	case "session":
		// Session-scoped approvals last 24 hours.
		t := time.Now().Add(24 * time.Hour)
		return &t
	case "forever":
		return nil
	default:
		t := time.Now().Add(1 * time.Hour)
		return &t
	}
}

// RequestApproval checks for an existing valid approval or prompts the user.
// Returns the decision and any error.
func (m *Manager) RequestApproval(decisionKey, command, reason, sessionID string, hookMode bool) (core.Decision, error) {
	// First, check for an existing valid approval.
	existing, err := m.ConsumeApproval(decisionKey, sessionID)
	if err != nil {
		return core.DecisionBlocked, fmt.Errorf("check existing approval: %w", err)
	}
	if existing != "" {
		return existing, nil
	}

	// No existing approval — prompt the user.
	approved, scope, err := PromptUser(command, reason, hookMode)
	if err != nil {
		return core.DecisionBlocked, fmt.Errorf("prompt user: %w", err)
	}

	if !approved {
		return core.DecisionBlocked, nil
	}

	// Store the approval using the canonical Decision constant.
	if err := m.CreateApproval(decisionKey, string(core.DecisionApproval), scope, sessionID); err != nil {
		return core.DecisionBlocked, fmt.Errorf("store approval: %w", err)
	}

	return core.DecisionApproval, nil
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
