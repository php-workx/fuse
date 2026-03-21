package sanitize

import "testing"

func TestString_StripCSI(t *testing.T) {
	input := "hello \x1b[31mred\x1b[0m world"
	got := String(input)
	if got != "hello red world" {
		t.Errorf("CSI not stripped: got %q", got)
	}
}

func TestString_Strip8BitCSI(t *testing.T) {
	// U+009B (8-bit CSI) in proper UTF-8 encoding: 0xC2 0x9B
	input := "hello \xc2\x9b31mred\xc2\x9b0m world"
	got := String(input)
	if got != "hello 31mred0m world" {
		t.Errorf("8-bit CSI not stripped: got %q", got)
	}
}

func TestString_StripOSC_BEL(t *testing.T) {
	input := "cmd \x1b]0;pwned\x07 rest"
	got := String(input)
	if got != "cmd  rest" {
		t.Errorf("BEL-terminated OSC not stripped: got %q", got)
	}
}

func TestString_StripOSC_ST(t *testing.T) {
	input := "cmd \x1b]0;pwned\x1b\\ rest"
	got := String(input)
	if got != "cmd  rest" {
		t.Errorf("ST-terminated OSC not stripped: got %q", got)
	}
}

func TestString_StripScreenClear(t *testing.T) {
	input := "\x1b[2J\x1b[Hfake prompt"
	got := String(input)
	if got != "fake prompt" {
		t.Errorf("screen clear not stripped: got %q", got)
	}
}

func TestString_StripControlChars(t *testing.T) {
	input := "hello\x00\x01\x07\x08world"
	got := String(input)
	if got != "helloworld" {
		t.Errorf("control chars not stripped: got %q", got)
	}
}

func TestString_PreserveTabNewline(t *testing.T) {
	input := "line1\n\tindented\r\nline3"
	got := String(input)
	if got != input {
		t.Errorf("tab/newline/CR modified: got %q, want %q", got, input)
	}
}

func TestString_PreserveUTF8(t *testing.T) {
	input := "terraform destroy 🔥 production"
	got := String(input)
	if got != input {
		t.Errorf("UTF-8 modified: got %q, want %q", got, input)
	}
}

func TestString_EmptyString(t *testing.T) {
	got := String("")
	if got != "" {
		t.Errorf("empty string modified: got %q", got)
	}
}

func TestString_SafeCommandUnchanged(t *testing.T) {
	input := "git push --force origin feat/my-branch"
	got := String(input)
	if got != input {
		t.Errorf("safe command modified: got %q, want %q", got, input)
	}
}

func TestString_Strip8BitC1Codes(t *testing.T) {
	// C1 control characters U+0080-U+009F in proper UTF-8 encoding.
	input := "a\xc2\x80\xc2\x85\xc2\x8e\xc2\x90\xc2\x9c\xc2\x9fz"
	got := String(input)
	if got != "az" {
		t.Errorf("C1 codes not stripped: got %q", got)
	}
}
