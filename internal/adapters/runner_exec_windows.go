//go:build windows

package adapters

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"
)

// errWindowsNotSupported signals a permanent platform limitation (not a transient failure).
// Callers can use errors.Is(err, errWindowsNotSupported) to suppress retries.
var errWindowsNotSupported = errors.New("not yet supported on Windows")

func executeShellCommand(command, cwd string, timeout time.Duration) (int, error) {
	return 1, fmt.Errorf("fuse run: %w (planned: Phase 2)", errWindowsNotSupported)
}

func executeCapturedShellCommandWithStdin(ctx context.Context, command, cwd string, timeout time.Duration, stdin io.Reader) (commandExecution, error) {
	return commandExecution{}, fmt.Errorf("shell execution: %w (planned: Phase 2)", errWindowsNotSupported)
}
