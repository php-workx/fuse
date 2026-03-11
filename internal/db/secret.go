package db

import (
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
)

// EnsureSecret reads the HMAC secret from path, creating it if it does
// not exist. The secret is 32 bytes of cryptographically random data.
// The file is created with mode 0o600.
func EnsureSecret(path string) ([]byte, error) {
	// Try to read existing secret.
	data, err := os.ReadFile(path)
	if err == nil && len(data) == 32 {
		return data, nil
	}

	// Ensure parent directory exists.
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create secret directory: %w", err)
	}

	// Generate new secret.
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return nil, fmt.Errorf("generate secret: %w", err)
	}

	// Write with restrictive permissions.
	if err := os.WriteFile(path, secret, 0o600); err != nil {
		return nil, fmt.Errorf("write secret: %w", err)
	}

	return secret, nil
}
