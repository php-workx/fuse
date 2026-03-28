//go:build unix

package adapters

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

// executeShellCommand runs a shell command with safety controls (SS10.1).
func executeShellCommand(command, cwd string, timeout time.Duration) (int, error) {
	ctx := context.Background()
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	cmd := exec.CommandContext( // nosemgrep: dangerous-exec-command
		ctx, "/bin/sh", "-c", command,
	)
	cmd.Dir = cwd
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = BuildChildEnv(os.Environ())

	// Platform-specific SysProcAttr (Setpgid, optionally Pdeathsig on Linux).
	cmd.SysProcAttr = platformSysProcAttr()

	if err := cmd.Start(); err != nil {
		return -1, fmt.Errorf("start command: %w", err)
	}

	return waitForManagedCommand(cmd)
}

func executeCapturedShellCommandWithStdin(ctx context.Context, command, cwd string, timeout time.Duration, stdin io.Reader) (commandExecution, error) {
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	cmd := exec.CommandContext( // nosemgrep: dangerous-exec-command
		ctx, "/bin/sh", "-c", command,
	)
	cmd.Dir = cwd
	cmd.Stdin = stdin
	cmd.Env = BuildChildEnv(os.Environ())
	cmd.SysProcAttr = platformSysProcAttr()

	var stdoutBuf strings.Builder
	var stderrBuf strings.Builder
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	if err := cmd.Start(); err != nil {
		return commandExecution{}, fmt.Errorf("start command: %w", err)
	}

	exitCode, err := waitForManagedCommand(cmd)
	if err != nil {
		return commandExecution{
			Stdout: stdoutBuf.String(),
			Stderr: stderrBuf.String(),
		}, err
	}

	return commandExecution{
		Stdout:   stdoutBuf.String(),
		Stderr:   stderrBuf.String(),
		ExitCode: exitCode,
	}, nil
}

func waitForManagedCommand(cmd *exec.Cmd) (int, error) {
	// Transfer foreground TTY ownership to child process group.
	restoreTTY, err := ForegroundChildProcessGroupIfTTY(cmd.Process.Pid)
	if err != nil {
		_ = cmd.Process.Kill()
		return -1, fmt.Errorf("foreground child process group: %w", err)
	}
	if restoreTTY != nil {
		defer restoreTTY()
	}

	// Forward signals to child process group.
	sigCh := make(chan os.Signal, 1)
	done := make(chan struct{})
	signal.Notify(sigCh,
		syscall.SIGINT,
		syscall.SIGTERM,
		syscall.SIGHUP,
		syscall.SIGTSTP,  // job control (Ctrl+Z)
		syscall.SIGWINCH, // terminal resize
	)
	go forwardSignals(cmd.Process.Pid, sigCh, done)

	waitErr := cmd.Wait()
	signal.Stop(sigCh)
	close(done)

	return interpretWaitError(waitErr)
}

// forwardSignals relays OS signals to a child process group until done is closed.
func forwardSignals(pid int, sigCh <-chan os.Signal, done <-chan struct{}) {
	for {
		select {
		case sig, ok := <-sigCh:
			if !ok {
				return
			}
			sysSig, isSyscall := sig.(syscall.Signal)
			if !isSyscall {
				continue
			}
			// Send to process group (negative PID).
			if err := syscall.Kill(-pid, sysSig); err != nil {
				// Fallback: retry direct child PID.
				_ = syscall.Kill(pid, sysSig)
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
		if status, ok := exitErr.Sys().(syscall.WaitStatus); ok && status.Signaled() {
			return 128 + int(status.Signal()), nil
		}
		return exitErr.ExitCode(), nil
	}
	return -1, fmt.Errorf("wait for command: %w", waitErr)
}
