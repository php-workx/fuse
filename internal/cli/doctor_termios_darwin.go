//go:build darwin

package cli

import "golang.org/x/sys/unix"

const (
	doctorIoctlGetTermios = unix.TIOCGETA
	doctorIoctlSetTermios = unix.TIOCSETA
)
