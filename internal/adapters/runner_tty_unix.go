//go:build unix

package adapters

import (
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"golang.org/x/sys/unix"
)

// ttyOwnershipMu serializes foreground process group transfers.
// signal.Ignore/Reset(SIGTTOU) changes process-wide signal disposition;
// without this lock, concurrent codex-shell goroutines could race on
// the disposition between Ignore and Reset calls.
var ttyOwnershipMu sync.Mutex

// ForegroundChildProcessGroupIfTTY transfers foreground TTY ownership to the
// child process group if stdin is a terminal. Returns a restore function that
// should be deferred to restore the original foreground group.
func ForegroundChildProcessGroupIfTTY(pid int) (restore func(), err error) {
	fd := int(os.Stdin.Fd())

	// Check if stdin is a terminal using the platform-specific ioctl.
	if _, termErr := unix.IoctlGetTermios(fd, ioctlGetTermios); termErr != nil {
		// Not a terminal — nothing to do.
		return nil, nil //nolint:nilerr // termErr means not-a-tty, which is a valid no-op
	}

	ttyOwnershipMu.Lock()

	// Get the current foreground process group.
	origPgrp, err := unix.IoctlGetInt(fd, unix.TIOCGPGRP)
	if err != nil {
		ttyOwnershipMu.Unlock()
		return nil, fmt.Errorf("get foreground pgrp: %w", err)
	}

	// Suppress SIGTTOU during foreground group changes.
	signal.Ignore(syscall.SIGTTOU)

	// Set the child's process group as the foreground group.
	childPgrp := pid
	if err := unix.IoctlSetInt(fd, unix.TIOCSPGRP, childPgrp); err != nil {
		signal.Reset(syscall.SIGTTOU)
		ttyOwnershipMu.Unlock()
		return nil, fmt.Errorf("set child foreground pgrp: %w", err)
	}

	restore = func() {
		_ = unix.IoctlSetInt(fd, unix.TIOCSPGRP, origPgrp)
		signal.Reset(syscall.SIGTTOU)
		ttyOwnershipMu.Unlock()
	}

	return restore, nil // nosemgrep: missing-unlock-before-return -- unlock is in the restore closure, called by defer restore()
}
