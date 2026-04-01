package cli

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/php-workx/fuse/internal/config"
	"github.com/php-workx/fuse/internal/judge"
)

func selectInstallProfile(cmd *cobra.Command) (string, error) {
	in := cmd.InOrStdin()
	if file, ok := in.(*os.File); ok && file == os.Stdin && !isTerminal(int(file.Fd())) {
		return config.ProfileRelaxed, nil
	}
	return promptInstallProfile(in, cmd.OutOrStdout())
}

func runInstallWithSelectedProfile(cmd *cobra.Command, target string, secure bool) error {
	profile, err := selectInstallProfile(cmd)
	if err != nil {
		return err
	}

	fmt.Fprintf(cmd.OutOrStdout(), "profile selected: %s\n", profile)
	warnIfJudgeProviderUnavailable(profile)

	switch target {
	case "claude":
		return installClaudeWithProfile(secure, profile)
	case "codex":
		return installCodexWithProfile(profile)
	default:
		return fmt.Errorf("unknown install target %q (supported: claude, codex)", target)
	}
}

func promptInstallProfile(in io.Reader, out io.Writer) (string, error) {
	if in == nil {
		in = strings.NewReader("")
	}

	fmt.Fprintln(out, "How should fuse handle suspicious commands?")
	fmt.Fprintln(out, "  1. Relaxed   Block dangerous commands, log suspicious ones, never interrupt.")
	fmt.Fprintln(out, "              Best for: experienced developers who want minimal friction.")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "  2. Balanced  Use an LLM judge to review suspicious commands.")
	fmt.Fprintln(out, "              You are only asked when the judge thinks it is necessary.")
	fmt.Fprintln(out, "              Best for: most users. Requires a Claude or Codex API.")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "  3. Strict    LLM judge reviews suspicious commands.")
	fmt.Fprintln(out, "              Critical commands always require your confirmation.")
	fmt.Fprintln(out, "              Best for: production environments or security-sensitive work.")
	fmt.Fprint(out, "Pick a profile [1-3] (default: 1): ")

	reader := bufio.NewReader(in)
	line, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", fmt.Errorf("read profile selection: %w", err)
	}

	selection := strings.TrimSpace(line)
	if selection == "" {
		return config.ProfileRelaxed, nil
	}

	switch selection {
	case "1":
		return config.ProfileRelaxed, nil
	case "2":
		return config.ProfileBalanced, nil
	case "3":
		return config.ProfileStrict, nil
	default:
		return "", fmt.Errorf("invalid profile selection %q (supported: 1, 2, 3)", selection)
	}
}

func warnIfJudgeProviderUnavailable(profile string) {
	switch profile {
	case config.ProfileBalanced, config.ProfileStrict:
	default:
		return
	}

	if _, err := judge.DetectProvider("", ""); err != nil {
		fmt.Fprintf(os.Stderr, "warning: %s profile selected but no judge provider found on PATH; install claude or codex to use judge mode.\n", profile)
	}
}
