//go:build windows

package cli

func checkLiveTTYAccess() checkResult {
	return checkResult{
		name:   "Live terminal access",
		status: "SKIP",
		detail: "Windows Console API support not yet implemented (planned: Phase 3)",
	}
}

func checkLiveRawMode() checkResult {
	return checkResult{
		name:   checkNameLiveRawMode,
		status: "SKIP",
		detail: "Windows Console API support not yet implemented (planned: Phase 3)",
	}
}

func checkLiveForegroundProcessGroup() checkResult {
	return checkResult{
		name:   checkNameLiveForegroundHandoff,
		status: "SKIP",
		detail: "Windows job object support not yet implemented (planned: Phase 4)",
	}
}
