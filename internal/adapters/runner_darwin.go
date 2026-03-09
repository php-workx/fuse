//go:build darwin

package adapters

import (
	"syscall"

	"golang.org/x/sys/unix"
)

// ioctlGetTermios is the ioctl request code for getting terminal attributes.
// On macOS this is TIOCGETA.
const ioctlGetTermios = unix.TIOCGETA

// trustedPath returns the platform-specific trusted PATH for macOS.
func trustedPath() string {
	return "/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin"
}

// platformSysProcAttr returns platform-specific SysProcAttr.
// macOS has no Pdeathsig.
func platformSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setpgid: true}
}
