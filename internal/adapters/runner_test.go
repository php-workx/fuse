package adapters

import (
	"os"
	"strings"
	"testing"
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

func TestExecuteCommand_SafeCommand(t *testing.T) {
	// This test actually executes a command, so use a safe one.
	// We need to set up a temporary working directory.
	tmpDir := t.TempDir()

	// Override HOME to avoid reading real config/policy/DB files.
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	exitCode, err := ExecuteCommand("echo hello", tmpDir)
	if err != nil {
		t.Fatalf("ExecuteCommand returned error: %v", err)
	}
	if exitCode != 0 {
		t.Errorf("exit code = %d, want 0", exitCode)
	}
}
