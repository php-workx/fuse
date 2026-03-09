//go:build linux

package adapters

import (
	"syscall"

	"golang.org/x/sys/unix"
)

// ioctlGetTermios is the ioctl request code for getting terminal attributes.
// On Linux this is TCGETS.
const ioctlGetTermios = unix.TCGETS

// trustedPath returns the platform-specific trusted PATH for Linux.
func trustedPath() string {
	return "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
}

// platformSysProcAttr returns platform-specific SysProcAttr.
// Linux supports Pdeathsig for orphan cleanup.
func platformSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		Setpgid:   true,
		Pdeathsig: syscall.SIGTERM,
	}
}
