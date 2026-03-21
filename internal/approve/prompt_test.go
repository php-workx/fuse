package approve

import "testing"

func TestSanitizePrompt_StripCSI(t *testing.T) {
	// 7-bit CSI: ESC[ sequences (colors, cursor movement)
	input := "hello \x1b[31mred\x1b[0m world"
	got := sanitizePrompt(input)
	if got != "hello red world" {
		t.Errorf("CSI not stripped: got %q", got)
	}
}

func TestSanitizePrompt_Strip8BitCSI(t *testing.T) {
	// U+009B (8-bit CSI) in proper UTF-8 encoding: 0xC2 0x9B
	input := "hello \xc2\x9b31mred\xc2\x9b0m world"
	got := sanitizePrompt(input)
	if got != "hello 31mred0m world" {
		t.Errorf("8-bit CSI not stripped: got %q", got)
	}
}

func TestSanitizePrompt_StripOSC_BEL(t *testing.T) {
	// OSC terminated with BEL (title injection)
	input := "cmd \x1b]0;pwned\x07 rest"
	got := sanitizePrompt(input)
	if got != "cmd  rest" {
		t.Errorf("BEL-terminated OSC not stripped: got %q", got)
	}
}

func TestSanitizePrompt_StripOSC_ST(t *testing.T) {
	// OSC terminated with ST (ESC\)
	input := "cmd \x1b]0;pwned\x1b\\ rest"
	got := sanitizePrompt(input)
	if got != "cmd  rest" {
		t.Errorf("ST-terminated OSC not stripped: got %q", got)
	}
}

func TestSanitizePrompt_StripScreenClear(t *testing.T) {
	// Screen clear + cursor home
	input := "\x1b[2J\x1b[Hfake prompt"
	got := sanitizePrompt(input)
	if got != "fake prompt" {
		t.Errorf("screen clear not stripped: got %q", got)
	}
}

func TestSanitizePrompt_StripControlChars(t *testing.T) {
	// Non-printable control characters (NUL, BEL, BS, etc.)
	input := "hello\x00\x01\x07\x08world"
	got := sanitizePrompt(input)
	if got != "helloworld" {
		t.Errorf("control chars not stripped: got %q", got)
	}
}

func TestSanitizePrompt_PreserveTabNewline(t *testing.T) {
	// Tab (\x09), newline (\x0a), carriage return (\x0d) should be preserved
	input := "line1\n\tindented\r\nline3"
	got := sanitizePrompt(input)
	if got != input {
		t.Errorf("tab/newline/CR modified: got %q, want %q", got, input)
	}
}

func TestSanitizePrompt_PreserveUTF8(t *testing.T) {
	// Multi-byte UTF-8 characters must pass through
	input := "terraform destroy 🔥 production"
	got := sanitizePrompt(input)
	if got != input {
		t.Errorf("UTF-8 modified: got %q, want %q", got, input)
	}
}

func TestSanitizePrompt_EmptyString(t *testing.T) {
	got := sanitizePrompt("")
	if got != "" {
		t.Errorf("empty string modified: got %q", got)
	}
}

func TestSanitizePrompt_SafeCommandUnchanged(t *testing.T) {
	input := "git push --force origin feat/my-branch"
	got := sanitizePrompt(input)
	if got != input {
		t.Errorf("safe command modified: got %q, want %q", got, input)
	}
}

func TestSanitizePrompt_Strip8BitC1Codes(t *testing.T) {
	// C1 control characters U+0080-U+009F in proper UTF-8 encoding.
	// U+0080=\xc2\x80, U+0085=\xc2\x85, U+008E=\xc2\x8e,
	// U+0090=\xc2\x90, U+009C=\xc2\x9c, U+009F=\xc2\x9f
	input := "a\xc2\x80\xc2\x85\xc2\x8e\xc2\x90\xc2\x9c\xc2\x9fz"
	got := sanitizePrompt(input)
	if got != "az" {
		t.Errorf("C1 codes not stripped: got %q", got)
	}
}
