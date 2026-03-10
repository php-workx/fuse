//go:build linux

package cli

import "golang.org/x/sys/unix"

const doctorIoctlGetTermios = unix.TCGETS
const doctorIoctlSetTermios = unix.TCSETS
