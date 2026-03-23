package judge

import (
	"os"
	"path/filepath"
	"testing"
)

// createFakeBinary creates a no-op executable in dir with the given name.
func createFakeBinary(t *testing.T, dir, name string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestDetectProvider_Claude(t *testing.T) {
	dir := t.TempDir()
	createFakeBinary(t, dir, "claude")
	t.Setenv("PATH", dir)

	p, err := DetectProvider("auto", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Name() != "claude" {
		t.Errorf("provider = %q, want claude", p.Name())
	}
}

func TestDetectProvider_Codex(t *testing.T) {
	dir := t.TempDir()
	createFakeBinary(t, dir, "codex")
	// Only codex on PATH, no claude.
	t.Setenv("PATH", dir)

	p, err := DetectProvider("auto", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Name() != "codex" {
		t.Errorf("provider = %q, want codex", p.Name())
	}
}

func TestDetectProvider_None(t *testing.T) {
	dir := t.TempDir()
	// Empty directory — no binaries.
	t.Setenv("PATH", dir)

	_, err := DetectProvider("auto", "")
	if err == nil {
		t.Error("expected error when no provider found")
	}
}

func TestDetectProvider_Preferred(t *testing.T) {
	dir := t.TempDir()
	createFakeBinary(t, dir, "claude")
	createFakeBinary(t, dir, "codex")
	t.Setenv("PATH", dir)

	p, err := DetectProvider("codex", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Name() != "codex" {
		t.Errorf("provider = %q, want codex (preferred)", p.Name())
	}
}

func TestDetectProvider_PreferredNotFound(t *testing.T) {
	dir := t.TempDir()
	// No binaries at all.
	t.Setenv("PATH", dir)

	_, err := DetectProvider("codex", "")
	if err == nil {
		t.Error("expected error when preferred provider not on PATH")
	}
}

func TestDetectProvider_UnknownProvider(t *testing.T) {
	dir := t.TempDir()
	createFakeBinary(t, dir, "unknown-llm")
	t.Setenv("PATH", dir)

	_, err := DetectProvider("unknown-llm", "")
	if err == nil {
		t.Error("expected error for unknown provider name")
	}
}

func TestDetectProvider_AutoPrefersClaudeOverCodex(t *testing.T) {
	dir := t.TempDir()
	createFakeBinary(t, dir, "claude")
	createFakeBinary(t, dir, "codex")
	t.Setenv("PATH", dir)

	p, err := DetectProvider("auto", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Name() != "claude" {
		t.Errorf("provider = %q, want claude (auto should prefer claude)", p.Name())
	}
}

func TestDetectProvider_ModelPassedThrough(t *testing.T) {
	dir := t.TempDir()
	createFakeBinary(t, dir, "claude")
	t.Setenv("PATH", dir)

	p, err := DetectProvider("claude", "claude-haiku-4-5-20251001")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	cp, ok := p.(*claudeProvider)
	if !ok {
		t.Fatal("expected *claudeProvider")
	}
	if cp.model != "claude-haiku-4-5-20251001" {
		t.Errorf("model = %q, want claude-haiku-4-5-20251001", cp.model)
	}
}
