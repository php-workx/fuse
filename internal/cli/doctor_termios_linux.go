//go:build linux

package cli

import "golang.org/x/sys/unix"

const (
	doctorIoctlGetTermios = unix.TCGETS
	doctorIoctlSetTermios = unix.TCSETS
)
