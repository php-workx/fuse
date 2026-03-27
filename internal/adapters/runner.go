package adapters

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/unix"

	"github.com/php-workx/fuse/internal/approve"
	"github.com/php-workx/fuse/internal/config"
	"github.com/php-workx/fuse/internal/core"
	"github.com/php-workx/fuse/internal/db"
	"github.com/php-workx/fuse/internal/judge"
	"github.com/php-workx/fuse/internal/policy"
)

var errApprovalDenied = errors.New("fuse denied command")

type commandExecution struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

// strippedEnvVars lists environment variables that are stripped from the child
// process environment for security (§10.1).
var strippedEnvVars = map[string]bool{
	"LD_PRELOAD":      true,
	"LD_LIBRARY_PATH": true,
	"PYTHONPATH":      true,
	"NODE_PATH":       true,
	"RUBYLIB":         true,
	"BASH_ENV":        true,
	"ENV":             true,
}

// BuildChildEnv sanitizes the environment for child process execution.
// It strips dangerous loader/module variables and resets PATH to a
// platform-specific trusted default (§10.1).
func BuildChildEnv(environ []string) []string {
	var result []string
	pathSet := false

	for _, env := range environ {
		parts := strings.SplitN(env, "=", 2)
		if len(parts) < 1 {
			continue
		}
		name := parts[0]

		// Strip dangerous variables.
		if strippedEnvVars[name] || strings.HasPrefix(name, "DYLD_") {
			continue
		}

		// Replace PATH with trusted platform-specific path.
		if name == "PATH" {
			result = append(result, "PATH="+trustedPath())
			pathSet = true
			continue
		}

		result = append(result, env)
	}

	// Ensure PATH is always set.
	if !pathSet {
		result = append(result, "PATH="+trustedPath())
	}

	return result
}

// ForegroundChildProcessGroupIfTTY transfers foreground TTY ownership to the
// child process group if stdin is a terminal. Returns a restore function that
// should be deferred to restore the original foreground group.
func ForegroundChildProcessGroupIfTTY(pid int) (restore func(), err error) {
	fd := int(os.Stdin.Fd())

	// Check if stdin is a terminal using the platform-specific ioctl.
	if _, termErr := unix.IoctlGetTermios(fd, ioctlGetTermios); termErr != nil {
		// Not a terminal — nothing to do.
		return nil, nil //nolint:nilerr // termErr means not-a-tty, which is a valid no-op
	}

	// Get the current foreground process group.
	origPgrp, err := unix.IoctlGetInt(fd, unix.TIOCGPGRP)
	if err != nil {
		return nil, fmt.Errorf("get foreground pgrp: %w", err)
	}

	// Suppress SIGTTOU during foreground group changes.
	signal.Ignore(syscall.SIGTTOU)

	// Set the child's process group as the foreground group.
	childPgrp := pid
	if err := unix.IoctlSetInt(fd, unix.TIOCSPGRP, childPgrp); err != nil {
		signal.Reset(syscall.SIGTTOU)
		return nil, fmt.Errorf("set child foreground pgrp: %w", err)
	}

	restore = func() {
		_ = unix.IoctlSetInt(fd, unix.TIOCSPGRP, origPgrp)
		signal.Reset(syscall.SIGTTOU)
	}

	return restore, nil
}

// runContext bundles parameters shared across ExecuteCommand helper functions.
type runContext struct {
	command   string
	cwd       string
	result    *core.ClassifyResult
	verdict   *judge.Verdict
	database  *db.DB
	cfg       *config.Config
	evaluator core.PolicyEvaluator
	req       core.ShellRequest
	dryRun    bool
}

// logWithVerdict logs an event with judge verdict fields.
func (rc *runContext) logWithVerdict(outcome string) {
	event := newEvent(rc.result, "run", "manual", "", rc.command, rc.cwd, outcome)
	applyVerdict(event, rc.verdict)
	logEvent(rc.database, event)
}

// handleBlockedCommand handles a BLOCKED classification result.
func (rc *runContext) handleBlockedCommand() (int, error) {
	rc.logWithVerdict("blocked")
	cleanupExecutionState(rc.database, rc.cfg)
	if !rc.dryRun {
		fmt.Fprintf(os.Stderr, "fuse: BLOCKED — %s\n", rc.result.Reason)
		return 1, nil
	}
	return 0, nil
}

// handleApprovalCommand handles an APPROVAL classification result.
func (rc *runContext) handleApprovalCommand() (int, error) {
	if rc.dryRun {
		rc.logWithVerdict("dry-run")
		return 0, nil
	}

	if rc.database == nil {
		fmt.Fprintf(os.Stderr, "fuse: approval required but database unavailable\n")
		return 1, fmt.Errorf("database unavailable for approval")
	}

	secret, secretErr := db.EnsureSecret(config.SecretPath())
	if secretErr != nil {
		return 1, fmt.Errorf("load HMAC secret: %w", secretErr)
	}

	mgr, mgrErr := approve.NewManager(rc.database, secret)
	if mgrErr != nil {
		return 1, fmt.Errorf("create approval manager: %w", mgrErr)
	}

	decision, promptErr := mgr.RequestApproval(context.Background(), approve.ApprovalRequest{
		DecisionKey: rc.result.DecisionKey,
		Command:     rc.command,
		Reason:      rc.result.Reason,
		Source:      "run",
	})
	if promptErr != nil {
		return 1, fmt.Errorf("approval: %w", promptErr)
	}

	if decision == core.DecisionBlocked {
		fmt.Fprintf(os.Stderr, "fuse: denied by user\n")
		rc.logWithVerdict("denied")
		cleanupExecutionState(rc.database, rc.cfg)
		return 1, nil
	}

	return 0, nil
}

// ExecuteCommand classifies and optionally runs a shell command.
// In run mode, it: classify -> prompt if needed -> execute with safety controls.
func ExecuteCommand(command, cwd string, timeout time.Duration) (exitCode int, err error) {
	// Load configuration.
	cfg := loadRuntimeConfig()

	mode := config.Mode()
	if mode == config.ModeDisabled {
		return executeShellCommand(command, cwd, timeout)
	}

	// Load policy.
	policyCfg, _ := policy.LoadPolicyWithLKG(config.PolicyPath(), 0)
	evaluator := policy.NewEvaluator(policyCfg)

	// Classify the command.
	req := core.ShellRequest{
		RawCommand: command,
		Cwd:        cwd,
		Source:     "run",
	}
	result, err := core.Classify(req, evaluator)
	if err != nil {
		return 1, fmt.Errorf("classify command: %w", err)
	}

	// LLM judge: get a second opinion on the classification.
	promptCtx := buildJudgeContext(command, cwd, "Bash", result)
	var verdict *judge.Verdict
	result, verdict = judge.MaybeJudge(context.Background(), cfg, result, promptCtx)

	// Open database for event logging and approvals.
	database, dbErr := db.OpenDB(config.DBPath())
	if dbErr != nil {
		// Non-fatal: we can still execute, just can't log.
		database = nil
	}
	if database != nil {
		defer func() { _ = database.Close() }()
	}

	rc := &runContext{
		command:   command,
		cwd:       cwd,
		result:    result,
		verdict:   verdict,
		database:  database,
		cfg:       cfg,
		evaluator: evaluator,
		req:       req,
		dryRun:    mode == config.ModeDryRun,
	}

	// Handle classification result.
	if code, handleErr := rc.handleDecision(); handleErr != nil || code != 0 {
		return code, handleErr
	}

	if !rc.dryRun {
		if verifyErr := reverifyDecisionKey(req, evaluator, result.DecisionKey); verifyErr != nil {
			return 1, verifyErr
		}
	}

	// Execute the command.
	exitCode, err = executeShellCommand(command, cwd, timeout)

	// Log event.
	outcome := "executed"
	if err != nil {
		outcome = "error"
	}
	rc.logWithVerdict(outcome)
	cleanupExecutionState(database, cfg)

	return exitCode, err
}

// handleDecision dispatches to the appropriate handler based on the classification decision.
// Returns (0, nil) when execution should proceed normally.
func (rc *runContext) handleDecision() (int, error) {
	switch rc.result.Decision {
	case core.DecisionBlocked:
		return rc.handleBlockedCommand()
	case core.DecisionApproval:
		return rc.handleApprovalCommand()
	default:
		// SAFE, CAUTION, or unknown — execute directly.
		return 0, nil
	}
}

func reverifyDecisionKey(req core.ShellRequest, evaluator core.PolicyEvaluator, expected string) error {
	result, err := core.Classify(req, evaluator)
	if err != nil {
		return fmt.Errorf("reverify classification: %w", err)
	}
	if result.DecisionKey != expected {
		return fmt.Errorf("command changed after approval or inspection; reclassification required")
	}
	return nil
}

// executeShellCommand runs a shell command with safety controls (§10.1).
func executeShellCommand(command, cwd string, timeout time.Duration) (int, error) {
	ctx := context.Background()
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	cmd := exec.CommandContext( // nosemgrep: dangerous-exec-command
		ctx, "/bin/sh", "-c", command,
	)
	cmd.Dir = cwd
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = BuildChildEnv(os.Environ())

	// Platform-specific SysProcAttr (Setpgid, optionally Pdeathsig on Linux).
	cmd.SysProcAttr = platformSysProcAttr()

	if err := cmd.Start(); err != nil {
		return -1, fmt.Errorf("start command: %w", err)
	}

	return waitForManagedCommand(cmd)
}

func waitForManagedCommand(cmd *exec.Cmd) (int, error) {
	// Transfer foreground TTY ownership to child process group.
	restoreTTY, err := ForegroundChildProcessGroupIfTTY(cmd.Process.Pid)
	if err != nil {
		_ = cmd.Process.Kill()
		return -1, fmt.Errorf("foreground child process group: %w", err)
	}
	if restoreTTY != nil {
		defer restoreTTY()
	}

	// Forward signals to child process group.
	sigCh := make(chan os.Signal, 1)
	done := make(chan struct{})
	signal.Notify(sigCh,
		syscall.SIGINT,
		syscall.SIGTERM,
		syscall.SIGHUP,
		syscall.SIGTSTP,  // job control (Ctrl+Z)
		syscall.SIGWINCH, // terminal resize
	)
	go forwardSignals(cmd.Process.Pid, sigCh, done)

	waitErr := cmd.Wait()
	signal.Stop(sigCh)
	close(done)

	return interpretWaitError(waitErr)
}

// forwardSignals relays OS signals to a child process group until done is closed.
func forwardSignals(pid int, sigCh <-chan os.Signal, done <-chan struct{}) {
	for {
		select {
		case sig, ok := <-sigCh:
			if !ok {
				return
			}
			sysSig, isSyscall := sig.(syscall.Signal)
			if !isSyscall {
				continue
			}
			// Send to process group (negative PID).
			if err := syscall.Kill(-pid, sysSig); err != nil {
				// Fallback: retry direct child PID.
				_ = syscall.Kill(pid, sysSig)
			}
		case <-done:
			return
		}
	}
}

// interpretWaitError converts a cmd.Wait error into an exit code.
func interpretWaitError(waitErr error) (int, error) {
	if waitErr == nil {
		return 0, nil
	}
	var exitErr *exec.ExitError
	if errors.As(waitErr, &exitErr) {
		if status, ok := exitErr.Sys().(syscall.WaitStatus); ok && status.Signaled() {
			return 128 + int(status.Signal()), nil
		}
		return exitErr.ExitCode(), nil
	}
	return -1, fmt.Errorf("wait for command: %w", waitErr)
}

func executeCapturedShellCommand(ctx context.Context, command, cwd string, timeout time.Duration) (commandExecution, error) {
	return executeCapturedShellCommandWithStdin(ctx, command, cwd, timeout, nil)
}

func executeCapturedShellCommandWithStdin(ctx context.Context, command, cwd string, timeout time.Duration, stdin io.Reader) (commandExecution, error) {
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	cmd := exec.CommandContext( // nosemgrep: dangerous-exec-command
		ctx, "/bin/sh", "-c", command,
	)
	cmd.Dir = cwd
	cmd.Stdin = stdin
	cmd.Env = BuildChildEnv(os.Environ())
	cmd.SysProcAttr = platformSysProcAttr()

	var stdoutBuf strings.Builder
	var stderrBuf strings.Builder
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	if err := cmd.Start(); err != nil {
		return commandExecution{}, fmt.Errorf("start command: %w", err)
	}

	exitCode, err := waitForManagedCommand(cmd)
	if err != nil {
		return commandExecution{
			Stdout: stdoutBuf.String(),
			Stderr: stderrBuf.String(),
		}, err
	}

	return commandExecution{
		Stdout:   stdoutBuf.String(),
		Stderr:   stderrBuf.String(),
		ExitCode: exitCode,
	}, nil
}

// logEvent logs an execution event to the database if available.
func logEvent(database *db.DB, record *db.EventRecord) {
	if database == nil {
		return
	}
	_ = database.LogEvent(record)
}

// newEvent builds an EventRecord from a ClassifyResult and context.
func newEvent(result *core.ClassifyResult, source, agent, sessionID, command, cwd, outcome string) *db.EventRecord {
	return &db.EventRecord{
		SessionID: sessionID,
		Command:   command,
		Decision:  string(result.Decision),
		RuleID:    result.RuleID,
		Reason:    result.Reason,
		Metadata:  outcome,
		Source:    source,
		Agent:     agent,
		Cwd:       cwd,
	}
}

func loadRuntimeConfig() *config.Config {
	cfg, err := config.LoadConfig(config.ConfigPath())
	if err != nil {
		return config.DefaultConfig()
	}
	// Apply URL trust policy from config to the URL inspection module.
	core.SetTrustedDomains(cfg.URLTrustPolicy.TrustedDomains)
	return cfg
}

func cleanupExecutionState(database *db.DB, cfg *config.Config) {
	if database == nil {
		return
	}
	_, _ = database.CleanupExpired()
	_, _ = database.PruneEvents(eventLogLimit(cfg))
	_ = database.WalCheckpoint()
}

func eventLogLimit(cfg *config.Config) int {
	if cfg == nil || cfg.MaxEventLogRows <= 0 {
		return 10000
	}
	return cfg.MaxEventLogRows
}
