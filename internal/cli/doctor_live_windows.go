//go:build windows

package cli

func checkLiveTTYAccess() checkResult {
	return checkResult{
		name:   "Live terminal access",
		status: "SKIP",
		detail: "not yet supported on Windows (planned: Phase 3)",
	}
}

func checkLiveRawMode() checkResult {
	return checkResult{
		name:   checkNameLiveRawMode,
		status: "SKIP",
		detail: "not yet supported on Windows (planned: Phase 3)",
	}
}

func checkLiveForegroundProcessGroup() checkResult {
	return checkResult{
		name:   checkNameLiveForegroundHandoff,
		status: "SKIP",
		detail: "not yet supported on Windows (planned: Phase 4)",
	}
}
