package approve

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"regexp"
	"sync"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

// reControlChars matches ANSI escape sequences and non-printable control characters.
// Used to prevent terminal injection via crafted command strings in approval prompts.
var reControlChars = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]|\x1b\][^\x07]*\x07|[\x00-\x08\x0b\x0c\x0e-\x1f\x7f]`)

func sanitizePrompt(s string) string {
	return reControlChars.ReplaceAllString(s, "")
}

var errNonInteractive = fmt.Errorf("fuse:NON_INTERACTIVE_MODE STOP. Approval requires an interactive terminal (/dev/tty unavailable)")

// ttyMu serializes concurrent TTY approval prompts. Without this, two
// goroutines could both open /dev/tty and fight over raw mode and keystrokes.
var ttyMu sync.Mutex

// PromptUser shows a TUI approval prompt on /dev/tty.
// Returns the user's decision (approved bool), chosen scope, and any error.
// hookMode: true = short TTY prompt timeout (25s), false = 5min timeout.
// Note: the TTY prompt timeout is intentionally shorter than the hook timeout
// (300s) because the DB poll continues after the prompt times out.
// The ctx is checked in the polling loop; cancellation denies immediately.
func PromptUser(ctx context.Context, command, reason string, hookMode, nonInteractive bool) (approved bool, scope string, err error) {
	// Fast path: non-interactive mode returns immediately without locking.
	if nonInteractive || os.Getenv("FUSE_NON_INTERACTIVE") != "" {
		return false, "", errNonInteractive
	}

	ttyMu.Lock()
	defer ttyMu.Unlock()

	tty, err := openTTY(false) // already checked non-interactive above
	if err != nil {
		return false, "", err
	}
	defer func() { _ = tty.Close() }()

	fd := int(tty.Fd())

	// Save original terminal state.
	origTermios, err := unix.IoctlGetTermios(fd, ioctlGetTermios)
	if err != nil {
		return false, "", fmt.Errorf("get terminal state: %w", err)
	}

	// Restore terminal on panic.
	defer func() {
		if r := recover(); r != nil {
			_ = unix.IoctlSetTermios(fd, ioctlSetTermios, origTermios)
			fmt.Fprintf(os.Stderr, "fuse: prompt panic recovered: %v\n", r)
			approved = false
			scope = ""
			err = fmt.Errorf("prompt panic: %v", r)
		}
	}()

	// Set up signal handling to restore terminal.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	defer signal.Stop(sigCh)

	// Enter raw mode (no echo, no canonical, single char read).
	rawTermios := *origTermios
	rawTermios.Lflag &^= unix.ECHO | unix.ICANON | unix.ISIG
	rawTermios.Cc[unix.VMIN] = 0  // Non-blocking with VTIME.
	rawTermios.Cc[unix.VTIME] = 1 // 100ms read timeout for polling.
	if err := unix.IoctlSetTermios(fd, ioctlSetTermios, &rawTermios); err != nil {
		return false, "", fmt.Errorf("set raw mode: %w", err)
	}

	// Ensure terminal is always restored.
	restoreTerminal := func() {
		_ = unix.IoctlSetTermios(fd, ioctlSetTermios, origTermios)
	}
	defer restoreTerminal()

	// Determine timeout.
	timeout := 5 * time.Minute
	if hookMode {
		timeout = 25 * time.Second
	}

	// Render the prompt and read the user's decision.
	renderPrompt(tty, command, reason)
	deadline := time.Now().Add(timeout)
	return readApprovalDecision(ctx, tty, fd, deadline, sigCh)
}

// readApprovalDecision polls the TTY for the user's approve/deny decision.
func readApprovalDecision(ctx context.Context, tty *os.File, fd int, deadline time.Time, sigCh <-chan os.Signal) (bool, string, error) {
	buf := make([]byte, 1)

	for {
		select {
		case <-ctx.Done():
			fmt.Fprintf(tty, "\n  Denied (shutdown).\n\n")
			return false, "", nil
		case <-sigCh:
			fmt.Fprintf(tty, "\n  Denied (signal received).\n\n")
			return false, "", nil
		default: // non-blocking: fall through to deadline + read
		}

		if time.Now().After(deadline) {
			fmt.Fprintf(tty, "\n  Denied (timeout).\n")
			fmt.Fprintf(tty, "  fuse:TIMEOUT_WAITING_FOR_USER STOP. The user did not approve this action in time. Do not retry this exact command.\n\n")
			return false, "", nil
		}

		n, err := tty.Read(buf)
		if err != nil || n == 0 {
			continue
		}

		ch := buf[0]

		if ch == 3 { // Ctrl-C
			fmt.Fprintf(tty, "\n  Denied (Ctrl-C).\n\n")
			return false, "", nil
		}

		switch ch {
		case 'a', 'A', 'y', 'Y':
			fmt.Fprintf(tty, "\n  Approved. Select scope:\n")
			fmt.Fprintf(tty, "    [o] once  |  [c] command  |  [s] session  |  [f] forever\n")
			fmt.Fprintf(tty, "  > ")

			scopeResult, denied := readScope(ctx, tty, fd, deadline, sigCh)
			if denied {
				return false, "", nil
			}
			fmt.Fprintf(tty, "\n  Scope: %s\n\n", scopeResult)
			return true, scopeResult, nil

		case 'd', 'D', 'n', 'N':
			fmt.Fprintf(tty, "\n  Denied.\n\n")
			return false, "", nil

		default:
			fmt.Fprintf(tty, "\r  Press: [a]pprove or [d]eny  ")
		}
	}
}

// openTTY opens /dev/tty for interactive prompts.
// Returns errNonInteractive if FUSE_NON_INTERACTIVE is set or /dev/tty is unavailable.
func openTTY(nonInteractive bool) (*os.File, error) {
	if nonInteractive || os.Getenv("FUSE_NON_INTERACTIVE") != "" {
		return nil, errNonInteractive
	}
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return nil, errNonInteractive
	}
	return tty, nil
}

// readScope reads the scope selection from the user.
// Returns the scope string and whether the user denied.
func readScope(ctx context.Context, tty *os.File, fd int, deadline time.Time, sigCh <-chan os.Signal) (string, bool) {
	buf := make([]byte, 1)
	for {
		select {
		case <-ctx.Done():
			fmt.Fprintf(tty, "\n  Denied (shutdown).\n\n")
			return "", true
		case <-sigCh:
			fmt.Fprintf(tty, "\n  Denied (signal received).\n\n")
			return "", true
		default:
		}

		if time.Now().After(deadline) {
			fmt.Fprintf(tty, "\n  Denied (timeout).\n\n")
			return "", true
		}

		n, err := tty.Read(buf)
		if err != nil || n == 0 {
			continue
		}

		ch := buf[0]

		// Ctrl-C.
		if ch == 3 {
			fmt.Fprintf(tty, "\n  Denied (Ctrl-C).\n\n")
			return "", true
		}

		switch ch {
		case 'o', 'O':
			return "once", false
		case 'c', 'C':
			return "command", false
		case 's', 'S':
			return "session", false
		case 'f', 'F':
			return "forever", false
		default:
			fmt.Fprintf(tty, "\r  Scope: [o]nce [c]ommand [s]ession [f]orever  > ")
		}
	}
}

// renderPrompt writes the approval prompt to the tty.
func renderPrompt(tty *os.File, command, reason string) {
	// Get environment context.
	contextVars := getContextVars()
	cwd, _ := os.Getwd()

	fmt.Fprintf(tty, "\n")
	fmt.Fprintf(tty, "  \033[1;33m--- fuse: approval required ---\033[0m\n")
	fmt.Fprintf(tty, "\n")
	fmt.Fprintf(tty, "  \033[1mAgent requested:\033[0m %s\n", sanitizePrompt(command))
	fmt.Fprintf(tty, "  \033[1mCwd:\033[0m            %s\n", sanitizePrompt(cwd))
	fmt.Fprintf(tty, "  \033[1mRisk:\033[0m           APPROVAL\n")
	if reason != "" {
		fmt.Fprintf(tty, "  \033[1mReason:\033[0m         %s\n", sanitizePrompt(reason))
	}
	if contextVars != "" {
		fmt.Fprintf(tty, "  \033[1mContext:\033[0m        %s\n", sanitizePrompt(contextVars))
	}
	fmt.Fprintf(tty, "\n")
	fmt.Fprintf(tty, "  \033[1;32m[A]pprove\033[0m  |  \033[1;31m[D]eny\033[0m\n")
	fmt.Fprintf(tty, "  > ")
}

// getContextVars returns relevant environment variables for the prompt.
func getContextVars() string {
	relevantVars := []string{
		"AWS_PROFILE", "AWS_REGION", "AWS_DEFAULT_REGION",
		"TF_WORKSPACE", "TF_VAR_environment",
		"KUBECONFIG", "KUBECONTEXT",
		"GCP_PROJECT", "GOOGLE_CLOUD_PROJECT",
		"AZURE_SUBSCRIPTION",
	}

	var result string
	for _, v := range relevantVars {
		val := os.Getenv(v)
		if val != "" {
			if result != "" {
				result += ", "
			}
			result += v + "=" + val
		}
	}
	return result
}
