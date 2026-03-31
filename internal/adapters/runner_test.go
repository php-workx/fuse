package adapters

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/php-workx/fuse/internal/core"
	"github.com/php-workx/fuse/internal/policy"
)

func TestBuildChildEnv(t *testing.T) {
	input := []string{
		"HOME=/home/user",
		"PATH=/usr/bin:/bin",
		"LD_PRELOAD=/evil/lib.so",
		"DYLD_INSERT_LIBRARIES=/evil/dylib",
		"DYLD_LIBRARY_PATH=/evil/path",
		"PYTHONPATH=/evil/python",
		"NODE_PATH=/evil/node",
		"RUBYLIB=/evil/ruby",
		"BASH_ENV=/evil/bashrc",
		"ENV=/evil/env",
		"LD_LIBRARY_PATH=/evil/ld",
		"EDITOR=vim",
	}

	result := BuildChildEnv(input)

	// Build a map for easy lookup.
	envMap := make(map[string]string)
	for _, e := range result {
		parts := strings.SplitN(e, "=", 2)
		if len(parts) == 2 {
			envMap[parts[0]] = parts[1]
		}
	}

	// PATH should be reset to trusted path.
	if path, ok := envMap["PATH"]; !ok {
		t.Error("PATH not set in output")
	} else if path != trustedPath() {
		t.Errorf("PATH = %q, want %q", path, trustedPath())
	}

	// HOME and EDITOR should be preserved.
	if home, ok := envMap["HOME"]; !ok || home != "/home/user" {
		t.Errorf("HOME = %q, want /home/user", home)
	}
	if editor, ok := envMap["EDITOR"]; !ok || editor != "vim" {
		t.Errorf("EDITOR = %q, want vim", editor)
	}

	// Dangerous vars should be stripped.
	for _, name := range []string{
		"LD_PRELOAD", "LD_LIBRARY_PATH", "DYLD_INSERT_LIBRARIES",
		"DYLD_LIBRARY_PATH", "PYTHONPATH", "NODE_PATH",
		"RUBYLIB", "BASH_ENV", "ENV",
	} {
		if _, ok := envMap[name]; ok {
			t.Errorf("%s should be stripped but was present", name)
		}
	}
}

func TestBuildChildEnv_PreservesOther(t *testing.T) {
	input := []string{
		"HOME=/home/user",
		"TERM=xterm-256color",
		"LANG=en_US.UTF-8",
		"SHELL=/bin/bash",
		"USER=testuser",
		"GOPATH=/home/user/go",
		"CUSTOM_VAR=custom_value",
	}

	result := BuildChildEnv(input)

	// Build a map for easy lookup.
	envMap := make(map[string]string)
	for _, e := range result {
		parts := strings.SplitN(e, "=", 2)
		if len(parts) == 2 {
			envMap[parts[0]] = parts[1]
		}
	}

	// All non-stripped vars should be preserved.
	expected := map[string]string{
		"HOME":       "/home/user",
		"TERM":       "xterm-256color",
		"LANG":       "en_US.UTF-8",
		"SHELL":      "/bin/bash",
		"USER":       "testuser",
		"GOPATH":     "/home/user/go",
		"CUSTOM_VAR": "custom_value",
	}

	for name, want := range expected {
		got, ok := envMap[name]
		if !ok {
			t.Errorf("%s not found in output", name)
		} else if got != want {
			t.Errorf("%s = %q, want %q", name, got, want)
		}
	}

	// PATH should be added even though it wasn't in input.
	if _, ok := envMap["PATH"]; !ok {
		t.Error("PATH not set in output when missing from input")
	}
}

func TestBuildChildEnv_StripsDangerous(t *testing.T) {
	// Test each dangerous variable individually to ensure comprehensive stripping.
	dangerous := []string{
		"LD_PRELOAD=/evil/lib.so",
		"LD_LIBRARY_PATH=/evil/path",
		"DYLD_INSERT_LIBRARIES=/evil/dylib",
		"DYLD_LIBRARY_PATH=/evil/dylib_path",
		"PYTHONPATH=/evil/python",
		"NODE_PATH=/evil/node",
		"RUBYLIB=/evil/ruby",
		"BASH_ENV=/evil/bashrc",
		"ENV=/evil/env",
		"JAVA_TOOL_OPTIONS=bad",
		"NODE_OPTIONS=--require /evil/hook.js",
	}

	for _, d := range dangerous {
		input := []string{d, "SAFE_VAR=keep"}
		result := BuildChildEnv(input)

		name := strings.SplitN(d, "=", 2)[0]
		for _, env := range result {
			if strings.HasPrefix(env, name+"=") {
				t.Errorf("dangerous var %s was not stripped", name)
			}
		}

		// Verify SAFE_VAR is preserved.
		found := false
		for _, env := range result {
			if env == "SAFE_VAR=keep" {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("SAFE_VAR was not preserved when stripping %s", name)
		}
	}
}

func TestBuildChildEnv_StripsDYLDPrefix(t *testing.T) {
	input := []string{
		"HOME=/home/user",
		"DYLD_FRAMEWORK_PATH=bad",
		"EDITOR=vim",
	}

	result := BuildChildEnv(input)

	for _, env := range result {
		if strings.HasPrefix(env, "DYLD_FRAMEWORK_PATH=") {
			t.Error("DYLD_FRAMEWORK_PATH should be stripped but was present")
		}
	}
}

func TestBuildChildEnv_ResetsPathToTrusted(t *testing.T) {
	input := []string{
		"PATH=/evil",
		"HOME=/home/user",
	}

	result := BuildChildEnv(input)

	envMap := make(map[string]string)
	for _, e := range result {
		parts := strings.SplitN(e, "=", 2)
		if len(parts) == 2 {
			envMap[parts[0]] = parts[1]
		}
	}

	if path, ok := envMap["PATH"]; !ok {
		t.Error("PATH not set in output")
	} else if path != trustedPath() {
		t.Errorf("PATH = %q, want %q", path, trustedPath())
	}
}

func TestBuildChildEnv_StripsLDPreload(t *testing.T) {
	input := []string{
		"LD_PRELOAD=/lib",
		"HOME=/home/user",
	}

	result := BuildChildEnv(input)

	for _, env := range result {
		if strings.HasPrefix(env, "LD_PRELOAD=") {
			t.Error("LD_PRELOAD should be stripped but was present")
		}
	}
}

func TestBuildChildEnv_WindowsVars(t *testing.T) {
	input := []string{
		"HOME=/home/user",
		"PATH=/usr/bin:/bin",
		"PSModulePath=C:\\Windows\\System32\\WindowsPowerShell\\v1.0\\Modules",
		"PSMODULEPATH=C:\\Windows\\System32\\WindowsPowerShell\\v1.0\\Modules",
		"PSExecutionPolicyPreference=Bypass",
		"COMPLUS_Version=v4.0.30319",
		"COMSPEC=C:\\Windows\\system32\\cmd.exe",
		"JAVA_TOOL_OPTIONS=bad",
		"NODE_OPTIONS=--require evil",
		"EDITOR=vim",
	}

	result := BuildChildEnv(input)

	envMap := make(map[string]string)
	for _, e := range result {
		parts := strings.SplitN(e, "=", 2)
		if len(parts) == 2 {
			envMap[parts[0]] = parts[1]
		}
	}

	// These are in strippedEnvVars and must always be stripped.
	for _, name := range []string{
		"PSModulePath", "PSMODULEPATH", "PSExecutionPolicyPreference",
		"COMPLUS_Version", "COMSPEC", "JAVA_TOOL_OPTIONS", "NODE_OPTIONS",
	} {
		if _, ok := envMap[name]; ok {
			t.Errorf("%s should be stripped but was present", name)
		}
	}

	// Safe vars should be preserved.
	if home, ok := envMap["HOME"]; !ok || home != "/home/user" {
		t.Errorf("HOME = %q, want /home/user", home)
	}
	if editor, ok := envMap["EDITOR"]; !ok || editor != "vim" {
		t.Errorf("EDITOR = %q, want vim", editor)
	}
}

func TestBuildChildEnv_COMPLUSPrefix(t *testing.T) {
	// COMPLUS_* prefix stripping is platform-gated to Windows only.
	input := []string{
		"HOME=/home/user",
		"COMPLUS_Foo=bar",
		"COMPLUS_EnableDiagnostics=1",
		"EDITOR=vim",
	}

	result := BuildChildEnv(input)

	envMap := make(map[string]string)
	for _, e := range result {
		parts := strings.SplitN(e, "=", 2)
		if len(parts) == 2 {
			envMap[parts[0]] = parts[1]
		}
	}

	if runtime.GOOS == "windows" {
		// On Windows, all COMPLUS_* vars should be stripped via prefix check.
		for _, name := range []string{"COMPLUS_Foo", "COMPLUS_EnableDiagnostics"} {
			if _, ok := envMap[name]; ok {
				t.Errorf("%s should be stripped on Windows but was present", name)
			}
		}
	} else {
		// On non-Windows, unknown COMPLUS_* vars (not in strippedEnvVars) are preserved.
		for _, name := range []string{"COMPLUS_Foo", "COMPLUS_EnableDiagnostics"} {
			if _, ok := envMap[name]; !ok {
				t.Errorf("%s should be preserved on %s but was missing", name, runtime.GOOS)
			}
		}
	}

	// EDITOR should always be preserved.
	if editor, ok := envMap["EDITOR"]; !ok || editor != "vim" {
		t.Errorf("EDITOR = %q, want vim", editor)
	}
}

// TestBuildChildEnv_CaseNormalization verifies the lookupName logic: on Windows
// env var names are case-insensitive (e.g. "comspec" must be stripped just like
// "COMSPEC"), while on all platforms original casing is preserved in the output.
// This is a cross-platform test — it exercises the normalization code path by
// constructing inputs that exercise it directly.
func TestBuildChildEnv_CaseNormalization(t *testing.T) {
	// comspec=evil.exe — lowercase variant of a stripped Windows var.
	// Lookup is always uppercased, so "comspec" → "COMSPEC" matches the map on all platforms.
	inputComspec := []string{"comspec=evil.exe", "SAFE=keep"}
	resultComspec := BuildChildEnv(inputComspec)
	comspecFound := false
	for _, env := range resultComspec {
		if strings.HasPrefix(env, "comspec=") || strings.HasPrefix(env, "COMSPEC=") {
			comspecFound = true
		}
	}
	if comspecFound {
		t.Error("comspec= should be stripped (uppercased lookup matches COMSPEC) but was present")
	}

	// Path=/attacker/bin — title-case PATH variant seen in Windows os.Environ output.
	// strings.EqualFold must match it and replace with trustedPath().
	inputPath := []string{"Path=/attacker/bin", "SAFE=keep"}
	resultPath := BuildChildEnv(inputPath)
	envMapPath := make(map[string]string)
	for _, e := range resultPath {
		parts := strings.SplitN(e, "=", 2)
		if len(parts) == 2 {
			envMapPath[parts[0]] = parts[1]
		}
	}
	// "Path" key should not survive — it must have been consumed and rewritten as "PATH".
	if val, ok := envMapPath["Path"]; ok {
		t.Errorf("Path= should be replaced by trusted PATH but original key survived with value %q", val)
	}
	// A PATH entry with the trusted value must be present.
	if path, ok := envMapPath["PATH"]; !ok {
		t.Error("PATH not set after processing title-case Path= input")
	} else if path != trustedPath() {
		t.Errorf("PATH = %q after title-case input, want trustedPath %q", path, trustedPath())
	}

	// Original casing must be preserved for non-stripped vars.
	// e.g. "MyVar=value" should appear as "MyVar=value", not "MYVAR=value".
	inputCasing := []string{"MyVar=value", "anotherVar=hello"}
	resultCasing := BuildChildEnv(inputCasing)
	envMapCasing := make(map[string]string)
	for _, e := range resultCasing {
		parts := strings.SplitN(e, "=", 2)
		if len(parts) == 2 {
			envMapCasing[parts[0]] = parts[1]
		}
	}
	if v, ok := envMapCasing["MyVar"]; !ok || v != "value" {
		t.Errorf("MyVar casing not preserved: got key present=%v value=%q, want present=true value=value", ok, v)
	}
	if v, ok := envMapCasing["anotherVar"]; !ok || v != "hello" {
		t.Errorf("anotherVar casing not preserved: got key present=%v value=%q, want present=true value=hello", ok, v)
	}
}

// TestTrustedPath_WindowsSystemRootValidation verifies that trustedPath() always
// returns a path containing System32 even when SystemRoot is attacker-controlled.
// On Windows, the validation in isValidWindowsRoot() rejects suspicious values and
// falls back to C:\Windows. On other platforms this test validates the platform PATH.
func TestTrustedPath_WindowsSystemRootValidation(t *testing.T) {
	if runtime.GOOS != "windows" {
		// On non-Windows, trustedPath() returns a static trusted path.
		path := trustedPath()
		if path == "" {
			t.Error("trustedPath() returned empty string on non-Windows")
		}
		return
	}

	// On Windows: verify that attacker-controlled SystemRoot values are rejected.
	cases := []struct {
		name       string
		systemRoot string
		wantPrefix string
	}{
		{
			name:       "normal value",
			systemRoot: `C:\Windows`,
			wantPrefix: `C:\Windows\System32`,
		},
		{
			name:       "empty value falls back to C:\\Windows",
			systemRoot: "",
			wantPrefix: `C:\Windows\System32`,
		},
		{
			name:       "UNC path is rejected",
			systemRoot: `\\attacker\share`,
			wantPrefix: `C:\Windows\System32`,
		},
		{
			name:       "relative path is rejected",
			systemRoot: `evil\path`,
			wantPrefix: `C:\Windows\System32`,
		},
		{
			name:       "forward slash is rejected",
			systemRoot: `C:/Windows`,
			wantPrefix: `C:\Windows\System32`,
		},
		{
			name:       "digit drive letter is rejected",
			systemRoot: `1:\Windows`,
			wantPrefix: `C:\Windows\System32`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			orig := os.Getenv("SystemRoot")
			if tc.systemRoot == "" {
				os.Unsetenv("SystemRoot")
			} else {
				os.Setenv("SystemRoot", tc.systemRoot)
			}
			defer func() {
				if orig == "" {
					os.Unsetenv("SystemRoot")
				} else {
					os.Setenv("SystemRoot", orig)
				}
			}()

			path := trustedPath()
			if !strings.HasPrefix(path, tc.wantPrefix) {
				t.Errorf("trustedPath() = %q, want prefix %q", path, tc.wantPrefix)
			}
		})
	}
}

func TestExecuteCommand_SafeCommand(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test uses Unix-specific shell commands")
	}
	// This test actually executes a command, so use a safe one.
	// We need to set up a temporary working directory.
	tmpDir := t.TempDir()

	// Override HOME to avoid reading real config/policy/DB files.
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	exitCode, err := ExecuteCommand("echo hello", tmpDir, time.Minute)
	if err != nil {
		t.Fatalf("ExecuteCommand returned error: %v", err)
	}
	if exitCode != 0 {
		t.Errorf("exit code = %d, want 0", exitCode)
	}
}

func TestExecuteCommand_DryRunAllowsBlockedCommand(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test uses Unix-specific shell commands")
	}
	withFuseHome(t)
	enableDryRunForTest(t)
	exitCode, err := ExecuteCommand("printf dryrun", t.TempDir(), time.Minute)
	if err != nil {
		t.Fatalf("ExecuteCommand in dry-run returned error: %v", err)
	}
	if exitCode != 0 {
		t.Errorf("exit code = %d, want 0 in dry-run", exitCode)
	}
}

func TestExecuteCommand_DisabledPassesThrough(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test uses Unix-specific shell commands")
	}
	withFuseHome(t)
	// Neither enabled nor dry-run — fully disabled.

	exitCode, err := ExecuteCommand("printf disabled", t.TempDir(), time.Minute)
	if err != nil {
		t.Fatalf("ExecuteCommand when disabled returned error: %v", err)
	}
	if exitCode != 0 {
		t.Errorf("exit code = %d, want 0 when disabled", exitCode)
	}
}

func TestExecuteCommand_EnabledBlockedCommand(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test uses Unix-specific shell commands")
	}
	withFuseHome(t)
	enableFuseForTest(t)

	exitCode, err := ExecuteCommand("rm -rf /", t.TempDir(), time.Minute)
	if err != nil {
		t.Fatalf("ExecuteCommand returned error: %v", err)
	}
	if exitCode != 1 {
		t.Errorf("exit code = %d, want 1 for blocked command", exitCode)
	}
}

func TestReverifyDecisionKeyDetectsChangedScript(t *testing.T) {
	tmpDir := t.TempDir()
	scriptPath := filepath.Join(tmpDir, "task.py")
	if err := os.WriteFile(scriptPath, []byte("print('safe')\n"), 0o644); err != nil {
		t.Fatalf("failed to write script: %v", err)
	}

	evaluator := policy.NewEvaluator(nil)
	req := core.ShellRequest{
		RawCommand: "python task.py",
		Cwd:        tmpDir,
		Source:     "run",
	}

	result, err := core.Classify(req, evaluator)
	if err != nil {
		t.Fatalf("initial classify failed: %v", err)
	}

	if err := os.WriteFile(scriptPath, []byte("import boto3\nboto3.client('cloudformation').delete_stack(StackName='prod')\n"), 0o644); err != nil {
		t.Fatalf("failed to modify script: %v", err)
	}

	if err := reverifyDecisionKey(req, evaluator, result.DecisionKey); err == nil {
		t.Fatal("expected reverifyDecisionKey to detect modified script")
	}
}
