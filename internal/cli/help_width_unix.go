//go:build unix

package cli

import (
	"os"

	"golang.org/x/sys/unix"
)

func terminalWidth() int {
	ws, err := unix.IoctlGetWinsize(int(os.Stdout.Fd()), unix.TIOCGWINSZ)
	if err != nil || ws.Col == 0 {
		return 80
	}
	return int(ws.Col)
}

func isTerminal(fd int) bool {
	_, err := unix.IoctlGetWinsize(fd, unix.TIOCGWINSZ)
	return err == nil
}

// supportsANSI returns true if the terminal supports ANSI escape sequences.
// All modern Unix terminals support ANSI.
func supportsANSI() bool {
	return true
}
