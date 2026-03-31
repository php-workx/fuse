package adapters

import (
	"strings"

	"github.com/php-workx/fuse/internal/config"
	"github.com/php-workx/fuse/internal/core"
	"github.com/php-workx/fuse/internal/judge"
)

// StructuralDecision returns the classifier's pre-fallback decision. When the
// judge reviewed a command, Verdict.OriginalDecision is the structural tier.
func StructuralDecision(result *core.ClassifyResult, verdict *judge.Verdict) core.Decision {
	if verdict != nil && verdict.OriginalDecision != "" {
		return verdict.OriginalDecision
	}
	if result == nil {
		return ""
	}
	return result.Decision
}

// EffectiveDecision returns the enforced decision after applying profile-level
// CAUTION fallback behavior on top of the classifier and judge result.
func EffectiveDecision(result *core.ClassifyResult, verdict *judge.Verdict, cfg *config.Config) core.Decision {
	if result == nil {
		return ""
	}
	decision := result.Decision
	if decision != core.DecisionCaution {
		return decision
	}
	if cfg == nil || !strings.EqualFold(strings.TrimSpace(cfg.CautionFallback), "approve") {
		return decision
	}
	if judgeReviewedActively(cfg, verdict) {
		return decision
	}
	return core.DecisionApproval
}

func judgeReviewedActively(cfg *config.Config, verdict *judge.Verdict) bool {
	if cfg == nil || verdict == nil {
		return false
	}
	if !strings.EqualFold(strings.TrimSpace(cfg.LLMJudge.Mode), "active") {
		return false
	}
	return verdict.Error == ""
}

func resolvedProfile(cfg *config.Config) string {
	if cfg == nil || strings.TrimSpace(cfg.Profile) == "" {
		return config.ProfileRelaxed
	}
	return cfg.Profile
}
