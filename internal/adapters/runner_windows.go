//go:build windows

package adapters

import (
	"os"
	"syscall"
)

func trustedPath() string {
	systemRoot := os.Getenv("SystemRoot")
	if systemRoot == "" {
		systemRoot = `C:\Windows`
	}
	return systemRoot + `\System32;` + systemRoot + `;` + systemRoot + `\System32\Wbem`
}

// platformSysProcAttr returns Windows-specific process attributes.
// Not called in Phase 1 — all callers are in Unix-only files.
func platformSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{}
}
