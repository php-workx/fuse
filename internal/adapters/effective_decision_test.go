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

func TestEffectiveDecision_CautionApproveOnJudgeError(t *testing.T) {
	result := &core.ClassifyResult{Decision: core.DecisionCaution}
	cfg := &config.Config{
		CautionFallback: "approve",
		LLMJudge:        config.LLMJudgeConfig{Mode: "active"},
	}
	verdict := &judge.Verdict{OriginalDecision: core.DecisionCaution, Error: "timeout"}
	if got := EffectiveDecision(result, verdict, cfg); got != core.DecisionApproval {
		t.Fatalf("EffectiveDecision = %q, want APPROVAL", got)
	}
}

func TestEffectiveDecision_ApprovalStaysApproval(t *testing.T) {
	result := &core.ClassifyResult{Decision: core.DecisionApproval}
	cfg := &config.Config{CautionFallback: "approve"}
	if got := EffectiveDecision(result, nil, cfg); got != core.DecisionApproval {
		t.Fatalf("EffectiveDecision = %q, want APPROVAL", got)
	}
}
