package cli

import (
	"bytes"
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

// withNoColor pins color off for the duration of the test.
func withNoColor(t *testing.T) {
	t.Helper()
	old := shouldColorizeFunc
	shouldColorizeFunc = func() bool { return false }
	t.Cleanup(func() { shouldColorizeFunc = old })
}

func TestRootHelpGrouped(t *testing.T) {
	withNoColor(t)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"--help"})
	err := rootCmd.Execute()
	rootCmd.SetOut(nil)
	rootCmd.SetErr(nil)
	rootCmd.SetArgs(nil)
	if err != nil {
		t.Fatal(err)
	}

	output := buf.String()

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
		"events", "stats", "doctor", "test",
		"Additional Commands:",
		"version", "help", "completion",
	} {
		if !strings.Contains(output, want) {
			t.Errorf("help output missing %q", want)
		}
	}

	// Verify command ordering within groups matches the spec.
	assertOrder(t, output, "Setup", []string{"install", "uninstall", "enable", "disable", "dryrun"})
	assertOrder(t, output, "Runtime", []string{"run", "hook", "proxy"})
	assertOrder(t, output, "Observe", []string{"events", "stats", "doctor", "test"})
}

// assertOrder verifies that commands appear in the expected order within the help output.
func assertOrder(t *testing.T, output, group string, cmds []string) {
	t.Helper()
	prev := 0
	for _, cmd := range cmds {
		idx := strings.Index(output[prev:], cmd)
		if idx < 0 {
			t.Errorf("in %s group: %q not found after position %d", group, cmd, prev)
			return
		}
		prev += idx + len(cmd)
	}
}

func TestSubcommandHelpFlat(t *testing.T) {
	withNoColor(t)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"hook", "--help"})
	err := rootCmd.Execute()
	rootCmd.SetOut(nil)
	rootCmd.SetErr(nil)
	rootCmd.SetArgs(nil)
	if err != nil {
		t.Fatal(err)
	}

	output := buf.String()

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
	old := termWidthFunc
	termWidthFunc = func() int { return 30 }
	defer func() { termWidthFunc = old }()

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"--help"})
	err := rootCmd.Execute()
	rootCmd.SetOut(nil)
	rootCmd.SetErr(nil)
	rootCmd.SetArgs(nil)
	if err != nil {
		t.Fatal(err)
	}

	output := buf.String()

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
	defer func() { shouldColorizeFunc = old }()

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"--help"})
	err := rootCmd.Execute()
	rootCmd.SetOut(nil)
	rootCmd.SetErr(nil)
	rootCmd.SetArgs(nil)
	if err != nil {
		t.Fatal(err)
	}

	output := buf.String()

	// Should contain ANSI escape codes.
	if !strings.Contains(output, "\033[") {
		t.Error("colored output should contain ANSI escape codes")
	}

	// Should contain dim (SGR 2) and bold (SGR 1) codes.
	if !strings.Contains(output, "\033[2m") {
		t.Error("colored output should contain dim code")
	}
	if !strings.Contains(output, "\033[1m") {
		t.Error("colored output should contain bold code")
	}
	if !strings.Contains(output, "\033[1;36m") {
		t.Error("colored output should contain bold-cyan title code")
	}

	// All groups and commands still present in the colored output.
	for _, want := range []string{
		"Setup", "Runtime", "Observe",
		"install", "run", "events", "doctor",
		"Additional Commands:",
	} {
		if !strings.Contains(output, want) {
			t.Errorf("colored output missing %q", want)
		}
	}

	// Order preserved in colored output.
	assertOrder(t, output, "Setup", []string{"install", "uninstall", "enable", "disable", "dryrun"})
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
