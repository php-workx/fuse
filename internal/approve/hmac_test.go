package approve

import (
	"testing"
)

func TestSignVerify(t *testing.T) {
	secret := []byte("test-secret-key-32-bytes-long!!!")
	decisionKey := "abc123"
	decision := "approve"
	scope := "once"

	mac := SignApproval(secret, decisionKey, decision, scope)
	if mac == "" {
		t.Fatal("SignApproval returned empty string")
	}

	if !VerifyApproval(secret, decisionKey, decision, scope, mac) {
		t.Fatal("VerifyApproval failed for valid signature")
	}
}

func TestVerify_WrongKey(t *testing.T) {
	secret1 := []byte("test-secret-key-32-bytes-long!!!")
	secret2 := []byte("different-secret-key-32-bytes!!!")
	decisionKey := "abc123"
	decision := "approve"
	scope := "once"

	mac := SignApproval(secret1, decisionKey, decision, scope)

	if VerifyApproval(secret2, decisionKey, decision, scope, mac) {
		t.Fatal("VerifyApproval should fail with wrong key")
	}
}

func TestVerify_Tampered(t *testing.T) {
	secret := []byte("test-secret-key-32-bytes-long!!!")

	tests := []struct {
		name        string
		decisionKey string
		decision    string
		scope       string
	}{
		{"tampered decision key", "tampered", "approve", "once"},
		{"tampered decision", "abc123", "deny", "once"},
		{"tampered scope", "abc123", "approve", "forever"},
	}

	// Sign with original values.
	mac := SignApproval(secret, "abc123", "approve", "once")

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if VerifyApproval(secret, tt.decisionKey, tt.decision, tt.scope, mac) {
				t.Fatalf("VerifyApproval should reject tampered %s", tt.name)
			}
		})
	}

	// Also test a directly tampered MAC string.
	t.Run("tampered mac string", func(t *testing.T) {
		tamperedMAC := mac[:len(mac)-1] + "0" // flip last char
		if mac[len(mac)-1] == '0' {
			tamperedMAC = mac[:len(mac)-1] + "1"
		}
		if VerifyApproval(secret, "abc123", "approve", "once", tamperedMAC) {
			t.Fatal("VerifyApproval should reject tampered MAC")
		}
	})
}

func TestSign_Deterministic(t *testing.T) {
	secret := []byte("test-secret-key-32-bytes-long!!!")
	decisionKey := "decision-key-value"
	decision := "approve"
	scope := "session"

	mac1 := SignApproval(secret, decisionKey, decision, scope)
	mac2 := SignApproval(secret, decisionKey, decision, scope)
	mac3 := SignApproval(secret, decisionKey, decision, scope)

	if mac1 != mac2 || mac2 != mac3 {
		t.Fatalf("SignApproval is not deterministic: %q, %q, %q", mac1, mac2, mac3)
	}

	// Verify the result is a valid hex string of expected length (SHA-256 = 32 bytes = 64 hex chars).
	if len(mac1) != 64 {
		t.Fatalf("expected 64 hex chars, got %d: %q", len(mac1), mac1)
	}
}
