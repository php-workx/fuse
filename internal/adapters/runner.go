package adapters

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"

	"github.com/runger/fuse/internal/approve"
	"github.com/runger/fuse/internal/config"
	"github.com/runger/fuse/internal/core"
	"github.com/runger/fuse/internal/db"
	"github.com/runger/fuse/internal/policy"
)

// strippedEnvVars lists environment variables that are stripped from the child
// process environment for security (§10.1).
var strippedEnvVars = map[string]bool{
	"LD_PRELOAD":            true,
	"LD_LIBRARY_PATH":       true,
	"DYLD_INSERT_LIBRARIES": true,
	"DYLD_LIBRARY_PATH":     true,
	"PYTHONPATH":            true,
	"NODE_PATH":             true,
	"RUBYLIB":               true,
	"BASH_ENV":              true,
	"ENV":                   true,
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
		if strippedEnvVars[name] {
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
	if _, err := unix.IoctlGetTermios(fd, ioctlGetTermios); err != nil {
		// Not a terminal — nothing to do.
		return nil, nil
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

// ExecuteCommand classifies and optionally runs a shell command.
// In run mode, it: classify -> prompt if needed -> execute with safety controls.
func ExecuteCommand(command string, cwd string) (exitCode int, err error) {
	// Load configuration.
	cfg, err := config.LoadConfig(config.ConfigPath())
	if err != nil {
		// Non-fatal: proceed with defaults.
		cfg = nil
	}
	_ = cfg // config used for future features

	// Load policy.
	policyCfg, _ := policy.LoadPolicy(config.PolicyPath())
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

	// Open database for event logging and approvals.
	database, dbErr := db.OpenDB(config.DBPath())
	if dbErr != nil {
		// Non-fatal: we can still execute, just can't log.
		database = nil
	}
	if database != nil {
		defer func() { _ = database.Close() }()
	}

	// Handle classification result.
	switch result.Decision {
	case core.DecisionBlocked:
		fmt.Fprintf(os.Stderr, "fuse: BLOCKED — %s\n", result.Reason)
		logEvent(database, command, result, "blocked")
		return 1, nil

	case core.DecisionSafe:
		// Execute directly.

	case core.DecisionCaution, core.DecisionApproval:
		// Prompt user for approval.
		if database == nil {
			fmt.Fprintf(os.Stderr, "fuse: approval required but database unavailable\n")
			return 1, fmt.Errorf("database unavailable for approval")
		}

		secret, secretErr := db.EnsureSecret(config.SecretPath())
		if secretErr != nil {
			return 1, fmt.Errorf("load HMAC secret: %w", secretErr)
		}

		mgr, mgrErr := approve.NewManager(database, secret)
		if mgrErr != nil {
			return 1, fmt.Errorf("create approval manager: %w", mgrErr)
		}
		decision, promptErr := mgr.RequestApproval(
			result.DecisionKey,
			command,
			result.Reason,
			"",    // no session ID in run mode
			false, // not hook mode — 5min timeout
		)
		if promptErr != nil {
			return 1, fmt.Errorf("approval: %w", promptErr)
		}
		if decision == core.DecisionBlocked {
			fmt.Fprintf(os.Stderr, "fuse: denied by user\n")
			logEvent(database, command, result, "denied")
			return 1, nil
		}
	}

	// Execute the command.
	exitCode, err = executeShellCommand(command, cwd)

	// Log event.
	outcome := "executed"
	if err != nil {
		outcome = "error"
	}
	logEvent(database, command, result, outcome)

	return exitCode, err
}

// executeShellCommand runs a shell command with safety controls (§10.1).
func executeShellCommand(command string, cwd string) (int, error) {
	cmd := exec.Command("/bin/sh", "-c", command)
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
	)
	go func() {
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
				if err := syscall.Kill(-cmd.Process.Pid, sysSig); err != nil {
					// Fallback: retry direct child PID.
					_ = syscall.Kill(cmd.Process.Pid, sysSig)
				}
			case <-done:
				return
			}
		}
	}()

	waitErr := cmd.Wait()
	signal.Stop(sigCh)
	close(done)
	if waitErr != nil {
		if exitErr, ok := waitErr.(*exec.ExitError); ok {
			if status, ok := exitErr.Sys().(syscall.WaitStatus); ok && status.Signaled() {
				return 128 + int(status.Signal()), nil
			}
			return exitErr.ExitCode(), nil
		}
		return -1, fmt.Errorf("wait for command: %w", waitErr)
	}
	return 0, nil
}

// logEvent logs an execution event to the database if available.
func logEvent(database *db.DB, command string, result *core.ClassifyResult, outcome string) {
	if database == nil {
		return
	}
	_ = database.LogEvent(
		"",                      // sessionID (none in run mode)
		command,                 // command
		string(result.Decision), // decision
		result.RuleID,           // ruleID
		result.Reason,           // reason
		0,                       // durationMs
		outcome,                 // metadata (used for outcome)
	)
}
