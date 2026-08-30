package cli

import (
	"strings"
	"testing"
)

func TestNotificationDiagnosticResultDistinguishesHelperStates(t *testing.T) {
	tests := []struct {
		name       string
		diagnostic notifierDiagnostic
		status     string
		detail     string
	}{
		{name: "missing", diagnostic: notifierDiagnostic{state: notifierStateMissing, detail: "no valid Fuse Notifier.app found"}, status: "WARN", detail: "not found"},
		{name: "tampered", diagnostic: notifierDiagnostic{state: notifierStateTampered, detail: "code object is not signed at all"}, status: "FAIL", detail: "signature"},
		{name: "incompatible", diagnostic: notifierDiagnostic{state: notifierStateIncompatible, detail: "protocol range 2-3 does not include 1"}, status: "FAIL", detail: "protocol"},
		{name: "ad hoc", diagnostic: notifierDiagnostic{state: notifierStateAdHoc, detail: "ad-hoc signature"}, status: "WARN", detail: "privacy grants can reset"},
		{name: "denied", diagnostic: notifierDiagnostic{state: notifierStateDenied, detail: "notification authorization is denied"}, status: "WARN", detail: "denied"},
		{name: "healthy", diagnostic: notifierDiagnostic{state: notifierStateHealthy, detail: "authorized; alerts enabled; sound enabled"}, status: "PASS", detail: "authorized"},
		{name: "custom home suppression", diagnostic: notifierDiagnostic{state: notifierStateSuppressed, detail: "native notifications are disabled for custom FUSE_HOME"}, status: "WARN", detail: "custom fuse_home"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := notificationDiagnosticResult(tt.diagnostic)
			if result.name != checkNameNativeNotifications {
				t.Fatalf("result name = %q, want %q", result.name, checkNameNativeNotifications)
			}
			if result.status != tt.status {
				t.Fatalf("result status = %q, want %q; detail = %q", result.status, tt.status, result.detail)
			}
			if !strings.Contains(strings.ToLower(result.detail), strings.ToLower(tt.detail)) {
				t.Fatalf("result detail = %q, want %q", result.detail, tt.detail)
			}
		})
	}
}

func TestNotificationDiagnosticResultReportsUnsupportedPlatformsWithoutFailure(t *testing.T) {
	result := notificationDiagnosticResult(notifierDiagnostic{state: notifierStateUnsupported, detail: "native notifications are unsupported on linux"})
	if result.status != "PASS" {
		t.Fatalf("unsupported notification diagnostic status = %q, want PASS", result.status)
	}
	if !strings.Contains(result.detail, "unsupported") {
		t.Fatalf("unsupported notification diagnostic detail = %q", result.detail)
	}
}

func TestNotificationDiagnosticLiveResultNamesExplicitLiveCheck(t *testing.T) {
	result := notificationLiveDiagnosticResult(notifierDiagnostic{state: notifierStateHealthy, detail: "target focus verified"})
	if result.name != checkNameNativeNotificationsLive {
		t.Fatalf("live result name = %q, want %q", result.name, checkNameNativeNotificationsLive)
	}
	if result.status != "PASS" {
		t.Fatalf("live result status = %q, want PASS", result.status)
	}
}

func TestLiveNotificationWarningExplainsAutomationPromptRisk(t *testing.T) {
	warning := liveNotificationWarning()
	if !strings.Contains(warning, "Automation") || !strings.Contains(warning, "prompt") {
		t.Fatalf("live notification warning = %q, want Automation prompt risk", warning)
	}
}

func TestNotifierFocusDiagnosticRequiresExactModeForPass(t *testing.T) {
	tests := []struct {
		mode   string
		status string
		detail string
	}{
		{mode: "exact", status: "PASS", detail: "verified"},
		{mode: "dispatched", status: "WARN", detail: "unverified"},
		{mode: "application_fallback", status: "WARN", detail: "fallback"},
	}
	for _, tt := range tests {
		t.Run(tt.mode, func(t *testing.T) {
			result := notificationLiveDiagnosticResult(notifierFocusDiagnostic("focus_result", tt.mode, "terminal response"))
			if result.status != tt.status {
				t.Fatalf("focus status = %q, want %q; detail = %q", result.status, tt.status, result.detail)
			}
			if !strings.Contains(strings.ToLower(result.detail), tt.detail) {
				t.Fatalf("focus detail = %q, want %q", result.detail, tt.detail)
			}
		})
	}
}
