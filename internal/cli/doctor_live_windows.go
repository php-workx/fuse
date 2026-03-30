//go:build windows

package cli

import (
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

func checkLiveTTYAccess() checkResult {
	f, err := os.OpenFile("CONIN$", os.O_RDWR, 0)
	if err != nil {
		return checkResult{
			name:   "Live console CONIN$ access",
			status: "WARN",
			detail: fmt.Sprintf("cannot open CONIN$: %v", err),
		}
	}
	defer func() { _ = f.Close() }()

	var mode uint32
	if err := windows.GetConsoleMode(windows.Handle(f.Fd()), &mode); err != nil {
		return checkResult{
			name:   "Live console CONIN$ access",
			status: "WARN",
			detail: fmt.Sprintf("CONIN$ is not a console: %v", err),
		}
	}
	return checkResult{
		name:   "Live console CONIN$ access",
		status: "PASS",
		detail: "CONIN$ opened and verified as console",
	}
}

func checkLiveRawMode() checkResult {
	f, err := os.OpenFile("CONIN$", os.O_RDWR, 0)
	if err != nil {
		return checkResult{
			name:   checkNameLiveRawMode,
			status: "WARN",
			detail: fmt.Sprintf("cannot open CONIN$: %v", err),
		}
	}
	defer func() { _ = f.Close() }()

	handle := windows.Handle(f.Fd())

	var origMode uint32
	if err := windows.GetConsoleMode(handle, &origMode); err != nil {
		return checkResult{
			name:   checkNameLiveRawMode,
			status: "WARN",
			detail: fmt.Sprintf("raw mode not available: %v", err),
		}
	}

	rawMode := origMode &^ (windows.ENABLE_LINE_INPUT | windows.ENABLE_ECHO_INPUT)
	if err := windows.SetConsoleMode(handle, rawMode); err != nil {
		return checkResult{
			name:   checkNameLiveRawMode,
			status: "WARN",
			detail: fmt.Sprintf("enter raw mode: %v", err),
		}
	}

	if err := windows.SetConsoleMode(handle, origMode); err != nil {
		return checkResult{
			name:   checkNameLiveRawMode,
			status: "WARN",
			detail: fmt.Sprintf("restore console mode: %v", err),
		}
	}

	return checkResult{
		name:   checkNameLiveRawMode,
		status: "PASS",
		detail: "entered and restored raw mode on CONIN$",
	}
}

func checkLiveForegroundProcessGroup() checkResult {
	return checkResult{
		name:   checkNameLiveForegroundHandoff,
		status: "SKIP",
		detail: "Windows job object support not yet implemented (planned: Phase 4)",
	}
}
