package core

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// maxInputSize is the maximum allowed raw command length (64 KB).
const maxInputSize = 64 * 1024

// ClassifyResult holds the full result of command classification.
type ClassifyResult struct {
	Decision             Decision
	Reason               string
	RuleID               string
	DecisionKey          string
	SubResults           []SubCommandResult
	DryRunMatches        []BuiltinMatch // rules that matched but were not enforced (per-tag dryrun)
	TagOverrideEnforced  bool           // true when the decision was enforced by a tag_override (should block even in dryrun)
	InlineBody           string         // extracted inline script content for judge
	ExtractionIncomplete bool           // true when body truncated or depth exhausted
	FailClosed           bool           // true when APPROVAL is due to incomplete analysis (not risk assessment)
}

// WithDecision returns a deep copy of the result with a new decision and reason.
// Slices are deep-copied to avoid aliasing with the original.
func (r *ClassifyResult) WithDecision(d Decision, reason string) *ClassifyResult {
	result := *r
	result.Decision = d
	result.Reason = reason
	result.SubResults = append([]SubCommandResult(nil), r.SubResults...)
	result.DryRunMatches = append([]BuiltinMatch(nil), r.DryRunMatches...)
	return &result
}

// SubCommandResult holds the classification result for a single sub-command.
type SubCommandResult struct {
	Command              string
	Decision             Decision
	Reason               string
	RuleID               string
	FileHash             string
	DryRunMatches        []BuiltinMatch
	TagOverrideEnforced  bool   // true when a tag_override explicitly enforced this decision
	InlineBody           string // extracted inline script body
	ExtractionIncomplete bool   // true when extraction was incomplete
	FailClosed           bool   // true when APPROVAL is due to incomplete analysis
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
	reInlineShC           = regexp.MustCompile(`\b(ba)?sh\s+-c\s+`)
	reInlinePythonC       = regexp.MustCompile(`\bpython[23]?\s+-c\s+`)
	reInlineNodeE         = regexp.MustCompile(`\bnode\s+-e\s+`)
	reInlinePerlE         = regexp.MustCompile(`\bperl\s+-e\s+`)
	reInlineRubyE         = regexp.MustCompile(`\bruby\s+-e\s+`)
	reInlineEval          = regexp.MustCompile(`\beval\s+`)
	reInlineHeredoc       = regexp.MustCompile(`<<[-]?\s*['"]?\w+['"]?`)
	reInlinePipeSh        = regexp.MustCompile(`\|\s*(ba)?sh\b`)
	reInlinePipePy        = regexp.MustCompile(`\|\s*python[23]?\b`)
	reInlinePipeNode      = regexp.MustCompile(`\|\s*node\b`)
	reInlinePipeRuby      = regexp.MustCompile(`\|\s*(ruby|perl)\b`)
	reInlineBase64Sh      = regexp.MustCompile(`base64\s+(-d|--decode).*\|\s*(ba)?sh`)
	reInlinePHPR          = regexp.MustCompile(`\bphp\s+-[ra]\s+`)
	reInlineLuaE          = regexp.MustCompile(`\blua\s+-e\s+`)
	reInlineGroovyE       = regexp.MustCompile(`\bgroovy\s+-e\s+`)
	reInlineOsascriptE    = regexp.MustCompile(`\bosascript\s+-e\s+`)
	reInlinePipePHP       = regexp.MustCompile(`\|\s*php\b`)
	reInlinePipeLua       = regexp.MustCompile(`\|\s*lua\b`)
	reInlinePipeOsascript = regexp.MustCompile(`\|\s*osascript\b`)
	reInlineCmdSubst      = regexp.MustCompile(`\$\(`)
	reInlineExportPATH    = regexp.MustCompile(`\bexport\s+PATH=`)
	reInlineShellConfig   = regexp.MustCompile(`(>|>>)\s*.*\.(bashrc|zshrc|profile|bash_profile)\b`)
)

// inlineScriptPatterns maps compiled regexes to whether they trigger APPROVAL (true) or CAUTION (false).
var inlineScriptPatterns = []struct {
	re       *regexp.Regexp
	approval bool // true = APPROVAL, false = CAUTION
}{
	// All inline patterns produce CAUTION. The inline body extraction pipeline
	// (classifyInlineBodies) analyzes the actual content and escalates to APPROVAL
	// or BLOCKED when the extracted code is dangerous. Pattern detection alone
	// should not interrupt the user — it just flags for logging and judge triage.
	{reInlineShC, false},
	{reInlinePythonC, false},
	{reInlineNodeE, false},
	{reInlinePerlE, false},
	{reInlineRubyE, false},
	{reInlineEval, false},
	{reInlineHeredoc, false},
	{reInlinePipeSh, false},
	{reInlinePipePy, false},
	{reInlinePipeNode, false},
	{reInlinePipeRuby, false},
	{reInlineBase64Sh, false},
	{reInlinePHPR, false},
	{reInlineLuaE, false},
	{reInlineGroovyE, false},
	{reInlineOsascriptE, false},
	{reInlinePipePHP, false},
	{reInlinePipeLua, false},
	{reInlinePipeOsascript, false},
	{reInlineCmdSubst, false},
	{reInlineExportPATH, false},
	{reInlineShellConfig, false},
}

// Sensitive env var detection (§5.3 from the issue description).
var reSensitiveEnvVar = regexp.MustCompile(
	`\$\{?(AWS_SECRET_ACCESS_KEY|AWS_SESSION_TOKEN|GITHUB_TOKEN|GH_TOKEN|DATABASE_URL|DB_PASSWORD|API_KEY|SECRET_KEY|PRIVATE_KEY)`,
)

// Security-sensitive environment variable prefixes that trigger APPROVAL
// when used as command-line env assignments (§5.3 from spec).
// Only includes variables that enable binary/library injection or config
// resolution attacks. Routine dev variables (PYTHONPATH, NODE_PATH, etc.)
// are excluded — agents set these constantly for project imports.
var sensitiveEnvPrefixes = []string{
	"PATH=", "LD_PRELOAD=", "LD_LIBRARY_PATH=",
	"DYLD_",
	"NODE_OPTIONS=",  // allows --require injection
	"GIT_EXEC_PATH=", // substitutes git binaries
	"HOME=",          // redirects config resolution
}

// Classify runs the full classification pipeline on a shell request (§5.2).
// The evaluator parameter provides policy rule evaluation; pass nil to skip
// all policy/builtin rule checks (only safe-command heuristics will apply).
func Classify(req ShellRequest, evaluator PolicyEvaluator) (*ClassifyResult, error) {
	result := &ClassifyResult{}

	// Step 1: Input validation — oversized commands are fail-closed APPROVAL.
	if len(req.RawCommand) > maxInputSize {
		displayNorm := DisplayNormalize(req.RawCommand)
		result.Decision = DecisionApproval
		result.Reason = fmt.Sprintf("command exceeds maximum size of %d bytes", maxInputSize)
		result.FailClosed = true
		result.DecisionKey = ComputeDecisionKey(req.Source, displayNorm, "")
		return result, nil
	}

	// Step 2: Display normalize.
	displayNorm := DisplayNormalize(req.RawCommand)

	// Step 3: Compound command splitting.
	subCmds, err := SplitCompoundCommand(displayNorm)
	if err != nil {
		return classifyCompoundSplitError(result, displayNorm, req.Source, evaluator, err)
	}

	// Classify all sub-commands and aggregate results.
	fileHashes := classifyAllSubCommands(result, subCmds, evaluator, req.Cwd)

	// Apply compound-level modifiers.
	applyCompoundModifiers(result, subCmds, displayNorm)

	// Step 12: Compute decision key.
	combinedHash := strings.Join(fileHashes, ":")
	result.DecisionKey = ComputeDecisionKey(req.Source, displayNorm, combinedHash)

	return result, nil
}

// classifyCompoundSplitError handles the case where compound splitting fails.
// Checks hardcoded rules before falling back to fail-closed APPROVAL.
func classifyCompoundSplitError(result *ClassifyResult, displayNorm, source string, evaluator PolicyEvaluator, splitErr error) (*ClassifyResult, error) {
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
				result.DecisionKey = ComputeDecisionKey(source, displayNorm, "")
				return result, nil
			}
		}
	}
	// Fail-closed: treat as APPROVAL.
	result.Decision = DecisionApproval
	result.Reason = fmt.Sprintf("compound split error (fail-closed): %v", splitErr)
	result.FailClosed = true
	result.DecisionKey = ComputeDecisionKey(source, displayNorm, "")
	return result, nil
}

// classifyAllSubCommands classifies each sub-command and aggregates results into the overall result.
// Returns collected file hashes for decision key computation.
func classifyAllSubCommands(result *ClassifyResult, subCmds []string, evaluator PolicyEvaluator, cwd string) []string {
	overallDecision := DecisionSafe
	overallReason := "all sub-commands safe"
	overallRuleID := ""
	var fileHashes []string

	for _, subCmd := range subCmds {
		sub := classifySubCommand(subCmd, evaluator, cwd)
		result.SubResults = append(result.SubResults, sub)
		result.DryRunMatches = append(result.DryRunMatches, sub.DryRunMatches...)

		newOverall := MaxDecision(overallDecision, sub.Decision)
		if newOverall != overallDecision {
			overallDecision = newOverall
			overallReason = sub.Reason
			overallRuleID = sub.RuleID
		}

		mergeSubCommandFlags(result, &sub)
		fileHashes = collectFileHash(fileHashes, sub.FileHash)
	}

	result.Decision = overallDecision
	result.Reason = overallReason
	result.RuleID = overallRuleID

	return fileHashes
}

// mergeSubCommandFlags merges boolean flags and inline bodies from a sub-command into the overall result.
func mergeSubCommandFlags(result *ClassifyResult, sub *SubCommandResult) {
	if sub.TagOverrideEnforced {
		result.TagOverrideEnforced = true
	}
	if sub.InlineBody != "" {
		if result.InlineBody == "" {
			result.InlineBody = sub.InlineBody
		} else {
			result.InlineBody += "\n---\n" + sub.InlineBody
		}
	}
	if sub.ExtractionIncomplete {
		result.ExtractionIncomplete = true
	}
	if sub.FailClosed {
		result.FailClosed = true
	}
}

// collectFileHash gathers a file hash if a referenced file was inspected.
func collectFileHash(fileHashes []string, fileHash string) []string {
	if fileHash != "" {
		fileHashes = append(fileHashes, fileHash)
	}
	return fileHashes
}

// applyCompoundModifiers applies compound-level modifiers: inline pipe-script
// detection and CWD change escalation.
func applyCompoundModifiers(result *ClassifyResult, subCmds []string, displayNorm string) {
	// Preserve inline pipe-script detection across compound splitting.
	if len(subCmds) > 1 && strings.Contains(displayNorm, "|") {
		compoundInlineDecision, compoundInlineReason := detectInlineScript(displayNorm)
		if compoundInlineDecision != "" {
			combined := MaxDecision(result.Decision, compoundInlineDecision)
			if combined != result.Decision {
				result.Decision = combined
				result.Reason = compoundInlineReason
				result.RuleID = ""
			}
		}
	}

	// CWD change in compound: escalate to at least CAUTION for logging/judge triage.
	if len(subCmds) > 1 && ContainsCwdChange(subCmds) {
		combined := MaxDecision(result.Decision, DecisionCaution)
		if combined != result.Decision {
			result.Decision = combined
			result.Reason = "compound command contains cwd-changing builtin (cd/pushd/popd)"
			result.RuleID = ""
		}
	}
}

// classifySubCommand runs the per-sub-command pipeline (steps 4-11).
func classifySubCommand(subCmd string, evaluator PolicyEvaluator, cwd string) SubCommandResult {
	sub := SubCommandResult{Command: subCmd}

	// Pre-normalization hardcoded check: the tokenizer treats backslashes as
	// escape characters (Bash semantics), which mangles Windows paths like
	// C:\Windows into C:Windows. Check hardcoded rules against the raw
	// sub-command before normalization strips the backslashes.
	if evaluator != nil {
		if d, reason := evaluator.EvaluateHardcoded(subCmd); d != "" {
			sub.Decision = d
			sub.Reason = reason
			return sub
		}
	}

	// Step 4a: Classification normalize.
	classified := ClassificationNormalize(subCmd)
	fileInspection := inspectReferencedFile(subCmd, cwd)
	if fileInspection != nil {
		sub.FileHash = fileInspection.Hash
	}

	// Fail-closed: if inner command extraction failed, force APPROVAL.
	// This covers bash -c, powershell -EncodedCommand, powershell -File, etc.
	if classified.ExtractionFailed {
		sub.Decision = DecisionApproval
		sub.Reason = "inner command extraction failed (fail-closed)"
		sub.FailClosed = true
		return sub
	}

	// Classify all commands (outer + inner), take most restrictive.
	classifyAllNormalizedCommands(&sub, classified, evaluator, cwd, fileInspection)

	// Apply post-classification modifiers.
	applyEnvVarEscalations(&sub, subCmd, classified)

	// Detect fail-closed APPROVAL from file inspection or TOFU verification.
	detectFailClosedApproval(&sub, fileInspection)

	// Extract and classify inline script bodies (heredocs, $() contents).
	applyInlineClassification(&sub, subCmd, evaluator, cwd)

	// URL inspection: scan command and extracted inline bodies for URLs (SEC-006).
	applyURLInspection(&sub, subCmd, sub.InlineBody)

	return sub
}

// classifyAllNormalizedCommands classifies the outer and inner commands from
// normalization and populates the SubCommandResult with the most restrictive decision.
func classifyAllNormalizedCommands(sub *SubCommandResult, classified ClassifiedCommand, evaluator PolicyEvaluator, cwd string, fileInspection *FileInspection) {
	allCmds := []string{classified.Outer}
	allCmds = append(allCmds, classified.Inner...)

	bestDecision := DecisionSafe
	bestReason := "default safe"
	bestRuleID := ""
	var allDryRunMatches []BuiltinMatch
	tagOverrideEnforced := false

	for i, cmd := range allCmds {
		if cmd == "" {
			continue
		}
		currentInspection := (*FileInspection)(nil)
		if i == 0 {
			currentInspection = fileInspection
		}
		classification := classifySingleCommand(cmd, evaluator, cwd, currentInspection)
		combined := MaxDecision(bestDecision, classification.decision)
		if combined != bestDecision {
			bestDecision = combined
			bestReason = classification.reason
			bestRuleID = classification.ruleID
			tagOverrideEnforced = classification.tagOverrideEnforced
		}
		if classification.failClosed {
			sub.FailClosed = true
		}
		allDryRunMatches = append(allDryRunMatches, classification.dryRunMatches...)
	}

	// Step 10: Apply sudo/doas escalation modifier.
	if classified.EscalateClassification {
		bestDecision, bestReason = escalateDecision(bestDecision, bestReason)
		bestRuleID = ""
	}

	sub.Decision = bestDecision
	sub.Reason = bestReason
	sub.RuleID = bestRuleID
	sub.DryRunMatches = allDryRunMatches
	sub.TagOverrideEnforced = tagOverrideEnforced
}

// applyEnvVarEscalations applies sensitive environment variable detection and
// assignment escalation modifiers to the sub-command result.
func applyEnvVarEscalations(sub *SubCommandResult, subCmd string, classified ClassifiedCommand) {
	// Sensitive env var detection (§5.3 from issue).
	if reSensitiveEnvVar.MatchString(subCmd) {
		escalated := MaxDecision(sub.Decision, DecisionCaution)
		if escalated != sub.Decision {
			sub.Decision = escalated
			sub.Reason = "references sensitive environment variable"
			sub.RuleID = ""
		}
	}

	// Security-sensitive env var assignment detected during normalization.
	// Applied after the classification loop so it doesn't short-circuit
	// BLOCKED detection (e.g., LD_PRELOAD=/evil rm -rf / should be BLOCKED, not APPROVAL).
	if classified.SensitiveEnvAssignment {
		combined := MaxDecision(sub.Decision, DecisionApproval)
		if combined != sub.Decision {
			sub.Decision = combined
			sub.Reason = "security-sensitive environment variable assignment (via env or bare prefix)"
			sub.RuleID = ""
		}
	}
}

// detectFailClosedApproval marks APPROVAL results as fail-closed when the
// referenced file cannot be fully analyzed (missing or truncated with no signals).
// Reuses the inspection already performed by inspectReferencedFile to avoid a
// redundant file open (TOCTOU + wasted I/O).
func detectFailClosedApproval(sub *SubCommandResult, inspection *FileInspection) {
	if sub.Decision != DecisionApproval || inspection == nil {
		return
	}
	if !inspection.Exists || (inspection.Truncated && len(inspection.Signals) == 0) {
		sub.FailClosed = true
	}
}

// applyInlineClassification extracts and classifies inline script bodies
// (heredocs, $() contents) and merges the result into the sub-command result.
func applyInlineClassification(sub *SubCommandResult, subCmd string, evaluator PolicyEvaluator, cwd string) {
	inlineResult := classifyInlineBodies(subCmd, evaluator, cwd)
	sub.InlineBody = inlineResult.body
	sub.ExtractionIncomplete = !inlineResult.complete
	sub.DryRunMatches = append(sub.DryRunMatches, inlineResult.dryRunMatches...)
	if inlineResult.tagOverrideEnforced {
		sub.TagOverrideEnforced = true
	}
	if inlineResult.failClosed {
		sub.FailClosed = true
	}
	if inlineResult.decision != "" {
		combined := MaxDecision(sub.Decision, inlineResult.decision)
		if combined != sub.Decision {
			sub.Decision = combined
			sub.Reason = inlineResult.reason
			sub.RuleID = "" // inline analysis wins — clear stale RuleID
		}
	}
	if !inlineResult.complete && DecisionSeverity(sub.Decision) < DecisionSeverity(DecisionApproval) {
		sub.Decision = DecisionApproval
		sub.Reason = "inline script extraction incomplete (fail-closed)"
		sub.RuleID = ""
		sub.FailClosed = true
	}
}

// applyURLInspection scans the command and inline body for URL-based threats.
func applyURLInspection(sub *SubCommandResult, cmd, inlineBody string) {
	escalate := func(d Decision, r string) {
		combined := MaxDecision(sub.Decision, d)
		if combined != sub.Decision {
			sub.Decision = combined
			sub.Reason = r
			sub.RuleID = ""
		}
	}
	if d, r := InspectCommandURLs(cmd); d != "" {
		escalate(d, r)
	}
	if inlineBody != "" {
		for _, line := range strings.Split(inlineBody, "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			if d, r := InspectCommandURLs(line); d != "" {
				escalate(d, r)
			}
		}
	}
}

type commandClassificationResult struct {
	decision            Decision
	reason              string
	ruleID              string
	dryRunMatches       []BuiltinMatch
	tagOverrideEnforced bool
	failClosed          bool
}

// classifySingleCommand classifies a single (already classification-normalized) command string.
// tagOverrideEnforced is true when the decision was enforced by an explicit tag_override.
func classifySingleCommand(cmd string, evaluator PolicyEvaluator, cwd string, fileInspection *FileInspection) commandClassificationResult {
	var dryRunMatches []BuiltinMatch

	// Step 5: Inline script detection (§5.4).
	inlineDecision, inlineReason := detectInlineScript(cmd)

	// Step 6: Context sanitization.
	basename := extractBasename(cmd)

	// Step 6.5: Binary identity TOFU — verify interpreter binaries haven't changed mid-session.
	if resolvedPath, ok := resolveCommandPath(cmd, cwd); ok {
		if tofuD, tofuR := VerifyBinaryIdentity(resolvedPath); tofuD != "" {
			return commandClassificationResult{
				decision:   tofuD,
				reason:     tofuR,
				failClosed: tofuD == DecisionApproval,
			}
		}
	}

	knownSafe := KnownSafeVerbs[basename]
	sanitized := SanitizeForClassification(cmd, knownSafe)

	// Step 7-8: Detect and inspect referenced files.
	if fileInspection == nil {
		fileInspection = inspectReferencedFile(cmd, cwd)
	}

	// Check for security-sensitive env var assignments at start of command.
	if hasSensitiveEnvPrefix(cmd) {
		return commandClassificationResult{
			decision: DecisionApproval,
			reason:   "security-sensitive environment variable assignment",
		}
	}

	// Evaluate policy rules (hardcoded, user, builtins).
	if evaluator != nil {
		pr := evaluatePolicyRules(cmd, sanitized, evaluator, fileInspection, dryRunMatches)
		if pr.matched {
			return commandClassificationResult{
				decision:            pr.decision,
				reason:              pr.reason,
				ruleID:              pr.ruleID,
				dryRunMatches:       pr.dryRunMatches,
				tagOverrideEnforced: pr.tagOverrideEnforced,
				failClosed:          pr.failClosed,
			}
		}
		dryRunMatches = pr.dryRunMatches
	}

	// Layer 4-6 and inline fallbacks.
	return classifyFallbackLayers(cmd, basename, fileInspection, inlineDecision, inlineReason, dryRunMatches)
}

func resolveCommandPath(cmd, cwd string) (string, bool) {
	classified := ClassificationNormalize(cmd)
	fields := strings.Fields(classified.Outer)
	if len(fields) == 0 {
		return "", false
	}
	command := fields[0]
	if strings.Contains(command, "/") {
		return resolveExecutablePath(command, cwd)
	}
	resolvedPath, err := exec.LookPath(command)
	if err != nil {
		return "", false
	}
	if !filepath.IsAbs(resolvedPath) {
		return resolveExecutablePath(resolvedPath, cwd)
	}
	return resolvedPath, true
}

func resolveExecutablePath(path, cwd string) (string, bool) {
	if !filepath.IsAbs(path) {
		path = filepath.Join(cwd, path)
	}
	resolvedPath, err := filepath.Abs(path)
	if err != nil {
		return "", false
	}
	return filepath.Clean(resolvedPath), true
}

// inspectReferencedFile detects and inspects a file referenced in the command.
func inspectReferencedFile(cmd, cwd string) *FileInspection {
	refFile := DetectReferencedFile(cmd)
	if refFile == "" {
		return nil
	}
	resolvedPath := resolvePath(refFile, cwd)
	inspection, err := InspectFile(resolvedPath, DefaultMaxBytes)
	if err != nil {
		return nil
	}
	return inspection
}

// evaluatePolicyRules runs hardcoded, user, and builtin rule evaluation.
// Returns matched=true if a terminal decision was reached and the caller should return.
// The dryRunMatches slice may be updated with new matches.
// policyResult holds the outcome of evaluating policy rules against a command.
type policyResult struct {
	decision            Decision
	reason              string
	ruleID              string
	dryRunMatches       []BuiltinMatch
	tagOverrideEnforced bool
	failClosed          bool
	matched             bool // true if a policy rule matched (caller should stop)
}

func evaluatePolicyRules(
	cmd, sanitized string,
	evaluator PolicyEvaluator,
	fileInspection *FileInspection,
	dryRunMatches []BuiltinMatch,
) policyResult {
	// Hardcoded rules must see the unsanitized normalized command.
	if d, reason := evaluator.EvaluateHardcoded(cmd); d != "" {
		return policyResult{decision: d, reason: reason, matched: true}
	}

	// Layer 2: User policy rules.
	if d, reason := evaluator.EvaluateUserRules(sanitized); d != "" {
		return policyResult{decision: d, reason: reason, matched: true}
	}

	// Layer 2.5: Safe build directory cleanup.
	if IsSafeBuildCleanup(cmd) {
		return policyResult{decision: DecisionSafe, reason: "safe build directory cleanup", matched: true}
	}

	// Layer 3: Built-in preset rules.
	if match := evaluator.EvaluateBuiltins(sanitized); match != nil {
		if match.DryRun {
			dryRunMatches = append(dryRunMatches, *match)
		} else {
			if isInspectTriggerRule(match.RuleID) && fileInspection != nil {
				return policyResult{
					decision: fileInspection.Decision, reason: fileInspection.Reason,
					dryRunMatches: dryRunMatches, tagOverrideEnforced: match.TagOverrideEnforced,
					failClosed: inspectionIsFailClosed(fileInspection), matched: true,
				}
			}
			return policyResult{
				decision: match.Decision, reason: match.Reason, ruleID: match.RuleID,
				dryRunMatches: dryRunMatches, tagOverrideEnforced: match.TagOverrideEnforced, matched: true,
			}
		}
	}

	return policyResult{dryRunMatches: dryRunMatches}
}

// classifyFallbackLayers checks safe commands, file inspection, inline scripts,
// and produces the fallback CAUTION decision.
func classifyFallbackLayers(
	cmd, basename string,
	fileInspection *FileInspection,
	inlineDecision Decision, inlineReason string,
	dryRunMatches []BuiltinMatch,
) commandClassificationResult {
	// Layer 4: Unconditional safe commands.
	if IsUnconditionalSafe(basename) || IsUnconditionalSafeCmd(cmd) {
		return commandClassificationResult{decision: DecisionSafe, reason: "unconditionally safe command", dryRunMatches: dryRunMatches}
	}

	// Layer 5: Conditionally safe commands.
	if IsConditionallySafe(basename, cmd) {
		return commandClassificationResult{decision: DecisionSafe, reason: "conditionally safe command", dryRunMatches: dryRunMatches}
	}

	// Layer 6: File inspection result (if applicable).
	if fileInspection != nil {
		return commandClassificationResult{
			decision:      fileInspection.Decision,
			reason:        fileInspection.Reason,
			dryRunMatches: dryRunMatches,
			failClosed:    inspectionIsFailClosed(fileInspection),
		}
	}

	// Check inline script detection result (deferred from step 5).
	if inlineDecision != "" {
		return commandClassificationResult{decision: inlineDecision, reason: inlineReason, dryRunMatches: dryRunMatches}
	}

	// Check for explicitly safe inline patterns (e.g., python -c with safe imports).
	if isSafePythonInline(cmd) {
		return commandClassificationResult{decision: DecisionSafe, reason: "safe Python inline (read-only modules)", dryRunMatches: dryRunMatches}
	}

	// Fallback: CAUTION for unknown commands (enables judge triage).
	return commandClassificationResult{decision: DecisionCaution, reason: "unknown command (no matching rule)", dryRunMatches: dryRunMatches}
}

func inspectionIsFailClosed(fileInspection *FileInspection) bool {
	if fileInspection == nil || fileInspection.Decision != DecisionApproval {
		return false
	}
	return !fileInspection.Exists || (fileInspection.Truncated && len(fileInspection.Signals) == 0)
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
	`\bpython[23]?\s+(-c\s+.*\bimport\s+|-m\s+)(ast|json|sys|pathlib|` +
		`collections|re|math|hashlib|base64|struct|textwrap|inspect|tokenize|` +
		`configparser|tomllib|typing|dataclasses|enum|functools|itertools|operator|string|` +
		`platform|sysconfig|site|pprint|json\.tool)\b`,
)

// reSafePipePython matches piping to read-only Python module invocations.
var reSafePipePython = regexp.MustCompile(
	`\|\s*python[23]?\s+-m\s+(json\.tool|pprint|ast|tokenize)\b`,
)

// isExemptInlinePattern returns true if the matched pattern should be skipped
// for this command (safe python import, cat-heredoc substitution, safe heredoc usage,
// safe pipe-to-python-module).
func isExemptInlinePattern(re *regexp.Regexp, cmd string) bool {
	switch re {
	case reInlinePythonC:
		return isSafePythonInline(cmd)
	case reInlinePipePy:
		return reSafePipePython.MatchString(cmd)
	case reInlineHeredoc:
		return isCatHeredocSubstitution(cmd) || isSafeHeredocUsage(cmd)
	case reInlineCmdSubst:
		return isCatHeredocSubstitution(cmd)
	default:
		return false
	}
}

// detectInlineScript checks for inline script/heredoc patterns (§5.4).
// Returns the decision and reason if a pattern matches, or empty strings if none.
func detectInlineScript(cmd string) (Decision, string) {
	bestDecision := Decision("")
	bestReason := ""

	for _, p := range inlineScriptPatterns {
		if !p.re.MatchString(cmd) || isExemptInlinePattern(p.re, cmd) {
			continue
		}

		d := DecisionCaution
		if p.approval {
			d = DecisionApproval
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
// Uses \b word boundary (not ^) because classification normalization may strip
// env var prefixes, leaving residual tokens before the actual command.
var reSafeHeredocCmd = regexp.MustCompile(`\b(git\s+commit|git\s+tag|gh\s+pr\s+create|gh\s+issue\s+create)\b`)

func isSafeHeredocUsage(cmd string) bool {
	return reSafeHeredocCmd.MatchString(cmd)
}

// reCatHeredoc matches $(cat <<'EOF' or $(cat <<EOF or $(\ncat <<
var reCatHeredoc = regexp.MustCompile(`\$\(\s*\n?\s*cat\s+<<`)

// isCatHeredocSubstitution returns true when the $() in the command is
// specifically a cat<<heredoc pattern (passing a multi-line string literal).
func isCatHeredocSubstitution(cmd string) bool {
	return reCatHeredoc.MatchString(cmd)
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
	// Normalize backslashes to forward slashes for consistent cross-platform parsing.
	normalizedPath := strings.ReplaceAll(fields[0], `\`, "/")
	return filepath.Base(normalizedPath)
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

// inlineBodiesResult holds the full result from inline body classification.
type inlineBodiesResult struct {
	decision            Decision
	reason              string
	body                string
	complete            bool
	failClosed          bool
	dryRunMatches       []BuiltinMatch
	tagOverrideEnforced bool
}

// classifyInlineBodies extracts inline script content (heredocs, command substitutions)
// and classifies each extracted body through the classification pipeline.
func classifyInlineBodies(cmd string, evaluator PolicyEvaluator, cwd string) inlineBodiesResult {
	return classifyInlineBodiesRecursive(cmd, evaluator, cwd, 0)
}

func classifyInlineBodiesRecursive(cmd string, evaluator PolicyEvaluator, cwd string, depth int) inlineBodiesResult {
	if depth >= maxRecursionDepth {
		return inlineBodiesResult{complete: false} // depth exhausted → incomplete
	}

	heredocBody, heredocComplete := extractHeredocBody(cmd)
	cmdSubsts, cmdSubstComplete := extractCommandSubstitutions(cmd)

	if heredocBody == "" && len(cmdSubsts) == 0 {
		return inlineBodiesResult{complete: heredocComplete && cmdSubstComplete}
	}

	acc := &inlineAccumulator{complete: heredocComplete && cmdSubstComplete}

	if heredocBody != "" {
		acc.classifyHeredocBody(heredocBody, evaluator, cwd, depth)
	}
	for _, subst := range cmdSubsts {
		acc.classifyExtractedCmd(subst, "inline $()", evaluator, cwd, depth)
	}

	var allBodies []string
	if heredocBody != "" {
		allBodies = append(allBodies, heredocBody)
	}
	allBodies = append(allBodies, cmdSubsts...)
	allBodies = append(allBodies, acc.nestedBodies...)

	return inlineBodiesResult{
		decision:            acc.bestDecision,
		reason:              acc.bestReason,
		body:                strings.Join(allBodies, "\n---\n"),
		complete:            acc.complete,
		failClosed:          acc.failClosed,
		dryRunMatches:       acc.dryRunMatches,
		tagOverrideEnforced: acc.tagOverrideEnforced,
	}
}

// inlineAccumulator tracks the most restrictive decision across inline body classifications.
type inlineAccumulator struct {
	bestDecision        Decision
	bestReason          string
	complete            bool
	failClosed          bool
	nestedBodies        []string       // bodies from nested extraction (depth > 0)
	dryRunMatches       []BuiltinMatch // collected from inline rule evaluations
	tagOverrideEnforced bool           // OR across all inline evaluations
}

func (a *inlineAccumulator) update(d Decision, reason string) {
	if d != "" && (a.bestDecision == "" || DecisionSeverity(d) > DecisionSeverity(a.bestDecision)) {
		a.bestDecision = d
		a.bestReason = reason
	}
}

func (a *inlineAccumulator) applyResult(r extractedSubCommandResult, label string) {
	a.update(r.decision, label+": "+r.reason)
	a.dryRunMatches = append(a.dryRunMatches, r.dryRunMatches...)
	if r.tagOverrideEnforced {
		a.tagOverrideEnforced = true
	}
	if r.failClosed {
		a.failClosed = true
	}
}

func (a *inlineAccumulator) classifyHeredocBody(body string, evaluator PolicyEvaluator, cwd string, depth int) {
	subCmds, err := SplitCompoundCommand(body)
	if err != nil {
		a.complete = false // parse failure → incomplete extraction (SEC-009 fail-closed)
		r := classifyExtractedSubCommand(body, evaluator, cwd)
		a.applyResult(r, "inline heredoc")
		return
	}
	for _, sub := range subCmds {
		a.classifyExtractedCmd(sub, "inline heredoc", evaluator, cwd, depth)
	}
}

func (a *inlineAccumulator) classifyExtractedCmd(cmd, label string, evaluator PolicyEvaluator, cwd string, depth int) {
	r := classifyExtractedSubCommand(cmd, evaluator, cwd)
	a.applyResult(r, label)

	nested := classifyInlineBodiesRecursive(cmd, evaluator, cwd, depth+1)
	if !nested.complete {
		a.complete = false
	}
	if nested.failClosed {
		a.failClosed = true
	}
	a.update(nested.decision, nested.reason)
	a.dryRunMatches = append(a.dryRunMatches, nested.dryRunMatches...)
	if nested.tagOverrideEnforced {
		a.tagOverrideEnforced = true
	}
	if nested.body != "" {
		a.nestedBodies = append(a.nestedBodies, nested.body)
	}
}

// classifyExtractedSubCommand runs the full classification pipeline on an extracted
// inline command (heredoc body line or $() content). Unlike classifySingleCommand,
// this runs ClassificationNormalize first (wrapper stripping, bash -c extraction,
// sudo/doas escalation) and checks sensitive env vars — matching the full
// classifySubCommand pipeline. Does NOT recurse into inline extraction (that's
// handled by the caller via classifyInlineBodiesRecursive).
// extractedSubCommandResult holds the full result from classifying an extracted inline command.
type extractedSubCommandResult struct {
	decision            Decision
	reason              string
	failClosed          bool
	dryRunMatches       []BuiltinMatch
	tagOverrideEnforced bool
}

func classifyExtractedSubCommand(subCmd string, evaluator PolicyEvaluator, cwd string) extractedSubCommandResult {
	classified := ClassificationNormalize(subCmd)

	if classified.ExtractionFailed {
		return extractedSubCommandResult{
			decision:   DecisionApproval,
			reason:     "inline command extraction failed (fail-closed)",
			failClosed: true,
		}
	}

	allCmds := []string{classified.Outer}
	allCmds = append(allCmds, classified.Inner...)

	result := extractedSubCommandResult{decision: DecisionSafe, reason: "default safe"}
	for _, cmd := range allCmds {
		if cmd == "" {
			continue
		}
		mergeClassificationInto(&result, classifySingleCommand(cmd, evaluator, cwd, nil))
	}

	applyExtractedEscalations(&result, subCmd, classified)
	return result
}

// mergeClassificationInto folds a single-command classification into an
// accumulated extracted sub-command result, taking the most restrictive decision.
func mergeClassificationInto(result *extractedSubCommandResult, c commandClassificationResult) {
	if combined := MaxDecision(result.decision, c.decision); combined != result.decision {
		result.decision = combined
		result.reason = c.reason
	}
	if c.failClosed {
		result.failClosed = true
	}
	result.dryRunMatches = append(result.dryRunMatches, c.dryRunMatches...)
	if c.tagOverrideEnforced {
		result.tagOverrideEnforced = true
	}
}

// applyExtractedEscalations applies sudo/doas and env-var escalations to an
// extracted sub-command result.
func applyExtractedEscalations(result *extractedSubCommandResult, subCmd string, classified ClassifiedCommand) {
	if classified.EscalateClassification {
		result.decision, result.reason = escalateDecision(result.decision, result.reason)
	}
	if reSensitiveEnvVar.MatchString(subCmd) {
		if escalated := MaxDecision(result.decision, DecisionCaution); escalated != result.decision {
			result.decision = escalated
			result.reason = "references sensitive environment variable"
		}
	}
	if classified.SensitiveEnvAssignment {
		if combined := MaxDecision(result.decision, DecisionApproval); combined != result.decision {
			result.decision = combined
			result.reason = "security-sensitive environment variable assignment in inline body"
		}
	}
}

func isInspectTriggerRule(ruleID string) bool {
	switch ruleID {
	case "builtin:interp:python-file", "builtin:interp:node-file", "builtin:interp:bash-file":
		return true
	default:
		return false
	}
}
