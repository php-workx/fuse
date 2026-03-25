package core

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"
)

// binaryEntry caches hash and stat metadata for a verified binary.
type binaryEntry struct {
	hash  string
	mtime time.Time
	size  int64
}

// BinaryTOFU implements Trust-on-First-Use verification for interpreter binaries.
// On first invocation, the resolved binary path is hashed and cached. On subsequent
// invocations, the hash is verified. If the hash changes mid-session, the command
// is BLOCKED (binary substitution attack).
type BinaryTOFU struct {
	mu      sync.Mutex
	entries map[string]binaryEntry // resolved path → cached entry
}

// NewBinaryTOFU creates a new session-scoped TOFU verifier.
func NewBinaryTOFU() *BinaryTOFU {
	return &BinaryTOFU{
		entries: make(map[string]binaryEntry),
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
// Uses stat-based caching: only re-hashes when mtime or size changes.
func (t *BinaryTOFU) Verify(basename string) (Decision, string) {
	if !IsInterpreter(basename) {
		return "", ""
	}

	resolvedPath, err := exec.LookPath(basename)
	if err != nil {
		return "", "" // can't resolve — not a TOFU concern
	}

	info, err := os.Stat(resolvedPath)
	if err != nil {
		return DecisionApproval, fmt.Sprintf("cannot stat binary %s: %v", basename, err)
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	entry, exists := t.entries[resolvedPath]
	if exists && entry.mtime.Equal(info.ModTime()) && entry.size == info.Size() {
		return "", "" // stat unchanged — skip re-hash
	}

	currentHash, err := hashFile(resolvedPath)
	if err != nil {
		return DecisionApproval, fmt.Sprintf("cannot verify binary identity for %s: %v", basename, err)
	}

	if !exists {
		// First use — trust and store.
		t.entries[resolvedPath] = binaryEntry{hash: currentHash, mtime: info.ModTime(), size: info.Size()}
		return "", ""
	}

	if entry.hash != currentHash {
		return DecisionBlocked, fmt.Sprintf(
			"binary identity changed for %s (%s): expected %s, got %s",
			basename, resolvedPath, entry.hash[:12], currentHash[:12])
	}

	// Hash unchanged but stat changed (e.g., touch). Update stat cache.
	t.entries[resolvedPath] = binaryEntry{hash: currentHash, mtime: info.ModTime(), size: info.Size()}
	return "", ""
}

// hashFile returns the hex-encoded SHA-256 hash of a file using streaming I/O.
// Avoids reading the entire file into memory (prevents OOM on large binaries).
func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
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
