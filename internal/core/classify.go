package core

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

// maxInputSize is the maximum allowed raw command length (64 KB).
const maxInputSize = 64 * 1024

// ClassifyResult holds the full result of command classification.
type ClassifyResult struct {
	Decision    Decision
	Reason      string
	RuleID      string
	DecisionKey string
	SubResults  []SubCommandResult
}

// SubCommandResult holds the classification result for a single sub-command.
type SubCommandResult struct {
	Command  string
	Decision Decision
	Reason   string
	RuleID   string
}

// PolicyEvaluator abstracts policy evaluation to avoid circular imports
// between core and policy packages. The policy package provides a concrete
// implementation.
type PolicyEvaluator interface {
	// EvaluateHardcoded checks hardcoded BLOCKED rules. Returns decision and reason,
	// or empty decision if no match.
	EvaluateHardcoded(classNorm string) (Decision, string)

	// EvaluateUserRules checks user-defined policy rules. Returns decision and reason,
	// or empty decision if no match.
	EvaluateUserRules(classNorm string) (Decision, string)

	// EvaluateBuiltins checks built-in preset rules. Returns decision, reason, and
	// rule ID, or empty decision if no match.
	EvaluateBuiltins(classNorm string) (Decision, string, string)
}

// Compiled regexes for inline script detection (§5.4).
var (
	reInlineShC         = regexp.MustCompile(`\b(ba)?sh\s+-c\s+`)
	reInlinePythonC     = regexp.MustCompile(`\bpython[23]?\s+-c\s+`)
	reInlineNodeE       = regexp.MustCompile(`\bnode\s+-e\s+`)
	reInlinePerlE       = regexp.MustCompile(`\bperl\s+-e\s+`)
	reInlineRubyE       = regexp.MustCompile(`\bruby\s+-e\s+`)
	reInlineEval        = regexp.MustCompile(`\beval\s+`)
	reInlineHeredoc     = regexp.MustCompile(`<<[-]?\s*['"]?\w+['"]?`)
	reInlinePipeSh      = regexp.MustCompile(`\|\s*(ba)?sh\b`)
	reInlinePipePy      = regexp.MustCompile(`\|\s*python[23]?\b`)
	reInlinePipeNode    = regexp.MustCompile(`\|\s*node\b`)
	reInlinePipeRuby    = regexp.MustCompile(`\|\s*(ruby|perl)\b`)
	reInlineBase64Sh    = regexp.MustCompile(`base64\s+(-d|--decode).*\|\s*(ba)?sh`)
	reInlineCmdSubst    = regexp.MustCompile(`\$\(`)
	reInlineExportPATH  = regexp.MustCompile(`\bexport\s+PATH=`)
	reInlineShellConfig = regexp.MustCompile(`(>|>>)\s*.*\.(bashrc|zshrc|profile|bash_profile)\b`)
)

// inlineScriptPatterns maps compiled regexes to whether they trigger APPROVAL (true) or CAUTION (false).
var inlineScriptPatterns = []struct {
	re       *regexp.Regexp
	approval bool // true = APPROVAL, false = CAUTION
}{
	{reInlineShC, true},
	{reInlinePythonC, true},
	{reInlineNodeE, true},
	{reInlinePerlE, true},
	{reInlineRubyE, true},
	{reInlineEval, true},
	{reInlineHeredoc, true},
	{reInlinePipeSh, true},
	{reInlinePipePy, true},
	{reInlinePipeNode, true},
	{reInlinePipeRuby, true},
	{reInlineBase64Sh, true},
	{reInlineCmdSubst, false},      // CAUTION only
	{reInlineExportPATH, false},    // CAUTION only
	{reInlineShellConfig, false},   // CAUTION only
}

// Sensitive env var detection (§5.3 from the issue description).
var reSensitiveEnvVar = regexp.MustCompile(
	`\$\{?(AWS_SECRET_ACCESS_KEY|AWS_SESSION_TOKEN|GITHUB_TOKEN|GH_TOKEN|DATABASE_URL|DB_PASSWORD|API_KEY|SECRET_KEY|PRIVATE_KEY)`,
)

// Security-sensitive environment variable prefixes that trigger APPROVAL
// when used as command-line env assignments (§5.3 from spec).
var sensitiveEnvPrefixes = []string{
	"PATH=", "LD_PRELOAD=", "LD_LIBRARY_PATH=",
	"DYLD_", "PYTHONPATH=", "PYTHONHOME=",
	"NODE_PATH=", "NODE_OPTIONS=", "PERL5LIB=",
	"PERLLIB=", "RUBYLIB=", "RUBYOPT=",
	"GIT_EXEC_PATH=", "HOME=",
}

// Classify runs the full classification pipeline on a shell request (§5.2).
// The evaluator parameter provides policy rule evaluation; pass nil to skip
// all policy/builtin rule checks (only safe-command heuristics will apply).
func Classify(req ShellRequest, evaluator PolicyEvaluator) (*ClassifyResult, error) {
	result := &ClassifyResult{}

	// Step 1: Input validation — reject if > 64 KB or contains null bytes.
	if len(req.RawCommand) > maxInputSize {
		return nil, fmt.Errorf("command exceeds maximum size of %d bytes", maxInputSize)
	}
	if strings.ContainsRune(req.RawCommand, '\x00') {
		return nil, fmt.Errorf("command contains null bytes")
	}

	// Step 2: Display normalize.
	displayNorm := DisplayNormalize(req.RawCommand)

	// Step 3: Compound command splitting.
	subCmds, err := SplitCompoundCommand(displayNorm)
	if err != nil {
		// Fail-closed: treat as APPROVAL.
		result.Decision = DecisionApproval
		result.Reason = fmt.Sprintf("compound split error (fail-closed): %v", err)
		result.DecisionKey = ComputeDecisionKey(req.Source, displayNorm, "")
		return result, nil
	}

	// §5.2 step 3: If compound block contains cwd-changing builtins
	// before a later file-backed sub-command, classify whole block as APPROVAL.
	if len(subCmds) > 1 && ContainsCwdChange(subCmds) {
		result.Decision = DecisionApproval
		result.Reason = "compound command contains cwd-changing builtin (cd/pushd/popd)"
		result.DecisionKey = ComputeDecisionKey(req.Source, displayNorm, "")
		return result, nil
	}

	// Accumulate file hashes for decision key.
	var fileHashes []string

	// Step 4-11: Per sub-command classification.
	overallDecision := DecisionSafe
	overallReason := "all sub-commands safe"
	overallRuleID := ""

	for _, subCmd := range subCmds {
		sub := classifySubCommand(subCmd, evaluator, req.Cwd)
		result.SubResults = append(result.SubResults, sub)

		newOverall := MaxDecision(overallDecision, sub.Decision)
		if newOverall != overallDecision {
			overallDecision = newOverall
			overallReason = sub.Reason
			overallRuleID = sub.RuleID
		}

		// Gather file hash if a referenced file was inspected.
		refFile := DetectReferencedFile(subCmd)
		if refFile != "" {
			resolvedPath := resolvePath(refFile, req.Cwd)
			inspection, inspErr := InspectFile(resolvedPath, DefaultMaxBytes)
			if inspErr == nil && inspection != nil && inspection.Hash != "" {
				fileHashes = append(fileHashes, inspection.Hash)
			}
		}
	}

	result.Decision = overallDecision
	result.Reason = overallReason
	result.RuleID = overallRuleID

	// Step 12: Compute decision key.
	combinedHash := strings.Join(fileHashes, ":")
	result.DecisionKey = ComputeDecisionKey(req.Source, displayNorm, combinedHash)

	return result, nil
}

// classifySubCommand runs the per-sub-command pipeline (steps 4-11).
func classifySubCommand(subCmd string, evaluator PolicyEvaluator, cwd string) SubCommandResult {
	sub := SubCommandResult{Command: subCmd}

	// Step 4a: Classification normalize.
	classified := ClassificationNormalize(subCmd)
	outerCmd := classified.Outer

	// Classify all commands (outer + inner), take most restrictive.
	allCmds := []string{outerCmd}
	allCmds = append(allCmds, classified.Inner...)

	bestDecision := DecisionSafe
	bestReason := "default safe"
	bestRuleID := ""

	for _, cmd := range allCmds {
		if cmd == "" {
			continue
		}

		d, reason, ruleID := classifySingleCommand(cmd, evaluator, cwd)
		combined := MaxDecision(bestDecision, d)
		if combined != bestDecision {
			bestDecision = combined
			bestReason = reason
			bestRuleID = ruleID
		}
	}

	// Step 10: Apply sudo/doas escalation modifier.
	if classified.EscalateClassification {
		bestDecision, bestReason = escalateDecision(bestDecision, bestReason)
	}

	// Sensitive env var detection (§5.3 from issue).
	if reSensitiveEnvVar.MatchString(subCmd) {
		escalated := MaxDecision(bestDecision, DecisionCaution)
		if escalated != bestDecision {
			bestDecision = escalated
			bestReason = "references sensitive environment variable"
		}
	}

	sub.Decision = bestDecision
	sub.Reason = bestReason
	sub.RuleID = bestRuleID
	return sub
}

// classifySingleCommand classifies a single (already classification-normalized) command string.
func classifySingleCommand(cmd string, evaluator PolicyEvaluator, cwd string) (Decision, string, string) {
	// Step 5: Inline script detection (§5.4).
	inlineDecision, inlineReason := detectInlineScript(cmd)

	// Step 6: Context sanitization.
	basename := extractBasename(cmd)
	knownSafe := KnownSafeVerbs[basename]
	sanitized := SanitizeForClassification(cmd, knownSafe)

	// Step 7: Detect referenced files.
	refFile := DetectReferencedFile(cmd)

	// Step 8: Inspect referenced file if detected.
	var fileInspection *FileInspection
	if refFile != "" {
		resolvedPath := resolvePath(refFile, cwd)
		inspection, err := InspectFile(resolvedPath, DefaultMaxBytes)
		if err == nil {
			fileInspection = inspection
		}
	}

	// Check for security-sensitive env var assignments at start of command.
	if hasSensitiveEnvPrefix(cmd) {
		return DecisionApproval, "security-sensitive environment variable assignment", ""
	}

	// Step 9: Evaluate rules in order (most restrictive wins within each layer).
	if evaluator != nil {
		// Layer 1: Hardcoded rules.
		if d, reason := evaluator.EvaluateHardcoded(sanitized); d != "" {
			return d, reason, ""
		}

		// Layer 2: User policy rules.
		if d, reason := evaluator.EvaluateUserRules(sanitized); d != "" {
			return d, reason, ""
		}

		// Layer 3: Built-in preset rules.
		if d, reason, ruleID := evaluator.EvaluateBuiltins(sanitized); d != "" {
			return d, reason, ruleID
		}
	}

	// Layer 4: Unconditional safe commands.
	if IsUnconditionalSafe(basename) || IsUnconditionalSafeCmd(cmd) {
		return DecisionSafe, "unconditionally safe command", ""
	}

	// Layer 5: Conditionally safe commands.
	if IsConditionallySafe(basename, cmd) {
		return DecisionSafe, "conditionally safe command", ""
	}

	// Layer 6: File inspection result (if applicable).
	if fileInspection != nil {
		return fileInspection.Decision, fileInspection.Reason, ""
	}

	// Check inline script detection result (deferred from step 5).
	if inlineDecision != "" {
		return inlineDecision, inlineReason, ""
	}

	// Fallback: SAFE (default-SAFE per spec §6.5).
	return DecisionSafe, "no matching rule (default safe)", ""
}

// detectInlineScript checks for inline script/heredoc patterns (§5.4).
// Returns the decision and reason if a pattern matches, or empty strings if none.
func detectInlineScript(cmd string) (Decision, string) {
	bestDecision := Decision("")
	bestReason := ""

	for _, p := range inlineScriptPatterns {
		if p.re.MatchString(cmd) {
			var d Decision
			if p.approval {
				d = DecisionApproval
			} else {
				d = DecisionCaution
			}
			if bestDecision == "" {
				bestDecision = d
				bestReason = "inline script detected: " + p.re.String()
			} else {
				combined := MaxDecision(bestDecision, d)
				if combined != bestDecision {
					bestDecision = combined
					bestReason = "inline script detected: " + p.re.String()
				}
			}
		}
	}

	return bestDecision, bestReason
}

// escalateDecision applies the sudo/doas escalation modifier.
// SAFE -> CAUTION, CAUTION -> APPROVAL, APPROVAL/BLOCKED unchanged.
func escalateDecision(d Decision, reason string) (Decision, string) {
	switch d {
	case DecisionSafe:
		return DecisionCaution, reason + " (escalated: sudo/doas)"
	case DecisionCaution:
		return DecisionApproval, reason + " (escalated: sudo/doas)"
	default:
		return d, reason
	}
}

// extractBasename returns the first whitespace-delimited token of a command,
// with any path components stripped.
func extractBasename(cmd string) string {
	fields := strings.Fields(cmd)
	if len(fields) == 0 {
		return ""
	}
	return filepath.Base(fields[0])
}

// resolvePath resolves a file path relative to a working directory.
// If the path is already absolute, it is returned as-is.
func resolvePath(path, cwd string) string {
	if filepath.IsAbs(path) {
		return path
	}
	if cwd == "" {
		return path
	}
	return filepath.Join(cwd, path)
}

// hasSensitiveEnvPrefix checks if the command starts with a security-sensitive
// environment variable assignment (§5.3).
func hasSensitiveEnvPrefix(cmd string) bool {
	fields := strings.Fields(cmd)
	for _, f := range fields {
		// Stop at the first non-assignment token.
		if !strings.Contains(f, "=") || !isEnvAssignmentToken(f) {
			break
		}
		for _, prefix := range sensitiveEnvPrefixes {
			if strings.HasPrefix(f, prefix) {
				return true
			}
		}
	}
	return false
}

// isEnvAssignmentToken checks if a token looks like VAR=value.
func isEnvAssignmentToken(t string) bool {
	idx := strings.Index(t, "=")
	if idx <= 0 {
		return false
	}
	name := t[:idx]
	for i, ch := range name {
		if i == 0 {
			if !isLetter(ch) && ch != '_' {
				return false
			}
		} else {
			if !isLetter(ch) && !isDigit(ch) && ch != '_' {
				return false
			}
		}
	}
	return true
}

func isLetter(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
}

func isDigit(r rune) bool {
	return r >= '0' && r <= '9'
}
