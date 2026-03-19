package adapters

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/runger/fuse/internal/config"
	"github.com/runger/fuse/internal/core"
)

var nativeClaudeFileTools = map[string]struct{}{
	"Read":      {},
	"Write":     {},
	"Edit":      {},
	"MultiEdit": {},
}

func isNativeClaudeFileTool(toolName string) bool {
	_, ok := nativeClaudeFileTools[toolName]
	return ok
}

func handleNativeFileTool(req HookRequest, stderr io.Writer, cfg *config.Config, dryRun bool) int {
	if len(req.ToolInput) == 0 {
		fmt.Fprintln(stderr, "fuse:POLICY_BLOCK STOP. Missing tool_input. Do not retry this exact command. Ask the user for guidance.")
		return 2
	}

	paths, err := extractNativeFilePaths(req.ToolName, req.ToolInput)
	if err != nil {
		fmt.Fprintln(stderr, "fuse:POLICY_BLOCK STOP. Invalid native file tool_input JSON. Do not retry this exact command. Ask the user for guidance.")
		return 2
	}
	if len(paths) == 0 {
		fmt.Fprintln(stderr, "fuse:POLICY_BLOCK STOP. Missing target file path. Do not retry this exact command. Ask the user for guidance.")
		return 2
	}

	result := classifyNativeFilePaths(req.ToolName, paths, req.Cwd)
	switch result.Decision {
	case core.DecisionSafe:
		return 0
	case core.DecisionBlocked:
		fmt.Fprintf(stderr, "fuse:POLICY_BLOCK STOP. %s Do not retry this exact command. Ask the user for guidance.\n", result.Reason)
		logHookEvent(req.SessionID, extractCommandFromResult(result), req.Cwd, result)
		if dryRun {
			return 0
		}
		return 2
	case core.DecisionApproval:
		return handleApproval(req, result, stderr, cfg, dryRun)
	default:
		fmt.Fprintln(stderr, "fuse:POLICY_BLOCK STOP. Unknown classification result. Do not retry this exact command. Ask the user for guidance.")
		return 2
	}
}

func extractNativeFilePaths(toolName string, raw json.RawMessage) ([]string, error) {
	var payload map[string]interface{}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}

	path := nativeTargetFilePath(toolName, payload)
	if path == "" {
		return nil, nil
	}
	return []string{path}, nil
}

func nativeTargetFilePath(toolName string, payload map[string]interface{}) string {
	switch toolName {
	case "Read", "Write", "Edit", "MultiEdit":
		if path, ok := payload["file_path"].(string); ok && strings.TrimSpace(path) != "" {
			return path
		}
		if path, ok := payload["path"].(string); ok && strings.TrimSpace(path) != "" {
			return path
		}
	default:
		return ""
	}
	return ""
}

func classifyNativeFilePaths(toolName string, paths []string, cwd string) *core.ClassifyResult {
	result := &core.ClassifyResult{
		Decision:    core.DecisionSafe,
		Reason:      "native file tool path is safe",
		DecisionKey: core.ComputeDecisionKey("claude-file", formatNativeFileCommand(toolName, paths), ""),
	}

	for _, path := range paths {
		decision, reason := classifyNativeFilePath(path, cwd)
		result.SubResults = append(result.SubResults, core.SubCommandResult{
			Command:  fmt.Sprintf("%s %s", toolName, path),
			Decision: decision,
			Reason:   reason,
		})
		if core.MaxDecision(result.Decision, decision) != result.Decision {
			result.Decision = decision
			result.Reason = reason
		}
	}

	return result
}

func classifyNativeFilePath(path, cwd string) (core.Decision, string) {
	info := nativeFilePathInfo(path, cwd)

	switch {
	case info.isUnder(config.BaseDir()):
		return core.DecisionBlocked, fmt.Sprintf("access to protected fuse path %s is blocked", path)
	case info.isUnder(filepath.Join(info.homeDir, ".fuse")):
		return core.DecisionBlocked, fmt.Sprintf("access to protected fuse path %s is blocked", path)
	case info.isClaudeSettingsPath():
		return core.DecisionBlocked, fmt.Sprintf("access to Claude settings path %s is blocked", path)
	case info.isCodexConfigPath():
		return core.DecisionBlocked, fmt.Sprintf("access to Codex config path %s is blocked", path)
	case (info.hasBase("fuse.db") || info.hasBase("secret.key")) &&
		(info.isUnder(config.BaseDir()) || info.isUnder(filepath.Join(info.homeDir, ".fuse"))):
		return core.DecisionBlocked, fmt.Sprintf("access to protected fuse state file %s is blocked", path)
	case info.isGitHookPath():
		return core.DecisionBlocked, fmt.Sprintf("access to git hooks path %s is blocked", path)
	case info.isEnvFile():
		return core.DecisionApproval, fmt.Sprintf("access to sensitive environment file %s requires approval", path)
	case info.isUnder(filepath.Join(cwd, "secrets")) || info.matchesRelative("secrets") || info.containsSegment("secrets"):
		return core.DecisionApproval, fmt.Sprintf("access to secret path %s requires approval", path)
	case info.isUnder(filepath.Join(info.homeDir, ".ssh")):
		return core.DecisionApproval, fmt.Sprintf("access to SSH path %s requires approval", path)
	case info.isUnder(filepath.Join(info.homeDir, ".aws")):
		return core.DecisionApproval, fmt.Sprintf("access to AWS credentials path %s requires approval", path)
	case info.isUnder(filepath.Join(info.homeDir, ".config", "gcloud")):
		return core.DecisionApproval, fmt.Sprintf("access to gcloud config path %s requires approval", path)
	case info.isUnder(filepath.Join(info.homeDir, ".azure")):
		return core.DecisionApproval, fmt.Sprintf("access to Azure config path %s requires approval", path)
	case info.hasBase("kubeconfig") || info.matchesAbsolute(filepath.Join(info.homeDir, ".kube", "config")):
		return core.DecisionApproval, fmt.Sprintf("access to Kubernetes config path %s requires approval", path)
	case info.isUnder(filepath.Join(info.homeDir, ".gnupg")):
		return core.DecisionApproval, fmt.Sprintf("access to GPG path %s requires approval", path)
	case info.isUnder(filepath.Join(info.homeDir, ".docker")):
		return core.DecisionApproval, fmt.Sprintf("access to Docker config path %s requires approval", path)
	case info.hasBase(".npmrc") || info.matchesAbsolute(filepath.Join(info.homeDir, ".npmrc")):
		return core.DecisionApproval, fmt.Sprintf("access to npm credentials %s requires approval", path)
	case info.hasBase(".pypirc") || info.matchesAbsolute(filepath.Join(info.homeDir, ".pypirc")):
		return core.DecisionApproval, fmt.Sprintf("access to PyPI credentials %s requires approval", path)
	case info.hasSensitiveExtension():
		return core.DecisionApproval, fmt.Sprintf("access to certificate or key file %s requires approval", path)
	default:
		return core.DecisionSafe, fmt.Sprintf("path %s is allowed", path)
	}
}

func formatNativeFileCommand(toolName string, paths []string) string {
	return toolName + " " + strings.Join(paths, " ")
}

type filePathInfo struct {
	raw      string
	cleanRaw string
	abs      string
	slashRaw string
	slashAbs string
	homeDir  string
	cwd      string
}

func nativeFilePathInfo(path, cwd string) filePathInfo {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		homeDir = "/nonexistent-home" // fail-closed: treat ~/ paths as unresolvable
	}
	cleanCwd := filepath.Clean(cwd)
	cleanRaw := filepath.Clean(path)
	resolved := cleanRaw
	if strings.HasPrefix(path, "~/") && homeDir != "" {
		resolved = filepath.Join(homeDir, strings.TrimPrefix(path, "~/"))
	}
	if !filepath.IsAbs(resolved) && cleanCwd != "" {
		resolved = filepath.Join(cleanCwd, resolved)
	}
	// Resolve symlinks to prevent bypass via symlinked paths
	// (e.g., safe.txt -> ~/.fuse/state/secret.key).
	// If the target doesn't exist yet, resolve the nearest existing parent
	// and recompose the path so containment checks use the real target.
	resolved = evalSymlinksWithFallback(resolved)
	return filePathInfo{
		raw:      path,
		cleanRaw: cleanRaw,
		abs:      filepath.Clean(resolved),
		slashRaw: filepath.ToSlash(cleanRaw),
		slashAbs: filepath.ToSlash(filepath.Clean(resolved)),
		homeDir:  evalSymlinksWithFallback(filepath.Clean(homeDir)),
		cwd:      cleanCwd,
	}
}

// evalSymlinksWithFallback resolves symlinks on the given path. If the target
// doesn't exist, it walks up to the nearest existing parent, resolves that,
// and recomposes the path with the remaining suffix.
func evalSymlinksWithFallback(path string) string {
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return resolved
	}
	// Walk up to the nearest existing parent.
	current := path
	var suffix string
	for {
		parent := filepath.Dir(current)
		if parent == current {
			return path // reached root without finding existing dir
		}
		suffix = filepath.Join(filepath.Base(current), suffix)
		if resolved, err := filepath.EvalSymlinks(parent); err == nil {
			return filepath.Join(resolved, suffix)
		}
		current = parent
	}
}

func (p *filePathInfo) matchesRelative(want string) bool {
	want = filepath.ToSlash(filepath.Clean(want))
	return p.slashRaw == want || strings.HasPrefix(p.slashRaw, want+"/")
}

func (p *filePathInfo) matchesAbsolute(want string) bool {
	if want == "" {
		return false
	}
	return p.slashAbs == filepath.ToSlash(filepath.Clean(want))
}

func (p *filePathInfo) containsSegment(segment string) bool {
	for _, s := range strings.Split(p.slashAbs, "/") {
		if s == segment {
			return true
		}
	}
	return false
}

func (p *filePathInfo) endsWithPathSuffix(suffix string) bool {
	suffix = filepath.ToSlash(filepath.Clean(suffix))
	return p.slashRaw == suffix ||
		p.slashAbs == suffix ||
		strings.HasSuffix(p.slashRaw, "/"+suffix) ||
		strings.HasSuffix(p.slashAbs, "/"+suffix)
}

func (p *filePathInfo) isClaudeSettingsPath() bool {
	return p.matchesAbsolute(filepath.Join(p.homeDir, ".claude", "settings.json")) ||
		p.endsWithPathSuffix(".claude/settings.json")
}

func (p *filePathInfo) isCodexConfigPath() bool {
	return p.matchesAbsolute(filepath.Join(p.homeDir, ".codex", "config.toml")) ||
		p.endsWithPathSuffix(".codex/config.toml")
}

func (p *filePathInfo) isUnder(base string) bool {
	if base == "" {
		return false
	}
	base = filepath.Clean(base)
	rel, err := filepath.Rel(base, p.abs)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

func (p *filePathInfo) hasBase(name string) bool {
	return filepath.Base(p.cleanRaw) == name || filepath.Base(p.abs) == name
}

func (p *filePathInfo) isGitHookPath() bool {
	return p.matchesRelative(".git/hooks") || strings.Contains(p.slashAbs, "/.git/hooks/")
}

func (p *filePathInfo) isEnvFile() bool {
	base := filepath.Base(p.cleanRaw)
	if base == "." || base == string(filepath.Separator) {
		base = filepath.Base(p.abs)
	}
	return base == ".env" || strings.HasPrefix(base, ".env.")
}

func (p *filePathInfo) hasSensitiveExtension() bool {
	// Check both the raw path and the resolved target (for symlinks).
	for _, path := range []string{p.cleanRaw, p.abs} {
		switch strings.ToLower(filepath.Ext(path)) {
		case ".pem", ".key", ".crt", ".p12", ".pfx", ".jks", ".keystore":
			return true
		}
	}
	return false
}
