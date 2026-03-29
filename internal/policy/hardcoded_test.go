package policy

import (
	"testing"

	"github.com/php-workx/fuse/internal/core"
)

// TestHardcodedBlocked_WindowsDestructive verifies PowerShell Remove-Item rules
// block catastrophic paths but allow non-catastrophic ones.
func TestHardcodedBlocked_WindowsDestructive(t *testing.T) {
	blocked := []struct {
		cmd    string
		reason string
	}{
		{`Remove-Item -Recurse -Force C:\Windows`, "Remove-Item on C:\\Windows should be blocked"},
		{`remove-item -recurse -force C:\Windows\System32`, "case-insensitive Remove-Item on System32"},
		{`Remove-Item -Recurse C:\`, "Remove-Item on C:\\ root"},
		{`Remove-Item -Recurse -Force C:\Users`, "Remove-Item on C:\\Users"},
		{`Remove-Item -Recurse -Force C:\Program Files`, "Remove-Item on C:\\Program Files"},
		{`Remove-Item -Recurse -Force %SystemRoot%`, "Remove-Item with %SystemRoot% env var"},
		{`Remove-Item -Recurse -Force %ProgramFiles%`, "Remove-Item with %ProgramFiles% env var"},
	}
	for _, tc := range blocked {
		dec, reason := EvaluateHardcoded(tc.cmd)
		if dec != core.DecisionBlocked {
			t.Errorf("%s: got decision %q (reason: %q), want BLOCKED", tc.reason, dec, reason)
		}
	}

	notBlocked := []struct {
		cmd    string
		reason string
	}{
		{`Remove-Item -Recurse -Force C:\Users\me\tmp`, "Remove-Item on user subdirectory should NOT be blocked"},
		{`Remove-Item -Force C:\Windows`, "Remove-Item without -Recurse should NOT be blocked"},
		{`Remove-Item myfile.txt`, "simple Remove-Item should NOT be blocked"},
	}
	for _, tc := range notBlocked {
		dec, _ := EvaluateHardcoded(tc.cmd)
		if dec == core.DecisionBlocked {
			t.Errorf("%s: got BLOCKED, want no match", tc.reason)
		}
	}
}

// TestHardcodedBlocked_CMDDestructive verifies CMD del/rd/rmdir rules block
// catastrophic paths when used with recursive flags.
func TestHardcodedBlocked_CMDDestructive(t *testing.T) {
	blocked := []struct {
		cmd    string
		reason string
	}{
		{`del /s /q C:\`, "del /s /q on C:\\ should be blocked"},
		{`rd /s /q C:\Windows\System32`, "rd /s /q on System32 should be blocked"},
		{`rmdir /s /q C:\Windows`, "rmdir /s /q on C:\\Windows should be blocked"},
		{`DEL /S /Q C:\Users`, "case-insensitive DEL on C:\\Users"},
		{`rd /s C:\Program Files`, "rd /s on C:\\Program Files"},
	}
	for _, tc := range blocked {
		dec, reason := EvaluateHardcoded(tc.cmd)
		if dec != core.DecisionBlocked {
			t.Errorf("%s: got decision %q (reason: %q), want BLOCKED", tc.reason, dec, reason)
		}
	}

	notBlocked := []struct {
		cmd    string
		reason string
	}{
		{`del /s /q C:\Users\me\tmp`, "del on user subdirectory should NOT be blocked"},
		{`del myfile.txt`, "simple del should NOT be blocked"},
		{`rd C:\Windows`, "rd without /s should NOT be blocked"},
	}
	for _, tc := range notBlocked {
		dec, _ := EvaluateHardcoded(tc.cmd)
		if dec == core.DecisionBlocked {
			t.Errorf("%s: got BLOCKED, want no match", tc.reason)
		}
	}
}

// TestHardcodedBlocked_WindowsSelfProtection verifies Windows self-protection
// rules block modification and deletion of fuse/claude config files.
func TestHardcodedBlocked_WindowsSelfProtection(t *testing.T) {
	blocked := []struct {
		cmd    string
		reason string
	}{
		{`copy malicious.json .claude\settings.json`, "copy to .claude\\settings.json should be blocked"},
		{`move bad.json C:\Users\me\.claude\settings.json`, "move to .claude\\settings.json should be blocked"},
		{`Set-Content .fuse\config\policy.yaml`, "Set-Content to .fuse\\ should be blocked"},
		{`tee .fuse\config\policy.yaml`, "tee to .fuse\\ should be blocked"},
		{`copy evil.yaml .fuse\config\policy.yaml`, "copy to .fuse\\ should be blocked"},
		{`Remove-Item .claude\settings.json`, "Remove-Item .claude\\settings.json should be blocked"},
		{`del .claude\settings.json`, "del .claude\\settings.json should be blocked"},
		{`Remove-Item .fuse\config\`, "Remove-Item .fuse\\ should be blocked"},
		{`del .fuse\something`, "del .fuse\\ should be blocked"},
	}
	for _, tc := range blocked {
		dec, reason := EvaluateHardcoded(tc.cmd)
		if dec != core.DecisionBlocked {
			t.Errorf("%s: got decision %q (reason: %q), want BLOCKED", tc.reason, dec, reason)
		}
	}
}

// TestHardcodedBlocked_DiskFormat verifies PowerShell disk formatting commands are blocked.
func TestHardcodedBlocked_DiskFormat(t *testing.T) {
	blocked := []struct {
		cmd    string
		reason string
	}{
		{"Format-Volume -DriveLetter C", "Format-Volume should be blocked"},
		{"format-volume -DriveLetter D", "case-insensitive Format-Volume should be blocked"},
		{"Clear-Disk -Number 0 -RemoveData", "Clear-Disk should be blocked"},
		{"clear-disk -Number 1", "case-insensitive Clear-Disk should be blocked"},
		{"Initialize-Disk -Number 0", "Initialize-Disk should be blocked"},
		{"initialize-disk -Number 0 -PartitionStyle GPT", "case-insensitive Initialize-Disk should be blocked"},
	}
	for _, tc := range blocked {
		dec, reason := EvaluateHardcoded(tc.cmd)
		if dec != core.DecisionBlocked {
			t.Errorf("%s: got decision %q (reason: %q), want BLOCKED", tc.reason, dec, reason)
		}
	}
}

// TestHardcodedBlocked_UnixRulesUnchanged verifies that existing Unix hardcoded
// rules still match after adding Windows rules.
func TestHardcodedBlocked_UnixRulesUnchanged(t *testing.T) {
	blocked := []struct {
		cmd    string
		reason string
	}{
		{"rm -rf /", "rm -rf / should be blocked"},
		{"rm -rf /*", "rm -rf /* should be blocked"},
		{"rm -rf ~", "rm -rf ~ should be blocked"},
		{"mkfs.ext4 /dev/sda1", "mkfs should be blocked"},
		{"mkswap /dev/sda2", "mkswap on device should be blocked"},
		{"dd if=/dev/zero of=/dev/sda", "dd writing to device should be blocked"},
		{":() { :|: & }; :", "fork bomb should be blocked"},
		{"chmod 777 / ", "chmod 777 on root should be blocked"},
		{"fuse disable", "fuse disable should be blocked"},
		{"tee ~/.fuse/config/policy.yaml", "writing to fuse config should be blocked"},
		{"cp malicious.json .claude/settings.json", "writing to claude settings should be blocked"},
		{"rm -rf ~/.fuse/", "deleting fuse directory should be blocked"},
		{"rm .claude/settings.json", "deleting claude settings should be blocked"},
	}
	for _, tc := range blocked {
		dec, reason := EvaluateHardcoded(tc.cmd)
		if dec != core.DecisionBlocked {
			t.Errorf("%s: got decision %q (reason: %q), want BLOCKED", tc.reason, dec, reason)
		}
	}

	notBlocked := []struct {
		cmd    string
		reason string
	}{
		{"rm file.txt", "simple rm should not be blocked"},
		{"ls /dev/sda", "listing a device should not be blocked"},
		{"echo hello", "echo should not be blocked"},
		{"fuse status", "fuse status should not be blocked"},
		{"cat ~/.fuse/config/policy.yaml", "reading fuse config should not be blocked"},
	}
	for _, tc := range notBlocked {
		dec, _ := EvaluateHardcoded(tc.cmd)
		if dec == core.DecisionBlocked {
			t.Errorf("%s: got BLOCKED, want no match", tc.reason)
		}
	}
}

// TestIsWindowsCatastrophicTarget tests the predicate function directly.
func TestIsWindowsCatastrophicTarget(t *testing.T) {
	catastrophic := []struct {
		cmd    string
		reason string
	}{
		{`Remove-Item -Recurse C:\Windows`, `C:\Windows is catastrophic`},
		{`del /s /q C:\`, `C:\ is catastrophic`},
		{`rd /s C:\Windows\System32`, `C:\Windows\System32 is catastrophic`},
		{`Remove-Item C:\Program Files`, `C:\Program Files is catastrophic`},
		{`Remove-Item C:\Program Files (x86)`, `C:\Program Files (x86) is catastrophic`},
		{`Remove-Item C:\Users`, `C:\Users is catastrophic`},
		{`something %SystemRoot%`, `%SystemRoot% env var is catastrophic`},
		{`something %ProgramFiles%`, `%ProgramFiles% env var is catastrophic`},
		{`something %UserProfile%`, `%UserProfile% env var is catastrophic`},
		// Case insensitivity
		{`Remove-Item c:\WINDOWS`, `case-insensitive C:\WINDOWS is catastrophic`},
		{`Remove-Item C:\WINDOWS\SYSTEM32`, `case-insensitive System32`},
		// Forward-slash normalization
		{`Remove-Item C:/Windows`, `forward-slash C:/Windows is catastrophic`},
		{`Remove-Item C:/Windows/System32`, `forward-slash System32 is catastrophic`},
		// Trailing backslash handling
		{`Remove-Item C:\Windows\`, `trailing backslash on C:\Windows\ is catastrophic`},
	}
	for _, tc := range catastrophic {
		if !isWindowsCatastrophicTarget(tc.cmd) {
			t.Errorf("%s: expected true, got false", tc.reason)
		}
	}

	notCatastrophic := []struct {
		cmd    string
		reason string
	}{
		{`Remove-Item C:\Users\me\tmp`, `user subdirectory is not catastrophic`},
		{`Remove-Item C:\Users\me\Documents\file.txt`, `user file is not catastrophic`},
		{`Remove-Item D:\data`, `D:\data is not catastrophic`},
		{`Remove-Item myfile.txt`, `relative path is not catastrophic`},
		{`echo hello world`, `no path at all`},
	}
	for _, tc := range notCatastrophic {
		if isWindowsCatastrophicTarget(tc.cmd) {
			t.Errorf("%s: expected false, got true", tc.reason)
		}
	}
}

// TestIsWindowsCatastrophicTarget_PSEnvSyntax verifies that PowerShell $env:
// variable syntax is detected as catastrophic by the predicate.
func TestIsWindowsCatastrophicTarget_PSEnvSyntax(t *testing.T) {
	catastrophic := []struct {
		cmd    string
		reason string
	}{
		{`Remove-Item -Recurse $env:SystemRoot`, `$env:SystemRoot is catastrophic`},
		{`Remove-Item -Recurse $env:ProgramFiles`, `$env:ProgramFiles is catastrophic`},
		{`Remove-Item -Recurse $env:UserProfile`, `$env:UserProfile is catastrophic`},
		{`Remove-Item -Recurse $env:SystemDrive`, `$env:SystemDrive is catastrophic`},
		// Case-insensitive — $env: names are case-insensitive in PowerShell
		{`Remove-Item -Recurse $Env:SYSTEMROOT`, `$Env:SYSTEMROOT upper-case is catastrophic`},
		{`Remove-Item -Recurse $ENV:programfiles`, `$ENV:programfiles lower-case is catastrophic`},
	}
	for _, tc := range catastrophic {
		if !isWindowsCatastrophicTarget(tc.cmd) {
			t.Errorf("%s: expected true, got false", tc.reason)
		}
	}
}

// TestHardcodedBlocked_PSEnvVarRemoveItem verifies that Remove-Item -Recurse
// targeting PowerShell $env: paths is BLOCKED end-to-end.
func TestHardcodedBlocked_PSEnvVarRemoveItem(t *testing.T) {
	blocked := []struct {
		cmd    string
		reason string
	}{
		{`Remove-Item -Recurse $env:SystemRoot`, `Remove-Item -Recurse $env:SystemRoot should be blocked`},
		{`Remove-Item -Recurse -Force $env:ProgramFiles`, `Remove-Item -Recurse $env:ProgramFiles should be blocked`},
		{`Remove-Item -Recurse $env:UserProfile`, `Remove-Item -Recurse $env:UserProfile should be blocked`},
		{`Remove-Item -Recurse $env:SystemDrive`, `Remove-Item -Recurse $env:SystemDrive should be blocked`},
		{`remove-item -recurse $Env:SYSTEMROOT`, `case-insensitive $Env:SYSTEMROOT should be blocked`},
	}
	for _, tc := range blocked {
		dec, reason := EvaluateHardcoded(tc.cmd)
		if dec != core.DecisionBlocked {
			t.Errorf("%s: got decision %q (reason: %q), want BLOCKED", tc.reason, dec, reason)
		}
	}
}

// TestHardcodedBlocked_InlineInterpreterPowerShell verifies that pwsh/powershell
// used to access fuse-managed files is BLOCKED.
func TestHardcodedBlocked_InlineInterpreterPowerShell(t *testing.T) {
	blocked := []struct {
		cmd    string
		reason string
	}{
		{`pwsh -Command Get-Content ~/.fuse/state/secret.key`, `pwsh -Command reading secret.key should be blocked`},
		{`powershell -Command Get-Content ~/.fuse/config/policy.yaml`, `powershell -Command reading fuse config should be blocked`},
		{`pwsh -Command "cat .fuse/config/policy.yaml"`, `pwsh -Command on fuse config should be blocked`},
		{`PWSH -Command Get-Content .fuse/config/policy.yaml`, `upper-case PWSH should be blocked`},
		{`powershell.exe -Command cat .fuse/config/policy.yaml`, `powershell.exe -Command should be blocked`},
		{`pwsh -Command sqlite3 fuse.db "SELECT * FROM events"`, `pwsh accessing fuse.db should be blocked`},
		{`pwsh -Command "Get-Content .claude/settings.json"`, `pwsh reading .claude/settings.json should be blocked`},
	}
	for _, tc := range blocked {
		dec, reason := EvaluateHardcoded(tc.cmd)
		if dec != core.DecisionBlocked {
			t.Errorf("%s: got decision %q (reason: %q), want BLOCKED", tc.reason, dec, reason)
		}
	}
}

// TestHardcodedBlocked_EncodedCommandUnconditional verifies that
// -EncodedCommand is BLOCKED for any powershell/pwsh invocation regardless
// of what payload it carries.
func TestHardcodedBlocked_EncodedCommandUnconditional(t *testing.T) {
	blocked := []struct {
		cmd    string
		reason string
	}{
		{`powershell -EncodedCommand dQBuAGkAeAAgAHMAbwBtAGUAdABoAGkAbgBnAA==`, `powershell -EncodedCommand with arbitrary payload should be blocked`},
		{`pwsh -EncodedCommand aGVsbG8=`, `pwsh -EncodedCommand with benign payload should be blocked`},
		{`POWERSHELL -EncodedCommand AAAA`, `upper-case POWERSHELL -EncodedCommand should be blocked`},
		{`powershell.exe -EncodedCommand dQBuAGkAeAA=`, `powershell.exe -EncodedCommand should be blocked`},
		// Simulate a real attack: base64 of arbitrary command
		{`powershell -EncodedCommand UgBlAG0AbwB2AGUALQBJAHQAZQBtAA==`, `encoded Remove-Item payload should be blocked`},
	}
	for _, tc := range blocked {
		dec, reason := EvaluateHardcoded(tc.cmd)
		if dec != core.DecisionBlocked {
			t.Errorf("%s: got decision %q (reason: %q), want BLOCKED", tc.reason, dec, reason)
		}
	}
}

// TestHardcodedBlocked_WindowsWriteCmdlets verifies that Out-File, Add-Content,
// and Tee-Object writing to fuse/claude files are BLOCKED.
func TestHardcodedBlocked_WindowsWriteCmdlets(t *testing.T) {
	blocked := []struct {
		cmd    string
		reason string
	}{
		{`Out-File -FilePath ~/.fuse/config/policy.yaml`, `Out-File to fuse config should be blocked`},
		{`Add-Content -Path .fuse/config/policy.yaml -Value "evil"`, `Add-Content to fuse config should be blocked`},
		{`Tee-Object -FilePath .fuse/config/policy.yaml`, `Tee-Object to fuse config should be blocked`},
		{`Out-File -FilePath .claude/settings.json`, `Out-File to .claude/settings.json should be blocked`},
		{`Add-Content -Path .claude/settings.json -Value "{}"`, `Add-Content to .claude/settings.json should be blocked`},
		{`Tee-Object -FilePath C:\Users\me\.claude\settings.json`, `Tee-Object to .claude/settings.json should be blocked`},
		{`out-file -filepath .fuse\config\policy.yaml`, `case-insensitive out-file to fuse config should be blocked`},
		{`add-content .fuse\config\policy.yaml`, `case-insensitive add-content should be blocked`},
	}
	for _, tc := range blocked {
		dec, reason := EvaluateHardcoded(tc.cmd)
		if dec != core.DecisionBlocked {
			t.Errorf("%s: got decision %q (reason: %q), want BLOCKED", tc.reason, dec, reason)
		}
	}
}
