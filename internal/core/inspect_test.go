package core

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/php-workx/fuse/internal/inspect"
)

// testdataDir returns the absolute path to the testdata/scripts directory at
// the repository root.
func testdataDir(t *testing.T) string {
	t.Helper()
	// internal/core -> ../../testdata/scripts
	dir, err := filepath.Abs(filepath.Join("..", "..", "testdata", "scripts"))
	if err != nil {
		t.Fatalf("failed to resolve testdata dir: %v", err)
	}
	return dir
}

// --- InspectFile tests ---

func TestInspectFile_SafePython(t *testing.T) {
	path := filepath.Join(testdataDir(t), "safe_script.py")
	result, err := InspectFile(path, DefaultMaxBytes)
	if err != nil {
		t.Fatalf("InspectFile returned error: %v", err)
	}
	if !result.Exists {
		t.Fatal("expected file to exist")
	}
	if len(result.Signals) != 0 {
		t.Errorf("expected 0 signals, got %d: %+v", len(result.Signals), result.Signals)
	}
	if result.Decision != DecisionSafe {
		t.Errorf("expected decision SAFE, got %s", result.Decision)
	}
}

func TestInspectFile_DangerousPython(t *testing.T) {
	path := filepath.Join(testdataDir(t), "dangerous_boto3.py")
	result, err := InspectFile(path, DefaultMaxBytes)
	if err != nil {
		t.Fatalf("InspectFile returned error: %v", err)
	}
	if !result.Exists {
		t.Fatal("expected file to exist")
	}
	if len(result.Signals) == 0 {
		t.Fatal("expected signals for dangerous Python file, got 0")
	}
	if result.Decision != DecisionCaution {
		t.Errorf("expected CAUTION for boto3/subprocess file, got %s", result.Decision)
	}
}

func TestInspectFile_SafeShell(t *testing.T) {
	path := filepath.Join(testdataDir(t), "safe_script.sh")
	result, err := InspectFile(path, DefaultMaxBytes)
	if err != nil {
		t.Fatalf("InspectFile returned error: %v", err)
	}
	if !result.Exists {
		t.Fatal("expected file to exist")
	}
	if len(result.Signals) != 0 {
		t.Errorf("expected 0 signals, got %d: %+v", len(result.Signals), result.Signals)
	}
	if result.Decision != DecisionSafe {
		t.Errorf("expected decision SAFE, got %s", result.Decision)
	}
}

func TestInspectFile_DangerousShell(t *testing.T) {
	path := filepath.Join(testdataDir(t), "dangerous_script.sh")
	result, err := InspectFile(path, DefaultMaxBytes)
	if err != nil {
		t.Fatalf("InspectFile returned error: %v", err)
	}
	if !result.Exists {
		t.Fatal("expected file to exist")
	}
	if len(result.Signals) == 0 {
		t.Fatal("expected signals for dangerous shell file, got 0")
	}
	// Should detect rm -rf, aws, kubectl delete, terraform destroy, etc.
	if result.Decision == DecisionSafe {
		t.Error("expected decision other than SAFE for dangerous shell script")
	}
}

func TestInspectFile_SafeJS(t *testing.T) {
	path := filepath.Join(testdataDir(t), "safe_script.js")
	result, err := InspectFile(path, DefaultMaxBytes)
	if err != nil {
		t.Fatalf("InspectFile returned error: %v", err)
	}
	if !result.Exists {
		t.Fatal("expected file to exist")
	}
	if len(result.Signals) != 0 {
		t.Errorf("expected 0 signals, got %d: %+v", len(result.Signals), result.Signals)
	}
	if result.Decision != DecisionSafe {
		t.Errorf("expected decision SAFE, got %s", result.Decision)
	}
}

func TestInspectFile_DangerousJS(t *testing.T) {
	path := filepath.Join(testdataDir(t), "dangerous_script.js")
	result, err := InspectFile(path, DefaultMaxBytes)
	if err != nil {
		t.Fatalf("InspectFile returned error: %v", err)
	}
	if !result.Exists {
		t.Fatal("expected file to exist")
	}
	if len(result.Signals) == 0 {
		t.Fatal("expected signals for dangerous JS file, got 0")
	}
	if result.Decision == DecisionSafe {
		t.Error("expected decision other than SAFE for dangerous JS script")
	}
}

func TestInspectFile_MissingFile(t *testing.T) {
	result, err := InspectFile("/nonexistent/path/to/script.py", DefaultMaxBytes)
	if err != nil {
		t.Fatalf("InspectFile returned error for missing file: %v", err)
	}
	if result.Exists {
		t.Error("expected Exists=false for missing file")
	}
	if result.Decision != DecisionApproval {
		t.Errorf("expected APPROVAL for missing file, got %s", result.Decision)
	}
	if result.Reason != "file not found" {
		t.Errorf("expected reason 'file not found', got %q", result.Reason)
	}
}

func TestInspectFile_NonRegularFileRequiresApproval(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "subdir")
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	result, err := InspectFile(path, DefaultMaxBytes)
	if err != nil {
		t.Fatalf("InspectFile returned error for directory: %v", err)
	}
	if result.Decision != DecisionApproval {
		t.Fatalf("InspectFile decision = %s, want %s", result.Decision, DecisionApproval)
	}
	if result.Reason != "non-regular file requires approval" {
		t.Fatalf("InspectFile reason = %q, want non-regular approval reason", result.Reason)
	}
}

func TestInspectFile_UnsupportedType(t *testing.T) {
	// Create a temporary .rb file.
	tmpDir := t.TempDir()
	rbFile := filepath.Join(tmpDir, "test.rb")
	if err := os.WriteFile(rbFile, []byte("puts 'hello'\n"), 0o644); err != nil {
		t.Fatalf("failed to create temp .rb file: %v", err)
	}

	result, err := InspectFile(rbFile, DefaultMaxBytes)
	if err != nil {
		t.Fatalf("InspectFile returned error: %v", err)
	}
	if !result.Exists {
		t.Fatal("expected file to exist")
	}
	if result.Decision != DecisionCaution {
		t.Errorf("expected CAUTION for unsupported type, got %s", result.Decision)
	}
	if result.Reason != "unsupported file type" {
		t.Errorf("expected reason 'unsupported file type', got %q", result.Reason)
	}
}

func TestInspectFile_Symlink(t *testing.T) {
	// Create a temporary directory with a real file and a symlink to it.
	tmpDir := t.TempDir()
	realFile := filepath.Join(tmpDir, "real.py")
	content := []byte("import math\nprint(math.pi)\n")
	if err := os.WriteFile(realFile, content, 0o644); err != nil {
		t.Fatalf("failed to write real file: %v", err)
	}

	link := filepath.Join(tmpDir, "link.py")
	if err := os.Symlink(realFile, link); err != nil {
		t.Fatalf("failed to create symlink: %v", err)
	}

	result, err := InspectFile(link, DefaultMaxBytes)
	if err != nil {
		t.Fatalf("InspectFile returned error: %v", err)
	}

	// The resolved path should be the real file, not the symlink.
	// Use EvalSymlinks on realFile too, because on macOS /var -> /private/var.
	canonicalReal, err := filepath.EvalSymlinks(realFile)
	if err != nil {
		t.Fatalf("failed to resolve real file path: %v", err)
	}
	if result.Path != canonicalReal {
		t.Errorf("expected resolved path %s, got %s", canonicalReal, result.Path)
	}
	if !result.Exists {
		t.Fatal("expected file to exist")
	}
	if result.Decision != DecisionSafe {
		t.Errorf("expected SAFE for simple math script, got %s", result.Decision)
	}
}

func TestInspectFile_Hash(t *testing.T) {
	// Create a temporary file with known content.
	tmpDir := t.TempDir()
	pyFile := filepath.Join(tmpDir, "hash_test.py")
	content := []byte("print('hello world')\n")
	if err := os.WriteFile(pyFile, content, 0o644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	// Compute expected hash.
	expected := sha256.Sum256(content)
	expectedHex := hex.EncodeToString(expected[:])

	result, err := InspectFile(pyFile, DefaultMaxBytes)
	if err != nil {
		t.Fatalf("InspectFile returned error: %v", err)
	}
	if result.Hash != expectedHex {
		t.Errorf("expected hash %s, got %s", expectedHex, result.Hash)
	}
}

func TestInspectFile_Truncated(t *testing.T) {
	// Create a file larger than a small maxBytes to test truncation.
	tmpDir := t.TempDir()
	pyFile := filepath.Join(tmpDir, "big.py")
	// Create safe content that's 200 bytes.
	content := []byte(strings.Repeat("x = 1\n", 40))
	if err := os.WriteFile(pyFile, content, 0o644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	// Use a very small maxBytes to trigger truncation.
	result, err := InspectFile(pyFile, 50)
	if err != nil {
		t.Fatalf("InspectFile returned error: %v", err)
	}
	if !result.Truncated {
		t.Error("expected Truncated=true")
	}
	// Truncated with no signals should be APPROVAL.
	if result.Decision != DecisionApproval {
		t.Errorf("expected APPROVAL for truncated file with no signals, got %s", result.Decision)
	}
}

func TestInspectFile_TruncatedHashUsesFullContent(t *testing.T) {
	tmpDir := t.TempDir()
	pyFile := filepath.Join(tmpDir, "big_hash.py")
	content := []byte(strings.Repeat("print('hello world')\n", 128))
	if err := os.WriteFile(pyFile, content, 0o644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	expected := sha256.Sum256(content)
	expectedHex := hex.EncodeToString(expected[:])

	result, err := InspectFile(pyFile, 64)
	if err != nil {
		t.Fatalf("InspectFile returned error: %v", err)
	}
	if !result.Truncated {
		t.Fatal("expected truncated result")
	}
	if result.Hash != expectedHex {
		t.Fatalf("expected full-content hash %s, got %s", expectedHex, result.Hash)
	}
}

// --- DetectReferencedFile tests ---

func TestDetectReferencedFile(t *testing.T) {
	tests := []struct {
		name     string
		command  string
		expected string
	}{
		{
			name:     "python with .py file",
			command:  "python script.py",
			expected: "script.py",
		},
		{
			name:     "python3 with .py file",
			command:  "python3 /path/to/script.py",
			expected: "/path/to/script.py",
		},
		{
			name:     "python with flags before file",
			command:  "python -u script.py",
			expected: "script.py",
		},
		{
			name:     "node with .js file",
			command:  "node app.js",
			expected: "app.js",
		},
		{
			name:     "node with .ts file",
			command:  "node server.ts",
			expected: "server.ts",
		},
		{
			name:     "bash with .sh file",
			command:  "bash deploy.sh",
			expected: "deploy.sh",
		},
		{
			name:     "sh with .sh file",
			command:  "sh /opt/scripts/run.sh",
			expected: "/opt/scripts/run.sh",
		},
		{
			name:     "ruby with .rb file",
			command:  "ruby app.rb",
			expected: "app.rb",
		},
		{
			name:     "perl with .pl file",
			command:  "perl script.pl",
			expected: "script.pl",
		},
		{
			name:     "unknown invoker",
			command:  "go run main.go",
			expected: "",
		},
		{
			name:     "empty command",
			command:  "",
			expected: "",
		},
		{
			name:     "no file argument",
			command:  "python",
			expected: "",
		},
		{
			name:     "positional arg without matching extension",
			command:  "python some_module",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DetectReferencedFile(tt.command)
			if got != tt.expected {
				t.Errorf("DetectReferencedFile(%q) = %q, want %q", tt.command, got, tt.expected)
			}
		})
	}
}

func TestDetectReferencedFile_DirectExecutablePath(t *testing.T) {
	tmpDir := t.TempDir()
	scriptPath := filepath.Join(tmpDir, "run.sh")
	if err := os.WriteFile(scriptPath, []byte("#!/bin/sh\necho hi\n"), 0o755); err != nil {
		t.Fatalf("failed to create script: %v", err)
	}

	got := DetectReferencedFile(scriptPath)
	if got != scriptPath {
		t.Fatalf("DetectReferencedFile(%q) = %q, want %q", scriptPath, got, scriptPath)
	}
}

func TestInspectFile_UnknownExtensionReturnsCaution(t *testing.T) {
	tmpDir := t.TempDir()
	luaFile := filepath.Join(tmpDir, "script.lua")
	if err := os.WriteFile(luaFile, []byte("print('hello')\n"), 0o644); err != nil {
		t.Fatalf("failed to create temp .lua file: %v", err)
	}

	result, err := InspectFile(luaFile, DefaultMaxBytes)
	if err != nil {
		t.Fatalf("InspectFile returned error: %v", err)
	}
	if !result.Exists {
		t.Fatal("expected file to exist")
	}
	if result.Decision != DecisionCaution {
		t.Errorf("expected CAUTION for unknown extension, got %s", result.Decision)
	}
	if result.Reason != "unknown file type, no scanner available" {
		t.Errorf("expected reason 'unknown file type, no scanner available', got %q", result.Reason)
	}
}

func TestInferDecisionFromSignals_CloudSDKAloneIsCaution(t *testing.T) {
	signals := []inspect.Signal{
		{Category: "cloud_sdk", Pattern: "boto3", Line: 1, Match: "import boto3"},
	}
	got := inferDecisionFromSignals(signals)
	if got != DecisionCaution {
		t.Errorf("expected CAUTION for cloud_sdk alone, got %s", got)
	}
}

func TestInferDecisionFromSignals_CloudSDKPlusDestructiveIsApproval(t *testing.T) {
	signals := []inspect.Signal{
		{Category: "cloud_sdk", Pattern: "boto3", Line: 1, Match: "import boto3"},
		{Category: "destructive_fs", Pattern: "rm -rf", Line: 2, Match: "rm -rf /tmp"},
	}
	got := inferDecisionFromSignals(signals)
	if got != DecisionCaution {
		t.Errorf("expected CAUTION for cloud_sdk + destructive_fs, got %s", got)
	}
}

func TestDetectReferencedFile_ScriptlessMode(t *testing.T) {
	tests := []struct {
		name    string
		command string
	}{
		{
			name:    "python -c",
			command: "python -c 'print(1)'",
		},
		{
			name:    "python -m",
			command: "python -m http.server 8080",
		},
		{
			name:    "python3 -c",
			command: "python3 -c 'import os; os.system(\"ls\")'",
		},
		{
			name:    "node -e",
			command: "node -e 'console.log(1)'",
		},
		{
			name:    "node --eval",
			command: "node --eval 'process.exit(0)'",
		},
		{
			name:    "node -p",
			command: "node -p '1 + 2'",
		},
		{
			name:    "node --print",
			command: "node --print 'Math.PI'",
		},
		{
			name:    "bash -c",
			command: "bash -c 'echo hello'",
		},
		{
			name:    "sh -c",
			command: "sh -c 'ls -la'",
		},
		{
			name:    "perl -e",
			command: "perl -e 'print 42'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DetectReferencedFile(tt.command)
			if got != "" {
				t.Errorf("DetectReferencedFile(%q) = %q, want empty (scriptless mode)", tt.command, got)
			}
		})
	}
}
