package adapters

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/php-workx/fuse/internal/approve"
	"github.com/php-workx/fuse/internal/config"
	"github.com/php-workx/fuse/internal/core"
	"github.com/php-workx/fuse/internal/db"
	"github.com/php-workx/fuse/internal/judge"
)

func TestProfileBehavior_ConfigDefaults(t *testing.T) {
	withFuseHome(t)

	cases := []struct {
		name          string
		contents      string
		wantProfile   string
		wantMode      string
		wantTriggers  []string
		wantFallback  string
		wantDowngrade float64
	}{
		{
			name:          "missing profile defaults to relaxed",
			contents:      "",
			wantProfile:   config.ProfileRelaxed,
			wantMode:      "off",
			wantTriggers:  []string{},
			wantFallback:  "log",
			wantDowngrade: 0.95,
		},
		{
			name:          "legacy active judge resolves to balanced",
			contents:      "llm_judge:\n  mode: active\n",
			wantProfile:   config.ProfileBalanced,
			wantMode:      "active",
			wantTriggers:  []string{"caution", "approval"},
			wantFallback:  "log",
			wantDowngrade: 0.9,
		},
		{
			name:          "explicit strict profile resolves strict defaults",
			contents:      "profile: strict\n",
			wantProfile:   config.ProfileStrict,
			wantMode:      "active",
			wantTriggers:  []string{"caution"},
			wantFallback:  "log",
			wantDowngrade: 0.95,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := writeProfileConfigForBehaviorTest(t, tc.contents)
			cfg, err := config.LoadConfig(path)
			if err != nil {
				t.Fatalf("LoadConfig: %v", err)
			}
			if cfg.Profile != tc.wantProfile {
				t.Fatalf("Profile = %q, want %q", cfg.Profile, tc.wantProfile)
			}
			if cfg.LLMJudge.Mode != tc.wantMode {
				t.Fatalf("LLMJudge.Mode = %q, want %q", cfg.LLMJudge.Mode, tc.wantMode)
			}
			if cfg.CautionFallback != tc.wantFallback {
				t.Fatalf("CautionFallback = %q, want %q", cfg.CautionFallback, tc.wantFallback)
			}
			if cfg.LLMJudge.DowngradeThreshold != tc.wantDowngrade {
				t.Fatalf("LLMJudge.DowngradeThreshold = %v, want %v", cfg.LLMJudge.DowngradeThreshold, tc.wantDowngrade)
			}
			if got := strings.Join(cfg.LLMJudge.TriggerDecisions, ","); got != strings.Join(tc.wantTriggers, ",") {
				t.Fatalf("LLMJudge.TriggerDecisions = %v, want %v", cfg.LLMJudge.TriggerDecisions, tc.wantTriggers)
			}
		})
	}
}

func TestProfileBehavior_JudgeRouting(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test uses Unix shell commands")
	}
	logFile := setFakeJudgePathForBehaviorTest(t)

	cases := []struct {
		name                 string
		profile              string
		timeout              string
		wantCautionDecision  string
		wantCautionVerdict   bool
		wantApprovalDecision string
		wantApprovalVerdict  bool
		wantLogCount         int
	}{
		{
			name:                 "relaxed",
			profile:              config.ProfileRelaxed,
			timeout:              "11s",
			wantCautionDecision:  string(core.DecisionCaution),
			wantCautionVerdict:   false,
			wantApprovalDecision: string(core.DecisionApproval),
			wantApprovalVerdict:  false,
			wantLogCount:         0,
		},
		{
			name:                 "balanced",
			profile:              config.ProfileBalanced,
			timeout:              "12s",
			wantCautionDecision:  string(core.DecisionSafe),
			wantCautionVerdict:   true,
			wantApprovalDecision: string(core.DecisionCaution),
			wantApprovalVerdict:  true,
			wantLogCount:         2,
		},
		{
			name:                 "strict",
			profile:              config.ProfileStrict,
			timeout:              "13s",
			wantCautionDecision:  string(core.DecisionSafe),
			wantCautionVerdict:   true,
			wantApprovalDecision: string(core.DecisionApproval),
			wantApprovalVerdict:  false,
			wantLogCount:         1,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			truncateJudgeLog(t, logFile)
			cfg := config.ProfileDefaults(tc.profile)
			cfg.LLMJudge.Timeout = tc.timeout

			cautionOut, cautionVerdict := judge.MaybeJudge(context.Background(), cfg, &core.ClassifyResult{
				Decision:    core.DecisionCaution,
				Reason:      "profile behavior test",
				DecisionKey: "caution-key",
			}, profileJudgePromptContext(core.DecisionCaution))
			if string(cautionOut.Decision) != tc.wantCautionDecision {
				t.Fatalf("caution decision = %q, want %q", cautionOut.Decision, tc.wantCautionDecision)
			}
			if (cautionVerdict != nil) != tc.wantCautionVerdict {
				t.Fatalf("caution verdict present = %v, want %v", cautionVerdict != nil, tc.wantCautionVerdict)
			}

			approvalOut, approvalVerdict := judge.MaybeJudge(context.Background(), cfg, &core.ClassifyResult{
				Decision:    core.DecisionApproval,
				Reason:      "profile behavior test",
				DecisionKey: "approval-key",
				FailClosed:  false,
			}, profileJudgePromptContext(core.DecisionApproval))
			if string(approvalOut.Decision) != tc.wantApprovalDecision {
				t.Fatalf("approval decision = %q, want %q", approvalOut.Decision, tc.wantApprovalDecision)
			}
			if (approvalVerdict != nil) != tc.wantApprovalVerdict {
				t.Fatalf("approval verdict present = %v, want %v", approvalVerdict != nil, tc.wantApprovalVerdict)
			}

			if got := judgeInvocationCount(t, logFile); got != tc.wantLogCount {
				t.Fatalf("judge invocation count = %d, want %d", got, tc.wantLogCount)
			}
		})
	}
}

func TestProfileBehavior_RunHook_CautionCommand(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test uses Unix shell commands")
	}
	enableHookForTest(t)
	logFile := setFakeJudgePathForBehaviorTest(t)

	cases := []struct {
		name            string
		profile         string
		timeout         string
		wantCautionText bool
		wantLogCount    int
	}{
		{name: "relaxed", profile: config.ProfileRelaxed, timeout: "21s", wantCautionText: true, wantLogCount: 0},
		{name: "balanced", profile: config.ProfileBalanced, timeout: "22s", wantCautionText: false, wantLogCount: 1},
		{name: "strict", profile: config.ProfileStrict, timeout: "23s", wantCautionText: false, wantLogCount: 1},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			truncateJudgeLog(t, logFile)
			writeProfileConfigForBehaviorTest(t, profileConfigContents(tc.profile, tc.timeout))

			stderr := &bytes.Buffer{}
			exitCode := RunHook(strings.NewReader(`{"tool_name":"Bash","tool_input":{"command":"rm -rf ./build-cache"},"session_id":"profile-behavior","cwd":"/tmp"}`), stderr)
			if exitCode != 0 {
				t.Fatalf("RunHook exit code = %d, want 0", exitCode)
			}
			gotStderr := stderr.String()
			if tc.wantCautionText {
				if !strings.Contains(gotStderr, "CAUTION") {
					t.Fatalf("stderr = %q, want CAUTION", gotStderr)
				}
			} else if strings.Contains(gotStderr, "CAUTION") {
				t.Fatalf("stderr = %q, want no CAUTION output", gotStderr)
			}
			if got := judgeInvocationCount(t, logFile); got != tc.wantLogCount {
				t.Fatalf("judge invocation count = %d, want %d", got, tc.wantLogCount)
			}
		})
	}
}

func TestProfileBehavior_ExecuteCommand_CautionCommand(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test uses Unix shell commands")
	}
	withFuseHome(t)
	enableFuseForTest(t)
	logFile := setFakeJudgePathForBehaviorTest(t)

	cases := []struct {
		name         string
		profile      string
		timeout      string
		wantLogCount int
	}{
		{name: "relaxed", profile: config.ProfileRelaxed, timeout: "31s", wantLogCount: 0},
		{name: "balanced", profile: config.ProfileBalanced, timeout: "32s", wantLogCount: 1},
		{name: "strict", profile: config.ProfileStrict, timeout: "33s", wantLogCount: 1},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			truncateJudgeLog(t, logFile)
			writeProfileConfigForBehaviorTest(t, profileConfigContents(tc.profile, tc.timeout))

			cwd := t.TempDir()
			if err := os.MkdirAll(filepath.Join(cwd, "build-cache"), 0o755); err != nil {
				t.Fatalf("mkdir build-cache: %v", err)
			}

			exitCode, err := ExecuteCommand("rm -rf ./build-cache", cwd, time.Minute)
			if err != nil {
				t.Fatalf("ExecuteCommand: %v", err)
			}
			if exitCode != 0 {
				t.Fatalf("ExecuteCommand exit code = %d, want 0", exitCode)
			}
			if _, statErr := os.Stat(filepath.Join(cwd, "build-cache")); !os.IsNotExist(statErr) {
				t.Fatalf("build-cache should be removed, stat err=%v", statErr)
			}
			if got := judgeInvocationCount(t, logFile); got != tc.wantLogCount {
				t.Fatalf("judge invocation count = %d, want %d", got, tc.wantLogCount)
			}
		})
	}
}

func TestProfileBehavior_ExecuteCodexShellCommand_CautionCommand(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test uses Unix shell commands")
	}
	withFuseHome(t)
	enableFuseForTest(t)
	logFile := setFakeJudgePathForBehaviorTest(t)

	cases := []struct {
		name         string
		profile      string
		timeout      string
		wantLogCount int
	}{
		{name: "relaxed", profile: config.ProfileRelaxed, timeout: "41s", wantLogCount: 0},
		{name: "balanced", profile: config.ProfileBalanced, timeout: "42s", wantLogCount: 1},
		{name: "strict", profile: config.ProfileStrict, timeout: "43s", wantLogCount: 1},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			truncateJudgeLog(t, logFile)
			writeProfileConfigForBehaviorTest(t, profileConfigContents(tc.profile, tc.timeout))

			cwd := t.TempDir()
			if err := os.MkdirAll(filepath.Join(cwd, "build-cache"), 0o755); err != nil {
				t.Fatalf("mkdir build-cache: %v", err)
			}

			_, _, exitCode, err := executeCodexShellCommand(context.Background(), "rm -rf ./build-cache", cwd, "profile-behavior", time.Minute)
			if err != nil {
				t.Fatalf("executeCodexShellCommand: %v", err)
			}
			if exitCode != 0 {
				t.Fatalf("executeCodexShellCommand exit code = %d, want 0", exitCode)
			}
			if _, statErr := os.Stat(filepath.Join(cwd, "build-cache")); !os.IsNotExist(statErr) {
				t.Fatalf("build-cache should be removed, stat err=%v", statErr)
			}
			if got := judgeInvocationCount(t, logFile); got != tc.wantLogCount {
				t.Fatalf("judge invocation count = %d, want %d", got, tc.wantLogCount)
			}
		})
	}
}

func TestProfileBehavior_RunHook_ApprovalCommand(t *testing.T) {
	enableHookForTest(t)
	t.Setenv("FUSE_NON_INTERACTIVE", "1")
	logFile := setFakeJudgePathForBehaviorTest(t)

	projectDir := t.TempDir()

	cases := []struct {
		name             string
		profile          string
		timeout          string
		wantExitCode     int
		wantApprovalText bool
		wantLogCount     int
	}{
		{name: "relaxed", profile: config.ProfileRelaxed, timeout: "51s", wantExitCode: 2, wantApprovalText: true, wantLogCount: 0},
		{name: "balanced", profile: config.ProfileBalanced, timeout: "52s", wantExitCode: 0, wantApprovalText: false, wantLogCount: 1},
		{name: "strict", profile: config.ProfileStrict, timeout: "53s", wantExitCode: 2, wantApprovalText: true, wantLogCount: 0},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			truncateJudgeLog(t, logFile)
			writeProfileConfigForBehaviorTest(t, profileConfigContents(tc.profile, tc.timeout))

			stderr := &bytes.Buffer{}
			input := `{"tool_name":"Bash","tool_input":{"command":"HOME=/tmp /bin/pwd"},"session_id":"profile-behavior","cwd":"` + projectDir + `"}`
			exitCode := RunHook(strings.NewReader(input), stderr)
			if exitCode != tc.wantExitCode {
				t.Fatalf("RunHook exit code = %d, want %d", exitCode, tc.wantExitCode)
			}
			gotStderr := stderr.String()
			if tc.wantApprovalText {
				if !containsApprovalDirective(gotStderr) {
					t.Fatalf("stderr = %q, want approval directive", gotStderr)
				}
			} else if containsApprovalDirective(gotStderr) {
				t.Fatalf("stderr = %q, want no approval directive", gotStderr)
			}
			if got := judgeInvocationCount(t, logFile); got != tc.wantLogCount {
				t.Fatalf("judge invocation count = %d, want %d", got, tc.wantLogCount)
			}
		})
	}
}

func TestProfileBehavior_ExecuteCommand_ApprovalCommand(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test uses Unix shell commands")
	}
	withFuseHome(t)
	enableFuseForTest(t)
	t.Setenv("FUSE_NON_INTERACTIVE", "1")
	logFile := setFakeJudgePathForBehaviorTest(t)

	cases := []struct {
		name                string
		profile             string
		timeout             string
		wantLogCount        int
		wantApprovalGranted bool
	}{
		{name: "relaxed", profile: config.ProfileRelaxed, timeout: "61s", wantLogCount: 0, wantApprovalGranted: true},
		{name: "balanced", profile: config.ProfileBalanced, timeout: "62s", wantLogCount: 1, wantApprovalGranted: false},
		{name: "strict", profile: config.ProfileStrict, timeout: "63s", wantLogCount: 0, wantApprovalGranted: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			truncateJudgeLog(t, logFile)
			writeProfileConfigForBehaviorTest(t, profileConfigContents(tc.profile, tc.timeout))

			cwd := t.TempDir()

			var approvalCh <-chan approvalGrantResult
			if tc.wantApprovalGranted {
				approvalCh = startPendingApprovalGrant(t, "run", "", "HOME=/tmp /bin/pwd")
			}

			exitCode, err := ExecuteCommand("HOME=/tmp /bin/pwd", cwd, time.Minute)
			if err != nil {
				t.Fatalf("ExecuteCommand: %v", err)
			}
			if exitCode != 0 {
				t.Fatalf("ExecuteCommand exit code = %d, want 0", exitCode)
			}
			if tc.wantApprovalGranted {
				result := <-approvalCh
				if result.err != nil {
					t.Fatalf("approval helper: %v", result.err)
				}
				if !result.granted {
					t.Fatal("approval helper did not grant approval")
				}
			}
			if got := judgeInvocationCount(t, logFile); got != tc.wantLogCount {
				t.Fatalf("judge invocation count = %d, want %d", got, tc.wantLogCount)
			}
		})
	}
}

func TestProfileBehavior_ExecuteCodexShellCommand_ApprovalCommand(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test uses Unix shell commands")
	}
	withFuseHome(t)
	enableFuseForTest(t)
	t.Setenv("FUSE_NON_INTERACTIVE", "1")
	logFile := setFakeJudgePathForBehaviorTest(t)

	cases := []struct {
		name                string
		profile             string
		timeout             string
		wantLogCount        int
		wantApprovalGranted bool
	}{
		{name: "relaxed", profile: config.ProfileRelaxed, timeout: "71s", wantLogCount: 0, wantApprovalGranted: true},
		{name: "balanced", profile: config.ProfileBalanced, timeout: "72s", wantLogCount: 1, wantApprovalGranted: false},
		{name: "strict", profile: config.ProfileStrict, timeout: "73s", wantLogCount: 0, wantApprovalGranted: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			truncateJudgeLog(t, logFile)
			writeProfileConfigForBehaviorTest(t, profileConfigContents(tc.profile, tc.timeout))

			cwd := t.TempDir()

			var approvalCh <-chan approvalGrantResult
			if tc.wantApprovalGranted {
				approvalCh = startPendingApprovalGrant(t, "codex-shell", "profile-behavior", "HOME=/tmp /bin/pwd")
			}

			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			stdout, stderr, exitCode, err := executeCodexShellCommand(ctx, "HOME=/tmp /bin/pwd", cwd, "profile-behavior", time.Minute)
			if err != nil {
				t.Fatalf("executeCodexShellCommand: %v", err)
			}
			if exitCode != 0 {
				t.Fatalf("executeCodexShellCommand exit code = %d, want 0", exitCode)
			}
			if !strings.Contains(strings.TrimSpace(stdout), cwd) {
				t.Fatalf("stdout = %q, want cwd %q", stdout, cwd)
			}
			if stderr != "" {
				t.Fatalf("stderr = %q, want empty", stderr)
			}
			if tc.wantApprovalGranted {
				result := <-approvalCh
				if result.err != nil {
					t.Fatalf("approval helper: %v", result.err)
				}
				if !result.granted {
					t.Fatal("approval helper did not grant approval")
				}
			}
			if got := judgeInvocationCount(t, logFile); got != tc.wantLogCount {
				t.Fatalf("judge invocation count = %d, want %d", got, tc.wantLogCount)
			}
		})
	}
}

func writeProfileConfigForBehaviorTest(t *testing.T, contents string) string {
	t.Helper()
	if err := config.EnsureDirectories(); err != nil {
		t.Fatalf("EnsureDirectories: %v", err)
	}
	path := config.ConfigPath()
	if contents == "" {
		if err := os.WriteFile(path, nil, 0o644); err != nil {
			t.Fatalf("write config: %v", err)
		}
		return path
	}
	if err := os.WriteFile(path, []byte(strings.TrimSpace(contents)+"\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

func profileConfigContents(profile, timeout string) string {
	return "profile: " + profile + "\nllm_judge:\n  timeout: " + timeout + "\n"
}

func setFakeJudgePathForBehaviorTest(t *testing.T) string {
	t.Helper()
	fakeBin := t.TempDir()
	logFile := filepath.Join(fakeBin, "judge.log")
	script := `#!/bin/sh
input="$(cat)"
printf '%s\n' "$input" >> "$FAKE_JUDGE_LOG"
case "$input" in
  *"Current classification: CAUTION"*)
    printf '%s\n' '{"decision":"SAFE","confidence":0.99,"reasoning":"downgrade caution"}'
    ;;
  *"Current classification: APPROVAL"*)
    printf '%s\n' '{"decision":"SAFE","confidence":0.99,"reasoning":"downgrade approval"}'
    ;;
  *)
    printf '%s\n' '{"decision":"SAFE","confidence":0.99,"reasoning":"default"}'
    ;;
esac
`
	if err := os.WriteFile(filepath.Join(fakeBin, "claude"), []byte(script), 0o755); err != nil {
		t.Fatalf("write fake claude: %v", err)
	}
	t.Setenv("FAKE_JUDGE_LOG", logFile)
	t.Setenv("PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))
	return logFile
}

func truncateJudgeLog(t *testing.T, logFile string) {
	t.Helper()
	if err := os.WriteFile(logFile, nil, 0o644); err != nil {
		t.Fatalf("truncate judge log: %v", err)
	}
}

func judgeInvocationCount(t *testing.T, logFile string) int {
	t.Helper()
	data, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("read judge log: %v", err)
	}
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" {
		return 0
	}
	return strings.Count(trimmed, "Current classification:")
}

type approvalGrantResult struct {
	granted bool
	err     error
}

func startPendingApprovalGrant(t *testing.T, source, sessionID, commandSubstring string) <-chan approvalGrantResult {
	t.Helper()
	resultCh := make(chan approvalGrantResult, 1)
	go func() {
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) {
			if _, err := os.Stat(config.DBPath()); err != nil {
				time.Sleep(25 * time.Millisecond)
				continue
			}
			database, err := db.OpenDB(config.DBPath())
			if err == nil {
				requests, listErr := database.ListPendingRequests()
				if listErr == nil {
					for _, req := range requests {
						if req.Source != source || req.SessionID != sessionID || !strings.Contains(req.Command, commandSubstring) {
							continue
						}
						secret, secretErr := db.EnsureSecret(config.SecretPath())
						if secretErr != nil {
							_ = database.Close()
							resultCh <- approvalGrantResult{err: secretErr}
							return
						}
						mgr, mgrErr := approve.NewManager(database, secret)
						if mgrErr != nil {
							_ = database.Close()
							resultCh <- approvalGrantResult{err: mgrErr}
							return
						}
						createErr := mgr.CreateApproval(req.DecisionKey, string(core.DecisionApproval), "once", sessionID)
						_ = database.Close()
						if createErr != nil {
							resultCh <- approvalGrantResult{err: createErr}
							return
						}
						resultCh <- approvalGrantResult{granted: true}
						return
					}
				}
				_ = database.Close()
			}
			time.Sleep(50 * time.Millisecond)
		}
		resultCh <- approvalGrantResult{}
	}()
	return resultCh
}

func containsApprovalDirective(stderr string) bool {
	for _, needle := range []string{
		"PENDING_APPROVAL",
		"APPROVAL_NOT_AVAILABLE",
		"NON_INTERACTIVE_MODE",
		"USER_DENIED",
		"TIMEOUT_WAITING_FOR_USER",
	} {
		if strings.Contains(stderr, needle) {
			return true
		}
	}
	return false
}

func profileJudgePromptContext(decision core.Decision) judge.PromptContext {
	return judge.PromptContext{
		Command:         "rm -rf ./build-cache",
		Cwd:             "/tmp",
		WorkspaceRoot:   "/tmp",
		CurrentDecision: string(decision),
		Reason:          "profile behavior test",
		RuleID:          "test",
		ToolName:        "Bash",
	}
}
