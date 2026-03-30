package core_test

import (
	"testing"

	"github.com/php-workx/fuse/internal/core"
	"github.com/php-workx/fuse/internal/policy"
)

// TestClassify_WindowsPowerShell validates end-to-end classification of
// PowerShell commands through the full pipeline. These tests run on ALL
// platforms since the classification pipeline is platform-independent.
func TestClassify_WindowsPowerShell(t *testing.T) {
	evaluator := policy.NewEvaluator(nil)

	tests := []struct {
		name    string
		command string
		want    core.Decision
	}{
		// --- Safe PowerShell cmdlets (unconditionally safe) ---
		{
			name:    "PS safe cmdlet Get-ChildItem",
			command: "Get-ChildItem -Recurse",
			want:    core.DecisionSafe,
		},
		{
			name:    "PS safe cmdlet Get-Process",
			command: "Get-Process",
			want:    core.DecisionSafe,
		},
		{
			name:    "PS safe cmdlet Get-Content",
			command: "Get-Content C:\\Users\\me\\file.txt",
			want:    core.DecisionSafe,
		},
		{
			name:    "PS safe cmdlet Test-Path",
			command: "Test-Path C:\\Windows\\System32",
			want:    core.DecisionSafe,
		},
		{
			name:    "PS safe cmdlet Get-Service",
			command: "Get-Service -Name wuauserv",
			want:    core.DecisionSafe,
		},
		{
			name:    "PS safe cmdlet Get-Date",
			command: "Get-Date",
			want:    core.DecisionSafe,
		},
		{
			name:    "PS safe cmdlet Get-Help",
			command: "Get-Help Get-Process -Full",
			want:    core.DecisionSafe,
		},
		{
			name:    "PS safe cmdlet Get-Location",
			command: "Get-Location",
			want:    core.DecisionSafe,
		},
		{
			name:    "PS safe cmdlet Write-Output",
			command: "Write-Output 'hello world'",
			want:    core.DecisionSafe,
		},
		{
			name:    "PS safe cmdlet Format-Table",
			command: "Format-Table -AutoSize",
			want:    core.DecisionSafe,
		},
		{
			name:    "PS safe cmdlet Select-Object",
			command: "Select-Object -First 10",
			want:    core.DecisionSafe,
		},
		{
			name:    "PS safe cmdlet ConvertTo-Json",
			command: "ConvertTo-Json -Depth 5",
			want:    core.DecisionSafe,
		},
		{
			name:    "PS safe cmdlet case insensitive",
			command: "get-childitem -Path .",
			want:    core.DecisionSafe,
		},

		// --- PowerShell pipeline (compound) ---
		{
			name:    "PS pipeline safe",
			command: "Get-Process | Sort-Object CPU",
			want:    core.DecisionSafe,
		},
		{
			name:    "PS pipeline multi-stage safe",
			command: "Get-Process | Where-Object { $_.CPU -gt 10 } | Sort-Object CPU | Select-Object -First 5",
			want:    core.DecisionSafe,
		},

		// --- PowerShell compound with semicolons ---
		{
			name:    "PS compound semicolons safe",
			command: "Get-Date; Get-Location",
			want:    core.DecisionSafe,
		},
		{
			name:    "PS compound safe triple",
			command: "Get-ChildItem; Get-Process; Get-Service",
			want:    core.DecisionSafe,
		},

		// --- PowerShell wrapper extraction ---
		// Wrappers like powershell.exe classify as CAUTION because the outer
		// command (powershell.exe) is not in the safe-command list. The inner
		// command IS extracted and classified, but the max decision applies.
		{
			name:    "PS wrapper powershell.exe -Command inner extracted",
			command: "powershell.exe -Command Get-Process",
			want:    core.DecisionCaution,
		},
		{
			name:    "PS wrapper pwsh -Command inner extracted",
			command: "pwsh -Command Get-ChildItem",
			want:    core.DecisionCaution,
		},
		{
			name:    "PS wrapper powershell -NoProfile inner extracted",
			command: "powershell -NoProfile -Command Get-Date",
			want:    core.DecisionCaution,
		},
		// Wrapper with destructive inner command should be BLOCKED
		// (inner command escalates past the wrapper's CAUTION).
		{
			name:    "PS wrapper with destructive inner is blocked",
			command: "powershell.exe -Command Format-Volume -DriveLetter D",
			want:    core.DecisionBlocked,
		},

		// --- Blocked: Destructive PowerShell commands ---
		{
			name:    "PS destructive Remove-Item recursive force on system dir",
			command: "Remove-Item -Recurse -Force C:\\Windows",
			want:    core.DecisionBlocked,
		},
		{
			name:    "PS destructive Remove-Item recursive on C drive root",
			command: "Remove-Item -Recurse -Force C:\\",
			want:    core.DecisionBlocked,
		},
		{
			name:    "PS destructive Remove-Item recursive on Program Files",
			command: "Remove-Item -Recurse C:\\Program Files",
			want:    core.DecisionBlocked,
		},
		{
			name:    "PS destructive Remove-Item recursive on Users dir",
			command: "Remove-Item -Recurse C:\\Users",
			want:    core.DecisionBlocked,
		},
		{
			name:    "PS disk format Format-Volume blocked",
			command: "Format-Volume -DriveLetter D",
			want:    core.DecisionBlocked,
		},
		{
			name:    "PS disk format Clear-Disk blocked",
			command: "Clear-Disk -Number 0",
			want:    core.DecisionBlocked,
		},
		{
			name:    "PS disk format Initialize-Disk blocked",
			command: "Initialize-Disk -Number 1",
			want:    core.DecisionBlocked,
		},

		// --- Blocked: Self-protection via PowerShell ---
		{
			name:    "PS self-protection delete claude settings",
			command: "Remove-Item .claude\\settings.json",
			want:    core.DecisionBlocked,
		},
		{
			name:    "PS self-protection delete fuse config",
			command: "Remove-Item .fuse\\policy.yaml",
			want:    core.DecisionBlocked,
		},
		{
			name:    "PS self-protection Set-Content claude settings",
			command: "Set-Content .claude\\settings.json 'malicious'",
			want:    core.DecisionBlocked,
		},

		// --- Remove-Item without catastrophic target (not BLOCKED, but not SAFE either) ---
		{
			name:    "PS Remove-Item non-system path is not blocked",
			command: "Remove-Item C:\\Users\\me\\temp\\file.txt",
			want:    core.DecisionCaution, // not blocked (no catastrophic path), not safe (not in safe list)
		},
		{
			name:    "PS Remove-Item with WhatIf is safe",
			command: "Remove-Item C:\\file.txt -WhatIf",
			want:    core.DecisionSafe,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := core.ShellRequest{
				RawCommand: tt.command,
				Cwd:        "/tmp",
				Source:     "test",
				SessionID:  "test-session",
			}
			result, err := core.Classify(req, evaluator)
			if err != nil {
				t.Fatalf("classify error: %v", err)
			}
			if result.Decision != tt.want {
				t.Errorf("command %q: got %s, want %s (reason: %s, rule: %s)",
					tt.command, result.Decision, tt.want, result.Reason, result.RuleID)
			}
		})
	}
}

// TestClassify_WindowsCMD validates end-to-end classification of CMD.exe
// commands through the full pipeline. Runs on all platforms.
func TestClassify_WindowsCMD(t *testing.T) {
	evaluator := policy.NewEvaluator(nil)

	tests := []struct {
		name    string
		command string
		want    core.Decision
	}{
		// --- CMD wrapper extraction ---
		// Like PowerShell wrappers, cmd.exe is not in the safe-command list,
		// so the outer command gets CAUTION. Inner safe commands are extracted
		// and classified, but the max decision applies.
		{
			name:    "CMD wrapper cmd.exe /c dir inner extracted",
			command: "cmd.exe /c dir /b",
			want:    core.DecisionCaution,
		},
		{
			name:    "CMD wrapper cmd /c echo inner extracted",
			command: "cmd /c echo hello",
			want:    core.DecisionCaution,
		},
		{
			name:    "CMD wrapper cmd /C type inner extracted",
			command: "cmd /C type file.txt",
			want:    core.DecisionCaution,
		},

		// --- CMD destructive ---
		{
			name:    "CMD rd /s /q system dir blocked",
			command: "rd /s /q C:\\Windows",
			want:    core.DecisionBlocked,
		},
		{
			name:    "CMD del /s system root blocked",
			command: "del /s C:\\",
			want:    core.DecisionBlocked,
		},
		{
			name:    "CMD rmdir /s /q Users blocked",
			command: "rmdir /s /q C:\\Users",
			want:    core.DecisionBlocked,
		},

		// --- CMD self-protection ---
		{
			name:    "CMD del claude settings blocked",
			command: "del .claude\\settings.json",
			want:    core.DecisionBlocked,
		},
		{
			name:    "CMD del fuse config blocked",
			command: "del .fuse\\policy.yaml",
			want:    core.DecisionBlocked,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := core.ShellRequest{
				RawCommand: tt.command,
				Cwd:        "/tmp",
				Source:     "test",
				SessionID:  "test-session",
			}
			result, err := core.Classify(req, evaluator)
			if err != nil {
				t.Fatalf("classify error: %v", err)
			}
			if result.Decision != tt.want {
				t.Errorf("command %q: got %s, want %s (reason: %s, rule: %s)",
					tt.command, result.Decision, tt.want, result.Reason, result.RuleID)
			}
		})
	}
}

// TestClassify_WindowsCompoundMixedSeverity validates that compound PowerShell
// commands take the most restrictive decision across sub-commands.
func TestClassify_WindowsCompoundMixedSeverity(t *testing.T) {
	evaluator := policy.NewEvaluator(nil)

	tests := []struct {
		name    string
		command string
		want    core.Decision
	}{
		{
			name:    "safe + blocked = blocked",
			command: "Get-ChildItem; Format-Volume -DriveLetter D",
			want:    core.DecisionBlocked,
		},
		{
			name:    "safe + safe = safe",
			command: "Get-ChildItem; Get-Process",
			want:    core.DecisionSafe,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := core.ShellRequest{
				RawCommand: tt.command,
				Cwd:        "/tmp",
				Source:     "test",
				SessionID:  "test-session",
			}
			result, err := core.Classify(req, evaluator)
			if err != nil {
				t.Fatalf("classify error: %v", err)
			}
			if result.Decision != tt.want {
				t.Errorf("command %q: got %s, want %s (reason: %s, rule: %s)",
					tt.command, result.Decision, tt.want, result.Reason, result.RuleID)
			}
		})
	}
}
