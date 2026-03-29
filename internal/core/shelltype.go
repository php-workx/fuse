package core

import (
	"runtime"
	"strings"
)

// ShellType represents the detected shell language of a command.
type ShellType int

const (
	ShellBash       ShellType = iota // Default — Unix shells, Git Bash on Windows
	ShellPowerShell                  // PowerShell 5.1+ or pwsh 7+
	ShellCMD                         // cmd.exe
)

// String returns the lowercase name of the shell type.
func (s ShellType) String() string {
	switch s {
	case ShellBash:
		return "bash"
	case ShellPowerShell:
		return "powershell"
	case ShellCMD:
		return "cmd"
	default:
		return "bash"
	}
}

// knownCmdlets is the set of common PowerShell Verb-Noun cmdlets used for
// heuristic detection. Stored lowercase for case-insensitive matching.
var knownCmdlets map[string]bool

func init() {
	cmdlets := []string{
		"Get-ChildItem", "Get-Content", "Get-Item", "Get-Process",
		"Get-Service", "Get-Date", "Get-Help", "Get-Command",
		"Get-Alias", "Get-EventLog", "Get-WmiObject", "Get-Location",
		"Get-Member", "Get-Variable", "Get-History", "Get-ItemProperty",
		"Set-Location", "Set-Item", "Set-Content", "Set-Variable",
		"Set-ItemProperty",
		"Remove-Item", "Copy-Item", "Move-Item", "New-Item",
		"New-Object", "New-Variable",
		"Invoke-WebRequest", "Invoke-Expression", "Invoke-Command",
		"Invoke-RestMethod",
		"Write-Output", "Write-Host", "Write-Error", "Write-Warning",
		"Write-Verbose",
		"Test-Path", "Test-Connection",
		"Select-Object", "Where-Object", "Sort-Object", "ForEach-Object",
		"Format-Table", "Format-List",
		"Out-File", "Out-String", "Out-Null",
		"Start-Process", "Stop-Process", "Start-Service", "Stop-Service",
		"Add-Content", "Clear-Content",
		"Compare-Object", "Measure-Object", "Group-Object",
		"ConvertTo-Json", "ConvertFrom-Json",
		"Export-Csv", "Import-Csv",
		"Select-String",
		"Resolve-Path", "Split-Path", "Join-Path",
	}

	knownCmdlets = make(map[string]bool, len(cmdlets))
	for _, c := range cmdlets {
		knownCmdlets[strings.ToLower(c)] = true
	}
}

// cmdOnlyBuiltins are commands that exist only in cmd.exe and have no Unix
// counterpart with the same name. Used only on Windows (runtime.GOOS == "windows")
// to avoid mis-detecting Unix commands like "dir" as CMD.
var cmdOnlyBuiltins = map[string]bool{
	"dir":  true,
	"type": true,
	"set":  true,
	"ver":  true,
	"cls":  true,
	"copy": true,
	"move": true,
	"ren":  true,
	"del":  true,
	"rd":   true,
	"md":   true,
}

// DetectShellType classifies the command string as Bash, PowerShell, or CMD.
//
// Detection order:
//  1. Explicit cmd.exe /c wrapper → CMD
//  2. Explicit powershell.exe / pwsh wrapper → PowerShell
//  3. Known PowerShell Verb-Noun cmdlet in the command → PowerShell
//  4. (Windows only) First token is a CMD-only builtin → CMD
//  5. Default → Bash
func DetectShellType(command string) ShellType {
	fields := strings.Fields(command)
	if len(fields) == 0 {
		return ShellBash
	}

	first := strings.ToLower(fields[0])

	// Step 1: cmd.exe /c or cmd /c wrapper.
	if (first == "cmd.exe" || first == "cmd") && len(fields) >= 2 && strings.EqualFold(fields[1], "/c") {
		return ShellCMD
	}

	// Step 2: powershell.exe, pwsh.exe, powershell, or pwsh wrapper.
	if first == "powershell.exe" || first == "pwsh.exe" || first == "powershell" || first == "pwsh" {
		return ShellPowerShell
	}

	// Step 3: Scan tokens for a known Verb-Noun PowerShell cmdlet.
	for _, tok := range fields {
		if knownCmdlets[strings.ToLower(tok)] {
			return ShellPowerShell
		}
	}

	// Step 4: On Windows only, check if the first token is a CMD-only builtin.
	if runtime.GOOS == "windows" {
		if cmdOnlyBuiltins[first] {
			return ShellCMD
		}
	}

	// Step 5: Default to Bash.
	return ShellBash
}
