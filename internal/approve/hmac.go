// Package approve implements the approval manager and TUI prompt for fuse.
package approve

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
)

// SignApproval computes an HMAC-SHA256 over the approval fields using the
// per-install secret. The returned string is hex-encoded.
func SignApproval(secret []byte, decisionKey, decision, scope string) string {
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(decisionKey))
	mac.Write([]byte(decision))
	mac.Write([]byte(scope))
	return hex.EncodeToString(mac.Sum(nil))
}

// VerifyApproval checks that mac is a valid HMAC for the given fields.
// Uses constant-time comparison to prevent timing attacks.
func VerifyApproval(secret []byte, decisionKey, decision, scope, mac string) bool {
	expected := SignApproval(secret, decisionKey, decision, scope)
	// Both are hex strings of the same hash, so lengths match for valid MACs.
	return subtle.ConstantTimeCompare([]byte(expected), []byte(mac)) == 1
}
