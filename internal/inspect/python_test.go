package inspect

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// testdataDir returns the absolute path to the testdata/scripts directory.
func testdataDir(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not determine test file path")
	}
	return filepath.Join(filepath.Dir(filename), "..", "..", "testdata", "scripts")
}

func readTestFile(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(testdataDir(t), name))
	if err != nil {
		t.Fatalf("failed to read test file %s: %v", name, err)
	}
	return data
}

func TestScanPython_SafeScript(t *testing.T) {
	content := readTestFile(t, "safe_script.py")
	signals := ScanPython(content)

	if len(signals) != 0 {
		t.Errorf("expected 0 signals for safe_script.py, got %d:", len(signals))
		for _, s := range signals {
			t.Logf("  line %d: category=%s match=%q", s.Line, s.Category, s.Match)
		}
	}
}

func TestScanPython_DangerousBoto3(t *testing.T) {
	content := readTestFile(t, "dangerous_boto3.py")
	signals := ScanPython(content)

	if len(signals) == 0 {
		t.Fatal("expected signals for dangerous_boto3.py, got 0")
	}

	// Check for expected signal categories.
	categories := signalCategories(signals)

	expectedCategories := []string{"cloud_sdk", "subprocess"}
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

func TestScanPython_SubprocessDanger(t *testing.T) {
	content := readTestFile(t, "subprocess_danger.py")
	signals := ScanPython(content)

	if len(signals) == 0 {
		t.Fatal("expected signals for subprocess_danger.py, got 0")
	}

	categories := signalCategories(signals)

	expectedCategories := []string{"subprocess", "dynamic_exec", "dynamic_import", "destructive_fs"}
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

func TestScanPython_CommentSkipping(t *testing.T) {
	content := []byte(`# import subprocess
# os.system("rm -rf /")
# eval("malicious code")
print("hello")
`)
	signals := ScanPython(content)

	if len(signals) != 0 {
		t.Errorf("expected 0 signals for commented-out code, got %d:", len(signals))
		for _, s := range signals {
			t.Logf("  line %d: category=%s match=%q", s.Line, s.Category, s.Match)
		}
	}
}

func TestScanPython_SpecificPatterns(t *testing.T) {
	tests := []struct {
		name     string
		code     string
		category string
	}{
		{"import boto3", "import boto3", "cloud_sdk"},
		{"from botocore", "from botocore import session", "cloud_sdk"},
		{"import google.cloud", "from google.cloud import storage", "cloud_sdk"},
		{"import azure", "from azure.storage import blob", "cloud_sdk"},
		{"import oci", "import oci", "cloud_sdk"},
		{"subprocess.run", `subprocess.run(["ls"])`, "subprocess"},
		{"subprocess.Popen", `subprocess.Popen(["ls"])`, "subprocess"},
		{"subprocess.check_output", `subprocess.check_output(["ls"])`, "subprocess"},
		{"os.system", `os.system("echo hi")`, "subprocess"},
		{"os.remove", `os.remove("/tmp/file")`, "destructive_fs"},
		{"os.unlink", `os.unlink("/tmp/file")`, "destructive_fs"},
		{"os.rmdir", `os.rmdir("/tmp/dir")`, "destructive_fs"},
		{"shutil.rmtree", `shutil.rmtree("/tmp/dir")`, "destructive_fs"},
		{"shutil.move", `shutil.move("/tmp/a", "/tmp/b")`, "destructive_fs"},
		{"delete_bucket", `client.delete_bucket(Bucket='x')`, "cloud_sdk"},
		{"terminate_instances", `ec2.terminate_instances(InstanceIds=['i-123'])`, "cloud_sdk"},
		{"delete_table", `dynamodb.delete_table(TableName='t')`, "cloud_sdk"},
		{"delete_function", `lambda_client.delete_function(FunctionName='f')`, "cloud_sdk"},
		{"delete_cluster", `ecs.delete_cluster(cluster='c')`, "cloud_sdk"},
		{"delete_service", `ecs.delete_service(service='s')`, "cloud_sdk"},
		{"delete_secret", `sm.delete_secret(SecretId='x')`, "cloud_sdk"},
		{"requests.delete iam", `requests.delete("https://iam.example.com/role")`, "http_control_plane"},
		{"requests.post s3", `requests.post("https://s3.amazonaws.com/bucket")`, "http_control_plane"},
		{"eval", `result = eval("1+1")`, "dynamic_exec"},
		{"exec", `exec("print('hi')")`, "dynamic_exec"},
		{"__import__", `mod = __import__('os')`, "dynamic_import"},
		{"importlib.import_module", `importlib.import_module('os')`, "dynamic_import"},
		{"compile with exec", `code = compile("x=1", "<string>", "exec")`, "dynamic_exec"},

		// Network import aliases: guard alias/import coverage gap where
		// "from urllib.request import urlopen" or "import httpx" surfaces
		// only a generic inline CAUTION instead of a network-scoped signal.
		{"import urllib", `import urllib.request`, "network_io"},
		{"from urllib import", `from urllib.request import urlopen`, "network_io"},
		{"import http.client", `import http.client`, "network_io"},
		{"import socket", `import socket`, "network_io"},
		{"import requests", `import requests`, "network_io"},
		{"import httpx", `import httpx`, "network_io"},

		// Network call primitives.
		{"urlopen call", `urlopen("https://example.com/")`, "network_io"},
		{"urllib.request.urlopen", `urllib.request.urlopen("https://example.com/")`, "network_io"},
		{"requests.get", `requests.get("https://example.com/")`, "network_io"},
		{"httpx.post", `httpx.post("https://example.com/", json={})`, "network_io"},
		{"httpx.AsyncClient", `async with httpx.AsyncClient() as client:`, "network_io"},
		{"socket.socket", `s = socket.socket(socket.AF_INET, socket.SOCK_STREAM)`, "network_io"},
		{"http.client.HTTPSConnection", `conn = http.client.HTTPSConnection("example.com")`, "network_io"},

		// Write-mode opens and pathlib.Path write/delete — previously only
		// surfaced the generic inline marker when aliased through pathlib.
		{"open write mode", `open("out.txt", "w")`, "destructive_fs"},
		{"open append mode", `open("out.txt", "a")`, "destructive_fs"},
		{"open exclusive mode", `open("out.txt", "x")`, "destructive_fs"},
		{"Path write_text", `Path("out.txt").write_text("data")`, "destructive_fs"},
		{"Path write_bytes", `Path("out.bin").write_bytes(b"data")`, "destructive_fs"},
		{"Path mkdir", `Path("/tmp/new").mkdir(parents=True)`, "destructive_fs"},
		{"Path unlink", `Path("/tmp/file").unlink()`, "destructive_fs"},
		{"Path rename", `Path("/tmp/a").rename("/tmp/b")`, "destructive_fs"},
		{"Path replace", `Path("/tmp/a").replace("/tmp/b")`, "destructive_fs"},
		{"Path.open write", `Path("out.txt").open("w").write("data")`, "destructive_fs"},
		{"pathlib.Path write_text", `pathlib.Path("out.txt").write_text("data")`, "destructive_fs"},

		// Additional os destructive ops.
		{"os.makedirs", `os.makedirs("/tmp/new")`, "destructive_fs"},
		{"os.rename", `os.rename("/tmp/a", "/tmp/b")`, "destructive_fs"},
		{"shutil.copytree", `shutil.copytree("/tmp/a", "/tmp/b")`, "destructive_fs"},
		{"shutil.copyfile", `shutil.copyfile("/tmp/a", "/tmp/b")`, "destructive_fs"},

		// Secret-like reads: alias through pathlib and bare open both fire.
		{"open .env", `open(".env").read()`, "secret_read"},
		{"Path id_rsa", `Path("~/.ssh/id_rsa").read_text()`, "secret_read"},
		{"pathlib.Path secret", `pathlib.Path("/etc/credentials").read_text()`, "secret_read"},
		{"open token file", `open("token.txt", "r")`, "secret_read"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			signals := ScanPython([]byte(tt.code))
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

// TestScanPython_AliasImportRetained verifies that "from X import <dangerous-fn>"
// style imports are NOT filtered by scopeImportSignals even when no prefixed call
// (e.g. os.system, shutil.rmtree) appears in the body.  The programmer
// explicitly aliased in the dangerous symbol, so the signal must survive.
//
// It also verifies the complementary case: bare "import os" / "import shutil"
// without any dangerous call continue to be filtered (avoiding false positives
// for code that uses os.path or shutil.which).
func TestScanPython_AliasImportRetained(t *testing.T) {
	tests := []struct {
		name     string
		code     string
		wantCat  string // expected category in surviving signals
		wantKeep bool   // true → expect a signal; false → expect no signals
	}{
		// Alias imports that introduce dangerous os functions must be retained.
		{
			name:     "from os import system with bare call",
			code:     "from os import system\nsystem('rm -rf /tmp/demo')\n",
			wantCat:  "subprocess",
			wantKeep: true,
		},
		{
			name:     "from os import remove with bare call",
			code:     "from os import remove\nremove('/tmp/file')\n",
			wantCat:  "subprocess",
			wantKeep: true,
		},
		{
			name:     "from os import unlink with bare call",
			code:     "from os import unlink\nunlink('/tmp/file')\n",
			wantCat:  "subprocess",
			wantKeep: true,
		},
		// Alias imports that introduce dangerous shutil functions must be retained.
		{
			name:     "from shutil import rmtree with bare call",
			code:     "from shutil import rmtree\nrmtree('/tmp/dir')\n",
			wantCat:  "destructive_fs",
			wantKeep: true,
		},
		{
			name:     "from shutil import move with bare call",
			code:     "from shutil import move\nmove('/tmp/a', '/tmp/b')\n",
			wantCat:  "destructive_fs",
			wantKeep: true,
		},
		// Bare module imports without any dangerous call should still be filtered.
		{
			name:     "import os alone is filtered",
			code:     "import os\nprint(os.getcwd())\n",
			wantCat:  "subprocess",
			wantKeep: false,
		},
		{
			name:     "import shutil alone is filtered",
			code:     "import shutil\nprint(shutil.which('git'))\n",
			wantCat:  "destructive_fs",
			wantKeep: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			signals := ScanPython([]byte(tt.code))
			found := false
			for _, s := range signals {
				if s.Category == tt.wantCat {
					found = true
					break
				}
			}
			if tt.wantKeep && !found {
				t.Errorf("expected category %q to survive scopeImportSignals for code:\n%s\ngot signals: %v",
					tt.wantCat, tt.code, signals)
			}
			if !tt.wantKeep && found {
				t.Errorf("expected category %q to be filtered by scopeImportSignals for code:\n%s\ngot signals: %v",
					tt.wantCat, tt.code, signals)
			}
		})
	}
}

// signalCategories returns a set of category strings found in the signals.
func signalCategories(signals []Signal) map[string]bool {
	cats := make(map[string]bool)
	for _, s := range signals {
		cats[s.Category] = true
	}
	return cats
}
