package policy

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadPolicyWithLKG_Success(t *testing.T) {
	dir := t.TempDir()
	policyPath := filepath.Join(dir, "policy.yaml")

	// Write a valid policy.
	err := os.WriteFile(policyPath, []byte(`
version: "1"
rules:
  - pattern: "^rm -rf /"
    action: "block"
    reason: "dangerous"
`), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadPolicyWithLKG(policyPath, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(cfg.Rules))
	}

	// Verify LKG was created.
	lkgPath := policyPath + lkgSuffix
	if _, statErr := os.Stat(lkgPath); statErr != nil {
		t.Fatalf("LKG file not created: %v", statErr)
	}
}

func TestLoadPolicyWithLKG_ParseError_FallsBack(t *testing.T) {
	dir := t.TempDir()
	policyPath := filepath.Join(dir, "policy.yaml")
	lkgPath := policyPath + lkgSuffix

	// Write a valid LKG first.
	err := os.WriteFile(lkgPath, []byte(`# LKG saved: 2026-03-24T10:00:00Z
# Original: policy.yaml (sha256: abc123)
version: "1"
rules:
  - pattern: "^rm -rf /"
    action: "block"
    reason: "from LKG"
`), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	// Write a broken policy.
	err = os.WriteFile(policyPath, []byte(`{invalid yaml content`), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadPolicyWithLKG(policyPath, 0)
	if err != nil {
		t.Fatalf("expected LKG fallback, got error: %v", err)
	}
	if len(cfg.Rules) != 1 {
		t.Fatalf("expected 1 rule from LKG, got %d", len(cfg.Rules))
	}
	if cfg.Rules[0].Reason != "from LKG" {
		t.Errorf("expected reason 'from LKG', got %q", cfg.Rules[0].Reason)
	}
}

func TestLoadPolicyWithLKG_NoLKG_Fails(t *testing.T) {
	dir := t.TempDir()
	policyPath := filepath.Join(dir, "policy.yaml")

	// Write a broken policy, no LKG exists.
	err := os.WriteFile(policyPath, []byte(`{invalid yaml`), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	_, err = LoadPolicyWithLKG(policyPath, 0)
	if err == nil {
		t.Fatal("expected error when both policy and LKG are unavailable")
	}
}

func TestLoadPolicyWithLKG_StaleLKG(t *testing.T) {
	dir := t.TempDir()
	policyPath := filepath.Join(dir, "policy.yaml")
	lkgPath := policyPath + lkgSuffix

	// Write LKG.
	err := os.WriteFile(lkgPath, []byte(`version: "1"
rules: []
`), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	// Set LKG modification time to 8 days ago.
	staleTime := time.Now().Add(-8 * 24 * time.Hour)
	if err := os.Chtimes(lkgPath, staleTime, staleTime); err != nil {
		t.Fatal(err)
	}

	// Write broken policy.
	err = os.WriteFile(policyPath, []byte(`{broken`), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	_, err = LoadPolicyWithLKG(policyPath, 7*24*time.Hour)
	if err == nil {
		t.Fatal("expected error for stale LKG")
	}
}
