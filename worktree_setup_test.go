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
	scriptPath := worktreeScriptPath(t)
	repoRoot, worktreeRoot := newWorktreeFixture(t)

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

func TestSetupWorktreeRefusesIgnoredTicketsFiles(t *testing.T) {
	scriptPath := worktreeScriptPath(t)
	_, worktreeRoot := newWorktreeFixture(t)

	writeWorktreeFile(t, filepath.Join(worktreeRoot, ".tickets", "local", "scratch.md"), "ignored\n")

	output, err := runWorktreeCmdErr(worktreeRoot, "bash", scriptPath)
	if err == nil {
		t.Fatal("expected setup-worktree to fail when ignored files exist in .tickets")
	}
	if !strings.Contains(output, "refusing to replace") {
		t.Fatalf("expected refusal output, got:\n%s", output)
	}
	if !strings.Contains(output, "!! .tickets/local/") && !strings.Contains(output, "!! .tickets/local/scratch.md") {
		t.Fatalf("expected ignored-file status in output, got:\n%s", output)
	}

	info := mustLstat(t, filepath.Join(worktreeRoot, ".tickets"))
	if info.Mode()&os.ModeSymlink != 0 {
		t.Fatal("expected .tickets to remain a real directory after refusal")
	}
}

func TestAssertSymlinkTargetResolvesRelativeLinks(t *testing.T) {
	root := t.TempDir()
	targetDir := filepath.Join(root, "shared")
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		t.Fatalf("mkdir shared dir: %v", err)
	}
	linkPath := filepath.Join(root, "worktree", ".tickets")
	if err := os.MkdirAll(filepath.Dir(linkPath), 0o755); err != nil {
		t.Fatalf("mkdir link parent dir: %v", err)
	}
	if err := os.Symlink(filepath.Join("..", "shared"), linkPath); err != nil {
		t.Fatalf("create relative symlink: %v", err)
	}

	assertSymlinkTarget(t, linkPath, targetDir)
}

func runWorktreeCmd(t *testing.T, cwd, name string, args ...string) string {
	t.Helper()

	out, err := runWorktreeCmdErr(cwd, name, args...)
	if err != nil {
		t.Fatalf("%s %v failed: %v\n%s", name, args, err, out)
	}

	return out
}

func runWorktreeCmdErr(cwd, name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = cwd
	cmd.Env = filteredGitEnv()
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func filteredGitEnv() []string {
	env := os.Environ()
	filtered := env[:0]
	for _, item := range env {
		if strings.HasPrefix(item, "GIT_") {
			switch {
			case strings.HasPrefix(item, "GIT_CONFIG_"),
				strings.HasPrefix(item, "GIT_EXEC_PATH="),
				strings.HasPrefix(item, "GIT_SSH="),
				strings.HasPrefix(item, "GIT_SSH_COMMAND="),
				strings.HasPrefix(item, "GIT_TRACE="),
				strings.HasPrefix(item, "GIT_TRACE2="),
				strings.HasPrefix(item, "GIT_TRACE_PACKET="),
				strings.HasPrefix(item, "GIT_TRACE_PERFORMANCE="),
				strings.HasPrefix(item, "GIT_TRACE_SETUP="):
				filtered = append(filtered, item)
			}
			continue
		}
		filtered = append(filtered, item)
	}
	return filtered
}

func worktreeScriptPath(t *testing.T) string {
	t.Helper()

	scriptPath, err := filepath.Abs("scripts/setup-worktree.sh")
	if err != nil {
		t.Fatalf("abs script path: %v", err)
	}

	return scriptPath
}

func newWorktreeFixture(t *testing.T) (string, string) {
	t.Helper()

	repoRoot := t.TempDir()
	runWorktreeCmd(t, repoRoot, "git", "init")
	disabledHooksDir := filepath.Join(repoRoot, ".git-hooks-disabled")
	if err := os.MkdirAll(disabledHooksDir, 0o755); err != nil {
		t.Fatalf("mkdir disabled hooks dir: %v", err)
	}
	runWorktreeCmd(t, repoRoot, "git", "config", "core.hooksPath", disabledHooksDir)
	runWorktreeCmd(t, repoRoot, "git", "config", "user.name", "Test User")
	runWorktreeCmd(t, repoRoot, "git", "config", "user.email", "test@example.com")

	writeWorktreeFile(t, filepath.Join(repoRoot, ".gitignore"), ".agents/\n.worktrees/\n.tickets/local/\n")
	writeWorktreeFile(t, filepath.Join(repoRoot, "README.md"), "fixture\n")
	writeWorktreeFile(t, filepath.Join(repoRoot, ".tickets", "shared.md"), "ticket\n")
	if err := os.MkdirAll(filepath.Join(repoRoot, ".agents"), 0o755); err != nil {
		t.Fatalf("mkdir .agents: %v", err)
	}

	runWorktreeCmd(t, repoRoot, "git", "add", ".")
	runWorktreeCmd(t, repoRoot, "git", "commit", "-m", "initial")

	worktreeRoot := filepath.Join(repoRoot, ".worktrees", "feature")
	runWorktreeCmd(t, repoRoot, "git", "worktree", "add", worktreeRoot, "-b", "feature")

	return repoRoot, worktreeRoot
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
	if !filepath.IsAbs(got) {
		got = filepath.Join(filepath.Dir(path), got)
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
