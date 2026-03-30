//go:build !windows

package fuse_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCrossBuild_WindowsCompiles(t *testing.T) {
	// Filter existing GOOS/GOARCH to avoid duplicates.
	var env []string
	for _, e := range os.Environ() {
		if !strings.HasPrefix(e, "GOOS=") && !strings.HasPrefix(e, "GOARCH=") {
			env = append(env, e)
		}
	}
	env = append(env, "GOOS=windows", "GOARCH=amd64")

	// Build the full binary to a temp dir to avoid dropping fuse.exe in the repo root.
	outPath := filepath.Join(t.TempDir(), "fuse.exe")
	cmd := exec.Command("go", "build", "-o", outPath, "./cmd/fuse")
	cmd.Env = env
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Windows cross-compilation failed:\n%s", out)
	}

	// Also verify go vet passes for Windows.
	vetCmd := exec.Command("go", "vet", "./...")
	vetCmd.Env = env
	if out, err := vetCmd.CombinedOutput(); err != nil {
		t.Fatalf("Windows go vet failed:\n%s", out)
	}
}
