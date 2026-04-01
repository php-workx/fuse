package adapters

import (
	"testing"

	"github.com/php-workx/fuse/internal/config"
	"github.com/php-workx/fuse/internal/core"
	"github.com/php-workx/fuse/internal/judge"
)

func TestEffectiveDecision_CautionApproveWithoutJudgeReview(t *testing.T) {
	result := &core.ClassifyResult{Decision: core.DecisionCaution}
	cfg := &config.Config{CautionFallback: "approve"}
	if got := EffectiveDecision(result, nil, cfg); got != core.DecisionApproval {
		t.Fatalf("EffectiveDecision = %q, want APPROVAL", got)
	}
}

func TestEffectiveDecision_CautionStaysCautionAfterActiveJudgeReview(t *testing.T) {
	result := &core.ClassifyResult{Decision: core.DecisionCaution}
	cfg := &config.Config{
		CautionFallback: "approve",
		LLMJudge:        config.LLMJudgeConfig{Mode: "active"},
	}
	verdict := &judge.Verdict{OriginalDecision: core.DecisionCaution}
	if got := EffectiveDecision(result, verdict, cfg); got != core.DecisionCaution {
		t.Fatalf("EffectiveDecision = %q, want CAUTION", got)
	}
}

func TestEffectiveDecision_CautionStaysCautionOnJudgeError(t *testing.T) {
	result := &core.ClassifyResult{Decision: core.DecisionCaution}
	cfg := &config.Config{
		CautionFallback: "approve",
		LLMJudge:        config.LLMJudgeConfig{Mode: "active"},
	}
	verdict := &judge.Verdict{OriginalDecision: core.DecisionCaution, Error: "timeout"}
	if got := EffectiveDecision(result, verdict, cfg); got != core.DecisionCaution {
		t.Fatalf("EffectiveDecision = %q, want CAUTION", got)
	}
}

func TestEffectiveDecision_CautionStaysCautionWhenJudgeFails(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *config.Config
		verdict *judge.Verdict
	}{
		{
			name: "balanced profile with approve fallback",
			cfg: &config.Config{
				Profile:         config.ProfileBalanced,
				CautionFallback: "approve",
				LLMJudge:        config.LLMJudgeConfig{Mode: "active"},
			},
			verdict: &judge.Verdict{OriginalDecision: core.DecisionCaution, Error: "timeout"},
		},
		{
			name: "strict profile with log fallback",
			cfg: &config.Config{
				Profile:         config.ProfileStrict,
				CautionFallback: "log",
				LLMJudge:        config.LLMJudgeConfig{Mode: "active"},
			},
			verdict: &judge.Verdict{OriginalDecision: core.DecisionCaution, Error: "rate limited"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := &core.ClassifyResult{Decision: core.DecisionCaution}
			if got := EffectiveDecision(result, tc.verdict, tc.cfg); got != core.DecisionCaution {
				t.Fatalf("EffectiveDecision = %q, want CAUTION", got)
			}
		})
	}
}

func TestEffectiveDecision_ApprovalStaysApproval(t *testing.T) {
	result := &core.ClassifyResult{Decision: core.DecisionApproval}
	cfg := &config.Config{CautionFallback: "approve"}
	if got := EffectiveDecision(result, nil, cfg); got != core.DecisionApproval {
		t.Fatalf("EffectiveDecision = %q, want APPROVAL", got)
	}
}
