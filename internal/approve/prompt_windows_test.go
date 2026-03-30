//go:build windows

package approve

import (
	"os"
	"strings"
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

// TestRenderPromptPlain_RendersContent verifies that the plain prompt renderer
// outputs the command, reason, and header.
func TestRenderPromptPlain_RendersContent(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "prompt-test-*")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}

	renderPromptPlain(f, "echo hello", "test reason")
	_ = f.Close()

	content, err := os.ReadFile(f.Name())
	if err != nil {
		t.Fatalf("read temp file: %v", err)
	}
	got := string(content)

	for _, want := range []string{"echo hello", "test reason", "fuse: approval required"} {
		if !strings.Contains(got, want) {
			t.Errorf("output missing %q:\n%s", want, got)
		}
	}
}
