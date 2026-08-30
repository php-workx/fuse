package cli

import "fmt"

const (
	checkNameNativeNotifications     = "Native approval notifications"
	checkNameNativeNotificationsLive = "Native notification live focus"
	checkNameNativeNotificationTerminal = "Native notification terminal target"
)

type notifierState string

const (
	notifierStateMissing      notifierState = "missing"
	notifierStateTampered     notifierState = "tampered"
	notifierStateIncompatible notifierState = "incompatible"
	notifierStateAdHoc        notifierState = "ad_hoc"
	notifierStateDenied       notifierState = "denied"
	notifierStateHealthy      notifierState = "healthy"
	notifierStateUnsupported  notifierState = "unsupported"
	notifierStateSuppressed   notifierState = "suppressed"
	notifierStateUnverified   notifierState = "unverified"
)

type notifierDiagnostic struct {
	state  notifierState
	detail string
}
func notificationDiagnosticResult(diagnostic notifierDiagnostic) checkResult {
	result := checkResult{name: checkNameNativeNotifications, detail: diagnostic.detail}
	switch diagnostic.state {
	case notifierStateMissing:
		result.status = "WARN"
		result.detail = fmt.Sprintf("helper not found: %s", diagnostic.detail)
	case notifierStateTampered:
		result.status = "FAIL"
		result.detail = fmt.Sprintf("helper signature or bundle integrity check failed: %s", diagnostic.detail)
	case notifierStateIncompatible:
		result.status = "FAIL"
		result.detail = fmt.Sprintf("helper protocol or bundle compatibility check failed: %s", diagnostic.detail)
	case notifierStateAdHoc:
		result.status = "WARN"
		result.detail = fmt.Sprintf("%s; privacy grants can reset after each ad-hoc rebuild", diagnostic.detail)
	case notifierStateDenied, notifierStateSuppressed, notifierStateUnverified:
		result.status = "WARN"
	case notifierStateUnsupported, notifierStateHealthy:
		result.status = "PASS"
	default:
		result.status = "FAIL"
		result.detail = fmt.Sprintf("unknown notifier diagnostic state %q: %s", diagnostic.state, diagnostic.detail)
	}
	return result
}

func liveNotificationWarning() string {
	return "warning: --live may trigger a macOS Automation permission prompt while testing terminal focus"
}

func notificationLiveDiagnosticResult(diagnostic notifierDiagnostic) checkResult {
	result := notificationDiagnosticResult(diagnostic)
	result.name = checkNameNativeNotificationsLive
	return result
}

func notifierFocusDiagnostic(responseType, mode, detail string) notifierDiagnostic {
	if responseType != "focus_result" {
		return notifierDiagnostic{state: notifierStateIncompatible, detail: "live focus returned " + responseType}
	}
	switch mode {
	case "exact":
		return notifierDiagnostic{state: notifierStateHealthy, detail: "exact terminal focus verified: " + detail}
	case "dispatched":
		return notifierDiagnostic{state: notifierStateUnverified, detail: "terminal focus dispatched but exact focus is unverified: " + detail}
	default:
		return notifierDiagnostic{state: notifierStateUnverified, detail: "application fallback focus; exact focus is unverified: " + detail}
	}
}
