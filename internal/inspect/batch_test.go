package inspect

import "testing"

func TestScanBatch_SafeScript(t *testing.T) {
	content := []byte(`@echo off
setlocal
echo hello
dir C:\Windows
`)

	signals := ScanBatch(content)
	if len(signals) != 0 {
		t.Errorf("expected 0 signals for safe batch content, got %d:", len(signals))
		for _, s := range signals {
			t.Logf("  line %d: category=%s match=%q", s.Line, s.Category, s.Match)
		}
	}
}

func TestScanBatch_CommentSkipping(t *testing.T) {
	content := []byte(`@echo off
REM certutil -decode payload.b64 payload.exe
:: net user evil P@ssw0rd! /add
echo safe
`)

	signals := ScanBatch(content)
	if len(signals) != 0 {
		t.Errorf("expected 0 signals for commented-out batch content, got %d:", len(signals))
		for _, s := range signals {
			t.Logf("  line %d: category=%s match=%q", s.Line, s.Category, s.Match)
		}
	}
}

func TestScanBatch_DetectsWindowsSignals(t *testing.T) {
	content := []byte(`@echo off
certutil -decode payload.b64 payload.exe
reg add HKCU\Software\Microsoft\Windows\CurrentVersion\Run /v Evil /d C:\Temp\evil.exe
schtasks /create /tn Evil /tr C:\Temp\evil.exe /sc onlogon
del /s /q C:\Temp\logs\*
net user evil P@ssw0rd! /add
netsh advfirewall firewall add rule name="evil" dir=in action=allow program="C:\Temp\evil.exe"
`)

	signals := ScanBatch(content)
	if len(signals) == 0 {
		t.Fatal("expected signals for malicious batch content, got 0")
	}

	categories := batchSignalCategories(signals)
	expectedCategories := []string{"lolbin", "registry_modify", "persistence", "destructive_fs", "user_modify", "firewall_modify"}
	for _, cat := range expectedCategories {
		if !categories[cat] {
			t.Errorf("expected category %q in signals, not found", cat)
		}
	}

	t.Logf("found %d signals:", len(signals))
	for _, s := range signals {
		t.Logf("  line %d: category=%s match=%q", s.Line, s.Category, s.Match)
	}
}

func batchSignalCategories(signals []Signal) map[string]bool {
	cats := make(map[string]bool)
	for _, s := range signals {
		cats[s.Category] = true
	}
	return cats
}
