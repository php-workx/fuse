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
	Decision            Decision
	Reason              string
	RuleID              string
	DecisionKey         string
	SubResults          []SubCommandResult
	DryRunMatches       []BuiltinMatch // rules that matched but were not enforced (per-tag dryrun)
	TagOverrideEnforced bool           // true when the decision was enforced by a tag_override (should block even in dryrun)
}

// SubCommandResult holds the classification result for a single sub-command.
type SubCommandResult struct {
	Command             string
	Decision            Decision
	Reason              string
	RuleID              string
	DryRunMatches       []BuiltinMatch
	TagOverrideEnforced bool // true when a tag_override explicitly enforced this decision
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

	// EvaluateBuiltins checks built-in preset rules. Returns a BuiltinMatch
	// if a rule matched, or nil if no match. DryRun indicates the match
	// should be logged but not enforced (per-tag override or global dryrun).
	EvaluateBuiltins(classNorm string) *BuiltinMatch
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
	{reInlineCmdSubst, false},    // CAUTION only
	{reInlineExportPATH, false},  // CAUTION only
	{reInlineShellConfig, false}, // CAUTION only
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
func Classify(req ShellRequest, evaluator PolicyEvaluator) (*ClassifyResult, error) { //nolint:funlen // classify pipeline
	result := &ClassifyResult{}

	// Step 1: Input validation — oversized commands are fail-closed APPROVAL.
	if len(req.RawCommand) > maxInputSize {
		displayNorm := DisplayNormalize(req.RawCommand)
		result.Decision = DecisionApproval
		result.Reason = fmt.Sprintf("command exceeds maximum size of %d bytes", maxInputSize)
		result.DecisionKey = ComputeDecisionKey(req.Source, displayNorm, "")
		return result, nil
	}

	// Step 2: Display normalize.
	displayNorm := DisplayNormalize(req.RawCommand)

	// Step 3: Compound command splitting.
	subCmds, err := SplitCompoundCommand(displayNorm)
	if err != nil {
		if evaluator != nil {
			classified := ClassificationNormalize(displayNorm)
			candidates := []string{displayNorm, classified.Outer}
			candidates = append(candidates, classified.Inner...)
			for _, candidate := range candidates {
				if candidate == "" {
					continue
				}
				if d, reason := evaluator.EvaluateHardcoded(candidate); d != "" {
					result.Decision = d
					result.Reason = reason
					result.DecisionKey = ComputeDecisionKey(req.Source, displayNorm, "")
					return result, nil
				}
			}
		}
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

	// Preserve inline pipe-script detection across compound splitting. A pipeline
	// like "curl ... | bash" is structurally split into safe-looking sub-commands,
	// so the compound form must still contribute its higher-risk decision.
	compoundInlineDecision := Decision("")
	compoundInlineReason := ""
	if len(subCmds) > 1 && strings.Contains(displayNorm, "|") {
		compoundInlineDecision, compoundInlineReason = detectInlineScript(displayNorm)
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
		result.DryRunMatches = append(result.DryRunMatches, sub.DryRunMatches...)

		newOverall := MaxDecision(overallDecision, sub.Decision)
		if newOverall != overallDecision {
			overallDecision = newOverall
			overallReason = sub.Reason
			overallRuleID = sub.RuleID
		}
		// OR across all sub-commands: if ANY sub-command was tag-override-enforced,
		// the overall result should be enforced even in dryrun mode.
		if sub.TagOverrideEnforced {
			result.TagOverrideEnforced = true
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

	if compoundInlineDecision != "" {
		combined := MaxDecision(result.Decision, compoundInlineDecision)
		if combined != result.Decision {
			result.Decision = combined
			result.Reason = compoundInlineReason
			result.RuleID = ""
		}
	}

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

	// Fail-closed: if bash -c extraction failed, force APPROVAL.
	if classified.ExtractionFailed {
		sub.Decision = DecisionApproval
		sub.Reason = "bash -c extraction failed (fail-closed)"
		return sub
	}

	// Classify all commands (outer + inner), take most restrictive.
	allCmds := []string{outerCmd}
	allCmds = append(allCmds, classified.Inner...)

	bestDecision := DecisionSafe
	bestReason := "default safe"
	bestRuleID := ""
	var allDryRunMatches []BuiltinMatch
	tagOverrideEnforced := false

	for _, cmd := range allCmds {
		if cmd == "" {
			continue
		}

		d, reason, ruleID, dryMatches, override := classifySingleCommand(cmd, evaluator, cwd)
		combined := MaxDecision(bestDecision, d)
		if combined != bestDecision {
			bestDecision = combined
			bestReason = reason
			bestRuleID = ruleID
			tagOverrideEnforced = override
		}
		allDryRunMatches = append(allDryRunMatches, dryMatches...)
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
	sub.DryRunMatches = allDryRunMatches
	sub.TagOverrideEnforced = tagOverrideEnforced
	return sub
}

// classifySingleCommand classifies a single (already classification-normalized) command string.
// classifySingleCommand returns (decision, reason, ruleID, dryRunMatches, tagOverrideEnforced).
// tagOverrideEnforced is true when the decision was enforced by an explicit tag_override.
func classifySingleCommand(cmd string, evaluator PolicyEvaluator, cwd string) (Decision, string, string, []BuiltinMatch, bool) {
	var dryRunMatches []BuiltinMatch

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
		return DecisionApproval, "security-sensitive environment variable assignment", "", nil, false
	}

	// Hardcoded rules must see the unsanitized normalized command so inline
	// self-protection patterns are not masked by quote sanitization.
	if evaluator != nil {
		if d, reason := evaluator.EvaluateHardcoded(cmd); d != "" {
			return d, reason, "", nil, false
		}
	}

	// Step 9: Evaluate rules in order (most restrictive wins within each layer).

	if evaluator != nil {
		// Layer 2: User policy rules (always evaluated first — user can override anything).
		if d, reason := evaluator.EvaluateUserRules(sanitized); d != "" {
			return d, reason, "", nil, false
		}

		// Layer 2.5: Safe build directory cleanup (rm -rf node_modules, dist, etc.)
		// After user rules so project policies can still block/approve if needed.
		if IsSafeBuildCleanup(cmd) {
			return DecisionSafe, "safe build directory cleanup", "", nil, false
		}

		// Layer 3: Built-in preset rules.
		if match := evaluator.EvaluateBuiltins(sanitized); match != nil {
			if match.DryRun {
				// Per-tag dryrun: collect for logging but don't enforce.
				dryRunMatches = append(dryRunMatches, *match)
			} else {
				if isInspectTriggerRule(match.RuleID) && fileInspection != nil {
					return fileInspection.Decision, fileInspection.Reason, "", dryRunMatches, match.TagOverrideEnforced
				}
				return match.Decision, match.Reason, match.RuleID, dryRunMatches, match.TagOverrideEnforced
			}
		}
	}

	// Layer 4: Unconditional safe commands.
	if IsUnconditionalSafe(basename) || IsUnconditionalSafeCmd(cmd) {
		return DecisionSafe, "unconditionally safe command", "", dryRunMatches, false
	}

	// Layer 5: Conditionally safe commands.
	if IsConditionallySafe(basename, cmd) {
		return DecisionSafe, "conditionally safe command", "", dryRunMatches, false
	}

	// Layer 6: File inspection result (if applicable).
	if fileInspection != nil {
		return fileInspection.Decision, fileInspection.Reason, "", dryRunMatches, false
	}

	// Check inline script detection result (deferred from step 5).
	if inlineDecision != "" {
		return inlineDecision, inlineReason, "", dryRunMatches, false
	}

	// Fallback: SAFE (default-SAFE per spec §6.5).
	return DecisionSafe, "no matching rule (default safe)", "", dryRunMatches, false
}

// Patterns that indicate dangerous inline Python code.
var dangerousPythonInline = regexp.MustCompile(
	`(?i)\b(subprocess|os\s*\.\s*(system|exec|popen|remove|unlink|rmdir|rename|makedirs)|` +
		`shutil\s*\.\s*(rmtree|move|copy)|` +
		`pathlib\s*\..*\.\s*(unlink|rmdir|rename|write_text|write_bytes|mkdir)|` +
		`__import__|eval\s*\(|exec\s*\(|compile\s*\(|getattr\s*\(|` +
		`open\s*\([^)]*,\s*['"][wa]|` +
		`requests\s*\.|urllib\s*\.|http\.client|socket\s*\.|` +
		`ctypes|cffi|pty\s*\.\s*spawn|multiprocessing|` +
		`importlib\s*\.\s*import_module|` +
		`code\s*\.\s*interact|codeop|` +
		`pip\s*\.\s*main|setuptools|pkg_resources\s*\.\s*require|` +
		`boto3|google\.cloud|azure\.)`,
)

// Safe Python modules commonly used by agents for read-only introspection.
// Excludes os.path (allows os.remove via import os.path; os.remove),
// pip/setuptools/pkg_resources (can install/remove packages).
var safePythonInline = regexp.MustCompile(
	`\bpython[23]?\s+-c\s+.*\bimport\s+(ast|json|sys|pathlib|` +
		`collections|re|math|hashlib|base64|struct|textwrap|inspect|tokenize|` +
		`configparser|tomllib|typing|dataclasses|enum|functools|itertools|operator|string|` +
		`platform|sysconfig|site)\b`,
)

// detectInlineScript checks for inline script/heredoc patterns (§5.4).
// Returns the decision and reason if a pattern matches, or empty strings if none.
func detectInlineScript(cmd string) (Decision, string) {
	bestDecision := Decision("")
	bestReason := ""

	for _, p := range inlineScriptPatterns {
		if !p.re.MatchString(cmd) {
			continue
		}
		// Check if this is a safe python -c pattern.
		if p.re == reInlinePythonC && isSafePythonInline(cmd) {
			continue
		}
		// Skip heredoc detection for git commit / gh pr create — the heredoc
		// is just a message body, not code execution. Do NOT skip $() detection:
		// command substitutions in git commit -m ARE executed by the shell.
		if p.re == reInlineHeredoc && isSafeHeredocUsage(cmd) {
			continue
		}

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

	return bestDecision, bestReason
}

// isSafePythonInline returns true if a python -c command uses only safe,
// read-only modules and contains no dangerous patterns.
// reSafeHeredocCmd matches commands that safely use heredocs for message bodies,
// not for code execution. Covers git commit, gh pr create, and similar.
var reSafeHeredocCmd = regexp.MustCompile(`^\s*(git\s+commit|git\s+tag|gh\s+pr\s+create|gh\s+issue\s+create)\b`)

func isSafeHeredocUsage(cmd string) bool {
	return reSafeHeredocCmd.MatchString(cmd)
}

func isSafePythonInline(cmd string) bool {
	if !safePythonInline.MatchString(cmd) {
		return false
	}
	return !dangerousPythonInline.MatchString(cmd)
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

func isInspectTriggerRule(ruleID string) bool {
	switch ruleID {
	case "builtin:interp:python-file", "builtin:interp:node-file", "builtin:interp:bash-file":
		return true
	default:
		return false
	}
}
