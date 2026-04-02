package core

import (
	"path/filepath"
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
var (
	knownCmdlets           map[string]bool
	knownPowerShellAliases map[string]bool
)

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
		"Start-BitsTransfer",
		"Compress-Archive", "Get-Credential", "New-ItemProperty",
		"Enter-PSSession", "Invoke-WmiMethod", "New-NetFirewallRule",
		"New-PSSession",
		"Set-ExecutionPolicy",
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

	// Only include aliases that are strongly PowerShell-specific and do not
	// collide with common Unix commands.
	aliases := []string{"iex", "iwr", "irm", "icm", "saps", "spps", "nsn", "etsn"}
	knownPowerShellAliases = make(map[string]bool, len(aliases))
	for _, a := range aliases {
		knownPowerShellAliases[a] = true
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

// windowsCommandUtilities are standalone Windows executables whose command-line
// syntax should be preserved instead of sent through the Bash parser.
var windowsCommandUtilities = map[string]bool{
	"auditpol":  true,
	"bitsadmin": true,
	"certutil":  true,
	"cmdkey":    true,
	"cscript":   true,
	"forfiles":  true,
	"hh":        true,
	"logman":    true,
	"mshta":     true,
	"msiexec":   true,
	"net":       true,
	"netsh":     true,
	"ntdsutil":  true,
	"pcalua":    true,
	"reg":       true,
	"regsvr32":  true,
	"rundll32":  true,
	"sc":        true,
	"schtasks":  true,
	"vaultcmd":  true,
	"wevtutil":  true,
	"wmic":      true,
	"wscript":   true,
}

// DetectShellType classifies the command string as Bash, PowerShell, or CMD.
//
// Detection order:
//  1. Explicit cmd.exe /c wrapper → CMD
//  2. Explicit powershell.exe / pwsh wrapper → PowerShell
//  3. Known PowerShell Verb-Noun cmdlet or alias as first token → PowerShell
//  4. Known cmdlet/alias in pipeline context (after "|") → PowerShell
//  5. PowerShell-specific alias or type literal syntax → PowerShell
//  6. (Windows only) First token is a CMD-only builtin → CMD
//  7. Default → Bash
func DetectShellType(command string) ShellType {
	fields := strings.Fields(command)
	if len(fields) == 0 {
		return ShellBash
	}

	first := normalizeShellCommandToken(fields[0])
	if first == "" {
		return ShellBash
	}

	// Step 1: cmd.exe /c or cmd /c wrapper.
	if first == "cmd" && len(fields) >= 2 && strings.EqualFold(strings.Trim(fields[1], `"'`), "/c") {
		return ShellCMD
	}

	// Step 2: powershell.exe, pwsh.exe, powershell, or pwsh wrapper.
	if first == "powershell" || first == "pwsh" {
		return ShellPowerShell
	}

	// Step 3: Check first token for a known Verb-Noun PowerShell cmdlet/alias.
	if knownCmdlets[first] || knownPowerShellAliases[first] {
		return ShellPowerShell
	}

	// Step 4: Detect cmdlets in explicit pipeline context.
	for i := 1; i < len(fields); i++ {
		tok := normalizeShellCommandToken(fields[i])
		if tok == "" || (!knownCmdlets[tok] && !knownPowerShellAliases[tok]) {
			continue
		}
		prev := strings.TrimSpace(fields[i-1])
		if prev == "|" || strings.HasSuffix(prev, "|") {
			return ShellPowerShell
		}
	}

	// Step 5: PowerShell type-literal or static member syntax, e.g.
	// [System.Net.WebClient]::new() or [Ref].Assembly.GetType(...).
	lowerCommand := strings.ToLower(command)
	if strings.Contains(lowerCommand, "]::") || strings.Contains(lowerCommand, "[ref].") {
		return ShellPowerShell
	}

	// Step 6: Standalone Windows utilities should also bypass the Bash parser so
	// backslashes survive on non-Windows hosts classifying Windows commands.
	if windowsCommandUtilities[first] {
		return ShellCMD
	}

	// Step 7: On Windows only, check if the first token is a CMD-only builtin.
	if runtime.GOOS == "windows" {
		if cmdOnlyBuiltins[first] {
			return ShellCMD
		}
	}

	// Step 8: Default to Bash.
	return ShellBash
}

func normalizeShellCommandToken(token string) string {
	cleaned := strings.TrimSpace(token)
	cleaned = strings.Trim(cleaned, `"'`)
	if cleaned == "" {
		return ""
	}

	normalized := strings.ReplaceAll(cleaned, `\`, "/")
	base := strings.ToLower(filepath.Base(normalized))
	return strings.TrimSuffix(base, ".exe")
}
