package core

import (
	"bytes"
	"fmt"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// SplitCompoundCommand splits a display-normalized command string into
// individual sub-commands by parsing it with a Bash-compatible shell parser and
// walking the AST. It splits on ;, &&, ||, |, and newline-separated
// commands (respecting quoting).
//
// For commands that fail Bash parsing, detection of the shell type is used as a
// fallback: PowerShell and CMD commands are split using a simple token-based
// splitter, while commands that look like Bash still fail-closed (i.e., require
// APPROVAL).
//
// A single command with no operators returns a slice of 1 element.
func SplitCompoundCommand(displayNorm string) ([]string, error) {
	// Detect shell type first. PowerShell and CMD commands must not go
	// through the Bash parser — it consumes backslashes as escape characters,
	// mangling Windows paths like C:\Windows into C:Windows.
	shellType := DetectShellType(displayNorm)
	if shellType == ShellPowerShell || shellType == ShellCMD {
		return splitSimpleCompound(displayNorm), nil
	}

	// Bash (or unknown): use the full Bash parser.
	parser := syntax.NewParser(syntax.Variant(syntax.LangBash))
	prog, err := parser.Parse(strings.NewReader(displayNorm), "")
	if err != nil {
		return nil, fmt.Errorf("shell parse error: %w", err)
	}

	var result []string

	for _, stmt := range prog.Stmts {
		cmds := extractFromStmt(stmt)
		result = append(result, cmds...)
	}

	if len(result) == 0 {
		// Edge case: empty or whitespace-only input.
		return []string{displayNorm}, nil
	}

	return result, nil
}

// splitSimpleCompound is a token-based splitter that splits on ;, |, and &
// while respecting quoted strings (single and double quotes). It does NOT use
// mvdan.cc/sh. Returns sub-commands as string slices with whitespace trimmed.
// Empty results are skipped.
func splitSimpleCompound(displayNorm string) []string {
	var result []string
	var current strings.Builder
	inSingle := false
	inDouble := false

	for i := 0; i < len(displayNorm); i++ {
		ch := displayNorm[i]

		switch {
		case ch == '\'' && !inDouble:
			inSingle = !inSingle
			current.WriteByte(ch)
		case ch == '"' && !inSingle:
			inDouble = !inDouble
			current.WriteByte(ch)
		case !inSingle && !inDouble && (ch == ';' || ch == '|' || ch == '&'):
			// Split on command separators. In CMD, & and && are separators.
			// In PowerShell, & is the call operator, but splitting on it is
			// conservative (the called executable still gets classified).
			sub := strings.TrimSpace(current.String())
			if sub != "" {
				result = append(result, sub)
			}
			current.Reset()
		default:
			current.WriteByte(ch)
		}
	}

	// Flush any remaining content.
	sub := strings.TrimSpace(current.String())
	if sub != "" {
		result = append(result, sub)
	}

	if len(result) == 0 {
		return []string{displayNorm}
	}

	return result
}

// extractFromStmt extracts sub-commands from a single statement,
// recursing into binary expressions (&&, ||) and pipelines (|).
func extractFromStmt(stmt *syntax.Stmt) []string {
	if stmt == nil || stmt.Cmd == nil {
		return nil
	}

	switch cmd := stmt.Cmd.(type) {
	case *syntax.BinaryCmd:
		return extractFromBinaryCmd(cmd)
	default:
		// For CallExpr, pipelines, and other command types,
		// check if it's a pipeline with multiple commands.
		return extractFromCommand(stmt)
	}
}

// extractFromBinaryCmd recursively splits binary commands (&&, ||, ;).
func extractFromBinaryCmd(bin *syntax.BinaryCmd) []string {
	var left, right []string

	// Recurse into left side.
	if bin.X != nil {
		left = extractFromStmt(bin.X)
	}

	// Recurse into right side.
	if bin.Y != nil {
		right = extractFromStmt(bin.Y)
	}

	var result []string
	result = append(result, left...)
	result = append(result, right...)
	return result
}

// extractFromCommand handles a statement that may be a pipeline or
// a simple command. Pipelines (|) are split into individual commands.
func extractFromCommand(stmt *syntax.Stmt) []string {
	// Check if the command is a pipeline with multiple parts.
	if pipe, ok := stmt.Cmd.(*syntax.BinaryCmd); ok && pipe.Op == syntax.Pipe {
		return extractFromBinaryCmd(pipe)
	}

	// Single command: print it back to a string.
	text := printNode(stmt)
	if text == "" {
		return nil
	}
	return []string{text}
}

// printNode uses the shell printer to convert an AST node back to text.
func printNode(node syntax.Node) string {
	var buf bytes.Buffer
	printer := syntax.NewPrinter()
	if err := printer.Print(&buf, node); err != nil {
		return ""
	}
	return strings.TrimSpace(buf.String())
}

// ContainsCwdChange checks if any of the sub-commands starts with a command
// that changes the working directory. This includes Unix builtins (cd, pushd,
// popd), PowerShell cmdlets (Set-Location, sl, Push-Location, Pop-Location),
// and CMD aliases (chdir). PowerShell cmdlets are matched case-insensitively.
// This is used to detect when cwd changes might affect file-backed
// sub-commands that follow.
func ContainsCwdChange(subCommands []string) bool {
	for _, cmd := range subCommands {
		trimmed := strings.TrimSpace(cmd)
		// Extract the first word (the command name).
		first := firstWord(trimmed)

		// Case-insensitive match for cwd-changing commands across all shells.
		lower := strings.ToLower(first)
		switch lower {
		case "cd", "pushd", "popd",
			"set-location", "sl", "push-location", "pop-location", "chdir":
			return true
		}
	}
	return false
}

// firstWord returns the first whitespace-delimited token from s.
func firstWord(s string) string {
	fields := strings.Fields(s)
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}
