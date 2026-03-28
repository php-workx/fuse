//go:build windows

package approve

import "context"

// PromptUser is not yet implemented on Windows (planned: Phase 3).
// Returns errNonInteractive, which causes RequestApproval to fall back to
// DB polling. If no TUI resolves the request, the command is BLOCKED.
func PromptUser(_ context.Context, _, _ string, _, _ bool) (bool, string, error) {
	return false, "", errNonInteractive
}
