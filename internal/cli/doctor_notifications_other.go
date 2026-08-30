//go:build !darwin

package cli

import "runtime"

func notificationDoctorChecks(bool) []checkResult {
	return []checkResult{notificationDiagnosticResult(notifierDiagnostic{
		state:  notifierStateUnsupported,
		detail: "native approval notifications are unsupported on " + runtime.GOOS,
	})}
}
