package judge

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/php-workx/fuse/internal/config"
	"github.com/php-workx/fuse/internal/core"
)

// Judge evaluates command classifications using an LLM provider.
type Judge struct {
	provider           Provider
	mode               string // "shadow", "active"
	upgradeThreshold   float64
	downgradeThreshold float64
	triggerDecisions   map[core.Decision]bool
	timeout            time.Duration
	rateLimiter        *rateLimiter
}

// Verdict holds the LLM judge's evaluation result.
type Verdict struct {
	OriginalDecision core.Decision
	JudgeDecision    core.Decision
	Confidence       float64
	Reasoning        string
	Applied          bool // true if the judge's decision was used
	ProviderName     string
	LatencyMs        int64
	Error            string // non-empty if judge failed
}

// NewJudge creates a Judge from config. Detects the provider and parses settings.
func NewJudge(cfg config.LLMJudgeConfig) (*Judge, error) {
	provider, err := DetectProvider(cfg.Provider, cfg.Model)
	if err != nil {
		return nil, fmt.Errorf("detect provider: %w", err)
	}

	timeout := 10 * time.Second
	if cfg.Timeout != "" {
		if d, parseErr := time.ParseDuration(cfg.Timeout); parseErr == nil && d > 0 {
			timeout = d
		}
	}

	upgradeThreshold := 0.7
	if cfg.UpgradeThreshold > 0 {
		upgradeThreshold = cfg.UpgradeThreshold
	}

	downgradeThreshold := 0.95
	if cfg.DowngradeThreshold > 0 {
		downgradeThreshold = cfg.DowngradeThreshold
	}

	triggerDecisions := map[core.Decision]bool{
		core.DecisionCaution:  true,
		core.DecisionApproval: true,
	}
	if len(cfg.TriggerDecisions) > 0 {
		triggerDecisions = make(map[core.Decision]bool)
		for _, d := range cfg.TriggerDecisions {
			triggerDecisions[core.Decision(strings.ToUpper(d))] = true
		}
	}

	maxCalls := 30
	if cfg.MaxCallsPerMinute > 0 {
		maxCalls = cfg.MaxCallsPerMinute
	}

	return &Judge{
		provider:           provider,
		mode:               cfg.Mode,
		upgradeThreshold:   upgradeThreshold,
		downgradeThreshold: downgradeThreshold,
		triggerDecisions:   triggerDecisions,
		timeout:            timeout,
		rateLimiter:        newRateLimiter(maxCalls),
	}, nil
}

// Evaluate queries the LLM judge for a second opinion on the classification.
// Returns nil if the judge is not triggered (SAFE, BLOCKED, or not in trigger list).
func (j *Judge) Evaluate(ctx context.Context, result *core.ClassifyResult, promptCtx PromptContext) *Verdict {
	verdict := &Verdict{
		OriginalDecision: result.Decision,
		ProviderName:     j.provider.Name(),
	}

	// Only trigger on configured decisions (CAUTION, APPROVAL by default).
	if !j.triggerDecisions[result.Decision] {
		return nil
	}

	// Never override BLOCKED (defense-in-depth, redundant with trigger check).
	if result.Decision == core.DecisionBlocked {
		return nil
	}

	// Check rate limit.
	if !j.rateLimiter.Allow() {
		slog.Warn("llm judge rate limited, skipping")
		return nil
	}

	// Query the LLM with timeout.
	judgeCtx, cancel := context.WithTimeout(ctx, j.timeout)
	defer cancel()

	start := time.Now()
	userPrompt := BuildUserPrompt(promptCtx)
	response, err := j.provider.Query(judgeCtx, systemPrompt, userPrompt)
	verdict.LatencyMs = time.Since(start).Milliseconds()

	if err != nil {
		verdict.Error = err.Error()
		return verdict // fail-open: original classification stands
	}

	// Parse and validate response.
	parsed, parseErr := ParseResponse(response)
	if parseErr != nil {
		verdict.Error = fmt.Sprintf("parse: %v", parseErr)
		return verdict
	}

	verdict.JudgeDecision = core.Decision(parsed.Decision)
	verdict.Confidence = parsed.Confidence
	verdict.Reasoning = parsed.Reasoning

	// Apply confidence thresholds (active mode only).
	if j.mode != "active" {
		verdict.Applied = false
		return verdict // shadow mode: log only
	}

	judgeSeverity := core.DecisionSeverity(verdict.JudgeDecision)
	origSeverity := core.DecisionSeverity(result.Decision)

	switch {
	case judgeSeverity > origSeverity:
		// Upgrade (more restrictive) — lower threshold needed.
		verdict.Applied = parsed.Confidence >= j.upgradeThreshold
	case judgeSeverity < origSeverity:
		// Downgrade (more permissive) — higher threshold needed.
		verdict.Applied = parsed.Confidence >= j.downgradeThreshold
	default:
		// Same decision — judge agrees.
		verdict.Applied = false
	}

	return verdict
}

// rateLimiter is a simple token bucket that refills per minute.
type rateLimiter struct {
	mu        sync.Mutex
	tokens    int
	maxTokens int
	lastReset time.Time
}

func newRateLimiter(maxPerMinute int) *rateLimiter {
	return &rateLimiter{
		tokens:    maxPerMinute,
		maxTokens: maxPerMinute,
		lastReset: time.Now(),
	}
}

func (r *rateLimiter) Allow() bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Refill if a minute has passed.
	if time.Since(r.lastReset) >= time.Minute {
		r.tokens = r.maxTokens
		r.lastReset = time.Now()
	}

	if r.tokens <= 0 {
		return false
	}
	r.tokens--
	return true
}

// --- MaybeJudge: adapter integration helper ---

var (
	cachedJudge   *Judge
	cachedJudgeMu sync.Mutex
	cachedCfgHash string
)

// configHash returns a stable string for cache invalidation.
func configHash(cfg config.LLMJudgeConfig) string {
	return fmt.Sprintf("%+v", cfg)
}

// MaybeJudge runs the LLM judge if configured. Returns the (possibly modified)
// ClassifyResult and a Verdict for logging. If judge is disabled or not triggered,
// returns the original result and nil verdict.
//
// The Judge instance is cached and re-initialized when the config changes,
// supporting hot-reload for long-running adapters (codex-shell).
func MaybeJudge(ctx context.Context, cfg *config.Config, result *core.ClassifyResult, promptCtx PromptContext) (*core.ClassifyResult, *Verdict) {
	if cfg == nil || cfg.LLMJudge.Mode == "off" {
		return result, nil
	}

	// Default mode to "shadow" when llm_judge section is present but mode is empty.
	judgeCfg := cfg.LLMJudge
	if judgeCfg.Mode == "" {
		judgeCfg.Mode = "shadow"
	}

	cachedJudgeMu.Lock()
	hash := configHash(judgeCfg)
	if cachedJudge == nil || hash != cachedCfgHash {
		j, err := NewJudge(judgeCfg)
		if err != nil {
			cachedJudgeMu.Unlock()
			slog.Warn("llm judge init failed", "error", err)
			return result, nil
		}
		cachedJudge = j
		cachedCfgHash = hash
	}
	j := cachedJudge
	cachedJudgeMu.Unlock()

	verdict := j.Evaluate(ctx, result, promptCtx)
	if verdict == nil {
		return result, nil
	}

	if verdict.Applied {
		result = result.WithDecision(verdict.JudgeDecision,
			fmt.Sprintf("LLM judge (%s): %s", verdict.ProviderName, verdict.Reasoning))
	}

	return result, verdict
}
