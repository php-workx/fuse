package core

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
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
	mu            sync.Mutex
	entries       map[string]binaryEntry // resolved path → cached entry
	isInterpreter func(string) bool
}

// NewBinaryTOFU creates a new session-scoped TOFU verifier.
func NewBinaryTOFU() *BinaryTOFU {
	return NewBinaryTOFUWithInterpreter(IsInterpreter)
}

// NewBinaryTOFUWithInterpreter creates a TOFU verifier with a custom
// interpreter predicate, primarily for tests.
func NewBinaryTOFUWithInterpreter(isInterpreter func(string) bool) *BinaryTOFU {
	return &BinaryTOFU{
		entries:       make(map[string]binaryEntry),
		isInterpreter: isInterpreter,
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

// IsInterpreter returns true if the basename or path resolves to a tracked interpreter.
func IsInterpreter(command string) bool {
	return interpreterBasenames[filepath.Base(command)]
}

// Verify checks the binary identity for a resolved interpreter path.
// Returns (decision, reason). Empty decision means TOFU passed or not applicable.
// Uses stat-based caching: only re-hashes when mtime or size changes.
func (t *BinaryTOFU) Verify(resolvedPath string) (Decision, string) {
	isInterpreter := t.isInterpreter
	if isInterpreter == nil {
		isInterpreter = IsInterpreter
	}
	if !isInterpreter(resolvedPath) {
		return "", ""
	}
	basename := filepath.Base(resolvedPath)

	for {
		info, err := os.Stat(resolvedPath)
		if err != nil {
			return DecisionApproval, fmt.Sprintf("cannot stat binary %s: %v", basename, err)
		}

		t.mu.Lock()
		entry, exists := t.entries[resolvedPath]
		if exists && entry.mtime.Equal(info.ModTime()) && entry.size == info.Size() {
			t.mu.Unlock()
			return "", ""
		}
		t.mu.Unlock()

		currentHash, err := hashFile(resolvedPath)
		if err != nil {
			return DecisionApproval, fmt.Sprintf("cannot verify binary identity for %s: %v", basename, err)
		}

		latestInfo, err := os.Stat(resolvedPath)
		if err != nil {
			return DecisionApproval, fmt.Sprintf("cannot stat binary %s: %v", basename, err)
		}
		if !latestInfo.ModTime().Equal(info.ModTime()) || latestInfo.Size() != info.Size() {
			continue
		}

		t.mu.Lock()
		entry, exists = t.entries[resolvedPath]
		if exists && entry.mtime.Equal(latestInfo.ModTime()) && entry.size == latestInfo.Size() {
			t.mu.Unlock()
			return "", ""
		}
		if !exists {
			t.entries[resolvedPath] = binaryEntry{hash: currentHash, mtime: latestInfo.ModTime(), size: latestInfo.Size()}
			t.mu.Unlock()
			return "", ""
		}
		if entry.hash != currentHash {
			t.mu.Unlock()
			return DecisionBlocked, fmt.Sprintf(
				"binary identity changed for %s (%s): expected %s, got %s",
				basename, resolvedPath, hashPrefix(entry.hash), hashPrefix(currentHash))
		}

		t.entries[resolvedPath] = binaryEntry{hash: currentHash, mtime: latestInfo.ModTime(), size: latestInfo.Size()}
		t.mu.Unlock()
		return "", ""
	}
}

func hashPrefix(hash string) string {
	if len(hash) <= 12 {
		return hash
	}
	return hash[:12]
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
// Protected by defaultTOFUMu for safe Reset during tests.
var (
	defaultTOFU   = NewBinaryTOFU()
	defaultTOFUMu sync.Mutex
)

// VerifyBinaryIdentity checks the interpreter binary using the session-scoped TOFU.
func VerifyBinaryIdentity(resolvedPath string) (Decision, string) {
	defaultTOFUMu.Lock()
	t := defaultTOFU
	defaultTOFUMu.Unlock()
	return t.Verify(resolvedPath)
}

// ResetBinaryTOFU clears the session cache. Safe for concurrent use.
func ResetBinaryTOFU() {
	defaultTOFUMu.Lock()
	defaultTOFU = NewBinaryTOFU()
	defaultTOFUMu.Unlock()
}
