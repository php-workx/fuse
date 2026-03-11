package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/runger/fuse/internal/config"
	"github.com/runger/fuse/internal/core"
	"github.com/runger/fuse/internal/policy"
)

var testCmd = &cobra.Command{
	Use:   "test",
	Short: "Test commands for dry-run classification and inspection",
}

var testClassifyCmd = &cobra.Command{
	Use:   "classify [command...]",
	Short: "Dry-run classification of a shell command (no execution)",
	Long:  "Classify a command and print the result without executing it.",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		command := strings.Join(args, " ")
		return runTestClassify(command)
	},
}

var testInspectCmd = &cobra.Command{
	Use:   "inspect <file>",
	Short: "Dry-run file inspection",
	Long:  "Inspect a file and print the signals found, risk assessment, and hash.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runTestInspect(args[0])
	},
}

func init() {
	testCmd.AddCommand(testClassifyCmd)
	testCmd.AddCommand(testInspectCmd)
	rootCmd.AddCommand(testCmd)
}

// runTestClassify performs a dry-run classification of a command.
func runTestClassify(command string) error {
	cwd, _ := os.Getwd()

	req := core.ShellRequest{
		RawCommand: command,
		Cwd:        cwd,
		Source:     "test",
	}

	// Load policy if available.
	evaluator := loadPolicyEvaluator()

	result, err := core.Classify(req, evaluator)
	if err != nil {
		return fmt.Errorf("classification error: %w", err)
	}

	// Print the result.
	fmt.Println("Classification Result")
	fmt.Println("=====================")
	fmt.Printf("  Command:    %s\n", command)
	fmt.Printf("  Decision:   %s\n", result.Decision)
	fmt.Printf("  Reason:     %s\n", result.Reason)
	if result.RuleID != "" {
		fmt.Printf("  Rule ID:    %s\n", result.RuleID)
	}

	if len(result.SubResults) > 0 {
		fmt.Println()
		fmt.Println("Sub-command Results")
		fmt.Println("-------------------")
		for i, sub := range result.SubResults {
			fmt.Printf("  [%d] %s\n", i+1, sub.Command)
			fmt.Printf("      Decision: %s\n", sub.Decision)
			fmt.Printf("      Reason:   %s\n", sub.Reason)
			if sub.RuleID != "" {
				fmt.Printf("      Rule ID:  %s\n", sub.RuleID)
			}
		}
	}

	return nil
}

// runTestInspect performs a dry-run file inspection.
func runTestInspect(path string) error {
	cwd, _ := os.Getwd()

	// Resolve relative path.
	if !isAbsPath(path) && cwd != "" {
		path = filepath.Join(cwd, path)
	}

	inspection, err := core.InspectFile(path, core.DefaultMaxBytes)
	if err != nil {
		return fmt.Errorf("inspection error: %w", err)
	}

	fmt.Println("File Inspection Result")
	fmt.Println("======================")
	fmt.Printf("  Path:       %s\n", inspection.Path)
	fmt.Printf("  Exists:     %v\n", inspection.Exists)

	if !inspection.Exists {
		fmt.Printf("  Decision:   %s\n", inspection.Decision)
		fmt.Printf("  Reason:     %s\n", inspection.Reason)
		return nil
	}

	fmt.Printf("  Size:       %d bytes\n", inspection.Size)
	fmt.Printf("  Truncated:  %v\n", inspection.Truncated)
	fmt.Printf("  Hash:       %s\n", inspection.Hash)
	fmt.Printf("  Decision:   %s\n", inspection.Decision)
	fmt.Printf("  Reason:     %s\n", inspection.Reason)

	if len(inspection.Signals) > 0 {
		fmt.Println()
		fmt.Println("Signals")
		fmt.Println("-------")
		for i, sig := range inspection.Signals {
			fmt.Printf("  [%d] Category: %s\n", i+1, sig.Category)
			fmt.Printf("      Pattern:  %s\n", sig.Pattern)
			fmt.Printf("      Line:     %d\n", sig.Line)
			fmt.Printf("      Match:    %s\n", sig.Match)
		}
	}

	return nil
}

// loadPolicyEvaluator loads the policy evaluator from disk.
func loadPolicyEvaluator() core.PolicyEvaluator {
	policyPath := config.PolicyPath()
	if _, err := os.Stat(policyPath); err == nil {
		pol, err := policy.LoadPolicy(policyPath)
		if err == nil {
			return policy.NewEvaluator(pol)
		}
	}
	return policy.NewEvaluator(nil)
}

// isAbsPath returns true if the path is absolute.
func isAbsPath(path string) bool {
	return path != "" && path[0] == '/'
}
