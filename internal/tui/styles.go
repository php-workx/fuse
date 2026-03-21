package tui

import (
	"strings"

	"charm.land/lipgloss/v2"

	sanitize_pkg "github.com/runger/fuse/internal/sanitize"
)

var (
	// Decision colors.
	styleSafe     = lipgloss.NewStyle().Foreground(lipgloss.Color("10")) // green
	styleCaution  = lipgloss.NewStyle().Foreground(lipgloss.Color("11")) // yellow
	styleApproval = lipgloss.NewStyle().Foreground(lipgloss.Color("12")) // blue
	styleBlocked  = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))  // red

	// Layout styles.
	styleHeader    = lipgloss.NewStyle().Bold(true).Background(lipgloss.Color("236")).Padding(0, 1)
	styleFooter    = lipgloss.NewStyle().Faint(true)
	styleActiveTab = lipgloss.NewStyle().Bold(true).Underline(true)
	styleTab       = lipgloss.NewStyle().Faint(true)
	styleCursor    = lipgloss.NewStyle().Reverse(true)
	styleColHeader = lipgloss.NewStyle().Bold(true).Faint(true)
	styleDetail    = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1)
	styleError     = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true)
	styleDim       = lipgloss.NewStyle().Faint(true)
)

// decisionStyle returns the lipgloss style for a decision string.
func decisionStyle(decision string) lipgloss.Style {
	switch strings.ToUpper(decision) {
	case "SAFE":
		return styleSafe
	case "CAUTION":
		return styleCaution
	case "APPROVAL":
		return styleApproval
	case "BLOCKED":
		return styleBlocked
	default:
		return lipgloss.NewStyle()
	}
}

// sanitize strips ANSI/OSC escape sequences, control characters, and Unicode
// C1 codes from a string before display. Delegates to the shared sanitize package.
func sanitize(s string) string {
	return sanitize_pkg.String(s)
}

// shorten truncates s to maxLen characters, adding "..." if truncated.
func shorten(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}

// fallbackValue returns "-" for empty strings, otherwise the string itself.
func fallbackValue(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
