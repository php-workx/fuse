package inspect

import (
	"testing"
)

func TestScanJavaScript_SafeScript(t *testing.T) {
	content := readTestFile(t, "safe_script.js")
	signals := ScanJavaScript(content)

	if len(signals) != 0 {
		t.Errorf("expected 0 signals for safe_script.js, got %d:", len(signals))
		for _, s := range signals {
			t.Logf("  line %d: category=%s match=%q", s.Line, s.Category, s.Match)
		}
	}
}

func TestScanJavaScript_DangerousScript(t *testing.T) {
	content := readTestFile(t, "dangerous_script.js")
	signals := ScanJavaScript(content)

	if len(signals) == 0 {
		t.Fatal("expected signals for dangerous_script.js, got 0")
	}

	categories := signalCategories(signals)

	expectedCategories := []string{"subprocess", "destructive_fs", "cloud_sdk"}
	for _, cat := range expectedCategories {
		if !categories[cat] {
			t.Errorf("expected category %q in signals, not found", cat)
		}
	}

	t.Logf("found %d signals:", len(signals))
	for _, s := range signals {
		t.Logf("  line %d: category=%s match=%q", s.Line, s.Category, s.Match)
	}
}

func TestScanJavaScript_CommentSkipping(t *testing.T) {
	content := []byte(`// const { exec } = require('child_process');
// exec('rm -rf /');
// eval("malicious");
console.log("safe");
`)
	signals := ScanJavaScript(content)

	if len(signals) != 0 {
		t.Errorf("expected 0 signals for commented-out code, got %d:", len(signals))
		for _, s := range signals {
			t.Logf("  line %d: category=%s match=%q", s.Line, s.Category, s.Match)
		}
	}
}

func TestScanJavaScript_BlockCommentSkipping(t *testing.T) {
	content := []byte(`/*
const { exec } = require('child_process');
exec('rm -rf /');
eval("malicious");
*/
console.log("safe");
`)
	signals := ScanJavaScript(content)

	if len(signals) != 0 {
		t.Errorf("expected 0 signals for block-commented code, got %d:", len(signals))
		for _, s := range signals {
			t.Logf("  line %d: category=%s match=%q", s.Line, s.Category, s.Match)
		}
	}
}

func TestScanJavaScript_InlineBlockComment(t *testing.T) {
	content := []byte(`const x = /* exec('ls') */ "safe";
console.log(x);
`)
	signals := ScanJavaScript(content)

	if len(signals) != 0 {
		t.Errorf("expected 0 signals for inline block comment, got %d:", len(signals))
		for _, s := range signals {
			t.Logf("  line %d: category=%s match=%q", s.Line, s.Category, s.Match)
		}
	}
}

func TestScanJavaScript_SpecificPatterns(t *testing.T) {
	tests := []struct {
		name     string
		code     string
		category string
	}{
		{"require child_process", `const cp = require('child_process');`, "subprocess"},
		{"import child_process", `import { exec } from 'child_process';`, "subprocess"},
		{"exec call", `exec('ls -la');`, "subprocess"},
		{"execSync call", `execSync('ls -la');`, "subprocess"},
		{"spawn call", `spawn('python3', ['-c', 'print("hi")']);`, "subprocess"},
		{"spawnSync call", `spawnSync('node', ['script.js']);`, "subprocess"},
		{"fork call", `fork('./worker.js');`, "subprocess"},
		{"fs.rmSync", `fs.rmSync('/tmp/data', { recursive: true });`, "destructive_fs"},
		{"fs.unlinkSync", `fs.unlinkSync('/tmp/file.txt');`, "destructive_fs"},
		{"fs.rmdirSync", `fs.rmdirSync('/tmp/dir');`, "destructive_fs"},
		{"fs.rm", `fs.rm('/tmp/data', { recursive: true }, cb);`, "destructive_fs"},
		{"fs.promises.rm", `await fs.promises.rm('/tmp/data');`, "destructive_fs"},
		{"fs.promises.unlink", `await fs.promises.unlink('/tmp/file');`, "destructive_fs"},
		{"fs.promises.rmdir", `await fs.promises.rmdir('/tmp/dir');`, "destructive_fs"},
		{"require @aws-sdk", `const { S3 } = require('@aws-sdk/client-s3');`, "cloud_sdk"},
		{"import @aws-sdk", `import { S3Client } from '@aws-sdk/client-s3';`, "cloud_sdk"},
		{"require @google-cloud", `const storage = require('@google-cloud/storage');`, "cloud_sdk"},
		{"import @google-cloud", `import { Storage } from '@google-cloud/storage';`, "cloud_sdk"},
		{"require @azure", `const { BlobClient } = require('@azure/storage-blob');`, "cloud_sdk"},
		{"DeleteCommand", `const cmd = new DeleteCommand({ TableName: 't' });`, "cloud_sdk"},
		{"TerminateCommand", `const cmd = new TerminateCommand({ InstanceIds: [] });`, "cloud_sdk"},
		{"DestroyCommand", `const cmd = new DestroyCommand({ StackName: 's' });`, "cloud_sdk"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			signals := ScanJavaScript([]byte(tt.code))
			found := false
			for _, s := range signals {
				if s.Category == tt.category {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected category %q for code %q, got signals: %v", tt.category, tt.code, signals)
			}
		})
	}
}
