package core

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestBinaryTOFU_FirstUseAllowed(t *testing.T) {
	tofu := NewBinaryTOFU()
	// "sh" should exist on all Unix systems
	shPath, err := exec.LookPath("sh")
	if err != nil {
		t.Fatalf("LookPath(sh): %v", err)
	}
	d, _ := tofu.Verify(shPath)
	if d != "" {
		t.Errorf("first use should be allowed, got %s", d)
	}
}

func TestBinaryTOFU_SecondUseSameHash(t *testing.T) {
	tofu := NewBinaryTOFU()
	shPath, err := exec.LookPath("sh")
	if err != nil {
		t.Fatalf("LookPath(sh): %v", err)
	}
	d1, _ := tofu.Verify(shPath)
	if d1 != "" {
		t.Fatalf("first use failed: %s", d1)
	}
	d2, _ := tofu.Verify(shPath)
	if d2 != "" {
		t.Errorf("second use with same binary should pass, got %s", d2)
	}
}

func TestBinaryTOFU_HashChangeBlocked(t *testing.T) {
	tofu := NewBinaryTOFUWithInterpreter(func(name string) bool { return filepath.Base(name) == "fakeinterp" })

	// Create a fake "interpreter" in a temp dir
	dir := t.TempDir()
	fakeBin := filepath.Join(dir, "fakeinterp")
	if err := os.WriteFile(fakeBin, []byte("#!/bin/sh\necho v1"), 0o755); err != nil {
		t.Fatal(err)
	}

	// First use — trusted
	d1, _ := tofu.Verify(fakeBin)
	if d1 != "" {
		t.Fatalf("first use should pass, got %s", d1)
	}

	time.Sleep(1100 * time.Millisecond)

	// Modify the binary
	if err := os.WriteFile(fakeBin, []byte("#!/bin/sh\necho EVIL"), 0o755); err != nil {
		t.Fatal(err)
	}
	newTime := time.Now().Add(time.Second)
	if err := os.Chtimes(fakeBin, newTime, newTime); err != nil {
		t.Fatalf("Chtimes(fakeinterp): %v", err)
	}

	// Second use — should be BLOCKED
	d2, reason := tofu.Verify(fakeBin)
	if d2 != DecisionBlocked {
		t.Errorf("expected BLOCKED after binary change, got %s (reason: %s)", d2, reason)
	}
}

func TestBinaryTOFU_NonInterpreterSkipped(t *testing.T) {
	tofu := NewBinaryTOFU()
	lsPath, err := exec.LookPath("ls")
	if err != nil {
		t.Fatalf("LookPath(ls): %v", err)
	}
	d, _ := tofu.Verify(lsPath)
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
