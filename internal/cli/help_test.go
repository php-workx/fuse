package cli

import (
	"bytes"
	"regexp"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestBoxTop(t *testing.T) {
	got := boxTop("Setup", 50)
	if !strings.HasPrefix(got, "╭─ Setup ") {
		t.Errorf("boxTop should start with '╭─ Setup ', got %q", got)
	}
	if !strings.HasSuffix(got, "╮") {
		t.Errorf("boxTop should end with '╮', got %q", got)
	}
	if w := utf8.RuneCountInString(got); w != 50 {
		t.Errorf("boxTop display width = %d, want 50", w)
	}
}

func TestBoxRow(t *testing.T) {
	got := boxRow("install  Install fuse", 50)
	if !strings.HasPrefix(got, "│  install") {
		t.Errorf("boxRow should start with '│  install', got %q", got)
	}
	if !strings.HasSuffix(got, " │") {
		t.Errorf("boxRow should end with ' │', got %q", got)
	}
	if w := utf8.RuneCountInString(got); w != 50 {
		t.Errorf("boxRow display width = %d, want 50", w)
	}
}

func TestBoxRowTruncation(t *testing.T) {
	longText := strings.Repeat("x", 200)
	got := boxRow(longText, 50)
	if w := utf8.RuneCountInString(got); w != 50 {
		t.Errorf("boxRow display width = %d, want 50", w)
	}
	if !strings.Contains(got, "...") {
		t.Error("truncated boxRow should contain '...'")
	}
}

func TestBoxBottom(t *testing.T) {
	got := boxBottom(50)
	if !strings.HasPrefix(got, "╰") {
		t.Errorf("boxBottom should start with '╰', got %q", got)
	}
	if !strings.HasSuffix(got, "╯") {
		t.Errorf("boxBottom should end with '╯', got %q", got)
	}
	if w := utf8.RuneCountInString(got); w != 50 {
		t.Errorf("boxBottom display width = %d, want 50", w)
	}
}

func TestBoxRowLayoutWidth(t *testing.T) {
	// Verify that boxRowLayout + renderer produce the correct display width
	// for both plain and colored output.
	r := helpRenderer{color: true}
	for _, width := range []int{40, 50, 60, 72} {
		row := r.row("install", "Install fuse as a hook for an AI coding agent", 12, width)
		stripped := stripANSI(row)
		if w := utf8.RuneCountInString(stripped); w != width {
			t.Errorf("colored row display width at %d = %d, want %d\nrow: %q", width, w, width, stripped)
		}
	}
}

func TestBoxRowLayoutTruncation(t *testing.T) {
	r := helpRenderer{color: true}
	longDesc := strings.Repeat("x", 200)
	row := r.row("cmd", longDesc, 12, 50)
	stripped := stripANSI(row)
	if w := utf8.RuneCountInString(stripped); w != 50 {
		t.Errorf("colored truncated row display width = %d, want 50", w)
	}
	if !strings.Contains(stripped, "...") {
		t.Error("colored truncated row should contain '...'")
	}
}

// withNoColor pins color off for the duration of the test.
func withNoColor(t *testing.T) {
	t.Helper()
	old := shouldColorizeFunc
	shouldColorizeFunc = func() bool { return false }
	t.Cleanup(func() { shouldColorizeFunc = old })
}

// withTermWidth overrides terminal width for the duration of the test.
func withTermWidth(t *testing.T, width int) {
	t.Helper()
	old := termWidthFunc
	termWidthFunc = func() int { return width }
	t.Cleanup(func() { termWidthFunc = old })
}

// captureHelp runs --help on rootCmd and returns the output.
func captureHelp(t *testing.T, args ...string) string {
	t.Helper()
	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs(args)
	t.Cleanup(func() {
		rootCmd.SetOut(nil)
		rootCmd.SetErr(nil)
		rootCmd.SetArgs(nil)
	})
	if err := rootCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

func TestRootHelpGrouped(t *testing.T) {
	withNoColor(t)
	output := captureHelp(t, "--help")

	// No ANSI escape codes in plain mode.
	if strings.Contains(output, "\033[") {
		t.Error("plain output should not contain ANSI escape codes")
	}

	// Verify all expected strings are present.
	for _, want := range []string{
		"Setup", "Runtime", "Observe",
		"╭", "╮", "│", "╰", "╯",
		"install", "uninstall", "enable", "disable", "dryrun",
		"run", "hook", "proxy",
		"events", "stats", "monitor", "doctor", "profile", "test",
		"Additional Commands:",
		"version", "help", "completion",
	} {
		if !strings.Contains(output, want) {
			t.Errorf("help output missing %q", want)
		}
	}

	// Verify command ordering within groups by extracting each box.
	assertGroupOrder(t, output, "Setup", []string{"install", "uninstall", "enable", "disable", "dryrun"})
	assertGroupOrder(t, output, "Runtime", []string{"run", "hook", "proxy"})
	assertGroupOrder(t, output, "Observe", []string{"events", "stats", "monitor", "doctor", "profile", "test"})
}

// extractGroupBox returns the text between "╭─ <title>" and the next "╯" (inclusive).
func extractGroupBox(output, title string) string {
	start := strings.Index(output, "╭─ "+title+" ")
	if start < 0 {
		// Try with ANSI codes stripped.
		stripped := stripANSI(output)
		start = strings.Index(stripped, "╭─ "+title+" ")
		if start < 0 {
			return ""
		}
		end := strings.Index(stripped[start:], "╯")
		if end < 0 {
			return ""
		}
		return stripped[start : start+end+len("╯")]
	}
	end := strings.Index(output[start:], "╯")
	if end < 0 {
		return ""
	}
	return output[start : start+end+len("╯")]
}

// assertGroupOrder verifies that commands appear in order within a specific group box.
// This scopes the search to the group's box, preventing false matches (e.g. "run" inside "dryrun").
func assertGroupOrder(t *testing.T, output, group string, cmds []string) {
	t.Helper()
	box := extractGroupBox(output, group)
	if box == "" {
		t.Errorf("could not find %s group box in output", group)
		return
	}

	// Split box into lines and extract command names (first word after "│  ").
	var found []string
	for _, line := range strings.Split(box, "\n") {
		stripped := stripANSI(line)
		trimmed := strings.TrimPrefix(stripped, "│")
		trimmed = strings.TrimSpace(trimmed)
		if trimmed == "" || strings.HasPrefix(trimmed, "╭") || strings.HasPrefix(trimmed, "╰") {
			continue
		}
		// First field is the command name.
		fields := strings.Fields(trimmed)
		if len(fields) > 0 && !strings.ContainsAny(fields[0], "─╭╮╰╯│") {
			found = append(found, fields[0])
		}
	}

	if len(found) != len(cmds) {
		t.Errorf("in %s group: found %v, want %v", group, found, cmds)
		return
	}
	for i, want := range cmds {
		if found[i] != want {
			t.Errorf("in %s group position %d: got %q, want %q", group, i, found[i], want)
		}
	}
}

// stripANSI removes ANSI escape sequences from a string.
var ansiPattern = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func stripANSI(s string) string {
	return ansiPattern.ReplaceAllString(s, "")
}

func TestSubcommandHelpFlat(t *testing.T) {
	withNoColor(t)
	output := captureHelp(t, "hook", "--help")

	// Subcommand help should NOT have box characters.
	if strings.Contains(output, "╭") {
		t.Error("subcommand help should not have box drawing characters")
	}

	// Should list subcommands normally.
	if !strings.Contains(output, "evaluate") {
		t.Error("hook help should list evaluate subcommand")
	}
}

func TestNarrowTerminalNoBorders(t *testing.T) {
	withNoColor(t)
	withTermWidth(t, 30)
	output := captureHelp(t, "--help")

	// Should NOT contain box characters.
	if strings.Contains(output, "╭") {
		t.Error("narrow terminal should not have box drawing characters")
	}

	// Groups should still be labeled.
	for _, want := range []string{"Setup:", "Runtime:", "Observe:"} {
		if !strings.Contains(output, want) {
			t.Errorf("narrow output missing group label %q", want)
		}
	}
}

func TestColoredOutput(t *testing.T) {
	old := shouldColorizeFunc
	shouldColorizeFunc = func() bool { return true }
	t.Cleanup(func() { shouldColorizeFunc = old })

	output := captureHelp(t, "--help")

	// Should contain ANSI escape codes.
	if !strings.Contains(output, "\033[") {
		t.Error("colored output should contain ANSI escape codes")
	}

	// Should contain dim, bold, and bold-cyan codes.
	for _, code := range []string{ansiDim, ansiBold, ansiBoldCyan} {
		if !strings.Contains(output, code) {
			t.Errorf("colored output missing ANSI code %q", code)
		}
	}

	// All groups and commands still present after stripping ANSI.
	stripped := stripANSI(output)
	for _, want := range []string{
		"Setup", "Runtime", "Observe",
		"install", "run", "events", "doctor",
		"Additional Commands:",
	} {
		if !strings.Contains(stripped, want) {
			t.Errorf("colored output (stripped) missing %q", want)
		}
	}

	// Order preserved in colored output.
	assertGroupOrder(t, output, "Setup", []string{"install", "uninstall", "enable", "disable", "dryrun"})
}

func TestNoColorEnvSuppressesColor(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	if shouldColorize() {
		t.Error("shouldColorize should return false when NO_COLOR is set")
	}
}

func TestTermDumbSuppressesColor(t *testing.T) {
	t.Setenv("TERM", "dumb")
	if shouldColorize() {
		t.Error("shouldColorize should return false when TERM=dumb")
	}
}
