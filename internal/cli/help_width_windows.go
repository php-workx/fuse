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
