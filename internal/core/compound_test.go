package core

import (
	"testing"
)

func TestSplitCompound_Semicolon(t *testing.T) {
	result, err := SplitCompoundCommand("echo a; echo b")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 commands, got %d: %v", len(result), result)
	}
	if result[0] != "echo a" {
		t.Errorf("expected first command %q, got %q", "echo a", result[0])
	}
	if result[1] != "echo b" {
		t.Errorf("expected second command %q, got %q", "echo b", result[1])
	}
}

func TestSplitCompound_And(t *testing.T) {
	result, err := SplitCompoundCommand("mkdir foo && cd foo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 commands, got %d: %v", len(result), result)
	}
	if result[0] != "mkdir foo" {
		t.Errorf("expected first command %q, got %q", "mkdir foo", result[0])
	}
	if result[1] != "cd foo" {
		t.Errorf("expected second command %q, got %q", "cd foo", result[1])
	}
}

func TestSplitCompound_Or(t *testing.T) {
	result, err := SplitCompoundCommand("cmd1 || cmd2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 commands, got %d: %v", len(result), result)
	}
	if result[0] != "cmd1" {
		t.Errorf("expected first command %q, got %q", "cmd1", result[0])
	}
	if result[1] != "cmd2" {
		t.Errorf("expected second command %q, got %q", "cmd2", result[1])
	}
}

func TestSplitCompound_Pipe(t *testing.T) {
	// Per §5.2: pipes (|) should be split into individual commands
	// for per-command classification.
	result, err := SplitCompoundCommand("ls | grep foo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 commands, got %d: %v", len(result), result)
	}
	if result[0] != "ls" {
		t.Errorf("expected first command %q, got %q", "ls", result[0])
	}
	if result[1] != "grep foo" {
		t.Errorf("expected second command %q, got %q", "grep foo", result[1])
	}
}

func TestSplitCompound_Newline(t *testing.T) {
	result, err := SplitCompoundCommand("echo a\necho b")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 commands, got %d: %v", len(result), result)
	}
	if result[0] != "echo a" {
		t.Errorf("expected first command %q, got %q", "echo a", result[0])
	}
	if result[1] != "echo b" {
		t.Errorf("expected second command %q, got %q", "echo b", result[1])
	}
}

func TestSplitCompound_QuotedOperators(t *testing.T) {
	// Operators inside quotes must NOT cause splits.
	result, err := SplitCompoundCommand(`echo "a && b"`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 command, got %d: %v", len(result), result)
	}
}

func TestSplitCompound_ParseFailure(t *testing.T) {
	// Invalid syntax should return an error so the caller can fail-closed.
	_, err := SplitCompoundCommand("echo $(")
	if err == nil {
		t.Fatal("expected parse error, got nil")
	}
}

func TestSplitCompound_SingleCommand(t *testing.T) {
	result, err := SplitCompoundCommand("echo hello world")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 command, got %d: %v", len(result), result)
	}
	if result[0] != "echo hello world" {
		t.Errorf("expected %q, got %q", "echo hello world", result[0])
	}
}

func TestSplitCompound_MultiPipe(t *testing.T) {
	// A multi-stage pipeline should split into individual commands.
	result, err := SplitCompoundCommand("cat file | grep pattern | wc -l")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 3 {
		t.Fatalf("expected 3 commands, got %d: %v", len(result), result)
	}
	if result[0] != "cat file" {
		t.Errorf("expected first command %q, got %q", "cat file", result[0])
	}
	if result[1] != "grep pattern" {
		t.Errorf("expected second command %q, got %q", "grep pattern", result[1])
	}
	if result[2] != "wc -l" {
		t.Errorf("expected third command %q, got %q", "wc -l", result[2])
	}
}

func TestSplitCompound_MixedOperators(t *testing.T) {
	// Mix of semicolons, && and pipes.
	result, err := SplitCompoundCommand("echo a; ls | grep foo && echo done")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// "echo a" ; ("ls" | "grep foo") && "echo done"
	// Should produce: echo a, ls, grep foo, echo done
	if len(result) != 4 {
		t.Fatalf("expected 4 commands, got %d: %v", len(result), result)
	}
}

func TestContainsCwdChange(t *testing.T) {
	tests := []struct {
		name     string
		cmds     []string
		expected bool
	}{
		{
			name:     "cd command",
			cmds:     []string{"mkdir foo", "cd foo"},
			expected: true,
		},
		{
			name:     "pushd command",
			cmds:     []string{"pushd /tmp"},
			expected: true,
		},
		{
			name:     "popd command",
			cmds:     []string{"popd"},
			expected: true,
		},
		{
			name:     "no cwd change",
			cmds:     []string{"echo hello", "ls -la"},
			expected: false,
		},
		{
			name:     "cd as argument not command",
			cmds:     []string{"echo cd"},
			expected: false,
		},
		{
			name:     "cd with path",
			cmds:     []string{"cd /home/user/project"},
			expected: true,
		},
		{
			name:     "empty slice",
			cmds:     []string{},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ContainsCwdChange(tt.cmds)
			if got != tt.expected {
				t.Errorf("ContainsCwdChange(%v) = %v, want %v", tt.cmds, got, tt.expected)
			}
		})
	}
}

func TestSplitCompoundCommand_PowerShellPipeline(t *testing.T) {
	result, err := SplitCompoundCommand("Get-Process | Sort-Object CPU")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 sub-commands, got %d: %v", len(result), result)
	}
	if result[0] != "Get-Process" {
		t.Errorf("expected first sub-command %q, got %q", "Get-Process", result[0])
	}
	if result[1] != "Sort-Object CPU" {
		t.Errorf("expected second sub-command %q, got %q", "Sort-Object CPU", result[1])
	}
}

func TestSplitCompoundCommand_PowerShellSemicolon(t *testing.T) {
	result, err := SplitCompoundCommand("Get-Date; Get-Location")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 sub-commands, got %d: %v", len(result), result)
	}
	if result[0] != "Get-Date" {
		t.Errorf("expected first sub-command %q, got %q", "Get-Date", result[0])
	}
	if result[1] != "Get-Location" {
		t.Errorf("expected second sub-command %q, got %q", "Get-Location", result[1])
	}
}

func TestSplitCompoundCommand_CMDChain(t *testing.T) {
	// CMD with syntax that fails the Bash parser: unmatched parenthesis in
	// "cmd /c (dir /s" is invalid Bash but detected as CMD via the cmd /c prefix.
	result, err := SplitCompoundCommand("cmd /c (dir /s & type file.txt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 sub-commands, got %d: %v", len(result), result)
	}
	if result[0] != "cmd /c (dir /s" {
		t.Errorf("expected first sub-command %q, got %q", "cmd /c (dir /s", result[0])
	}
	if result[1] != "type file.txt" {
		t.Errorf("expected second sub-command %q, got %q", "type file.txt", result[1])
	}
}

func TestSplitCompoundCommand_BashFallthrough(t *testing.T) {
	// A command that fails Bash parsing and looks like PowerShell should use the fallback.
	// "Get-Process | Where-Object { $_.CPU -gt 50 }" fails Bash parsing due to $_.CPU syntax
	// but is detected as PowerShell due to Get-Process cmdlet.
	result, err := SplitCompoundCommand("Get-Process | Where-Object { $_.CPU -gt 50 }")
	if err != nil {
		t.Fatalf("expected PowerShell fallback to succeed, got error: %v", err)
	}
	if len(result) == 0 {
		t.Fatal("expected non-empty result from PowerShell fallback")
	}

	// A command that fails Bash parsing and looks like Bash should return an error
	// (fail-closed behavior preserved).
	_, err = SplitCompoundCommand("echo $(")
	if err == nil {
		t.Fatal("expected parse error for invalid Bash command, got nil")
	}
}

func TestContainsCwdChange_PowerShell(t *testing.T) {
	tests := []struct {
		name     string
		cmds     []string
		expected bool
	}{
		{
			name:     "Set-Location",
			cmds:     []string{"Set-Location C:\\Users"},
			expected: true,
		},
		{
			name:     "sl alias",
			cmds:     []string{"sl C:\\Users"},
			expected: true,
		},
		{
			name:     "Push-Location",
			cmds:     []string{"Push-Location /tmp"},
			expected: true,
		},
		{
			name:     "Pop-Location",
			cmds:     []string{"Pop-Location"},
			expected: true,
		},
		{
			name:     "chdir CMD alias",
			cmds:     []string{"chdir C:\\Users"},
			expected: true,
		},
		{
			name:     "case-insensitive set-location",
			cmds:     []string{"set-location C:\\Users"},
			expected: true,
		},
		{
			name:     "case-insensitive SET-LOCATION",
			cmds:     []string{"SET-LOCATION C:\\Users"},
			expected: true,
		},
		{
			name:     "PowerShell non-cwd command",
			cmds:     []string{"Get-Process", "Sort-Object CPU"},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ContainsCwdChange(tt.cmds)
			if got != tt.expected {
				t.Errorf("ContainsCwdChange(%v) = %v, want %v", tt.cmds, got, tt.expected)
			}
		})
	}
}

func TestSplitSimpleCompound(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "semicolon split",
			input:    "echo a; echo b",
			expected: []string{"echo a", "echo b"},
		},
		{
			name:     "pipe split",
			input:    "ls | grep foo",
			expected: []string{"ls", "grep foo"},
		},
		{
			name:     "ampersand split",
			input:    "cmd1 & cmd2",
			expected: []string{"cmd1", "cmd2"},
		},
		{
			name:     "double ampersand splits into parts",
			input:    "cmd1 && cmd2",
			expected: []string{"cmd1", "cmd2"},
		},
		{
			name:     "double pipe splits into parts",
			input:    "cmd1 || cmd2",
			expected: []string{"cmd1", "cmd2"},
		},
		{
			name:     "quoted semicolon not split",
			input:    `echo "a; b"`,
			expected: []string{`echo "a; b"`},
		},
		{
			name:     "single-quoted pipe not split",
			input:    "echo 'a | b'",
			expected: []string{"echo 'a | b'"},
		},
		{
			name:     "mixed operators",
			input:    "cmd1; cmd2 | cmd3 & cmd4",
			expected: []string{"cmd1", "cmd2", "cmd3", "cmd4"},
		},
		{
			name:     "single command no operators",
			input:    "echo hello",
			expected: []string{"echo hello"},
		},
		{
			name:     "empty segments trimmed",
			input:    "cmd1;; cmd2",
			expected: []string{"cmd1", "cmd2"},
		},
		{
			name:     "whitespace trimmed",
			input:    "  cmd1  ;  cmd2  ",
			expected: []string{"cmd1", "cmd2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := splitSimpleCompound(tt.input)
			if len(got) != len(tt.expected) {
				t.Fatalf("splitSimpleCompound(%q) = %v (len %d), want %v (len %d)",
					tt.input, got, len(got), tt.expected, len(tt.expected))
			}
			for i := range got {
				if got[i] != tt.expected[i] {
					t.Errorf("splitSimpleCompound(%q)[%d] = %q, want %q",
						tt.input, i, got[i], tt.expected[i])
				}
			}
		})
	}
}
