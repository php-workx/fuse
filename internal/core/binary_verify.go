package core

import (
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"sync"
)

// BinaryTOFU implements Trust-on-First-Use verification for interpreter binaries.
// On first invocation, the resolved binary path is hashed and cached. On subsequent
// invocations, the hash is verified. If the hash changes mid-session, the command
// is BLOCKED (binary substitution attack).
type BinaryTOFU struct {
	mu     sync.RWMutex
	hashes map[string]string // resolved path → SHA-256 hash
}

// NewBinaryTOFU creates a new session-scoped TOFU verifier.
func NewBinaryTOFU() *BinaryTOFU {
	return &BinaryTOFU{
		hashes: make(map[string]string),
	}
}

// interpreterBasenames are commands that execute arbitrary code and warrant
// binary identity verification.
var interpreterBasenames = map[string]bool{
	"python": true, "python3": true, "python2": true,
	"node": true, "nodejs": true,
	"bash": true, "sh": true, "zsh": true,
	"ruby": true, "perl": true,
}

// IsInterpreter returns true if the basename is a tracked interpreter.
func IsInterpreter(basename string) bool {
	return interpreterBasenames[basename]
}

// Verify checks the binary identity for an interpreter command.
// Returns (decision, reason). Empty decision means TOFU passed or not applicable.
func (t *BinaryTOFU) Verify(basename string) (Decision, string) {
	if !IsInterpreter(basename) {
		return "", ""
	}

	resolvedPath, err := exec.LookPath(basename)
	if err != nil {
		// Can't resolve — not a TOFU concern, let classification proceed.
		return "", ""
	}

	currentHash, err := hashFile(resolvedPath)
	if err != nil {
		// Can't hash (permission denied, etc.) — fail-closed.
		return DecisionApproval, fmt.Sprintf("cannot verify binary identity for %s: %v", basename, err)
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	storedHash, exists := t.hashes[resolvedPath]
	if !exists {
		// First use — trust and store.
		t.hashes[resolvedPath] = currentHash
		return "", ""
	}

	if storedHash != currentHash {
		return DecisionBlocked, fmt.Sprintf(
			"binary identity changed for %s (%s): expected %s, got %s",
			basename, resolvedPath, storedHash[:12], currentHash[:12])
	}

	return "", ""
}

// hashFile returns the hex-encoded SHA-256 hash of a file.
func hashFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	h := sha256.Sum256(data)
	return fmt.Sprintf("%x", h), nil
}

// defaultTOFU is the session-scoped singleton used by the classification pipeline.
var defaultTOFU = NewBinaryTOFU()

// VerifyBinaryIdentity checks the interpreter binary using the session-scoped TOFU.
func VerifyBinaryIdentity(basename string) (Decision, string) {
	return defaultTOFU.Verify(basename)
}

// ResetBinaryTOFU clears the session cache. Used in tests.
func ResetBinaryTOFU() {
	defaultTOFU = NewBinaryTOFU()
}
