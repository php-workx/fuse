//go:build unix

package adapters

import "os/exec"

// proxyChildCleanup returns a cleanup function for the proxy's downstream
// server. On Unix, process group management is handled by the OS (Setpgid),
// so this just kills the direct child.
func proxyChildCleanup(cmd *exec.Cmd) func() {
	return func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
	}
}
