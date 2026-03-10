package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMergeCodexConfig(t *testing.T) {
	got := mergeCodexConfig("")
	for _, want := range []string{
		"[features]",
		"shell_tool = false",
		`[mcp_servers.fuse-shell]`,
		`command = "fuse"`,
		`args = ["proxy", "codex-shell"]`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("merged config missing %q:\n%s", want, got)
		}
	}
}

func TestRemoveCodexIntegration(t *testing.T) {
	input := `[features]
shell_tool = false

[mcp_servers.fuse-shell]
command = "fuse"
args = ["proxy", "codex-shell"]

[other]
value = "keep"
`
	got := removeCodexIntegration(input)
	if strings.Contains(got, "fuse-shell") {
		t.Fatalf("expected fuse-shell section removed:\n%s", got)
	}
	if strings.Contains(got, "shell_tool = false") {
		t.Fatalf("expected shell_tool override removed:\n%s", got)
	}
	if !strings.Contains(got, "[other]") {
		t.Fatalf("expected unrelated config preserved:\n%s", got)
	}
}

func TestCodexConfigPath_PrefersLocalRepoConfig(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("CODEX_HOME", "")

	localConfigDir := filepath.Join(tmpDir, ".codex")
	if err := os.MkdirAll(localConfigDir, 0755); err != nil {
		t.Fatalf("mkdir .codex: %v", err)
	}
	localConfigPath := filepath.Join(localConfigDir, "config.toml")
	if err := os.WriteFile(localConfigPath, []byte("[mcp_servers]\n"), 0644); err != nil {
		t.Fatalf("write local config: %v", err)
	}

	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer func() {
		if err := os.Chdir(origWd); err != nil {
			t.Fatalf("restore wd: %v", err)
		}
	}()

	got := codexConfigPath()
	gotEval, err := filepath.EvalSymlinks(got)
	if err != nil {
		t.Fatalf("EvalSymlinks(got): %v", err)
	}
	wantEval, err := filepath.EvalSymlinks(localConfigPath)
	if err != nil {
		t.Fatalf("EvalSymlinks(want): %v", err)
	}
	if gotEval != wantEval {
		t.Fatalf("codexConfigPath() = %q (%q), want %q (%q)", got, gotEval, localConfigPath, wantEval)
	}
}
