//go:build windows

package adapters

import (
	"context"
	"fmt"
	"io"
	"time"
)

func executeShellCommand(command, cwd string, timeout time.Duration) (int, error) {
	return 1, fmt.Errorf("fuse run is not yet supported on Windows (planned: Phase 2)")
}

func executeCapturedShellCommandWithStdin(ctx context.Context, command, cwd string, timeout time.Duration, stdin io.Reader) (commandExecution, error) {
	return commandExecution{}, fmt.Errorf("shell execution is not yet supported on Windows (planned: Phase 2)")
}
