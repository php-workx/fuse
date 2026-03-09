package cli

import (
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
