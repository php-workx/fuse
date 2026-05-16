package core

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/php-workx/fuse/internal/inspect"
)

// maxInputSize is the maximum allowed raw command length (10 KB). Tightened
// from 64 KB so the ~150 builtin regexes have a bounded worst-case input;
// realistic commands are well below 10 KB (see commands.yaml fixture corpus).
const maxInputSize = 10 * 1024

// Reason strings emitted by the fallback layers. Exported so tests and callers
// can distinguish an explicit safe-rule match (UnconditionallySafeReason /
// ConditionallySafeReason) from the default-safe fallback
// (UnknownCommandFallbackReason).
const (
	UnconditionallySafeReason    = "unconditionally safe command"
	ConditionallySafeReason      = "conditionally safe command"
	UnknownCommandFallbackReason = "unknown command (no matching rule)"
)

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

var criticalIncompleteAnalysisPatterns = []*regexp.Regexp{
	regexp.MustCompile(`\brm\s+.*-[^\s]*r[^\s]*f|-[^\s]*f[^\s]*r`),
	regexp.MustCompile(`\b(?:terraform|tofu)\s+(?:apply|destroy|plan\s+-destroy|state\s+rm|workspace\s+delete)\b`),
	regexp.MustCompile(`\bpulumi\s+(?:up|destroy|stack\s+rm|state\s+delete)\b`),
	regexp.MustCompile(`\bcdk\s+(?:deploy|destroy)\b`),
	regexp.MustCompile(`\bkubectl\s+(?:delete|replace\s+--force|drain)\b`),
	regexp.MustCompile(`\b(?:aws|gcloud|az)\b.*\b(?:delete|destroy|remove|purge|terminate)\b`),
	regexp.MustCompile(`\b(?:curl|wget)\b.*\|\s*(?:ba)?sh\b`),
	regexp.MustCompile(`/dev/tcp/|\b(?:nc|ncat|netcat)\b.*\s-e\s+`),
	regexp.MustCompile(`(?i)\b(?:AWS_SECRET_ACCESS_KEY|GITHUB_TOKEN|GH_TOKEN|DATABASE_URL|DB_PASSWORD|API_KEY|SECRET_KEY|PRIVATE_KEY)\b`),
	regexp.MustCompile(`(?i)(?:^|\s)(?:PATH|LD_PRELOAD|LD_LIBRARY_PATH|DYLD_[A-Z0-9_]*|NODE_OPTIONS|GIT_EXEC_PATH|HOME)=`),
	regexp.MustCompile(`(?:^|\s)(?:~?/)?\.?(?:aws/credentials|ssh/id_rsa|env)\b|\.ssh/authorized_keys\b`),
	regexp.MustCompile(`(?:~|/Users/[^/\s]+|/home/[^/\s]+)/\.fuse/|(?:~|/Users/[^/\s]+|/home/[^/\s]+)/\.claude/`),
	regexp.MustCompile(`\b(?:fuse|epos|tk)\s+(?:disable|uninstall|close|reopen|edit|new|claim|release)\b`),
}

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

var (
	rePowerShellDownloadPipeIEX    = regexp.MustCompile(`(?i)\b(Invoke-WebRequest|iwr|Invoke-RestMethod|irm)\b.*\|\s*(Invoke-Expression|iex)\b`)
	rePowerShellDownloadContentIEX = regexp.MustCompile(`(?i)\b(Invoke-Expression|iex)\b.*\b(Invoke-WebRequest|iwr)\b.*\.Content\b`)
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
		result.DecisionKey = ComputeDecisionKey(displayNorm, "")
		return result, nil
	}

	// Step 2: Display normalize.
	displayNorm := DisplayNormalize(req.RawCommand)

	if IsProvableMktempCleanup(displayNorm) {
		result.Decision = DecisionCaution
		result.Reason = "provable mktemp cleanup"
		result.DecisionKey = ComputeDecisionKey(displayNorm, "")
		return result, nil
	}

	// Step 3: Compound command splitting.
	subCmds, err := SplitCompoundCommand(displayNorm)
	if err != nil {
		return classifyCompoundSplitError(result, displayNorm, evaluator, err)
	}
	effectiveCwd, suppressCwdEscalation := simpleLeadingAbsoluteCD(subCmds, req.Cwd)

	// Classify all sub-commands and aggregate results.
	fileHashes := classifyAllSubCommands(result, subCmds, evaluator, effectiveCwd)

	// Apply compound-level modifiers.
	applyCompoundModifiers(result, subCmds, displayNorm, evaluator, suppressCwdEscalation)

	// Step 12: Compute decision key.
	combinedHash := strings.Join(fileHashes, ":")
	result.DecisionKey = ComputeDecisionKey(displayNorm, combinedHash)

	return result, nil
}

// classifyCompoundSplitError handles the case where compound splitting fails.
// Checks hardcoded rules before falling back to fail-closed APPROVAL.
func classifyCompoundSplitError(result *ClassifyResult, displayNorm string, evaluator PolicyEvaluator, splitErr error) (*ClassifyResult, error) {
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
				result.DecisionKey = ComputeDecisionKey(displayNorm, "")
				return result, nil
			}
		}
	}
	if hasCriticalIncompleteAnalysisIndicator(displayNorm, evaluator) {
		result.Decision = DecisionApproval
		result.Reason = fmt.Sprintf("compound split error with critical indicators (approval required): %v", splitErr)
		result.FailClosed = true
	} else {
		result.Decision = DecisionCaution
		result.Reason = fmt.Sprintf("compound split error without critical indicators (logged only): %v", splitErr)
	}
	result.DecisionKey = ComputeDecisionKey(displayNorm, "")
	return result, nil
}

// classifyAllSubCommands classifies each sub-command and aggregates results into the overall result.
// Returns collected file hashes for decision key computation.
func classifyAllSubCommands(result *ClassifyResult, subCmds []string, evaluator PolicyEvaluator, cwd string) []string {
	overallDecision := DecisionSafe
	overallReason := "all sub-commands safe"
	overallRuleID := ""
	var fileHashes []string
	singleSubCommand := len(subCmds) == 1

	for _, subCmd := range subCmds {
		sub := classifySubCommand(subCmd, evaluator, cwd)
		result.SubResults = append(result.SubResults, sub)
		result.DryRunMatches = append(result.DryRunMatches, sub.DryRunMatches...)

		newOverall := MaxDecision(overallDecision, sub.Decision)
		// Propagate the sub-command's reason when the decision escalates OR when
		// there is only one sub-command — otherwise a lone explicit SAFE rule is
		// hidden behind the generic "all sub-commands safe" aggregate.
		if newOverall != overallDecision || singleSubCommand {
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
func applyCompoundModifiers(result *ClassifyResult, subCmds []string, displayNorm string, evaluator PolicyEvaluator, suppressCwdEscalation bool) {
	if evaluator != nil {
		basename := extractBasename(displayNorm)
		sanitized := SanitizeForClassification(displayNorm, KnownSafeVerbs[basename])

		// Keep compound Windows IEX/download detections in the normal policy path
		// so dry-run/tag override semantics are honored.
		if rePowerShellDownloadContentIEX.MatchString(displayNorm) {
			pr := evaluatePolicyRules(displayNorm, sanitized, evaluator, nil, result.DryRunMatches)
			applyCompoundPolicyMatch(result, pr)
		}

		if rePowerShellDownloadPipeIEX.MatchString(displayNorm) {
			pr := evaluatePolicyRules(displayNorm, sanitized, evaluator, nil, result.DryRunMatches)
			applyCompoundPolicyMatch(result, pr)
		}
	}

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
	if !suppressCwdEscalation && len(subCmds) > 1 && ContainsCwdChange(subCmds) {
		combined := MaxDecision(result.Decision, DecisionCaution)
		if combined != result.Decision {
			result.Decision = combined
			result.Reason = "compound command contains cwd-changing builtin (cd/pushd/popd)"
			result.RuleID = ""
		}
	}
}

func simpleLeadingAbsoluteCD(subCmds []string, currentCwd string) (string, bool) {
	if len(subCmds) < 2 {
		return currentCwd, false
	}
	fields := strings.Fields(subCmds[0])
	if len(fields) != 2 || fields[0] != "cd" {
		return currentCwd, false
	}
	target := strings.Trim(fields[1], `"'`)
	if target == "" ||
		!filepath.IsAbs(target) ||
		strings.ContainsAny(target, "$`") ||
		strings.Contains(target, "$(") ||
		strings.Contains(target, "..") {
		return currentCwd, false
	}
	cleaned := filepath.Clean(target)
	if isSensitiveCDTarget(cleaned) || !isTrustedWorkspace(cleaned) {
		return currentCwd, false
	}
	return cleaned, true
}

// sensitiveCDTargetRoots are absolute path prefixes (and the prefixes
// themselves) considered system-sensitive. A leading `cd <prefix>` into one of
// these locations must retain the cwd-change CAUTION escalation, even when
// followed by an otherwise read-only chain. Subdirectories under these roots
// inherit the sensitivity (e.g. /etc/ssh, /usr/local/bin).
var sensitiveCDTargetRoots = []string{
	// Linux/Unix system roots.
	"/etc",
	"/usr",
	"/bin",
	"/sbin",
	"/boot",
	"/dev",
	"/proc",
	"/sys",
	"/lib",
	"/lib32",
	"/lib64",
	"/root",
	"/opt",
	"/var",
	// macOS system roots.
	"/System",
	"/Library",
	"/Applications",
	"/Volumes",
	"/cores",
	// Resolved firmlinks for the system roots above (macOS).
	"/private/etc",
	"/private/var",
	"/private/usr",
}

// isSensitiveCDTarget reports whether the given absolute, cleaned path is a
// sensitive system location. Sensitive targets must not have their cwd-change
// CAUTION suppressed, regardless of how benign subsequent commands look.
func isSensitiveCDTarget(target string) bool {
	if target == "/" {
		return true
	}
	for _, root := range sensitiveCDTargetRoots {
		if target == root || strings.HasPrefix(target, root+"/") {
			return true
		}
	}
	return false
}

// trustedWorkspaceRoots are the directory roots under which ordinary user
// workspace directories live. CAUTION suppression for a leading absolute cd is
// only allowed when the target is strictly inside one of these roots (the
// roots themselves are not considered trusted workspaces).
var trustedWorkspaceRoots = []string{
	"/home",  // Linux user home directories
	"/Users", // macOS user home directories
}

// isTrustedWorkspace reports whether the given absolute, cleaned path falls
// within a trusted user workspace hierarchy. Only paths strictly under /home/*
// or /Users/* qualify; paths like /tmp, /mnt, /srv, or the roots /home and
// /Users themselves do not.
func isTrustedWorkspace(target string) bool {
	for _, root := range trustedWorkspaceRoots {
		if strings.HasPrefix(target, root+"/") {
			return true
		}
	}
	return false
}

func applyCompoundPolicyMatch(result *ClassifyResult, pr policyResult) {
	result.DryRunMatches = pr.dryRunMatches
	if pr.tagOverrideEnforced {
		result.TagOverrideEnforced = true
	}
	if pr.failClosed {
		result.FailClosed = true
	}
	if !pr.matched {
		return
	}

	combined := MaxDecision(result.Decision, pr.decision)
	if combined != result.Decision {
		result.Decision = combined
		result.Reason = pr.reason
		result.RuleID = pr.ruleID
	}
}

// classifySubCommand runs the per-sub-command pipeline (steps 4-11).
func classifySubCommand(subCmd string, evaluator PolicyEvaluator, cwd string) SubCommandResult {
	sub := SubCommandResult{Command: subCmd}
	var rawPolicyMatch *policyResult

	if IsFuseTestClassify(subCmd) {
		sub.Decision = DecisionSafe
		sub.Reason = "fuse test classify payload is inert"
		return sub
	}

	// Pre-normalization rule checks: tokenization can treat backslashes as escape
	// characters, which mangles Windows paths like C:\Windows into C:Windows.
	// Check raw command text before normalization strips those separators.
	if evaluator != nil {
		if d, reason := evaluator.EvaluateHardcoded(subCmd); d != "" {
			sub.Decision = d
			sub.Reason = reason
			return sub
		}

		if DetectShellType(subCmd) != ShellBash || strings.Contains(subCmd, `\`) {
			rawBasename := extractBasename(subCmd)
			rawSanitized := SanitizeForClassification(subCmd, KnownSafeVerbs[rawBasename])
			if pr := evaluatePolicyRules(subCmd, rawSanitized, evaluator, nil, nil); pr.matched || len(pr.dryRunMatches) > 0 {
				rawPolicyMatch = &pr
			}
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

	if rawPolicyMatch != nil {
		sub.DryRunMatches = append(sub.DryRunMatches, rawPolicyMatch.dryRunMatches...)
		if rawPolicyMatch.tagOverrideEnforced {
			sub.TagOverrideEnforced = true
		}
		if rawPolicyMatch.failClosed {
			sub.FailClosed = true
		}
		if combined := MaxDecision(sub.Decision, rawPolicyMatch.decision); combined != sub.Decision {
			sub.Decision = combined
			sub.Reason = rawPolicyMatch.reason
			sub.RuleID = rawPolicyMatch.ruleID
			sub.TagOverrideEnforced = rawPolicyMatch.tagOverrideEnforced
			sub.FailClosed = rawPolicyMatch.failClosed
		}
	}

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
	sawClassification := false

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
		// Propagate the reason when the decision escalates, or when this is the
		// first real classification we've seen — so callers can tell whether an
		// explicit SAFE rule fired vs the default-safe fallback.
		if combined != bestDecision || !sawClassification {
			bestDecision = combined
			bestReason = classification.reason
			bestRuleID = classification.ruleID
			tagOverrideEnforced = classification.tagOverrideEnforced
		}
		sawClassification = true
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
		switch {
		case combined != sub.Decision:
			sub.Decision = combined
			sub.Reason = inlineResult.reason
			sub.RuleID = "" // inline analysis wins — clear stale RuleID
		case inlineResult.reason != "" &&
			DecisionSeverity(inlineResult.decision) == DecisionSeverity(sub.Decision) &&
			isGenericInlineReason(sub.Reason):
			// Same-severity body finding is more actionable than the generic
			// "inline script detected" marker (or an absent reason) that fired
			// earlier — prefer the inline body reason so operators see what
			// actually happened inside the heredoc.
			sub.Reason = inlineResult.reason
			sub.RuleID = ""
		}
	}
	if !inlineResult.complete && DecisionSeverity(sub.Decision) < DecisionSeverity(DecisionApproval) {
		if hasCriticalIncompleteAnalysisIndicator(subCmd+"\n"+inlineResult.body, evaluator) {
			sub.Decision = DecisionApproval
			sub.Reason = "inline script extraction incomplete with critical indicators (approval required)"
			sub.FailClosed = true
		} else {
			sub.Decision = DecisionCaution
			sub.Reason = "inline script extraction incomplete without critical indicators (logged only)"
		}
		sub.RuleID = ""
	}
}

func hasCriticalIncompleteAnalysisIndicator(text string, evaluator PolicyEvaluator) bool {
	if evaluator != nil {
		if d, _ := evaluator.EvaluateHardcoded(text); d != "" {
			return true
		}
		if match := evaluator.EvaluateBuiltins(text); match != nil && DecisionSeverity(match.Decision) >= DecisionSeverity(DecisionApproval) {
			return true
		}
	}
	for _, re := range criticalIncompleteAnalysisPatterns {
		if re.MatchString(text) {
			return true
		}
	}
	return false
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
	cmdForURLInspection := cmd
	if inlineBody != "" {
		cmdForURLInspection = stripHeredocBodiesForURLInspection(cmd)
	}
	if d, r := InspectCommandURLs(cmdForURLInspection); d != "" {
		escalate(d, r)
	}
	if inlineBody != "" {
		for _, line := range strings.Split(inlineBody, "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			if !shouldInspectInlineURLLine(line) {
				continue
			}
			if d, r := InspectCommandURLs(line); d != "" {
				escalate(d, r)
			}
		}
	}
}

var inlineActiveNetworkCallPatterns = []*regexp.Regexp{
	regexp.MustCompile(`\burlopen\s*\(`),
	regexp.MustCompile(`\brequests\.(?:request|get|post|put|patch|delete|head|options)\s*\(`),
	regexp.MustCompile(`\bhttpx\.(?:request|get|post|put|patch|delete|head|options|stream)\s*\(`),
}

var heredocDelimiterPattern = regexp.MustCompile(`<<-?\s*(?:"([^"]+)"|'([^']+)'|([A-Za-z_][A-Za-z0-9_]*))`)

func stripHeredocBodiesForURLInspection(cmd string) string {
	lines := strings.Split(cmd, "\n")
	if len(lines) < 2 {
		return cmd
	}

	out := make([]string, 0, len(lines))
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		out = append(out, line)
		for _, delimiter := range heredocDelimiters(line) {
			i++
			for i < len(lines) && strings.TrimSpace(lines[i]) != delimiter {
				i++
			}
			if i < len(lines) {
				out = append(out, lines[i])
			}
		}
	}
	return strings.Join(out, "\n")
}

func heredocDelimiters(line string) []string {
	matches := heredocDelimiterPattern.FindAllStringSubmatch(line, -1)
	delimiters := make([]string, 0, len(matches))
	for _, match := range matches {
		for _, group := range match[1:] {
			if group != "" {
				delimiters = append(delimiters, group)
				break
			}
		}
	}
	return delimiters
}

func shouldInspectInlineURLLine(line string) bool {
	if !strings.Contains(line, "://") {
		return false
	}
	if networkCommandBasenames[extractCmdBasename(line)] {
		return true
	}
	for _, pattern := range inlineActiveNetworkCallPatterns {
		if pattern.MatchString(line) {
			return true
		}
	}
	return false
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
		return commandClassificationResult{decision: DecisionSafe, reason: UnconditionallySafeReason, dryRunMatches: dryRunMatches}
	}

	// Layer 5: Conditionally safe commands.
	if IsConditionallySafe(basename, cmd) {
		return commandClassificationResult{decision: DecisionSafe, reason: ConditionallySafeReason, dryRunMatches: dryRunMatches}
	}

	if reason, ok := KnownUnsafeInspectionVariant(basename, cmd); ok {
		return commandClassificationResult{decision: DecisionCaution, reason: reason, dryRunMatches: dryRunMatches}
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

	// Fallback: SAFE for unknown commands (preserves the default-safe contract).
	return commandClassificationResult{decision: DecisionSafe, reason: UnknownCommandFallbackReason, dryRunMatches: dryRunMatches}
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

var pythonHeredocCommandPattern = regexp.MustCompile(`\b(?:uv\s+run\s+)?python[23]?(?:\s+-[A-Za-z0-9]+)*\s+-\s+<<-?\s*['"]?\w+['"]?`)

// Residual regexes that surface patterns not yet covered by the Python scanner.
// They are consulted as a supplement to inspect.ScanPython so heredoc analysis
// still catches packaging, FFI, getattr, and broader dangerous primitives.
var supplementalPythonHeredocPatterns = []struct {
	re       *regexp.Regexp
	category string
}{
	{regexp.MustCompile(`\bgetattr\s*\(`), "dynamic_exec"},
	{regexp.MustCompile(`\b(?:pty\s*\.\s*spawn|multiprocessing)\b`), "subprocess"},
	{regexp.MustCompile(`\bctypes\b|\bcffi\b`), "subprocess"},
	{regexp.MustCompile(`\bpip\s*\.\s*main\s*\(|\bpip\s+install\b|\bsetuptools\b|\bpkg_resources\s*\.\s*require\b`), "package_install"},
}

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

// genericInlineReasonPrefix is emitted by detectInlineScript when an inline
// script pattern (heredoc, pipe-to-interpreter, command substitution, etc.)
// matches but no more specific body analysis has replaced it. When a
// subsequent inline body classification produces a more actionable reason at
// the same severity, we replace the generic marker — it is less useful to an
// operator than a concrete "Python heredoc reads secret-like file" string.
const genericInlineReasonPrefix = "inline script detected: "

// isGenericInlineReason reports whether the given reason is the generic
// inline-script marker (or empty) and therefore safe to supersede with a more
// specific inline body reason at the same severity.
func isGenericInlineReason(reason string) bool {
	return reason == "" || strings.HasPrefix(reason, genericInlineReasonPrefix)
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
			bestReason = genericInlineReasonPrefix + p.re.String()
		} else {
			combined := MaxDecision(bestDecision, d)
			if combined != bestDecision {
				bestDecision = combined
				bestReason = genericInlineReasonPrefix + p.re.String()
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

func isPythonHeredocCommand(cmd string) bool {
	return pythonHeredocCommandPattern.MatchString(cmd)
}

// classifyPythonHeredocBody inspects a Python heredoc body using the shared
// Python scanner (inspect.ScanPython) plus supplemental heredoc-only regexes.
// The shared scanner provides alias-aware imports (e.g. "from subprocess
// import run"), pathlib write semantics (e.g. "Path(...).open('w')"), and
// network I/O detection; the supplemental patterns cover packaging, FFI,
// getattr, and other primitives not worth promoting to the general scanner.
// Category is mapped to a short actionable reason so callers can surface
// what specifically fired instead of a generic "side effect" string.
func classifyPythonHeredocBody(body string) extractedSubCommandResult {
	signals := inspect.ScanPython([]byte(body))

	// Secret-like file reads take precedence so the reason stays specific.
	for _, s := range signals {
		if s.Category == "secret_read" {
			return extractedSubCommandResult{
				decision: DecisionCaution,
				reason:   "Python heredoc " + pythonHeredocReasonFor(s.Category),
			}
		}
	}

	// Prefer the highest-priority scanner signal (ordering matches the list in
	// pythonHeredocCategoryPriority — more specific categories before generic
	// side effects).
	if sig := highestPriorityPythonSignal(signals); sig != nil {
		return extractedSubCommandResult{
			decision: DecisionCaution,
			reason:   "Python heredoc " + pythonHeredocReasonFor(sig.Category),
		}
	}

	// Fall back to supplemental heredoc-only patterns (packaging, FFI,
	// getattr, pty.spawn, multiprocessing) for classes of risk the shared
	// scanner deliberately does not flag in arbitrary Python files.
	for _, p := range supplementalPythonHeredocPatterns {
		if p.re.MatchString(body) {
			return extractedSubCommandResult{
				decision: DecisionCaution,
				reason:   "Python heredoc " + pythonHeredocReasonFor(p.category),
			}
		}
	}

	return extractedSubCommandResult{decision: DecisionSafe, reason: "Python heredoc body is read-only"}
}

// pythonHeredocCategoryPriority orders scanner categories so more specific
// risks (secret reads, subprocess, destructive filesystem) are reported
// before broader ones when multiple fire on the same body.
var pythonHeredocCategoryPriority = []string{
	"secret_read",
	"subprocess",
	"destructive_fs",
	"http_control_plane",
	"cloud_sdk",
	"dynamic_exec",
	"dynamic_import",
	"network_io",
	"package_install",
}

// highestPriorityPythonSignal returns the first signal whose category appears
// earliest in pythonHeredocCategoryPriority, or nil if none match.
func highestPriorityPythonSignal(signals []inspect.Signal) *inspect.Signal {
	for _, cat := range pythonHeredocCategoryPriority {
		for i := range signals {
			if signals[i].Category == cat {
				return &signals[i]
			}
		}
	}
	return nil
}

// pythonHeredocReasonFor maps a signal category to a short, actionable reason
// fragment appended to "Python heredoc ". Unknown categories fall back to a
// generic side-effect description so we never emit an empty reason.
func pythonHeredocReasonFor(category string) string {
	switch category {
	case "secret_read":
		return "reads secret-like file"
	case "subprocess":
		return "spawns subprocess or invokes os.system"
	case "destructive_fs":
		return "performs destructive filesystem operation"
	case "http_control_plane":
		return "calls cloud control-plane HTTP API"
	case "cloud_sdk":
		return "invokes cloud SDK destructive operation"
	case "dynamic_exec":
		return "executes dynamic code (eval/exec/getattr/compile)"
	case "dynamic_import":
		return "performs dynamic import"
	case "network_io":
		return "performs network I/O"
	case "package_install":
		return "installs or manipulates Python packages"
	default:
		return "contains side-effect or network operation"
	}
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

// extractBasename returns the first token of a command (quote-aware),
// with any path components stripped.
func extractBasename(cmd string) string {
	cmd = strings.TrimSpace(cmd)
	if cmd == "" {
		return ""
	}
	// Extract first token, respecting quotes for paths like "C:\Program Files\...\pwsh.exe".
	var firstToken string
	if cmd[0] == '"' || cmd[0] == '\'' {
		quote := cmd[0]
		if end := strings.IndexByte(cmd[1:], quote); end >= 0 {
			firstToken = cmd[1 : end+1]
		} else {
			firstToken = cmd[1:] // unmatched quote — use rest
		}
	} else {
		firstToken = strings.Fields(cmd)[0]
	}
	// Normalize backslashes to forward slashes for consistent cross-platform parsing.
	normalizedPath := strings.ReplaceAll(firstToken, `\`, "/")
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
	cmdForSubstitutionExtraction := cmd
	if heredocBody != "" {
		cmdForSubstitutionExtraction = stripHeredocBodiesForURLInspection(cmd)
	}
	cmdSubsts, cmdSubstComplete := extractCommandSubstitutions(cmdForSubstitutionExtraction)

	if heredocBody == "" && len(cmdSubsts) == 0 {
		return inlineBodiesResult{complete: heredocComplete && cmdSubstComplete}
	}

	acc := &inlineAccumulator{complete: heredocComplete && cmdSubstComplete}

	if heredocBody != "" {
		if isPythonHeredocCommand(cmd) {
			acc.applyResult(classifyPythonHeredocBody(heredocBody), "inline Python heredoc")
		} else {
			acc.classifyHeredocBody(heredocBody, evaluator, cwd, depth)
		}
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
