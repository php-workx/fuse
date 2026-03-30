//go:build unix

package cli

import (
	"io"
	"syscall"
	"testing"
	"time"
)

func TestStartForegroundProbeProcess_StaysAliveUntilKilled(t *testing.T) {
	cmd, err := startForegroundProbeProcess(nil, io.Discard, io.Discard)
	if err != nil {
		t.Fatalf("startForegroundProbeProcess: %v", err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()

	time.Sleep(200 * time.Millisecond)
	if err := syscall.Kill(cmd.Process.Pid, 0); err != nil {
		t.Fatalf("expected probe child to still be alive, got %v", err)
	}
}
