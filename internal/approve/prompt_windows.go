//go:build windows

package approve

import "context"

// PromptUser on Windows returns errNonInteractive immediately.
// This stub is only reached from run mode (runner.go handleApprovalCommand).
// Hook mode short-circuits at hook.go handleApproval before calling
// RequestApproval, so this function is never invoked from the hook path.
// Planned replacement: Phase 3 (Windows Console API prompts).
func PromptUser(_ context.Context, _, _ string, _, _ bool) (bool, string, error) {
	return false, "", errNonInteractive
}
