//go:build unix

package cli

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"syscall"

	"golang.org/x/sys/unix"

	"github.com/php-workx/fuse/internal/adapters"
)

func checkLiveTTYAccess() checkResult {
	if ok, detail := liveTerminalProbeReady(); !ok {
		return checkResult{
			name:   "Live terminal /dev/tty access",
			status: "WARN",
			detail: "/dev/tty not probed: " + detail,
		}
	}

	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return checkResult{
			name:   "Live terminal /dev/tty access",
			status: "WARN",
			detail: fmt.Sprintf("cannot open /dev/tty: %v", err),
		}
	}
	_ = tty.Close()
	return checkResult{
		name:   "Live terminal /dev/tty access",
		status: "PASS",
		detail: "/dev/tty opened successfully",
	}
}

func checkLiveRawMode() checkResult {
	if ok, detail := liveTerminalProbeReady(); !ok {
		return checkResult{
			name:   checkNameLiveRawMode,
			status: "WARN",
			detail: "raw mode not probed: " + detail,
		}
	}

	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return checkResult{
			name:   checkNameLiveRawMode,
			status: "WARN",
			detail: fmt.Sprintf("cannot open /dev/tty: %v", err),
		}
	}
	defer func() { _ = tty.Close() }()

	fd := int(tty.Fd())
	orig, err := unix.IoctlGetTermios(fd, doctorIoctlGetTermios)
	if err != nil {
		return checkResult{
			name:   checkNameLiveRawMode,
			status: "WARN",
			detail: fmt.Sprintf("raw mode not available: %v", err),
		}
	}
	raw := *orig
	raw.Lflag &^= unix.ICANON | unix.ECHO
	if len(raw.Cc) > unix.VMIN {
		raw.Cc[unix.VMIN] = 1
	}
	if len(raw.Cc) > unix.VTIME {
		raw.Cc[unix.VTIME] = 0
	}
	if err := unix.IoctlSetTermios(fd, doctorIoctlSetTermios, &raw); err != nil {
		return checkResult{
			name:   checkNameLiveRawMode,
			status: "WARN",
			detail: fmt.Sprintf("enter raw mode: %v", err),
		}
	}
	if err := unix.IoctlSetTermios(fd, doctorIoctlSetTermios, orig); err != nil {
		return checkResult{
			name:   checkNameLiveRawMode,
			status: "WARN",
			detail: fmt.Sprintf("restore terminal state: %v", err),
		}
	}
	return checkResult{
		name:   checkNameLiveRawMode,
		status: "PASS",
		detail: "entered and restored raw mode on /dev/tty",
	}
}

func checkLiveForegroundProcessGroup() checkResult {
	if ok, detail := liveTerminalProbeReady(); !ok {
		return checkResult{
			name:   checkNameLiveForegroundHandoff,
			status: "WARN",
			detail: "foreground process-group handoff not probed: " + detail,
		}
	}

	cmd, err := startForegroundProbeProcess(os.Stdin, io.Discard, io.Discard)
	if err != nil {
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

	restore, err := adapters.ForegroundChildProcessGroupIfTTY(cmd.Process.Pid)
	if err != nil {
		return checkResult{
			name:   checkNameLiveForegroundHandoff,
			status: "FAIL",
			detail: fmt.Sprintf("handoff probe failed: %v", err),
		}
	}
	if restore != nil {
		restore()
	}
	return checkResult{
		name:   checkNameLiveForegroundHandoff,
		status: "PASS",
		detail: "foreground handoff to a child process group succeeded",
	}
}

func liveTerminalProbeReady() (bool, string) {
	fd := int(os.Stdin.Fd())
	foregroundPgrp, err := unix.IoctlGetInt(fd, unix.TIOCGPGRP)
	if err != nil {
		return false, "stdin is not a terminal"
	}
	if currentPgrp := unix.Getpgrp(); currentPgrp != foregroundPgrp {
		return false, "current process group is not foreground"
	}
	return true, ""
}

func startForegroundProbeProcess(stdin io.Reader, stdout, stderr io.Writer) (*exec.Cmd, error) {
	cmd := exec.Command("/bin/sh", "-c", "trap 'exit 0' TERM INT HUP; while :; do sleep 1; done")
	cmd.Stdin = stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return cmd, nil
}
