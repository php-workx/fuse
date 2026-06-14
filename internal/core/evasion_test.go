package core

import (
	"strings"
	"testing"
)

// ANSI-C quoting expansion ----------------------------------------------------

func TestExpandAnsiCQuoting_HexEscape(t *testing.T) {
	got := expandAnsiCQuoting(`$'\x72\x6d' -rf /`)
	if got != "rm -rf /" {
		t.Errorf("got %q, want %q", got, "rm -rf /")
	}
}

func TestExpandAnsiCQuoting_OctalEscape(t *testing.T) {
	got := expandAnsiCQuoting(`$'\162\155' -rf /`)
	if got != "rm -rf /" {
		t.Errorf("got %q, want %q", got, "rm -rf /")
	}
}

func TestExpandAnsiCQuoting_UnicodeEscape(t *testing.T) {
	got := expandAnsiCQuoting(`echo $'/'`)
	if got != "echo /" {
		t.Errorf("got %q, want %q", got, "echo /")
	}
	gotU := expandAnsiCQuoting(`echo $'\U0000002F'`)
	if gotU != "echo /" {
		t.Errorf("got %q, want %q", gotU, "echo /")
	}
}

func TestExpandAnsiCQuoting_StandardEscapes(t *testing.T) {
	got := expandAnsiCQuoting(`echo $'\n\t\\'`)
	if got != "echo \n\t\\" {
		t.Errorf("got %q, want %q", got, "echo \n\t\\")
	}
}

func TestExpandAnsiCQuoting_UnterminatedLeftAlone(t *testing.T) {
	in := `echo $'\xff oops`
	got := expandAnsiCQuoting(in)
	if got != in {
		t.Errorf("unterminated should preserve input; got %q, want %q", got, in)
	}
}

func TestExpandAnsiCQuoting_DoesNotInterpretVariables(t *testing.T) {
	for _, s := range []string{`echo $VAR`, `echo $(whoami)`, `echo ${HOME}`, `echo $((1+2))`} {
		got := expandAnsiCQuoting(s)
		if got != s {
			t.Errorf("variable form mutated: %q → %q", s, got)
		}
	}
}

func TestExpandAnsiCQuoting_NoLiteralLeavesInputUnchanged(t *testing.T) {
	in := "rm -rf /tmp/foo"
	got := expandAnsiCQuoting(in)
	if got != in {
		t.Errorf("input without $'... mutated: %q", got)
	}
}

func TestExpandAnsiCQuoting_ControlCharEscape(t *testing.T) {
	got := expandAnsiCQuoting(`echo $'\cA'`)
	if got != "echo \x01" {
		t.Errorf("got %q, want %q", got, "echo \x01")
	}
}

// URL percent decoding -------------------------------------------------------

func TestDecodeURLPercents_DecodesPath(t *testing.T) {
	got := decodeURLPercents(`curl https://example.com/%65vil`)
	if got != "curl https://example.com/evil" {
		t.Errorf("got %q", got)
	}
}

func TestDecodeURLPercents_LeavesQueryAlone(t *testing.T) {
	in := `curl https://example.com/path?q=%2F%65vil`
	got := decodeURLPercents(in)
	if got != in {
		t.Errorf("query string was decoded: %q", got)
	}
}

func TestDecodeURLPercents_NonURLTokenUnchanged(t *testing.T) {
	in := `curl --header X-Foo:%20bar`
	got := decodeURLPercents(in)
	if got != in {
		t.Errorf("non-URL token decoded: %q", got)
	}
}

func TestDecodeURLPercents_InvalidTripletPreserved(t *testing.T) {
	in := `curl https://example.com/%ZZpath`
	got := decodeURLPercents(in)
	if got != in {
		t.Errorf("invalid triplet decoded/dropped: %q", got)
	}
}

func TestDecodeURLPercents_NoPercentLeavesInputUnchanged(t *testing.T) {
	in := "git status --short"
	got := decodeURLPercents(in)
	if got != in {
		t.Errorf("input without %% mutated: %q", got)
	}
}

func TestDecodeURLPercents_MultipleTokens(t *testing.T) {
	got := decodeURLPercents(`wget https://%65vil.com/a https://%65vil.org/b`)
	want := "wget https://evil.com/a https://evil.org/b"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// Path collapse --------------------------------------------------------------

func TestCollapsePaths_BasicTraversal(t *testing.T) {
	got := collapsePaths(`cat /foo/../etc/passwd`)
	if got != "cat /etc/passwd" {
		t.Errorf("got %q", got)
	}
}

func TestCollapsePaths_RelativeUnchanged(t *testing.T) {
	in := `cat foo/../bar`
	got := collapsePaths(in)
	if got != in {
		t.Errorf("relative path mutated: %q", got)
	}
}

func TestCollapsePaths_DepthCapped(t *testing.T) {
	chain := strings.Repeat("../", 32)
	in := "cat /" + chain + "etc/passwd"
	got := collapsePaths(in)
	// Bounded; must terminate without panic. path.Clean resolves in one
	// pass anyway, so the result is well-defined.
	if got == "" {
		t.Errorf("empty result")
	}
}

func TestCollapsePaths_WindowsBackslash(t *testing.T) {
	got := collapsePaths(`type C:\Foo\..\Bar`)
	if got != `type C:\Bar` {
		t.Errorf("got %q", got)
	}
}

func TestCollapsePaths_NoDotDotLeavesInputUnchanged(t *testing.T) {
	in := "git status --short"
	got := collapsePaths(in)
	if got != in {
		t.Errorf("input without .. mutated: %q", got)
	}
}

func TestCollapsePaths_AbsoluteForwardSlash(t *testing.T) {
	got := collapsePaths(`ls /usr/local/../bin`)
	if got != "ls /usr/bin" {
		t.Errorf("got %q", got)
	}
}

// Combined pipeline ----------------------------------------------------------

func TestDisplayNormalize_AnsiCThenPathCollapse(t *testing.T) {
	got := DisplayNormalize(`$'\x72\x6d' -rf /foo/../tmp`)
	if got != "rm -rf /tmp" {
		t.Errorf("got %q, want %q", got, "rm -rf /tmp")
	}
}

func TestDisplayNormalize_PercentEncodedURL(t *testing.T) {
	got := DisplayNormalize(`curl https://%65vil.com/%70wn`)
	if got != "curl https://evil.com/pwn" {
		t.Errorf("got %q", got)
	}
}

func TestDisplayNormalize_AnsiCDoesNotBreakSafeCommand(t *testing.T) {
	in := "git status --short"
	got := DisplayNormalize(in)
	if got != in {
		t.Errorf("safe command mutated: %q", got)
	}
}
