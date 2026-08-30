//go:build darwin

package cli

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/php-workx/fuse/internal/attention"
)

const notifierProtocolVersion = 1

func notificationDoctorChecks(live bool) []checkResult {
	diagnostic, helper := inspectNotifierBundle()
	results := []checkResult{
		notificationDiagnosticResult(diagnostic),
		notificationTerminalDiagnostic(),
	}
	if live {
		if helper == "" || diagnostic.state == notifierStateMissing || diagnostic.state == notifierStateTampered || diagnostic.state == notifierStateIncompatible || diagnostic.state == notifierStateSuppressed {
			results = append(results, notificationLiveDiagnosticResult(notifierDiagnostic{state: diagnostic.state, detail: "live focus unavailable: " + diagnostic.detail}))
		} else {
			results = append(results, notificationLiveDiagnosticResult(runNotifierFocusDiagnostic(helper)))
		}
	}
	return results
}

func notificationTerminalDiagnostic() checkResult {
	metadata, err := attention.CaptureCurrentTerminalTargetMetadata()
	if err != nil {
		return checkResult{name: checkNameNativeNotificationTerminal, status: "WARN", detail: "terminal target metadata unavailable: " + err.Error()}
	}
	targeting := "exact terminal targeting available"
	if metadata.Degradation.ExactTargetingDegraded() {
		targeting = "application fallback targeting"
	}
	detail := fmt.Sprintf("terminal %q version %q; session %q; tty %q; multiplexer=%t; ssh=%t; %s", metadata.TerminalProgram, metadata.TerminalProgramVersion, metadata.ITermSessionID, metadata.ControllingTTY, metadata.Degradation.Multiplexer, metadata.Degradation.RemoteShell, targeting)
	status := "PASS"
	if metadata.Degradation.ExactTargetingDegraded() {
		status = "WARN"
	}
	return checkResult{name: checkNameNativeNotificationTerminal, status: status, detail: detail}
}

func inspectNotifierBundle() (notifierDiagnostic, string) {
	home, homeErr := os.UserHomeDir()
	if homeErr != nil {
		return notifierDiagnostic{state: notifierStateSuppressed, detail: "native notifications unavailable because the user home directory could not be resolved: " + homeErr.Error()}, ""
	}
	defaultHome := filepath.Join(home, ".fuse")
	if configured := os.Getenv("FUSE_HOME"); configured != "" && filepath.Clean(configured) != defaultHome && os.Getenv("FUSE_NOTIFIER_ALLOW_CUSTOM_HOME") != "1" {
		return notifierDiagnostic{state: notifierStateSuppressed, detail: "native notifications are disabled for custom FUSE_HOME; set FUSE_NOTIFIER_ALLOW_CUSTOM_HOME=1 only for explicit development or test use"}, ""
	}
	bundle, err := discoverNotifierBundle()
	if err != nil {
		return notifierDiagnostic{state: notifierStateMissing, detail: "Fuse Notifier.app not found in the build, archive, or ~/.fuse/libexec locations; run `just install` (plain `go install` does not install the helper)"}, ""
	}
	helper, err := validateNotifierBundle(bundle)
	if err != nil {
		state := notifierStateTampered
		if strings.Contains(err.Error(), "metadata") || strings.Contains(err.Error(), "architecture") || strings.Contains(err.Error(), "protocol") || strings.Contains(err.Error(), "executable") {
			state = notifierStateIncompatible
		}
		return notifierDiagnostic{state: state, detail: fmt.Sprintf("%s: %v", bundle, err)}, ""
	}

	signature, err := notifierSignature(bundle)
	if err != nil {
		return notifierDiagnostic{state: notifierStateTampered, detail: fmt.Sprintf("%s: strict signature verification failed: %v", bundle, err)}, ""
	}
	ready, err := runNotifierQuery(helper)
	if err != nil {
		return notifierDiagnostic{state: notifierStateIncompatible, detail: fmt.Sprintf("%s: protocol handshake failed: %v", bundle, err)}, ""
	}
	if ready.ProtocolMin > notifierProtocolVersion || ready.ProtocolMax < notifierProtocolVersion {
		return notifierDiagnostic{state: notifierStateIncompatible, detail: fmt.Sprintf("%s: helper protocol range %d-%d does not include %d", bundle, ready.ProtocolMin, ready.ProtocolMax, notifierProtocolVersion)}, ""
	}

	detail := fmt.Sprintf("%s; helper %s; protocol %d-%d; notification authorization %s; alerts %s; sound %s; %s", bundle, ready.HelperVersion, ready.ProtocolMin, ready.ProtocolMax, ready.Authorization, ready.AlertSetting, ready.SoundSetting, signature)
	if ready.Authorization != "authorized" || ready.AlertSetting != "enabled" {
		return notifierDiagnostic{state: notifierStateDenied, detail: detail}, helper
	}
	if strings.Contains(signature, "ad-hoc") {
		return notifierDiagnostic{state: notifierStateAdHoc, detail: detail}, helper
	}
	return notifierDiagnostic{state: notifierStateHealthy, detail: detail}, helper
}

func discoverNotifierBundle() (string, error) {
	executable, err := os.Executable()
	if err != nil {
		return "", err
	}
	executable, err = filepath.EvalSymlinks(executable)
	if err != nil {
		return "", err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	candidates := []string{
		filepath.Join(filepath.Dir(executable), "..", "libexec", "Fuse Notifier.app"),
		filepath.Join(filepath.Dir(executable), "Fuse Notifier.app"),
		filepath.Join(home, ".fuse", "libexec", "Fuse Notifier.app"),
	}
	if os.Getenv("FUSE_NOTIFIER_DEVELOPMENT") == "1" {
		if configured := os.Getenv("FUSE_NOTIFIER_APP"); configured != "" {
			candidates = append(candidates, configured)
		}
	}
	for _, candidate := range candidates {
		if _, err := os.Lstat(candidate); err != nil {
			continue
		}
		canonical, err := filepath.EvalSymlinks(candidate)
		if err != nil {
			return "", fmt.Errorf("resolving %s: %w", candidate, err)
		}
		return canonical, nil
	}
	return "", os.ErrNotExist
}

func validateNotifierBundle(bundle string) (string, error) {
	canonicalBundle, err := filepath.EvalSymlinks(bundle)
	if err != nil {
		return "", err
	}
	for key, want := range map[string]string{
		"CFBundleIdentifier":     "com.php-workx.fuse.notifier",
		"CFBundleExecutable":     "FuseNotifier",
		"LSMinimumSystemVersion": "13.0",
	} {
		got, err := notifierPlistValue(filepath.Join(canonicalBundle, "Contents", "Info.plist"), key)
		if err != nil || got != want {
			return "", fmt.Errorf("bundle metadata %s = %q, want %q", key, got, want)
		}
	}
	helper, err := filepath.EvalSymlinks(filepath.Join(canonicalBundle, "Contents", "MacOS", "FuseNotifier"))
	if err != nil {
		return "", fmt.Errorf("helper executable: %w", err)
	}
	relative, err := filepath.Rel(canonicalBundle, helper)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(os.PathSeparator)) {
		return "", errors.New("helper executable escapes its app bundle")
	}
	info, err := os.Stat(helper)
	if err != nil || info.Mode()&0o111 == 0 {
		return "", errors.New("helper executable is not executable")
	}
	architectures, err := exec.Command("/usr/bin/lipo", "-archs", helper).Output()
	if err != nil {
		return "", fmt.Errorf("helper architecture: %w", err)
	}
	archSet := map[string]bool{}
	for _, architecture := range strings.Fields(string(architectures)) {
		archSet[architecture] = true
	}
	if !archSet["arm64"] || !archSet["x86_64"] {
		return "", fmt.Errorf("helper architecture %q is not universal (need arm64 and x86_64)", strings.TrimSpace(string(architectures)))
	}
	return helper, nil
}

func notifierPlistValue(path, key string) (string, error) {
	output, err := exec.Command("/usr/libexec/PlistBuddy", "-c", "Print :"+key, path).Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

func notifierSignature(bundle string) (string, error) {
	verify := exec.Command("/usr/bin/codesign", "--verify", "--strict", "--verbose=2", bundle)
	if output, err := verify.CombinedOutput(); err != nil {
		return "", errors.New(strings.TrimSpace(string(output)))
	}
	identity := exec.Command("/usr/bin/codesign", "-dv", "--verbose=4", bundle)
	output, err := identity.CombinedOutput()
	if err != nil {
		return "", errors.New(strings.TrimSpace(string(output)))
	}
	return assessNotifierSignature(string(output), attention.NotifierDeveloperTeamID(), func() error {
		assessment := exec.Command("/usr/sbin/spctl", "--assess", "--type", "execute", bundle)
		if output, err := assessment.CombinedOutput(); err != nil {
			return errors.New(strings.TrimSpace(string(output)))
		}
		return nil
	})
}

func assessNotifierSignature(identity, configuredTeamID string, assessGatekeeper func() error) (string, error) {
	if strings.Contains(identity, "Signature=adhoc") {
		if configuredTeamID != "" {
			return "", errors.New("ad-hoc notifier signature does not match the configured release Team ID")
		}
		return "ad-hoc signature", nil
	}
	team := ""
	for _, line := range strings.Split(identity, "\n") {
		if strings.HasPrefix(line, "TeamIdentifier=") {
			team = strings.TrimPrefix(line, "TeamIdentifier=")
			break
		}
	}
	if team == "" || team == "not set" {
		return "", errors.New("signature has no TeamIdentifier")
	}
	if configuredTeamID != "" {
		if team != configuredTeamID {
			return "", fmt.Errorf("notifier TeamIdentifier %q does not match configured release Team ID %q", team, configuredTeamID)
		}
		if err := assessGatekeeper(); err != nil {
			return "", fmt.Errorf("Gatekeeper assessment failed: %w", err)
		}
	}
	return "Developer ID Team ID " + team, nil
}

type notifierReady struct {
	Type         string `json:"type"`
	ProtocolMin  int    `json:"protocol_min"`
	ProtocolMax  int    `json:"protocol_max"`
	HelperVersion string `json:"helper_version"`
	Authorization string `json:"authorization"`
	AlertSetting string `json:"alert_setting"`
	SoundSetting string `json:"sound_setting"`
}

type notificationTerminalWire struct {
	TerminalProgram             string `json:"terminal_program"`
	TerminalProgramVersion      string `json:"terminal_program_version"`
	ITermSessionID              string `json:"iterm_session_id,omitempty"`
	TerminalTTY                 string `json:"terminal_tty,omitempty"`
	GhosttyTitleMarker          string `json:"ghostty_title_marker,omitempty"`
	ExactTargetingDegraded      bool   `json:"exact_targeting_degraded"`
}

func writeNotifierHello(input io.Writer) error {
	metadata, err := attention.CaptureCurrentTerminalTargetMetadata()
	if err != nil {
		return err
	}
	if metadata.TerminalProgram == "" {
		metadata.TerminalProgram = "unknown"
	}
	if metadata.TerminalProgramVersion == "" {
		metadata.TerminalProgramVersion = "unknown"
	}
	terminal := notificationTerminalWire{
		TerminalProgram:        metadata.TerminalProgram,
		TerminalProgramVersion: metadata.TerminalProgramVersion,
		ITermSessionID:         metadata.ITermSessionID,
		TerminalTTY:            metadata.ControllingTTY,
		GhosttyTitleMarker:     metadata.GhosttyTitleMarker,
		ExactTargetingDegraded: metadata.Degradation.ExactTargetingDegraded(),
	}
	message := struct {
		Version       int                    `json:"version"`
		Type          string                 `json:"type"`
		RequestID     int                    `json:"request_id"`
		ClientVersion string                 `json:"client_version"`
		Terminal      notificationTerminalWire `json:"terminal"`
	}{Version: 1, Type: "hello", RequestID: 1, ClientVersion: "doctor", Terminal: terminal}
	encoded, err := json.Marshal(message)
	if err != nil {
		return err
	}
	_, err = input.Write(append(encoded, '\n'))
	return err
}

func runNotifierQuery(helper string) (notifierReady, error) {
	command, input, output, err := startNotifier(helper)
	if err != nil {
		return notifierReady{}, err
	}
	defer stopNotifier(command, input)
	if err := writeNotifierHello(input); err != nil {
		return notifierReady{}, err
	}
	var ready notifierReady
	if err := readNotifierEvent(output, &ready); err != nil {
		return notifierReady{}, err
	}
	if ready.Type != "ready" {
		return notifierReady{}, fmt.Errorf("expected ready response, got %q", ready.Type)
	}
	return ready, nil
}

func runNotifierFocusDiagnostic(helper string) notifierDiagnostic {
	command, input, output, err := startNotifier(helper)
	if err != nil {
		return notifierDiagnostic{state: notifierStateIncompatible, detail: "could not start helper for live focus: " + err.Error()}
	}
	defer stopNotifier(command, input)
	if err := writeNotifierHello(input); err != nil {
		return notifierDiagnostic{state: notifierStateIncompatible, detail: "could not query helper before live focus: " + err.Error()}
	}
	var ready notifierReady
	if err := readNotifierEvent(output, &ready); err != nil || ready.Type != "ready" {
		return notifierDiagnostic{state: notifierStateIncompatible, detail: "helper did not become ready for live focus"}
	}
	if _, err := io.WriteString(input, `{"version":1,"type":"focus","request_id":2}`+"\n"); err != nil {
		return notifierDiagnostic{state: notifierStateIncompatible, detail: "could not request live focus: " + err.Error()}
	}
	var focus struct {
		Type   string `json:"type"`
		Mode   string `json:"mode"`
		Detail string `json:"detail"`
	}
	if err := readNotifierEvent(output, &focus); err != nil {
		return notifierDiagnostic{state: notifierStateIncompatible, detail: "live focus did not return a result: " + err.Error()}
	}
	return notifierFocusDiagnostic(focus.Type, focus.Mode, focus.Detail)
}

func startNotifier(helper string) (*exec.Cmd, io.WriteCloser, *bufio.Scanner, error) {
	command := exec.Command(helper)
	input, err := command.StdinPipe()
	if err != nil {
		return nil, nil, nil, err
	}
	outputPipe, err := command.StdoutPipe()
	if err != nil {
		return nil, nil, nil, err
	}
	var stderr bytes.Buffer
	command.Stderr = &stderr
	if err := command.Start(); err != nil {
		return nil, nil, nil, fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return command, input, bufio.NewScanner(outputPipe), nil
}

func readNotifierEvent(scanner *bufio.Scanner, target any) error {
	result := make(chan error, 1)
	go func() {
		if !scanner.Scan() {
			result <- fmt.Errorf("helper closed diagnostic stream: %v", scanner.Err())
			return
		}
		result <- json.Unmarshal(scanner.Bytes(), target)
	}()
	select {
	case err := <-result:
		return err
	case <-time.After(5 * time.Second):
		return errors.New("helper diagnostic timed out")
	}
}

func stopNotifier(command *exec.Cmd, input io.WriteCloser) {
	_, _ = io.WriteString(input, `{"version":1,"type":"shutdown","request_id":99}`+"\n")
	_ = input.Close()
	done := make(chan struct{})
	go func() { _ = command.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		_ = command.Process.Kill()
		<-done
	}
}
