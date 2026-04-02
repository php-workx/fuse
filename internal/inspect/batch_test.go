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

func TestScanBatch_CommentSkipping_REMVariants(t *testing.T) {
	content := []byte(`@REM certutil -decode payload.b64 payload.exe
REM
REM	certutil -decode payload.b64 payload.exe
  REM   net user evil dummyP@ssw0rd! /add
`)

	signals := ScanBatch(content)
	if len(signals) != 0 {
		t.Fatalf("expected 0 signals for REM variants, got %#v", signals)
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

func TestScanBatch_ReconstructsCaretContinuation(t *testing.T) {
	content := []byte(`@echo off
schtasks ^
  /create /tn Evil /tr C:\Temp\evil.exe /sc onlogon
reg ^
  add HKCU\Software\Microsoft\Windows\CurrentVersion\Run /v Evil /d C:\Temp\evil.exe
`)

	signals := ScanBatch(content)
	categories := batchSignalCategories(signals)
	if !categories["persistence"] {
		t.Fatalf("expected persistence signals with caret continuation, got %#v", signals)
	}
	if !categories["registry_modify"] {
		t.Fatalf("expected registry_modify with caret continuation, got %#v", signals)
	}
}

func TestScanBatch_CertutilAllowlistedModeNotEscalated(t *testing.T) {
	content := []byte(`@echo off
certutil -hashfile payload.exe SHA256
`)

	signals := ScanBatch(content)
	for _, s := range signals {
		if s.Category == "lolbin" {
			t.Fatalf("expected no lolbin signal for allow-listed certutil mode, got %#v", signals)
		}
	}
}

func batchSignalCategories(signals []Signal) map[string]bool {
	cats := make(map[string]bool)
	for _, s := range signals {
		cats[s.Category] = true
	}
	return cats
}
