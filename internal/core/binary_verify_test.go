package core

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBinaryTOFU_FirstUseAllowed(t *testing.T) {
	tofu := NewBinaryTOFU()
	// "sh" should exist on all Unix systems
	d, _ := tofu.Verify("sh")
	if d != "" {
		t.Errorf("first use should be allowed, got %s", d)
	}
}

func TestBinaryTOFU_SecondUseSameHash(t *testing.T) {
	tofu := NewBinaryTOFU()
	d1, _ := tofu.Verify("sh")
	if d1 != "" {
		t.Fatalf("first use failed: %s", d1)
	}
	d2, _ := tofu.Verify("sh")
	if d2 != "" {
		t.Errorf("second use with same binary should pass, got %s", d2)
	}
}

func TestBinaryTOFU_HashChangeBlocked(t *testing.T) {
	tofu := NewBinaryTOFUWithInterpreter(func(name string) bool { return name == "fakeinterp" })

	// Create a fake "interpreter" in a temp dir
	dir := t.TempDir()
	fakeBin := filepath.Join(dir, "fakeinterp")
	if err := os.WriteFile(fakeBin, []byte("#!/bin/sh\necho v1"), 0o755); err != nil {
		t.Fatal(err)
	}

	// Add to PATH so LookPath finds it
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))

	// First use — trusted
	d1, _ := tofu.Verify("fakeinterp")
	if d1 != "" {
		t.Fatalf("first use should pass, got %s", d1)
	}

	// Modify the binary
	if err := os.WriteFile(fakeBin, []byte("#!/bin/sh\necho EVIL"), 0o755); err != nil {
		t.Fatal(err)
	}

	// Second use — should be BLOCKED
	d2, reason := tofu.Verify("fakeinterp")
	if d2 != DecisionBlocked {
		t.Errorf("expected BLOCKED after binary change, got %s (reason: %s)", d2, reason)
	}
}

func TestBinaryTOFU_NonInterpreterSkipped(t *testing.T) {
	tofu := NewBinaryTOFU()
	d, _ := tofu.Verify("ls")
	if d != "" {
		t.Errorf("non-interpreter should be skipped, got %s", d)
	}
}

func TestIsInterpreter(t *testing.T) {
	for _, name := range []string{"python", "python3", "node", "bash", "sh", "ruby", "perl"} {
		if !IsInterpreter(name) {
			t.Errorf("%s should be an interpreter", name)
		}
	}
	for _, name := range []string{"ls", "cat", "grep", "curl", "git"} {
		if IsInterpreter(name) {
			t.Errorf("%s should NOT be an interpreter", name)
		}
	}
}
