package approve

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

var errNonInteractive = fmt.Errorf("fuse:NON_INTERACTIVE_MODE STOP. Approval requires an interactive terminal (/dev/tty unavailable)")

// PromptUser shows a TUI approval prompt on /dev/tty.
// Returns the user's decision (approved bool), chosen scope, and any error.
// hookMode: true = 25s timeout, false = 5min timeout.
// openTTY opens /dev/tty for interactive prompts.
// Returns errNonInteractive if FUSE_NON_INTERACTIVE is set or /dev/tty is unavailable.
func openTTY() (*os.File, error) {
	if os.Getenv("FUSE_NON_INTERACTIVE") != "" {
		return nil, errNonInteractive
	}
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return nil, errNonInteractive
	}
	return tty, nil
}

func PromptUser(command, reason string, hookMode bool) (approved bool, scope string, err error) {
	tty, err := openTTY()
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

	// Render the prompt.
	renderPrompt(tty, command, reason)

	// Read approval decision.
	deadline := time.Now().Add(timeout)
	buf := make([]byte, 1)

	for {
		// Check for signals.
		select {
		case <-sigCh:
			fmt.Fprintf(tty, "\n  Denied (signal received).\n\n")
			return false, "", nil
		default:
		}

		// Check timeout.
		if time.Now().After(deadline) {
			fmt.Fprintf(tty, "\n  Denied (timeout).\n")
			fmt.Fprintf(tty, "  fuse:TIMEOUT_WAITING_FOR_USER STOP. The user did not approve this action in time. Do not retry this exact command.\n\n")
			return false, "", nil
		}

		n, err := tty.Read(buf)
		if err != nil || n == 0 {
			continue // VTIME timeout, retry.
		}

		ch := buf[0]

		// Ctrl-C.
		if ch == 3 {
			fmt.Fprintf(tty, "\n  Denied (Ctrl-C).\n\n")
			return false, "", nil
		}

		switch ch {
		case 'a', 'A', 'y', 'Y':
			fmt.Fprintf(tty, "\n  Approved. Select scope:\n")
			fmt.Fprintf(tty, "    [o] once  |  [c] command  |  [s] session  |  [f] forever\n")
			fmt.Fprintf(tty, "  > ")

			// Read scope selection.
			scopeResult, denied := readScope(tty, fd, deadline, sigCh)
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

// readScope reads the scope selection from the user.
// Returns the scope string and whether the user denied.
func readScope(tty *os.File, fd int, deadline time.Time, sigCh <-chan os.Signal) (string, bool) {
	buf := make([]byte, 1)
	for {
		select {
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
	fmt.Fprintf(tty, "  \033[1mAgent requested:\033[0m %s\n", command)
	fmt.Fprintf(tty, "  \033[1mCwd:\033[0m            %s\n", cwd)
	fmt.Fprintf(tty, "  \033[1mRisk:\033[0m           APPROVAL\n")
	if reason != "" {
		fmt.Fprintf(tty, "  \033[1mReason:\033[0m         %s\n", reason)
	}
	if contextVars != "" {
		fmt.Fprintf(tty, "  \033[1mContext:\033[0m        %s\n", contextVars)
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
