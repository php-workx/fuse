//go:build darwin

package attention

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

var notifierDeveloperTeamID = ""

func New(ctx context.Context, options Options) (Service, error) {
	home, err := os.UserHomeDir()
	if err != nil { return nil, err }
	defaultFuseHome := filepath.Join(home, ".fuse")
	fuseHome := defaultFuseHome
	if configured := os.Getenv("FUSE_HOME"); configured != "" { fuseHome = configured }
	if options.ClientVersion == "" { options.ClientVersion = "unknown" }
	allowCustomHome := os.Getenv("FUSE_NOTIFIER_ALLOW_CUSTOM_HOME") == "1"
	if fuseHome != defaultFuseHome && !allowCustomHome {
		return newController(serviceDependencies{fuseHome: fuseHome, defaultFuseHome: defaultFuseHome, clientVersion: options.ClientVersion}), nil
	}
	terminal, err := CaptureCurrentTerminalTargetMetadata()
	if err != nil { return nil, err }
	app, err := discoverAndValidateHelper(home, options)
	if err != nil { return nil, err }
	dependencies := serviceDependencies{
		lock: newFileOwnerLock(home), starter: launchServicesStarter{app: app}, fuseHome: fuseHome, defaultFuseHome: defaultFuseHome, allowCustomHome: allowCustomHome, terminal: terminal, clientVersion: options.ClientVersion,
		restartDelay: func(ctx context.Context, duration time.Duration) error { timer := time.NewTimer(duration); defer timer.Stop(); select { case <-timer.C: return nil; case <-ctx.Done(): return ctx.Err() } },
		verifyHelperPID: verifiedHelperProcess(app), signalPID: func(pid int) error { return syscall.Kill(pid, syscall.SIGTERM) }, waitForExit: waitForProcessExit,
	}
	service := newController(dependencies)
	if status := service.Status(); status.Ownership == OwnershipDisabled && status.DegradedReason != "native notifications are disabled for custom FUSE_HOME" { return service, errors.New(status.DegradedReason) }
	return service, nil
}

func discoverAndValidateHelper(home string, options Options) (string, error) {
	executable, err := os.Executable()
	if err != nil { return "", err }
	executable, err = filepath.EvalSymlinks(executable)
	if err != nil { return "", err }
	directory := filepath.Dir(executable)
	candidates := []string{filepath.Join(directory, "..", "libexec", "Fuse Notifier.app"), filepath.Join(directory, "Fuse Notifier.app"), filepath.Join(home, ".fuse", "libexec", "Fuse Notifier.app")}
	if options.Development { if configured := os.Getenv("FUSE_NOTIFIER_APP"); configured != "" { candidates = append(candidates, configured) } }
	for _, candidate := range candidates {
		canonical, err := filepath.EvalSymlinks(candidate)
		if err != nil { continue }
		if err := validateHelperBundle(canonical, options); err == nil { return canonical, nil }
	}
	return "", errors.New("no valid Fuse Notifier.app found")
}

func validateHelperBundle(bundle string, options Options) error {
	infoPath := filepath.Join(bundle, "Contents", "Info.plist")
	if _, err := os.Stat(infoPath); err != nil { return err }
	expected := map[string]string{"CFBundleIdentifier": "com.php-workx.fuse.notifier", "CFBundleExecutable": "FuseNotifier", "LSMinimumSystemVersion": "13.0"}
	for key, value := range expected {
		actual, err := plistValue(infoPath, key)
		if err != nil || actual != value { return errors.New("notifier bundle metadata is invalid") }
	}
	canonicalBundle, err := filepath.EvalSymlinks(bundle)
	if err != nil { return err }
	helper, err := filepath.EvalSymlinks(filepath.Join(canonicalBundle, "Contents", "MacOS", "FuseNotifier"))
	if err != nil { return err }
	if !strings.HasPrefix(helper, canonicalBundle+string(os.PathSeparator)) { return errors.New("notifier executable escapes its app bundle") }
	if info, err := os.Stat(helper); err != nil || info.Mode()&0o111 == 0 { return errors.New("notifier executable is not executable") }
	if output, err := exec.Command("/usr/bin/codesign", "--verify", "--strict", "--verbose=2", canonicalBundle).CombinedOutput(); err != nil { return errors.New(strings.TrimSpace(string(output))) }
	if !options.Development {
		if notifierDeveloperTeamID == "" { return errors.New("notifier Developer ID Team ID is not configured") }
		identity, err := exec.Command("/usr/bin/codesign", "-dv", "--verbose=4", canonicalBundle).CombinedOutput()
		if err != nil || !strings.Contains(string(identity), "TeamIdentifier="+notifierDeveloperTeamID) { return errors.New("notifier signature does not match the configured Developer ID Team ID") }
		if output, err := exec.Command("/usr/sbin/spctl", "--assess", "--type", "execute", canonicalBundle).CombinedOutput(); err != nil { return errors.New(strings.TrimSpace(string(output))) }
	}
	return nil
}

func plistValue(path, key string) (string, error) {
	output, err := exec.Command("/usr/bin/plutil", "-extract", key, "raw", "-o", "-", path).Output()
	if err != nil { return "", err }
	return strings.TrimSpace(string(output)), nil
}

func verifiedHelperProcess(bundle string) func(int) bool {
	canonicalBundle, err := filepath.EvalSymlinks(bundle)
	if err != nil { return func(int) bool { return false } }
	prefix := canonicalBundle + string(os.PathSeparator)
	return func(pid int) bool {
		output, err := exec.Command("/bin/ps", "-p", strconv.Itoa(pid), "-o", "comm=").Output()
		if err != nil { return false }
		helper, err := filepath.EvalSymlinks(strings.TrimSpace(string(output)))
		return err == nil && strings.HasPrefix(helper, prefix)
	}
}

func waitForProcessExit(ctx context.Context, pid int, maximum time.Duration) error {
	deadline := time.NewTimer(maximum); defer deadline.Stop()
	ticker := time.NewTicker(50 * time.Millisecond); defer ticker.Stop()
	for {
		if err := syscall.Kill(pid, 0); err != nil { return nil }
		select { case <-ctx.Done(): return ctx.Err(); case <-deadline.C: return errors.New("stale notifier process did not exit"); case <-ticker.C: }
	}
}
