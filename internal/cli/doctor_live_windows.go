//go:build windows

package cli

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"

	"github.com/php-workx/fuse/internal/adapters"
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

// checkLiveForegroundProcessGroup verifies that job object creation and
// process assignment work. This is the Windows equivalent of the Unix
// foreground process group handoff check.
func checkLiveForegroundProcessGroup() checkResult {
	job, err := adapters.NewProbeJobObject()
	if err != nil {
		return checkResult{
			name:   checkNameLiveForegroundHandoff,
			status: "FAIL",
			detail: fmt.Sprintf("create job object: %v", err),
		}
	}
	defer adapters.CloseProbeJobObject(job)

	// Use ping as the probe command — unlike "timeout", it works in
	// non-interactive contexts (CI runners, SSH sessions, containers).
	cmd := exec.Command("cmd.exe", "/c", "ping -n 2 127.0.0.1 >nul")
	cmd.Stdin = nil
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP,
	}
	if err := cmd.Start(); err != nil {
		return checkResult{
			name:   checkNameLiveForegroundHandoff,
			status: "FAIL",
			detail: fmt.Sprintf("start probe child: %v", err),
		}
	}
	defer func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()

	if err := adapters.AssignProbeJobObject(job, cmd.Process.Pid); err != nil {
		return checkResult{
			name:   checkNameLiveForegroundHandoff,
			status: "FAIL",
			detail: fmt.Sprintf("job object assign failed: %v", err),
		}
	}

	return checkResult{
		name:   checkNameLiveForegroundHandoff,
		status: "PASS",
		detail: "job object creation and process assignment succeeded",
	}
}
