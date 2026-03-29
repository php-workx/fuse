//go:build windows

package adapters

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"time"

	"github.com/php-workx/fuse/internal/core"
)

// executeShellCommand runs a shell command with safety controls on Windows.
// It uses DetectShellType to choose between PowerShell and cmd.exe.
// Signal forwarding and process group (job object) management not yet supported on Windows (planned: Phase 4).
func executeShellCommand(command, cwd string, timeout time.Duration) (int, error) {
	ctx := context.Background()
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	cmd := buildWindowsCommand(ctx, command)
	cmd.Dir = cwd
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = BuildChildEnv(os.Environ())
	cmd.SysProcAttr = platformSysProcAttr()

	if err := cmd.Run(); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return 1, fmt.Errorf("command timed out: %w", ctxErr)
		}
		if exitErr, ok := err.(*exec.ExitError); ok {
			return exitErr.ExitCode(), nil
		}
		return 1, fmt.Errorf("run command: %w", err)
	}
	return 0, nil
}

// executeCapturedShellCommandWithStdin runs a shell command on Windows,
// capturing stdout and stderr into a commandExecution result.
func executeCapturedShellCommandWithStdin(ctx context.Context, command, cwd string, timeout time.Duration, stdin io.Reader) (commandExecution, error) {
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	cmd := buildWindowsCommand(ctx, command)
	cmd.Dir = cwd
	cmd.Stdin = stdin
	cmd.Env = BuildChildEnv(os.Environ())
	cmd.SysProcAttr = platformSysProcAttr()

	var stdoutBuf bytes.Buffer
	var stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	if err := cmd.Run(); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return commandExecution{
				Stdout:   stdoutBuf.String(),
				Stderr:   stderrBuf.String(),
				ExitCode: -1,
			}, fmt.Errorf("command timed out: %w", ctxErr)
		}
		if exitErr, ok := err.(*exec.ExitError); ok {
			return commandExecution{
				Stdout:   stdoutBuf.String(),
				Stderr:   stderrBuf.String(),
				ExitCode: exitErr.ExitCode(),
			}, nil
		}
		return commandExecution{
			Stdout: stdoutBuf.String(),
			Stderr: stderrBuf.String(),
		}, fmt.Errorf("run command: %w", err)
	}

	return commandExecution{
		Stdout:   stdoutBuf.String(),
		Stderr:   stderrBuf.String(),
		ExitCode: 0,
	}, nil
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
