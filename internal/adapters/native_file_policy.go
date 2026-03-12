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

func handleNativeFileTool(req HookRequest, stderr io.Writer, cfg *config.Config) int {
	if len(req.ToolInput) == 0 {
		fmt.Fprintln(stderr, "fuse:POLICY_BLOCK STOP. Missing tool_input. Do not retry this exact command. Ask the user for guidance.")
		return 2
	}

	paths, err := extractNativeFilePaths(req.ToolInput)
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
		logHookEvent(req.SessionID, extractCommandFromResult(result), result)
		return 2
	case core.DecisionApproval:
		return handleApproval(req, result, stderr, cfg)
	default:
		fmt.Fprintln(stderr, "fuse:POLICY_BLOCK STOP. Unknown classification result. Do not retry this exact command. Ask the user for guidance.")
		return 2
	}
}

func extractNativeFilePaths(raw json.RawMessage) ([]string, error) {
	var payload interface{}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}

	var paths []string
	seen := make(map[string]struct{})
	collectNativeFilePaths(payload, seen, &paths)
	return paths, nil
}

func collectNativeFilePaths(value interface{}, seen map[string]struct{}, out *[]string) {
	switch typed := value.(type) {
	case map[string]interface{}:
		for key, nested := range typed {
			if (key == "file_path" || key == "path") && isUsableFilePath(nested) {
				path := nested.(string)
				if _, ok := seen[path]; !ok {
					seen[path] = struct{}{}
					*out = append(*out, path)
				}
			}
			collectNativeFilePaths(nested, seen, out)
		}
	case []interface{}:
		for _, item := range typed {
			collectNativeFilePaths(item, seen, out)
		}
	}
}

func isUsableFilePath(value interface{}) bool {
	path, ok := value.(string)
	return ok && strings.TrimSpace(path) != ""
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
	case info.matchesRelative(".claude/settings.json") || info.matchesAbsolute(filepath.Join(info.homeDir, ".claude", "settings.json")):
		return core.DecisionBlocked, fmt.Sprintf("access to Claude settings path %s is blocked", path)
	case info.matchesRelative(".codex/config.toml") || info.matchesAbsolute(filepath.Join(info.homeDir, ".codex", "config.toml")):
		return core.DecisionBlocked, fmt.Sprintf("access to Codex config path %s is blocked", path)
	case info.hasBase("fuse.db") || info.hasBase("secret.key"):
		return core.DecisionBlocked, fmt.Sprintf("access to protected fuse state file %s is blocked", path)
	case info.isGitHookPath():
		return core.DecisionBlocked, fmt.Sprintf("access to git hooks path %s is blocked", path)
	case info.isEnvFile():
		return core.DecisionApproval, fmt.Sprintf("access to sensitive environment file %s requires approval", path)
	case info.isUnder(filepath.Join(cwd, "secrets")) || info.matchesRelative("secrets"):
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
}

func nativeFilePathInfo(path, cwd string) filePathInfo {
	homeDir, _ := os.UserHomeDir()
	cleanRaw := filepath.Clean(path)
	resolved := cleanRaw
	if strings.HasPrefix(path, "~/") && homeDir != "" {
		resolved = filepath.Join(homeDir, strings.TrimPrefix(path, "~/"))
	}
	if !filepath.IsAbs(resolved) && cwd != "" {
		resolved = filepath.Join(cwd, resolved)
	}
	return filePathInfo{
		raw:      path,
		cleanRaw: cleanRaw,
		abs:      filepath.Clean(resolved),
		slashRaw: filepath.ToSlash(cleanRaw),
		slashAbs: filepath.ToSlash(filepath.Clean(resolved)),
		homeDir:  filepath.Clean(homeDir),
	}
}

func (p filePathInfo) matchesRelative(want string) bool {
	want = filepath.ToSlash(filepath.Clean(want))
	return p.slashRaw == want || strings.HasPrefix(p.slashRaw, want+"/")
}

func (p filePathInfo) matchesAbsolute(want string) bool {
	if want == "" {
		return false
	}
	return p.slashAbs == filepath.ToSlash(filepath.Clean(want))
}

func (p filePathInfo) isUnder(base string) bool {
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

func (p filePathInfo) hasBase(name string) bool {
	return filepath.Base(p.cleanRaw) == name || filepath.Base(p.abs) == name
}

func (p filePathInfo) isGitHookPath() bool {
	return p.matchesRelative(".git/hooks") || strings.Contains(p.slashAbs, "/.git/hooks/")
}

func (p filePathInfo) isEnvFile() bool {
	base := filepath.Base(p.cleanRaw)
	if base == "." || base == string(filepath.Separator) {
		base = filepath.Base(p.abs)
	}
	return base == ".env" || strings.HasPrefix(base, ".env.")
}

func (p filePathInfo) hasSensitiveExtension() bool {
	ext := strings.ToLower(filepath.Ext(p.cleanRaw))
	switch ext {
	case ".pem", ".key", ".crt", ".p12", ".pfx", ".jks", ".keystore":
		return true
	default:
		return false
	}
}
