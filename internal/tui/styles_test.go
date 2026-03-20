package tui

import "testing"

func TestDecisionStyle(t *testing.T) {
	// Just verify they don't panic and return non-empty renders.
	for _, d := range []string{"SAFE", "CAUTION", "APPROVAL", "BLOCKED", "unknown"} {
		s := decisionStyle(d)
		rendered := s.Render(d)
		if rendered == "" {
			t.Errorf("decisionStyle(%q).Render returned empty", d)
		}
	}
}

func TestSanitize(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"hello", "hello"},
		{"hello\x1b[31mworld\x1b[0m", "helloworld"},
		{"clean text", "clean text"},
		{"\x1b]0;title\x07text", "text"},
		{"no\x00null", "nonull"},
		{"tab\ttab", "tab\ttab"}, // tabs are printable, kept
	}
	for _, tt := range tests {
		got := sanitize(tt.input)
		if got != tt.want {
			t.Errorf("sanitize(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestShorten(t *testing.T) {
	if got := shorten("hello", 10); got != "hello" {
		t.Errorf("shorten short = %q", got)
	}
	if got := shorten("hello world", 8); got != "hello..." {
		t.Errorf("shorten long = %q", got)
	}
	if got := shorten("ab", 2); got != "ab" {
		t.Errorf("shorten exact = %q", got)
	}
}

func TestFallbackValue(t *testing.T) {
	if got := fallbackValue(""); got != "-" {
		t.Errorf("fallbackValue empty = %q", got)
	}
	if got := fallbackValue("x"); got != "x" {
		t.Errorf("fallbackValue nonempty = %q", got)
	}
}
