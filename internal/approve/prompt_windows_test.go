//go:build windows

package approve

import (
	"os"
	"testing"
)

// TestOpenConsole_NonInteractiveFlag verifies that openConsole returns
// errNonInteractive when the nonInteractive flag is set.
func TestOpenConsole_NonInteractiveFlag(t *testing.T) {
	conIn, conOut, err := openConsole(true)
	if err != errNonInteractive {
		t.Errorf("expected errNonInteractive, got %v", err)
	}
	if conIn != nil {
		_ = conIn.Close()
		t.Error("conIn should be nil when nonInteractive is true")
	}
	if conOut != nil {
		_ = conOut.Close()
		t.Error("conOut should be nil when nonInteractive is true")
	}
}

// TestOpenConsole_NonInteractiveEnv verifies that openConsole returns
// errNonInteractive when FUSE_NON_INTERACTIVE is set.
func TestOpenConsole_NonInteractiveEnv(t *testing.T) {
	t.Setenv("FUSE_NON_INTERACTIVE", "1")
	conIn, conOut, err := openConsole(false)
	if err != errNonInteractive {
		t.Errorf("expected errNonInteractive, got %v", err)
	}
	if conIn != nil {
		_ = conIn.Close()
		t.Error("conIn should be nil when FUSE_NON_INTERACTIVE is set")
	}
	if conOut != nil {
		_ = conOut.Close()
		t.Error("conOut should be nil when FUSE_NON_INTERACTIVE is set")
	}
}

// TestPromptUser_NonInteractiveFlag verifies PromptUser returns
// errNonInteractive when the nonInteractive parameter is true.
func TestPromptUser_NonInteractiveFlag(t *testing.T) {
	_, _, err := PromptUser(t.Context(), "rm -rf /", "dangerous", false, true)
	if err != errNonInteractive {
		t.Errorf("expected errNonInteractive, got %v", err)
	}
}

// TestPromptUser_NonInteractiveEnv verifies PromptUser returns
// errNonInteractive when FUSE_NON_INTERACTIVE env var is set.
func TestPromptUser_NonInteractiveEnv(t *testing.T) {
	t.Setenv("FUSE_NON_INTERACTIVE", "1")
	_, _, err := PromptUser(t.Context(), "rm -rf /", "dangerous", false, false)
	if err != errNonInteractive {
		t.Errorf("expected errNonInteractive, got %v", err)
	}
}

// TestRenderPromptPlain_DoesNotPanic verifies that the plain prompt renderer
// can write to a temp file without panicking.
func TestRenderPromptPlain_DoesNotPanic(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "prompt-test-*")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	defer func() { _ = f.Close() }()

	// Should not panic.
	renderPromptPlain(f, "echo hello", "test reason")

	// Verify something was written.
	info, err := f.Stat()
	if err != nil {
		t.Fatalf("stat temp file: %v", err)
	}
	if info.Size() == 0 {
		t.Error("expected non-empty output from renderPromptPlain")
	}
}
