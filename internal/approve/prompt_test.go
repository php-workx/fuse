package approve

import (
	"os"
	"strings"
	"testing"
)

// Comprehensive sanitization tests are in internal/sanitize/sanitize_test.go.
// This test verifies the delegation wrapper works.

func TestSanitizePrompt_DelegatesToSharedPackage(t *testing.T) {
	input := "hello \x1b[31mred\x1b[0m world"
	got := sanitizePrompt(input)
	if got != "hello red world" {
		t.Errorf("sanitizePrompt did not strip CSI: got %q", got)
	}

	// Also verify C1 stripping works through delegation.
	input2 := "a\xc2\x9bz"
	got2 := sanitizePrompt(input2)
	if got2 != "az" {
		t.Errorf("sanitizePrompt did not strip C1: got %q", got2)
	}
}

func TestSanitizePrompt_StripsNewlines(t *testing.T) {
	// Newlines and carriage returns must be replaced with spaces
	// to prevent prompt layout injection.
	input := "line1\nline2\rline3"
	got := sanitizePrompt(input)
	if got != "line1 line2 line3" {
		t.Errorf("newlines not replaced: got %q", got)
	}
}

func TestGetContextVars_Empty(t *testing.T) {
	// With no relevant env vars set, should return empty string.
	// Save and clear any that might be set.
	vars := []string{
		"AWS_PROFILE", "AWS_REGION", "AWS_DEFAULT_REGION",
		"TF_WORKSPACE", "TF_VAR_environment",
		"KUBECONFIG", "KUBECONTEXT",
		"GCP_PROJECT", "GOOGLE_CLOUD_PROJECT",
		"AZURE_SUBSCRIPTION",
	}
	saved := make(map[string]string)
	for _, v := range vars {
		if val, ok := os.LookupEnv(v); ok {
			saved[v] = val
			t.Setenv(v, "")
			os.Unsetenv(v)
		}
	}
	t.Cleanup(func() {
		for k, v := range saved {
			os.Setenv(k, v)
		}
	})

	got := getContextVars()
	if got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

func TestGetContextVars_SingleVar(t *testing.T) {
	t.Setenv("AWS_PROFILE", "prod")
	got := getContextVars()
	if got != "AWS_PROFILE=prod" {
		t.Errorf("expected AWS_PROFILE=prod, got %q", got)
	}
}

func TestGetContextVars_MultipleVars(t *testing.T) {
	t.Setenv("AWS_PROFILE", "staging")
	t.Setenv("KUBECONFIG", "/home/user/.kube/config")
	got := getContextVars()
	// Both should appear, comma-separated.
	if !strings.Contains(got, "AWS_PROFILE=staging") {
		t.Errorf("missing AWS_PROFILE in %q", got)
	}
	if !strings.Contains(got, "KUBECONFIG=/home/user/.kube/config") {
		t.Errorf("missing KUBECONFIG in %q", got)
	}
}
