//go:build !windows

package fuse_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestSetupWorktreeLinksSharedDirectories(t *testing.T) {
	scriptPath, err := filepath.Abs("scripts/setup-worktree.sh")
	if err != nil {
		t.Fatalf("abs script path: %v", err)
	}

	repoRoot := t.TempDir()
	runWorktreeCmd(t, repoRoot, "git", "init")
	runWorktreeCmd(t, repoRoot, "git", "config", "user.name", "Test User")
	runWorktreeCmd(t, repoRoot, "git", "config", "user.email", "test@example.com")

	writeWorktreeFile(t, filepath.Join(repoRoot, ".gitignore"), ".agents/\n.worktrees/\n")
	writeWorktreeFile(t, filepath.Join(repoRoot, "README.md"), "fixture\n")
	writeWorktreeFile(t, filepath.Join(repoRoot, ".tickets", "shared.md"), "ticket\n")
	if err := os.MkdirAll(filepath.Join(repoRoot, ".agents"), 0o755); err != nil {
		t.Fatalf("mkdir .agents: %v", err)
	}

	runWorktreeCmd(t, repoRoot, "git", "add", ".")
	runWorktreeCmd(t, repoRoot, "git", "commit", "-m", "initial")

	worktreeRoot := filepath.Join(repoRoot, ".worktrees", "feature")
	runWorktreeCmd(t, repoRoot, "git", "worktree", "add", worktreeRoot, "-b", "feature")

	beforeTickets := mustLstat(t, filepath.Join(worktreeRoot, ".tickets"))
	if beforeTickets.Mode()&os.ModeSymlink != 0 {
		t.Fatal("expected .tickets to start as a real directory")
	}
	if _, err := os.Lstat(filepath.Join(worktreeRoot, ".agents")); !os.IsNotExist(err) {
		t.Fatalf("expected .agents to be absent before setup, got err=%v", err)
	}

	runWorktreeCmd(t, worktreeRoot, "bash", scriptPath)

	assertSymlinkTarget(t, filepath.Join(worktreeRoot, ".agents"), filepath.Join(repoRoot, ".agents"))
	assertSymlinkTarget(t, filepath.Join(worktreeRoot, ".tickets"), filepath.Join(repoRoot, ".tickets"))

	status := strings.TrimSpace(runWorktreeCmd(t, worktreeRoot, "git", "status", "--short"))
	if status != "" {
		t.Fatalf("expected clean status after setup, got:\n%s", status)
	}
}

func runWorktreeCmd(t *testing.T, cwd, name string, args ...string) string {
	t.Helper()

	cmd := exec.Command(name, args...)
	cmd.Dir = cwd
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %v failed: %v\n%s", name, args, err, string(out))
	}

	return string(out)
}

func writeWorktreeFile(t *testing.T, path, contents string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func mustLstat(t *testing.T, path string) os.FileInfo {
	t.Helper()

	info, err := os.Lstat(path)
	if err != nil {
		t.Fatalf("lstat %s: %v", path, err)
	}

	return info
}

func assertSymlinkTarget(t *testing.T, path, want string) {
	t.Helper()

	info := mustLstat(t, path)
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("expected %s to be a symlink", path)
	}

	got, err := os.Readlink(path)
	if err != nil {
		t.Fatalf("readlink %s: %v", path, err)
	}

	gotEval, err := filepath.EvalSymlinks(got)
	if err != nil {
		t.Fatalf("eval symlinks for %s: %v", got, err)
	}
	wantEval, err := filepath.EvalSymlinks(want)
	if err != nil {
		t.Fatalf("eval symlinks for %s: %v", want, err)
	}
	if gotEval != wantEval {
		t.Fatalf("expected %s -> %s, got %s", path, wantEval, gotEval)
	}
}
