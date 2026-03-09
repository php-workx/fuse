package approve

import (
	"testing"
)

func TestSignVerify(t *testing.T) {
	secret := []byte("test-secret-key-32-bytes-long!!!")
	id := "approval-1"
	decisionKey := "abc123"

	mac := SignApproval(secret, id, decisionKey)
	if mac == "" {
		t.Fatal("SignApproval returned empty string")
	}

	if !VerifyApproval(secret, id, decisionKey, mac) {
		t.Fatal("VerifyApproval failed for valid signature")
	}
}

func TestVerify_WrongKey(t *testing.T) {
	secret1 := []byte("test-secret-key-32-bytes-long!!!")
	secret2 := []byte("different-secret-key-32-bytes!!!")
	id := "approval-1"
	decisionKey := "abc123"

	mac := SignApproval(secret1, id, decisionKey)

	if VerifyApproval(secret2, id, decisionKey, mac) {
		t.Fatal("VerifyApproval should fail with wrong key")
	}
}

func TestVerify_Tampered(t *testing.T) {
	secret := []byte("test-secret-key-32-bytes-long!!!")

	tests := []struct {
		name        string
		id          string
		decisionKey string
	}{
		{"tampered id", "approval-2", "abc123"},
		{"tampered decision key", "approval-1", "tampered"},
	}

	// Sign with original values.
	mac := SignApproval(secret, "approval-1", "abc123")

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if VerifyApproval(secret, tt.id, tt.decisionKey, mac) {
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
		if VerifyApproval(secret, "approval-1", "abc123", tamperedMAC) {
			t.Fatal("VerifyApproval should reject tampered MAC")
		}
	})
}

func TestSign_Deterministic(t *testing.T) {
	secret := []byte("test-secret-key-32-bytes-long!!!")
	id := "approval-1"
	decisionKey := "decision-key-value"

	mac1 := SignApproval(secret, id, decisionKey)
	mac2 := SignApproval(secret, id, decisionKey)
	mac3 := SignApproval(secret, id, decisionKey)

	if mac1 != mac2 || mac2 != mac3 {
		t.Fatalf("SignApproval is not deterministic: %q, %q, %q", mac1, mac2, mac3)
	}

	// Verify the result is a valid hex string of expected length (SHA-256 = 32 bytes = 64 hex chars).
	if len(mac1) != 64 {
		t.Fatalf("expected 64 hex chars, got %d: %q", len(mac1), mac1)
	}
}
