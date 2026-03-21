package approve

import "testing"

// Comprehensive sanitization tests are in internal/sanitize/sanitize_test.go.
// This test verifies the delegation wrapper works.

func TestSanitizePrompt_DelegatesToSharedPackage(t *testing.T) {
	input := "hello \x1b[31mred\x1b[0m world"
	got := sanitizePrompt(input)
	if got != "hello red world" {
		t.Errorf("sanitizePrompt did not strip CSI: got %q", got)
	}

	// Also verify C1 stripping works through delegation.
	input2 := "a\xc2\x9bz"
	got2 := sanitizePrompt(input2)
	if got2 != "az" {
		t.Errorf("sanitizePrompt did not strip C1: got %q", got2)
	}
}

func TestSanitizePrompt_StripsNewlines(t *testing.T) {
	// Newlines and carriage returns must be replaced with spaces
	// to prevent prompt layout injection.
	input := "line1\nline2\rline3"
	got := sanitizePrompt(input)
	if got != "line1 line2 line3" {
		t.Errorf("newlines not replaced: got %q", got)
	}
}
