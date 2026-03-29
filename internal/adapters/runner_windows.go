//go:build windows

package adapters

import (
	"os"
	"syscall"
)

func trustedPath() string {
	systemRoot := os.Getenv("SystemRoot")
	if systemRoot == "" || !isValidWindowsRoot(systemRoot) {
		systemRoot = `C:\Windows`
	}
	return systemRoot + `\System32;` + systemRoot + `;` + systemRoot + `\System32\Wbem`
}

// isValidWindowsRoot checks that the path looks like a real Windows root (e.g., C:\Windows).
// It rejects attacker-controlled values such as UNC paths, relative paths, or empty strings.
func isValidWindowsRoot(path string) bool {
	// Must be at least 3 chars (e.g., "C:\")
	if len(path) < 3 {
		return false
	}
	// First char must be a letter (drive letter)
	drive := path[0]
	if !((drive >= 'A' && drive <= 'Z') || (drive >= 'a' && drive <= 'z')) {
		return false
	}
	// Second char must be ':'
	if path[1] != ':' {
		return false
	}
	// Third char must be backslash
	if path[2] != '\\' {
		return false
	}
	return true
}

// platformSysProcAttr returns Windows-specific process attributes.
// Note: CREATE_NEW_PROCESS_GROUP and job object support deferred to Phase 4.
// Without these, child processes spawned by the shell may orphan on timeout.
func platformSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{}
}
