//go:build darwin

package attention

import (
	"bytes"
	"testing"
)

type fakeTTYCapturer struct {
	input  string
	output string
	err    error
}

func (capturer fakeTTYCapturer) CaptureTTY(stream terminalStream) (string, error) {
	if capturer.err != nil {
		return "", capturer.err
	}
	if stream == terminalStandardInput {
		return capturer.input, nil
	}
	return capturer.output, nil
}

func TestCaptureTerminalTargetMetadataCapturesExactGhosttyFacts(t *testing.T) {
	metadata, err := captureTerminalTargetMetadata(
		terminalEnvironment{
			"TERM_PROGRAM":         "Ghostty",
			"TERM_PROGRAM_VERSION": "1.3.1",
			"ITERM_SESSION_ID":     "w0t0p0:ignored-by-ghostty",
		},
		fakeTTYCapturer{input: "/dev/ttys004", output: "/dev/ttys004"},
		bytes.NewReader(bytes.Repeat([]byte{0xab}, 16)),
	)
	if err != nil {
		t.Fatalf("capture metadata: %v", err)
	}
	if metadata.TerminalProgram != "Ghostty" || metadata.TerminalProgramVersion != "1.3.1" {
		t.Fatalf("terminal identity = %#v", metadata)
	}
	if metadata.ITermSessionID != "w0t0p0:ignored-by-ghostty" || metadata.ControllingTTY != "/dev/ttys004" {
		t.Fatalf("terminal metadata = %#v", metadata)
	}
	if metadata.GhosttyTitleMarker != "abababababababababababababababab" {
		t.Fatalf("marker = %q, want 128-bit hex marker", metadata.GhosttyTitleMarker)
	}
	if metadata.Degradation.ExactTargetingDegraded() {
		t.Fatalf("matching local terminal streams unexpectedly degraded: %#v", metadata.Degradation)
	}
}

func TestCaptureTerminalTargetMetadataMarksUnsupportedTopology(t *testing.T) {
	metadata, err := captureTerminalTargetMetadata(
		terminalEnvironment{
			"TERM_PROGRAM":    "Apple_Terminal",
			"TMUX":            "/tmp/tmux-501/default,1,0",
			"SSH_CONNECTION": "192.0.2.1 22 192.0.2.2 34567",
		},
		fakeTTYCapturer{input: "/dev/ttys004", output: "/dev/ttys005"},
		bytes.NewReader(make([]byte, 16)),
	)
	if err != nil {
		t.Fatalf("capture metadata: %v", err)
	}
	if !metadata.Degradation.Multiplexer || !metadata.Degradation.RemoteShell || !metadata.Degradation.StandardStreamsMismatch {
		t.Fatalf("topology degradation = %#v", metadata.Degradation)
	}
	if !metadata.Degradation.ExactTargetingDegraded() {
		t.Fatal("unsupported terminal topology claimed exact targeting")
	}
}
