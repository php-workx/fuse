//go:build windows

package cli

import "golang.org/x/sys/windows"

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

// supportsANSI probes whether the console supports ANSI/VT escape sequences
// by attempting to enable ENABLE_VIRTUAL_TERMINAL_PROCESSING. Legacy conhost
// (pre-Windows 10 1511) does not support this flag and the call fails.
func supportsANSI() bool {
	conOut, err := windows.GetStdHandle(windows.STD_OUTPUT_HANDLE)
	if err != nil || conOut == windows.InvalidHandle {
		return false
	}
	var mode uint32
	if err := windows.GetConsoleMode(conOut, &mode); err != nil {
		return false
	}
	if err := windows.SetConsoleMode(conOut, mode|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING); err != nil {
		return false
	}
	_ = windows.SetConsoleMode(conOut, mode) // restore original
	return true
}
