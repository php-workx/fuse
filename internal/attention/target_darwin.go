//go:build darwin

package attention

import (
	"crypto/rand"
	"encoding/hex"
	"io"
	"os"
	"os/exec"
	"strings"
)

type terminalStream uint8

const (
	terminalStandardInput terminalStream = iota
	terminalStandardOutput
)

type ttyCapturer interface {
	CaptureTTY(terminalStream) (string, error)
}

type terminalEnvironment map[string]string

func (environment terminalEnvironment) lookup(name string) string {
	return environment[name]
}

type TerminalTargetDegradation struct {
	Multiplexer               bool
	RemoteShell               bool
	StandardStreamsMismatch   bool
	ControllingTTYUnavailable bool
}

func (degradation TerminalTargetDegradation) ExactTargetingDegraded() bool {
	return degradation.Multiplexer || degradation.RemoteShell || degradation.StandardStreamsMismatch || degradation.ControllingTTYUnavailable
}

type TerminalTargetMetadata struct {
	TerminalProgram        string
	TerminalProgramVersion string
	ITermSessionID         string
	ControllingTTY         string
	GhosttyTitleMarker     string
	Degradation            TerminalTargetDegradation
}

func CaptureCurrentTerminalTargetMetadata() (TerminalTargetMetadata, error) {
	environment := terminalEnvironment{}
	for _, value := range os.Environ() {
		name, value, found := strings.Cut(value, "=")
		if found {
			environment[name] = value
		}
	}
	return captureTerminalTargetMetadata(environment, commandTTYCapturer{}, rand.Reader)
}

func captureTerminalTargetMetadata(environment terminalEnvironment, capturer ttyCapturer, random io.Reader) (TerminalTargetMetadata, error) {
	metadata := TerminalTargetMetadata{
		TerminalProgram:        environment.lookup("TERM_PROGRAM"),
		TerminalProgramVersion: environment.lookup("TERM_PROGRAM_VERSION"),
		ITermSessionID:         environment.lookup("ITERM_SESSION_ID"),
	}

	stdinTTY, stdinError := capturer.CaptureTTY(terminalStandardInput)
	stdoutTTY, stdoutError := capturer.CaptureTTY(terminalStandardOutput)
	metadata.ControllingTTY = stdinTTY
	metadata.Degradation.ControllingTTYUnavailable = stdinError != nil || stdoutError != nil || stdinTTY == "" || stdoutTTY == ""
	metadata.Degradation.StandardStreamsMismatch = stdinTTY != "" && stdoutTTY != "" && stdinTTY != stdoutTTY
	metadata.Degradation.Multiplexer = environment.lookup("TMUX") != "" || environment.lookup("STY") != ""
	metadata.Degradation.RemoteShell = environment.lookup("SSH_CONNECTION") != "" || environment.lookup("SSH_TTY") != ""

	if metadata.TerminalProgram == "Ghostty" {
		marker := make([]byte, 16)
		if _, err := io.ReadFull(random, marker); err != nil {
			return TerminalTargetMetadata{}, err
		}
		metadata.GhosttyTitleMarker = hex.EncodeToString(marker)
	}
	return metadata, nil
}

type commandTTYCapturer struct{}

func (commandTTYCapturer) CaptureTTY(stream terminalStream) (string, error) {
	command := exec.Command("/usr/bin/tty")
	if stream == terminalStandardInput {
		command.Stdin = os.Stdin
	} else {
		command.Stdin = os.Stdout
	}
	output, err := command.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}
