//go:build windows

package cli

import (
	"sync"

	"golang.org/x/sys/windows"
)

func terminalWidth() int {
	conOut, err := windows.GetStdHandle(windows.STD_OUTPUT_HANDLE)
	if err != nil || conOut == windows.InvalidHandle {
		return 80
	}
	var info windows.ConsoleScreenBufferInfo
	if err := windows.GetConsoleScreenBufferInfo(conOut, &info); err != nil {
		return 80
	}
	width := int(info.Window.Right - info.Window.Left + 1)
	if width > 0 {
		return width
	}
	return 80
}

func isTerminal(fd int) bool {
	var mode uint32
	return windows.GetConsoleMode(windows.Handle(fd), &mode) == nil
}

var (
	ansiOnce      sync.Once
	ansiSupported bool
)

// supportsANSI probes whether the console supports ANSI/VT escape sequences
// by enabling ENABLE_VIRTUAL_TERMINAL_PROCESSING. If the flag is accepted,
// VT processing is left enabled so subsequent ANSI output renders correctly.
// Legacy conhost (pre-Windows 10 1511) does not support this flag and the
// call fails. The probe runs exactly once per process via sync.Once.
func supportsANSI() bool {
	ansiOnce.Do(func() {
		conOut, err := windows.GetStdHandle(windows.STD_OUTPUT_HANDLE)
		if err != nil || conOut == windows.InvalidHandle {
			return
		}
		var mode uint32
		if err := windows.GetConsoleMode(conOut, &mode); err != nil {
			return
		}
		if err := windows.SetConsoleMode(conOut, mode|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING); err != nil {
			return
		}
		// VT processing is intentionally left enabled — restoring the
		// original mode would cause ANSI codes to render as raw text.
		ansiSupported = true
	})
	return ansiSupported
}
