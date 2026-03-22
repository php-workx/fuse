package judge

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/runger/fuse/internal/config"
	"github.com/runger/fuse/internal/core"
)

// mockProvider is a test double that returns configurable responses.
type mockProvider struct {
	name     string
	response string
	err      error
	delay    time.Duration
}

func (m *mockProvider) Name() string { return m.name }

func (m *mockProvider) Query(ctx context.Context, _, _ string) (string, error) {
	if m.delay > 0 {
		select {
		case <-time.After(m.delay):
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	return m.response, m.err
}

func testJudge(mode string, provider Provider) *Judge {
	return &Judge{
		provider:           provider,
		mode:               mode,
		upgradeThreshold:   0.7,
		downgradeThreshold: 0.95,
		triggerDecisions:   map[core.Decision]bool{core.DecisionCaution: true, core.DecisionApproval: true},
		timeout:            5 * time.Second,
		rateLimiter:        newRateLimiter(100),
	}
}

func testResult(decision core.Decision) *core.ClassifyResult {
	return &core.ClassifyResult{
		Decision: decision,
		Reason:   "test reason",
		RuleID:   "test-rule",
	}
}

func testPromptCtx() PromptContext {
	return PromptContext{
		Command:         "echo hello",
		Cwd:             "/tmp",
		CurrentDecision: "CAUTION",
		RuleID:          "test-rule",
	}
}

func TestJudge_ShadowModeDoesNotApply(t *testing.T) {
	j := testJudge("shadow", &mockProvider{
		name:     "test",
		response: `{"decision":"SAFE","confidence":0.95,"reasoning":"safe command"}`,
	})

	verdict := j.Evaluate(context.Background(), testResult(core.DecisionCaution), testPromptCtx())
	if verdict == nil {
		t.Fatal("expected verdict, got nil")
	}
	if verdict.Applied {
		t.Error("shadow mode should not apply verdict")
	}
	if verdict.JudgeDecision != core.DecisionSafe {
		t.Errorf("judge decision = %q, want SAFE", verdict.JudgeDecision)
	}
}

func TestJudge_ActiveModeDowngrade(t *testing.T) {
	j := testJudge("active", &mockProvider{
		name:     "test",
		response: `{"decision":"SAFE","confidence":0.97,"reasoning":"safe command"}`,
	})

	verdict := j.Evaluate(context.Background(), testResult(core.DecisionApproval), testPromptCtx())
	if verdict == nil {
		t.Fatal("expected verdict, got nil")
	}
	if !verdict.Applied {
		t.Error("expected verdict to be applied (0.97 >= 0.95 threshold)")
	}
}

func TestJudge_ActiveModeDowngradeLowConfidence(t *testing.T) {
	j := testJudge("active", &mockProvider{
		name:     "test",
		response: `{"decision":"SAFE","confidence":0.80,"reasoning":"probably safe"}`,
	})

	verdict := j.Evaluate(context.Background(), testResult(core.DecisionApproval), testPromptCtx())
	if verdict == nil {
		t.Fatal("expected verdict, got nil")
	}
	if verdict.Applied {
		t.Error("expected verdict NOT applied (0.80 < 0.95 threshold)")
	}
}

func TestJudge_ActiveModeUpgrade(t *testing.T) {
	j := testJudge("active", &mockProvider{
		name:     "test",
		response: `{"decision":"APPROVAL","confidence":0.75,"reasoning":"risky command"}`,
	})

	verdict := j.Evaluate(context.Background(), testResult(core.DecisionCaution), testPromptCtx())
	if verdict == nil {
		t.Fatal("expected verdict, got nil")
	}
	if !verdict.Applied {
		t.Error("expected verdict to be applied (0.75 >= 0.7 threshold)")
	}
}

func TestJudge_ActiveModeUpgradeLowConfidence(t *testing.T) {
	j := testJudge("active", &mockProvider{
		name:     "test",
		response: `{"decision":"APPROVAL","confidence":0.60,"reasoning":"maybe risky"}`,
	})

	verdict := j.Evaluate(context.Background(), testResult(core.DecisionCaution), testPromptCtx())
	if verdict == nil {
		t.Fatal("expected verdict, got nil")
	}
	if verdict.Applied {
		t.Error("expected verdict NOT applied (0.60 < 0.7 threshold)")
	}
}

func TestJudge_SkipsBlockedDecisions(t *testing.T) {
	j := testJudge("active", &mockProvider{name: "test", response: `{"decision":"SAFE","confidence":1.0,"reasoning":"safe"}`})

	verdict := j.Evaluate(context.Background(), testResult(core.DecisionBlocked), testPromptCtx())
	if verdict != nil {
		t.Error("expected nil verdict for BLOCKED decision")
	}
}

func TestJudge_SkipsSafeDecisions(t *testing.T) {
	j := testJudge("active", &mockProvider{name: "test", response: `{"decision":"SAFE","confidence":1.0,"reasoning":"safe"}`})

	verdict := j.Evaluate(context.Background(), testResult(core.DecisionSafe), testPromptCtx())
	if verdict != nil {
		t.Error("expected nil verdict for SAFE decision (not in trigger list)")
	}
}

func TestJudge_TimeoutFallsThrough(t *testing.T) {
	j := testJudge("active", &mockProvider{
		name:  "test",
		delay: 5 * time.Second,
	})
	j.timeout = 50 * time.Millisecond

	verdict := j.Evaluate(context.Background(), testResult(core.DecisionCaution), testPromptCtx())
	if verdict == nil {
		t.Fatal("expected verdict with error, got nil")
	}
	if verdict.Error == "" {
		t.Error("expected non-empty error on timeout")
	}
	if verdict.Applied {
		t.Error("verdict should not be applied on error")
	}
}

func TestJudge_MalformedResponseFallsThrough(t *testing.T) {
	j := testJudge("active", &mockProvider{
		name:     "test",
		response: "I cannot help with that request.",
	})

	verdict := j.Evaluate(context.Background(), testResult(core.DecisionCaution), testPromptCtx())
	if verdict == nil {
		t.Fatal("expected verdict with error, got nil")
	}
	if verdict.Error == "" {
		t.Error("expected parse error")
	}
}

func TestJudge_RejectsBlockedResponse(t *testing.T) {
	j := testJudge("active", &mockProvider{
		name:     "test",
		response: `{"decision":"BLOCKED","confidence":1.0,"reasoning":"dangerous"}`,
	})

	verdict := j.Evaluate(context.Background(), testResult(core.DecisionCaution), testPromptCtx())
	if verdict == nil {
		t.Fatal("expected verdict with error, got nil")
	}
	if verdict.Error == "" {
		t.Error("expected error for BLOCKED response")
	}
}

func TestJudge_RejectsUnknownDecision(t *testing.T) {
	j := testJudge("active", &mockProvider{
		name:     "test",
		response: `{"decision":"DELETE","confidence":0.9,"reasoning":"delete it"}`,
	})

	verdict := j.Evaluate(context.Background(), testResult(core.DecisionCaution), testPromptCtx())
	if verdict == nil {
		t.Fatal("expected verdict with error, got nil")
	}
	if verdict.Error == "" {
		t.Error("expected error for unknown decision")
	}
}

func TestJudge_RateLimited(t *testing.T) {
	j := testJudge("active", &mockProvider{
		name:     "test",
		response: `{"decision":"SAFE","confidence":0.95,"reasoning":"safe"}`,
	})
	j.rateLimiter = newRateLimiter(1) // only 1 call allowed

	// First call succeeds.
	v1 := j.Evaluate(context.Background(), testResult(core.DecisionCaution), testPromptCtx())
	if v1 == nil {
		t.Fatal("first call should succeed")
	}

	// Second call is rate limited.
	v2 := j.Evaluate(context.Background(), testResult(core.DecisionCaution), testPromptCtx())
	if v2 != nil {
		t.Error("second call should be rate limited (nil verdict)")
	}
}

func TestJudge_ConfidenceOutOfRange(t *testing.T) {
	j := testJudge("active", &mockProvider{
		name:     "test",
		response: `{"decision":"SAFE","confidence":1.5,"reasoning":"very safe"}`,
	})

	verdict := j.Evaluate(context.Background(), testResult(core.DecisionCaution), testPromptCtx())
	if verdict == nil {
		t.Fatal("expected verdict, got nil")
	}
	if verdict.Confidence != 1.0 {
		t.Errorf("confidence = %f, want 1.0 (clamped)", verdict.Confidence)
	}
}

func TestMaybeJudge_OffMode(t *testing.T) {
	cfg := &config.Config{LLMJudge: config.LLMJudgeConfig{Mode: "off"}}
	result := testResult(core.DecisionCaution)
	out, verdict := MaybeJudge(context.Background(), cfg, result, testPromptCtx())
	if verdict != nil {
		t.Error("expected nil verdict for off mode")
	}
	if out.Decision != core.DecisionCaution {
		t.Errorf("decision changed: %q", out.Decision)
	}
}

func TestMaybeJudge_NilConfig(t *testing.T) {
	result := testResult(core.DecisionCaution)
	out, verdict := MaybeJudge(context.Background(), nil, result, testPromptCtx())
	if verdict != nil {
		t.Error("expected nil verdict for nil config")
	}
	if out.Decision != core.DecisionCaution {
		t.Errorf("decision changed: %q", out.Decision)
	}
}

func TestProviderError(t *testing.T) {
	j := testJudge("active", &mockProvider{
		name: "test",
		err:  fmt.Errorf("connection refused"),
	})

	verdict := j.Evaluate(context.Background(), testResult(core.DecisionCaution), testPromptCtx())
	if verdict == nil {
		t.Fatal("expected verdict with error, got nil")
	}
	if verdict.Error == "" {
		t.Error("expected non-empty error")
	}
	if verdict.Applied {
		t.Error("verdict should not be applied on error")
	}
}
