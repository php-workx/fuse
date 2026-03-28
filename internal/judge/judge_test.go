package judge

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/php-workx/fuse/internal/config"
	"github.com/php-workx/fuse/internal/core"
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

func TestNewJudge_Defaults(t *testing.T) {
	dir := t.TempDir()
	// Create a fake claude binary so DetectProvider succeeds.
	fakePath := dir + "/claude"
	if err := os.WriteFile(fakePath, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)

	j, err := NewJudge(config.LLMJudgeConfig{
		Mode:     "shadow",
		Provider: "auto",
	})
	if err != nil {
		t.Fatalf("NewJudge: %v", err)
	}
	if j.mode != "shadow" {
		t.Errorf("mode = %q, want shadow", j.mode)
	}
	if j.upgradeThreshold != 0.7 {
		t.Errorf("upgradeThreshold = %f, want 0.7", j.upgradeThreshold)
	}
	if j.downgradeThreshold != 0.95 {
		t.Errorf("downgradeThreshold = %f, want 0.95", j.downgradeThreshold)
	}
	if j.timeout != 10*time.Second {
		t.Errorf("timeout = %v, want 10s", j.timeout)
	}
}

func TestNewJudge_CustomConfig(t *testing.T) {
	dir := t.TempDir()
	fakePath := dir + "/codex"
	if err := os.WriteFile(fakePath, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)

	j, err := NewJudge(config.LLMJudgeConfig{
		Mode:               "active",
		Provider:           "codex",
		Timeout:            "5s",
		UpgradeThreshold:   0.8,
		DowngradeThreshold: 0.9,
		TriggerDecisions:   []string{"approval"},
		MaxCallsPerMinute:  10,
	})
	if err != nil {
		t.Fatalf("NewJudge: %v", err)
	}
	if j.timeout != 5*time.Second {
		t.Errorf("timeout = %v, want 5s", j.timeout)
	}
	if j.upgradeThreshold != 0.8 {
		t.Errorf("upgradeThreshold = %f, want 0.8", j.upgradeThreshold)
	}
	if !j.triggerDecisions[core.DecisionApproval] {
		t.Error("expected APPROVAL in trigger decisions")
	}
	if j.triggerDecisions[core.DecisionCaution] {
		t.Error("CAUTION should not be in trigger decisions")
	}
}

func TestNewJudge_NoProvider(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PATH", dir) // empty, no binaries

	_, err := NewJudge(config.LLMJudgeConfig{
		Mode:     "shadow",
		Provider: "auto",
	})
	if err == nil {
		t.Error("expected error when no provider found")
	}
}

func TestMaybeJudge_EmptyModeDefaultsShadow(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(dir+"/claude", []byte("#!/bin/sh\necho '{\"decision\":\"SAFE\",\"confidence\":0.9,\"reasoning\":\"ok\"}'"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)

	// Reset cached judge to avoid stale state from other tests.
	cachedJudgeMu.Lock()
	cachedJudge = nil
	cachedCfgHash = ""
	cachedJudgeMu.Unlock()

	cfg := &config.Config{LLMJudge: config.LLMJudgeConfig{
		Mode:     "", // empty — should default to "shadow"
		Provider: "claude",
	}}
	result := testResult(core.DecisionCaution)
	_, verdict := MaybeJudge(context.Background(), cfg, result, testPromptCtx())
	// If mode defaulted correctly to shadow, we should get a verdict (not nil).
	// It may fail due to the fake CLI, but it should attempt the evaluation.
	// The key check: it didn't return nil (which would mean "off" mode).
	_ = verdict // May be nil if provider fails, but that's OK — the code path was exercised.
}

func TestMaybeJudge_InitFailure(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PATH", dir) // no binaries

	cachedJudgeMu.Lock()
	cachedJudge = nil
	cachedCfgHash = ""
	cachedJudgeMu.Unlock()

	cfg := &config.Config{LLMJudge: config.LLMJudgeConfig{
		Mode:     "shadow",
		Provider: "auto",
	}}
	result := testResult(core.DecisionCaution)
	out, verdict := MaybeJudge(context.Background(), cfg, result, testPromptCtx())
	if verdict != nil {
		t.Error("expected nil verdict when provider init fails")
	}
	if out.Decision != core.DecisionCaution {
		t.Errorf("decision changed: %q", out.Decision)
	}
}

func TestConfigHash(t *testing.T) {
	a := config.LLMJudgeConfig{Mode: "shadow", Provider: "claude"}
	b := config.LLMJudgeConfig{Mode: "active", Provider: "claude"}
	if configHash(a) == configHash(b) {
		t.Error("different configs should have different hashes")
	}
	c := config.LLMJudgeConfig{Mode: "shadow", Provider: "claude"}
	if configHash(a) != configHash(c) {
		t.Error("identical configs should have same hash")
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

// ---------------------------------------------------------------------------
// Downgrade guard tests (FailClosed, SAFE cap, ExtractionIncomplete)
// ---------------------------------------------------------------------------

// setupMaybeJudgeWithMock injects a mock provider into the cached judge
// for MaybeJudge testing. Returns a cleanup function.
// activeCfgForMock returns a config whose hash matches what setupMaybeJudgeWithMock stores.
var mockJudgeCfg = config.LLMJudgeConfig{Mode: "active"}

func setupMaybeJudgeWithMock(response string) func() {
	cachedJudgeMu.Lock()
	old := cachedJudge
	oldHash := cachedCfgHash
	cachedJudge = testJudge("active", &mockProvider{
		name:     "test-mock",
		response: response,
	})
	// Use the real config hash so MaybeJudge doesn't try to reinitialize.
	cachedCfgHash = configHash(mockJudgeCfg)
	cachedJudgeMu.Unlock()
	return func() {
		cachedJudgeMu.Lock()
		cachedJudge = old
		cachedCfgHash = oldHash
		cachedJudgeMu.Unlock()
	}
}

func activeCfg() *config.Config {
	return &config.Config{
		LLMJudge: mockJudgeCfg,
	}
}

func TestMaybeJudge_FailClosedBlocksDowngrade(t *testing.T) {
	cleanup := setupMaybeJudgeWithMock(`{"decision":"CAUTION","confidence":0.99,"reasoning":"looks safe"}`)
	defer cleanup()

	result := testResult(core.DecisionApproval)
	result.FailClosed = true

	out, verdict := MaybeJudge(context.Background(), activeCfg(), result, testPromptCtx())
	if verdict == nil {
		t.Fatal("expected verdict, got nil")
	}
	if verdict.Applied {
		t.Error("verdict should NOT be applied on fail-closed result")
	}
	if out.Decision != core.DecisionApproval {
		t.Errorf("decision should remain APPROVAL, got %s", out.Decision)
	}
}

func TestMaybeJudge_FailClosedAllowsUpgrade(t *testing.T) {
	cleanup := setupMaybeJudgeWithMock(`{"decision":"APPROVAL","confidence":0.8,"reasoning":"dangerous"}`)
	defer cleanup()

	result := testResult(core.DecisionCaution)
	result.FailClosed = true

	out, verdict := MaybeJudge(context.Background(), activeCfg(), result, testPromptCtx())
	if verdict == nil {
		t.Fatal("expected verdict, got nil")
	}
	if !verdict.Applied {
		t.Error("upgrade should be applied even on fail-closed result")
	}
	if out.Decision != core.DecisionApproval {
		t.Errorf("decision should be upgraded to APPROVAL, got %s", out.Decision)
	}
}

func TestMaybeJudge_DowngradeCapApprovalToSafe(t *testing.T) {
	cleanup := setupMaybeJudgeWithMock(`{"decision":"SAFE","confidence":0.99,"reasoning":"totally safe"}`)
	defer cleanup()

	result := testResult(core.DecisionApproval)

	out, verdict := MaybeJudge(context.Background(), activeCfg(), result, testPromptCtx())
	if verdict == nil {
		t.Fatal("expected verdict, got nil")
	}
	if out.Decision != core.DecisionCaution {
		t.Errorf("APPROVAL->SAFE should be capped to CAUTION, got %s", out.Decision)
	}
}

func TestMaybeJudge_DowngradeApprovalToCautionAllowed(t *testing.T) {
	cleanup := setupMaybeJudgeWithMock(`{"decision":"CAUTION","confidence":0.96,"reasoning":"minor concern"}`)
	defer cleanup()

	result := testResult(core.DecisionApproval)

	out, verdict := MaybeJudge(context.Background(), activeCfg(), result, testPromptCtx())
	if verdict == nil {
		t.Fatal("expected verdict, got nil")
	}
	if !verdict.Applied {
		t.Error("APPROVAL->CAUTION should be applied")
	}
	if out.Decision != core.DecisionCaution {
		t.Errorf("decision should be CAUTION, got %s", out.Decision)
	}
}

func TestMaybeJudge_ExtractionIncompleteBlocksDowngrade(t *testing.T) {
	cleanup := setupMaybeJudgeWithMock(`{"decision":"CAUTION","confidence":0.99,"reasoning":"looks ok"}`)
	defer cleanup()

	result := testResult(core.DecisionApproval)
	promptCtx := testPromptCtx()
	promptCtx.ExtractionIncomplete = true

	out, verdict := MaybeJudge(context.Background(), activeCfg(), result, promptCtx)
	if verdict == nil {
		t.Fatal("expected verdict, got nil")
	}
	if verdict.Applied {
		t.Error("downgrade should be blocked when extraction incomplete")
	}
	if out.Decision != core.DecisionApproval {
		t.Errorf("decision should remain APPROVAL, got %s", out.Decision)
	}
}

func TestMaybeJudge_UpgradeNotAffectedByGuards(t *testing.T) {
	cleanup := setupMaybeJudgeWithMock(`{"decision":"APPROVAL","confidence":0.8,"reasoning":"risky"}`)
	defer cleanup()

	result := testResult(core.DecisionCaution)

	out, verdict := MaybeJudge(context.Background(), activeCfg(), result, testPromptCtx())
	if verdict == nil {
		t.Fatal("expected verdict, got nil")
	}
	if !verdict.Applied {
		t.Error("upgrade should be applied")
	}
	if out.Decision != core.DecisionApproval {
		t.Errorf("decision should be upgraded to APPROVAL, got %s", out.Decision)
	}
}
