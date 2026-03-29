//go:build windows

package cli

// terminalWidth returns a default width on Windows.
// Real terminal width detection not yet supported on Windows (planned: Phase 3).
func terminalWidth() int {
	return 80
}

// isTerminal returns false on Windows as a conservative default.
// Real terminal detection not yet supported on Windows (planned: Phase 3).
func isTerminal(_ int) bool {
	return false
}
