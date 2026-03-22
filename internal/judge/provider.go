// Package judge provides an optional LLM-based second opinion on command
// safety classifications. It queries locally-installed CLI tools (claude, codex)
// to evaluate whether a CAUTION or APPROVAL classification is correct.
package judge

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// Provider sends a prompt to an LLM and returns the response.
type Provider interface {
	// Query sends system and user prompts via stdin and returns the response text.
	Query(ctx context.Context, systemPrompt, userPrompt string) (string, error)
	// Name returns the provider name for logging (e.g., "claude", "codex").
	Name() string
}

// claudeProvider invokes the Claude Code CLI in print mode.
type claudeProvider struct {
	model string
}

func (p *claudeProvider) Name() string { return "claude" }

func (p *claudeProvider) Query(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	args := []string{
		"-p",     // print mode: read stdin, print response, exit
		"--bare", // skip hooks, LSP, CLAUDE.md (faster startup)
		"--system-prompt", systemPrompt,
		"--output-format", "text",
	}
	if p.model != "" {
		args = append(args, "--model", p.model)
	}
	cmd := exec.CommandContext(ctx, "claude", args...)
	cmd.Stdin = strings.NewReader(userPrompt)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("claude CLI: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// codexProvider invokes the Codex CLI in exec mode.
type codexProvider struct {
	model string
}

func (p *codexProvider) Name() string { return "codex" }

func (p *codexProvider) Query(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	args := []string{
		"exec",
		"-",           // read prompt from stdin
		"--ephemeral", // don't persist session
	}
	if p.model != "" {
		args = append(args, "-m", p.model)
	}
	cmd := exec.CommandContext(ctx, "codex", args...)
	// Codex has no --system-prompt flag; prepend to user prompt.
	cmd.Stdin = strings.NewReader(systemPrompt + "\n\n" + userPrompt)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("codex CLI: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// newProvider creates a provider by name.
func newProvider(name, model string) (Provider, error) {
	switch name {
	case "claude":
		return &claudeProvider{model: model}, nil
	case "codex":
		return &codexProvider{model: model}, nil
	default:
		return nil, fmt.Errorf("unknown provider: %q (supported: claude, codex)", name)
	}
}

// DetectProvider finds an available LLM CLI provider.
// If preferred is set and not "auto", uses that provider directly.
// Otherwise tries claude → codex in order.
func DetectProvider(preferred, model string) (Provider, error) {
	if preferred != "" && preferred != "auto" {
		if _, err := exec.LookPath(preferred); err != nil {
			return nil, fmt.Errorf("provider %q not found on PATH", preferred)
		}
		return newProvider(preferred, model)
	}
	for _, name := range []string{"claude", "codex"} {
		if _, err := exec.LookPath(name); err == nil {
			return newProvider(name, model)
		}
	}
	return nil, fmt.Errorf("no LLM provider found (install claude or codex CLI)")
}
