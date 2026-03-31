//go:build windows

package adapters

import (
	"log/slog"
	"os/exec"
)

// proxyChildCleanup creates a job object for the proxy's downstream server
// and returns a cleanup function. On Windows, this ensures the entire process
// tree is terminated (not just the direct child) when the proxy shuts down.
func proxyChildCleanup(cmd *exec.Cmd) func() {
	job, err := newJobObject()
	if err != nil {
		slog.Warn("proxy: job object creation failed, grandchild cleanup not guaranteed", "err", err)
		return func() {
			if cmd.Process != nil {
				_ = cmd.Process.Kill()
			}
		}
	}

	// Race window: between cmd.Start() (in caller) and job.assign() here,
	// grandchildren spawned by the downstream server are not tracked.
	// Same sub-millisecond window as runner_exec_windows.go — accepted.
	if err := job.assign(cmd.Process.Pid); err != nil {
		slog.Warn("proxy: job object assign failed, grandchild cleanup not guaranteed", "pid", cmd.Process.Pid, "err", err)
	}

	return func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		job.close()
	}
}
