//go:build windows

package adapters

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"time"

	"golang.org/x/sys/windows"

	"github.com/php-workx/fuse/internal/core"
)

// executeShellCommand runs a shell command with safety controls on Windows.
// It creates a job object so the entire child process tree is terminated on
// timeout or parent exit, and forwards console Ctrl events to the child.
func executeShellCommand(command, cwd string, timeout time.Duration) (int, error) {
	ctx := context.Background()
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	// Fail-closed: if job object creation fails (e.g., restrictive parent
	// job in CI), refuse to execute. Run 'fuse doctor --security' to diagnose.
	job, err := newJobObject()
	if err != nil {
		return -1, fmt.Errorf("create job object (run 'fuse doctor --security' to diagnose): %w", err)
	}
	defer job.close()

	cmd := buildWindowsCommand(ctx, command)
	cmd.Dir = cwd
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = BuildChildEnv(os.Environ())
	cmd.SysProcAttr = platformSysProcAttr()

	// On context timeout, terminate the entire job (all child processes).
	cmd.Cancel = func() error {
		return job.terminate(1)
	}
	cmd.WaitDelay = 2 * time.Second

	if err := cmd.Start(); err != nil {
		return -1, fmt.Errorf("start command: %w", err)
	}

	// Race window: between cmd.Start() and job.assign(), the child is not
	// tracked. This is sub-millisecond and accepted — CREATE_SUSPENDED would
	// close it but Go's exec.Cmd doesn't expose the thread handle.
	if err := job.assign(cmd.Process.Pid); err != nil {
		slog.Warn("job object assign failed, child process tree will not be contained", "pid", cmd.Process.Pid, "err", err)
	}

	exitCode, waitErr := waitForManagedCommand(cmd)
	if ctxErr := ctx.Err(); ctxErr != nil {
		return -1, fmt.Errorf("command timed out: %w", ctxErr)
	}
	return exitCode, waitErr
}

// executeCapturedShellCommandWithStdin runs a shell command on Windows,
// capturing stdout and stderr into a commandExecution result.
func executeCapturedShellCommandWithStdin(ctx context.Context, command, cwd string, timeout time.Duration, stdin io.Reader) (commandExecution, error) {
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	job, err := newJobObject()
	if err != nil {
		return commandExecution{ExitCode: -1}, fmt.Errorf("create job object: %w", err)
	}
	defer job.close()

	cmd := buildWindowsCommand(ctx, command)
	cmd.Dir = cwd
	cmd.Stdin = stdin
	cmd.Env = BuildChildEnv(os.Environ())
	cmd.SysProcAttr = platformSysProcAttr()

	cmd.Cancel = func() error {
		return job.terminate(1)
	}
	cmd.WaitDelay = 2 * time.Second

	var stdoutBuf strings.Builder
	var stderrBuf strings.Builder
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	if err := cmd.Start(); err != nil {
		return commandExecution{ExitCode: -1}, fmt.Errorf("start command: %w", err)
	}

	// Race window: between cmd.Start() and job.assign(), the child is not
	// tracked. This is sub-millisecond and accepted — CREATE_SUSPENDED would
	// close it but Go's exec.Cmd doesn't expose the thread handle.
	if err := job.assign(cmd.Process.Pid); err != nil {
		slog.Warn("job object assign failed, child process tree will not be contained", "pid", cmd.Process.Pid, "err", err)
	}

	exitCode, waitErr := waitForManagedCommand(cmd)
	if ctxErr := ctx.Err(); ctxErr != nil {
		return commandExecution{
			Stdout:   stdoutBuf.String(),
			Stderr:   stderrBuf.String(),
			ExitCode: -1,
		}, fmt.Errorf("command timed out: %w", ctxErr)
	}
	if waitErr != nil {
		return commandExecution{
			Stdout:   stdoutBuf.String(),
			Stderr:   stderrBuf.String(),
			ExitCode: exitCode,
		}, waitErr
	}
	return commandExecution{
		Stdout:   stdoutBuf.String(),
		Stderr:   stderrBuf.String(),
		ExitCode: exitCode,
	}, nil
}

// waitForManagedCommand forwards console Ctrl events to the child's process
// group and waits for the command to exit.
func waitForManagedCommand(cmd *exec.Cmd) (int, error) {
	// Forward console Ctrl events to the child process group.
	// signal.Notify suppresses Go's default Ctrl+C handler, so fuse itself
	// survives the interrupt and can forward it to the child.
	sigCh := make(chan os.Signal, 1)
	done := make(chan struct{})
	signal.Notify(sigCh, os.Interrupt)
	go forwardConsoleCtrl(uint32(cmd.Process.Pid), sigCh, done)

	waitErr := cmd.Wait()
	signal.Stop(sigCh)
	close(done)

	return interpretWaitError(waitErr)
}

// forwardConsoleCtrl relays Ctrl+C / Ctrl+Break to the child process group
// until done is closed. This is the Windows equivalent of the Unix
// forwardSignals function.
//
// Because the child was started with CREATE_NEW_PROCESS_GROUP,
// GenerateConsoleCtrlEvent targets only the child's group.
func forwardConsoleCtrl(childPID uint32, sigCh <-chan os.Signal, done <-chan struct{}) {
	for {
		select {
		case <-sigCh:
			// CTRL_BREAK_EVENT is used because CTRL_C_EVENT is ignored by
			// processes started with CREATE_NEW_PROCESS_GROUP unless they
			// explicitly call SetConsoleCtrlHandler. CTRL_BREAK_EVENT is
			// always delivered. Note: unlike Unix SIGINT, CTRL_BREAK_EVENT
			// triggers immediate termination by default — child processes
			// that don't register a handler will exit abruptly.
			if err := windows.GenerateConsoleCtrlEvent(windows.CTRL_BREAK_EVENT, childPID); err != nil {
				slog.Warn("GenerateConsoleCtrlEvent failed, child may not receive interrupt", "pid", childPID, "err", err)
			}
		case <-done:
			return
		}
	}
}

// interpretWaitError converts a cmd.Wait error into an exit code.
func interpretWaitError(waitErr error) (int, error) {
	if waitErr == nil {
		return 0, nil
	}
	var exitErr *exec.ExitError
	if errors.As(waitErr, &exitErr) {
		return exitErr.ExitCode(), nil
	}
	return -1, fmt.Errorf("wait for command: %w", waitErr)
}

// buildWindowsCommand creates an exec.Cmd for the detected shell type.
func buildWindowsCommand(ctx context.Context, command string) *exec.Cmd {
	shellType := core.DetectShellType(command)
	switch shellType {
	case core.ShellPowerShell:
		return exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-Command", command)
	case core.ShellCMD:
		return exec.CommandContext(ctx, "cmd.exe", "/c", command)
	default:
		slog.Debug("buildWindowsCommand: unrecognized shell type, defaulting to PowerShell", "shellType", shellType)
		return exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-Command", command)
	}
}
