//go:build windows

package approve

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"time"

	"golang.org/x/sys/windows"
)

// ttyMu serializes concurrent console approval prompts. Without this, two
// goroutines could both open CONIN$/CONOUT$ and fight over console mode.
var ttyMu sync.Mutex

// PromptUser shows a TUI approval prompt on the Windows console (CONIN$/CONOUT$).
// Returns the user's decision (approved bool), chosen scope, and any error.
// hookMode: true = short prompt timeout (25s), false = 5min timeout.
func PromptUser(ctx context.Context, command, reason string, hookMode, nonInteractive bool) (approved bool, scope string, err error) {
	// Fast path: non-interactive mode returns immediately without locking.
	if nonInteractive || os.Getenv("FUSE_NON_INTERACTIVE") != "" {
		return false, "", errNonInteractive
	}

	// Use TryLock to avoid blocking on the mutex for minutes when another
	// approval prompt holds the lock. If the lock is unavailable, the DB poll
	// goroutine can still resolve the request via the TUI.
	if !ttyMu.TryLock() {
		return false, "", errNonInteractive
	}
	defer ttyMu.Unlock()

	conIn, conOut, err := openConsole(false) // already checked non-interactive above
	if err != nil {
		return false, "", err
	}
	defer func() { _ = conIn.Close() }()
	defer func() { _ = conOut.Close() }()

	inHandle := windows.Handle(conIn.Fd())

	// Save original console mode.
	var origMode uint32
	if err := windows.GetConsoleMode(inHandle, &origMode); err != nil {
		return false, "", fmt.Errorf("get console mode: %w", err)
	}

	// Restore console mode on panic.
	defer func() {
		if r := recover(); r != nil {
			_ = windows.SetConsoleMode(inHandle, origMode)
			fmt.Fprintf(os.Stderr, "fuse: prompt panic recovered: %v\n", r)
			approved = false
			scope = ""
			err = fmt.Errorf("prompt panic: %v", r)
		}
	}()

	// Set up signal handling.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt) // only os.Interrupt on Windows (no SIGTERM/SIGHUP)
	defer signal.Stop(sigCh)

	// Enter raw mode: clear line input, echo, processed input, mouse, and window events.
	rawMode := origMode &^ (windows.ENABLE_LINE_INPUT |
		windows.ENABLE_ECHO_INPUT |
		windows.ENABLE_PROCESSED_INPUT |
		windows.ENABLE_MOUSE_INPUT |
		windows.ENABLE_WINDOW_INPUT)
	if err := windows.SetConsoleMode(inHandle, rawMode); err != nil {
		return false, "", fmt.Errorf("set raw console mode: %w", err)
	}

	// Ensure console mode is always restored.
	restoreConsole := func() {
		_ = windows.SetConsoleMode(inHandle, origMode)
	}
	defer restoreConsole()

	// Flush any stale input before rendering the prompt.
	_ = windows.FlushConsoleInputBuffer(inHandle)

	// Determine timeout.
	timeout := 5 * time.Minute
	if hookMode {
		timeout = 25 * time.Second
	}

	// Render the prompt and read the user's decision.
	renderPrompt(conOut, command, reason)
	deadline := time.Now().Add(timeout)
	return readApprovalDecision(ctx, conIn, conOut, deadline, sigCh)
}

// openConsole opens CONIN$ and CONOUT$ for interactive prompts.
// Uses os.OpenFile (not GetStdHandle) for anti-spoofing: CONIN$/CONOUT$
// always refer to the real console, even if stdin/stdout are redirected.
func openConsole(nonInteractive bool) (conIn, conOut *os.File, err error) {
	if nonInteractive || os.Getenv("FUSE_NON_INTERACTIVE") != "" {
		return nil, nil, errNonInteractive
	}
	conIn, err = os.OpenFile("CONIN$", os.O_RDWR, 0)
	if err != nil {
		slog.Debug("failed to open CONIN$", "error", err)
		return nil, nil, errNonInteractive
	}
	conOut, err = os.OpenFile("CONOUT$", os.O_RDWR, 0)
	if err != nil {
		_ = conIn.Close()
		slog.Debug("failed to open CONOUT$", "error", err)
		return nil, nil, errNonInteractive
	}
	return conIn, conOut, nil
}

// readApprovalDecision polls the console for the user's approve/deny decision.
func readApprovalDecision(ctx context.Context, conIn, conOut *os.File, deadline time.Time, sigCh <-chan os.Signal) (bool, string, error) {
	inHandle := windows.Handle(conIn.Fd())
	buf := make([]byte, 1)

	for {
		select {
		case <-ctx.Done():
			fmt.Fprintf(conOut, "\n  Denied (shutdown).\n\n")
			return false, "", nil
		case <-sigCh:
			fmt.Fprintf(conOut, "\n  Denied (signal received).\n\n")
			return false, "", nil
		default: // non-blocking: fall through to deadline + read
		}

		if time.Now().After(deadline) {
			fmt.Fprintf(conOut, "\n  Timed out. The command remains pending — approve via fuse monitor.\n\n")
			return false, "", errPromptTimeout
		}

		// Wait up to 100ms for input to become available.
		event, _ := windows.WaitForSingleObject(inHandle, 100)
		if event != windows.WAIT_OBJECT_0 {
			continue // timeout or error — loop back to check ctx/deadline/signals
		}

		n, err := conIn.Read(buf)
		if err != nil {
			return false, "", fmt.Errorf("console read: %w", err)
		}
		if n == 0 {
			continue
		}

		ch := buf[0]

		// Ctrl-C arrives as byte 0x03 with ENABLE_PROCESSED_INPUT cleared.
		if ch == 3 {
			fmt.Fprintf(conOut, "\n  Denied (Ctrl-C).\n\n")
			return false, "", nil
		}

		switch ch {
		case 'a', 'A', 'y', 'Y':
			fmt.Fprintf(conOut, "\n  Approved. Select scope:\n")
			fmt.Fprintf(conOut, "    [o] once  |  [c] command  |  [s] session  |  [f] forever\n")
			fmt.Fprintf(conOut, "  > ")

			scopeResult, denied := readScope(ctx, conIn, conOut, deadline, sigCh)
			if denied {
				return false, "", nil
			}
			fmt.Fprintf(conOut, "\n  Scope: %s\n\n", scopeResult)
			return true, scopeResult, nil

		case 'd', 'D', 'n', 'N':
			fmt.Fprintf(conOut, "\n  Denied.\n\n")
			return false, "", nil

		default:
			fmt.Fprintf(conOut, "\r  Press: [a]pprove or [d]eny  ")
		}
	}
}

// readScope reads the scope selection from the user.
// Returns the scope string and whether the user denied.
func readScope(ctx context.Context, conIn, conOut *os.File, deadline time.Time, sigCh <-chan os.Signal) (string, bool) {
	inHandle := windows.Handle(conIn.Fd())
	buf := make([]byte, 1)

	for {
		select {
		case <-ctx.Done():
			fmt.Fprintf(conOut, "\n  Denied (shutdown).\n\n")
			return "", true
		case <-sigCh:
			fmt.Fprintf(conOut, "\n  Denied (signal received).\n\n")
			return "", true
		default:
		}

		if time.Now().After(deadline) {
			fmt.Fprintf(conOut, "\n  Denied (timeout).\n\n")
			return "", true
		}

		// Wait up to 100ms for input to become available.
		event, _ := windows.WaitForSingleObject(inHandle, 100)
		if event != windows.WAIT_OBJECT_0 {
			continue
		}

		n, err := conIn.Read(buf)
		if err != nil {
			slog.Debug("console read failed while selecting approval scope", "error", err)
			return "", true // console error — deny
		}
		if n == 0 {
			continue
		}

		ch := buf[0]

		// Ctrl-C.
		if ch == 3 {
			fmt.Fprintf(conOut, "\n  Denied (Ctrl-C).\n\n")
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
			fmt.Fprintf(conOut, "\r  Scope: [o]nce [c]ommand [s]ession [f]orever  > ")
		}
	}
}

// renderPrompt writes the approval prompt to the console output.
// Attempts ANSI color output first; falls back to plain text if VT processing
// is not available.
func renderPrompt(conOut *os.File, command, reason string) {
	outHandle := windows.Handle(conOut.Fd())

	// Try to enable ANSI/VT processing on the output handle.
	var outMode uint32
	if err := windows.GetConsoleMode(outHandle, &outMode); err == nil {
		if err := windows.SetConsoleMode(outHandle, outMode|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING); err == nil {
			// VT processing enabled — use ANSI colors.
			renderPromptANSI(conOut, command, reason)
			return
		}
	}

	// Fallback: plain text without ANSI escape sequences.
	renderPromptPlain(conOut, command, reason)
}

// renderPromptANSI writes the prompt with ANSI color escape sequences.
func renderPromptANSI(conOut *os.File, command, reason string) {
	contextVars := getContextVars()
	cwd, _ := os.Getwd()

	fmt.Fprintf(conOut, "\n")
	fmt.Fprintf(conOut, "  \033[1;33m--- fuse: approval required ---\033[0m\n")
	fmt.Fprintf(conOut, "\n")
	fmt.Fprintf(conOut, "  \033[1mAgent requested:\033[0m %s\n", sanitizePrompt(command))
	fmt.Fprintf(conOut, "  \033[1mCwd:\033[0m            %s\n", sanitizePrompt(cwd))
	fmt.Fprintf(conOut, "  \033[1mRisk:\033[0m           APPROVAL\n")
	if reason != "" {
		fmt.Fprintf(conOut, "  \033[1mReason:\033[0m         %s\n", sanitizePrompt(reason))
	}
	if contextVars != "" {
		fmt.Fprintf(conOut, "  \033[1mContext:\033[0m        %s\n", sanitizePrompt(contextVars))
	}
	fmt.Fprintf(conOut, "\n")
	fmt.Fprintf(conOut, "  \033[1;32m[A]pprove\033[0m  |  \033[1;31m[D]eny\033[0m\n")
	fmt.Fprintf(conOut, "  > ")
}

// renderPromptPlain writes the prompt without ANSI escape sequences.
func renderPromptPlain(conOut *os.File, command, reason string) {
	contextVars := getContextVars()
	cwd, _ := os.Getwd()

	fmt.Fprintf(conOut, "\n")
	fmt.Fprintf(conOut, "  --- fuse: approval required ---\n")
	fmt.Fprintf(conOut, "\n")
	fmt.Fprintf(conOut, "  Agent requested: %s\n", sanitizePrompt(command))
	fmt.Fprintf(conOut, "  Cwd:             %s\n", sanitizePrompt(cwd))
	fmt.Fprintf(conOut, "  Risk:            APPROVAL\n")
	if reason != "" {
		fmt.Fprintf(conOut, "  Reason:          %s\n", sanitizePrompt(reason))
	}
	if contextVars != "" {
		fmt.Fprintf(conOut, "  Context:         %s\n", sanitizePrompt(contextVars))
	}
	fmt.Fprintf(conOut, "\n")
	fmt.Fprintf(conOut, "  [A]pprove  |  [D]eny\n")
	fmt.Fprintf(conOut, "  > ")
}
