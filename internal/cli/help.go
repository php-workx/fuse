package cli

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"text/template" // nosemgrep: import-text-template -- CLI terminal output, not HTML
	"unicode/utf8"

	"github.com/spf13/cobra"
)

const (
	groupSetup   = "setup"
	groupRuntime = "runtime"
	groupObserve = "observe"

	maxBoxWidth = 72
	minBoxWidth = 40

	// ANSI SGR escape codes.
	ansiReset    = "\033[0m"
	ansiBold     = "\033[1m"
	ansiDim      = "\033[2m"
	ansiBoldCyan = "\033[1;36m"
)

// groupCommandOrder defines the display order for commands within each group.
// Commands not listed here fall to the end in their default (alphabetical) order.
var groupCommandOrder = map[string][]string{
	groupSetup:   {"install", "uninstall", "enable", "disable", "dryrun"},
	groupRuntime: {"run", "hook", "proxy"},
	groupObserve: {"events", "stats", "monitor", "doctor", "profile", "test"},
}

// termWidthFunc returns the terminal width. Overridable in tests.
var termWidthFunc = terminalWidth

// shouldColorizeFunc determines whether to emit ANSI color codes. Overridable in tests.
var shouldColorizeFunc = shouldColorize

func initHelp() {
	rootCmd.AddGroup(
		&cobra.Group{ID: groupSetup, Title: "Setup"},
		&cobra.Group{ID: groupRuntime, Title: "Runtime"},
		&cobra.Group{ID: groupObserve, Title: "Observe"},
	)

	cobra.AddTemplateFuncs(template.FuncMap{
		"groupedHelp": groupedHelp,
	})

	rootCmd.SetUsageTemplate(usageTemplate)
}

// shouldColorize returns true when stdout is a terminal that supports ANSI
// escape sequences and color is not suppressed.
func shouldColorize() bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	if os.Getenv("TERM") == "dumb" {
		return false
	}
	return isTerminal(int(os.Stdout.Fd())) && supportsANSI()
}

// helpRenderer applies optional ANSI styling to help output.
type helpRenderer struct {
	color bool
}

func (r helpRenderer) dim(s string) string {
	if !r.color {
		return s
	}
	return ansiDim + s + ansiReset
}

func (r helpRenderer) bold(s string) string {
	if !r.color {
		return s
	}
	return ansiBold + s + ansiReset
}

func (r helpRenderer) titleStyle(s string) string {
	if !r.color {
		return s
	}
	return ansiBoldCyan + s + ansiReset
}

// boxTopFill computes the number of fill characters for a box top border.
func boxTopFill(title string, width int) int {
	inner := width - 2
	titleW := utf8.RuneCountInString(title)
	fill := inner - 3 - titleW // prefix "─ " + title + " " is 3 + titleW cols
	if fill < 0 {
		fill = 0
	}
	return fill
}

// top renders a box top border: ╭─ Title ────────╮
func (r helpRenderer) top(title string, width int) string {
	fill := boxTopFill(title, width)
	filling := " " + strings.Repeat("─", fill) + "╮"
	return r.dim("╭─ ") + r.titleStyle(title) + r.dim(filling)
}

// boxRowLayout computes the display content and padding for a box row.
// Returns the formatted name field, possibly-truncated description, and padding count.
func boxRowLayout(name, short string, namePad, width int) (nameField, desc string, pad int) {
	inner := width - 2
	available := inner - 1 // 1 space before closing │

	nameField = fmt.Sprintf("%-*s", namePad, name)
	prefixW := 2 + utf8.RuneCountInString(nameField) + 1 // "  " + nameField + " "
	maxDescW := available - prefixW

	desc = short
	if utf8.RuneCountInString(short) > maxDescW {
		switch {
		case maxDescW > 3:
			desc = string([]rune(short)[:maxDescW-3]) + "..."
		case maxDescW > 0:
			desc = string([]rune(short)[:maxDescW])
		default:
			desc = ""
		}
	}

	contentW := prefixW + utf8.RuneCountInString(desc)
	pad = available - contentW
	if pad < 0 {
		pad = 0
	}
	return nameField, desc, pad
}

// row renders a box row: │  name     description   │
func (r helpRenderer) row(name, short string, namePad, width int) string {
	nameField, desc, pad := boxRowLayout(name, short, namePad, width)
	return r.dim("│") + "  " + r.bold(nameField) + " " +
		r.dim(desc+strings.Repeat(" ", pad)+" │")
}

// bottom renders a box bottom border: ╰──────────╯
func (r helpRenderer) bottom(width int) string {
	inner := width - 2
	return r.dim("╰" + strings.Repeat("─", inner) + "╯")
}

func groupedHelp(cmd *cobra.Command) string {
	groups := cmd.Groups()
	if len(groups) == 0 {
		return ""
	}

	w := boxWidth()
	namePad := cmd.NamePadding()
	r := helpRenderer{color: shouldColorizeFunc()}
	var b strings.Builder

	for _, group := range groups {
		cmds := commandsForGroup(cmd, group.ID)
		if len(cmds) == 0 {
			continue
		}
		sortByGroupOrder(cmds, group.ID)
		renderGroupSection(&b, r, group.Title, cmds, namePad, w)
	}

	renderUngroupedSection(&b, r, cmd, namePad)

	return b.String()
}

// commandsForGroup returns the available commands belonging to the given group.
func commandsForGroup(cmd *cobra.Command, groupID string) []*cobra.Command {
	var cmds []*cobra.Command
	for _, c := range cmd.Commands() {
		if c.GroupID == groupID && c.IsAvailableCommand() {
			cmds = append(cmds, c)
		}
	}
	return cmds
}

// renderGroupSection writes a single command group section to the builder.
func renderGroupSection(b *strings.Builder, r helpRenderer, title string, cmds []*cobra.Command, namePad, w int) {
	if w < minBoxWidth {
		b.WriteString(r.titleStyle(title) + ":\n")
		for _, c := range cmds {
			nameField := fmt.Sprintf("%-*s", namePad, c.Name())
			fmt.Fprintf(b, "  %s %s\n", r.bold(nameField), r.dim(c.Short))
		}
		b.WriteByte('\n')
	} else {
		b.WriteString(r.top(title, w))
		b.WriteByte('\n')
		for _, c := range cmds {
			b.WriteString(r.row(c.Name(), c.Short, namePad, w))
			b.WriteByte('\n')
		}
		b.WriteString(r.bottom(w))
		b.WriteByte('\n')
	}
}

// renderUngroupedSection writes the "Additional Commands" section for commands without a group.
func renderUngroupedSection(b *strings.Builder, r helpRenderer, cmd *cobra.Command, namePad int) {
	var ungrouped []*cobra.Command
	for _, c := range cmd.Commands() {
		if c.GroupID == "" && (c.IsAvailableCommand() || c.Name() == "help") {
			ungrouped = append(ungrouped, c)
		}
	}
	if len(ungrouped) > 0 {
		b.WriteString("\n" + r.dim("Additional Commands:") + "\n")
		for _, c := range ungrouped {
			nameField := fmt.Sprintf("%-*s", namePad, c.Name())
			fmt.Fprintf(b, "  %s %s\n", r.bold(nameField), r.dim(c.Short))
		}
	}
}

// boxTop, boxRow, boxBottom are the plain (no-color) structural helpers.
// Used directly by unit tests to verify display-width invariants.

func boxTop(title string, width int) string {
	fill := boxTopFill(title, width)
	return "╭─ " + title + " " + strings.Repeat("─", fill) + "╮"
}

func boxRow(text string, width int) string {
	inner := width - 2
	content := "  " + text
	contentW := utf8.RuneCountInString(content)
	available := inner - 1 // 1 space before closing │
	if contentW > available {
		runes := []rune(content)
		if available > 3 {
			content = string(runes[:available-3]) + "..."
		} else {
			content = string(runes[:available])
		}
		contentW = available
	}
	pad := available - contentW
	return "│" + content + strings.Repeat(" ", pad) + " │"
}

func boxBottom(width int) string {
	inner := width - 2
	return "╰" + strings.Repeat("─", inner) + "╯"
}

func boxWidth() int {
	w := termWidthFunc() - 2 // leave 2-column margin so boxes don't touch terminal edge
	if w > maxBoxWidth {
		w = maxBoxWidth
	}
	return w
}

// sortByGroupOrder sorts commands according to the explicit order defined in
// groupCommandOrder. Commands not in the order list sort to the end alphabetically.
func sortByGroupOrder(cmds []*cobra.Command, groupID string) {
	order, ok := groupCommandOrder[groupID]
	if !ok {
		return
	}
	rank := make(map[string]int, len(order))
	for i, name := range order {
		rank[name] = i
	}
	sort.SliceStable(cmds, func(i, j int) bool {
		ri, oki := rank[cmds[i].Name()]
		rj, okj := rank[cmds[j].Name()]
		if oki && okj {
			return ri < rj
		}
		if oki {
			return true
		}
		if okj {
			return false
		}
		return cmds[i].Name() < cmds[j].Name()
	})
}

const usageTemplate = `Usage:{{if .Runnable}}
  {{.UseLine}}{{end}}{{if .HasAvailableSubCommands}}
  {{.CommandPath}} [command]{{end}}{{if gt (len .Aliases) 0}}

Aliases:
  {{.NameAndAliases}}{{end}}{{if .HasExample}}

Examples:
{{.Example}}{{end}}{{if .HasAvailableSubCommands}}{{$cmds := .Commands}}{{if eq (len .Groups) 0}}

Available Commands:{{range $cmds}}{{if (or .IsAvailableCommand (eq .Name "help"))}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{else}}

{{groupedHelp .}}{{end}}{{end}}{{if .HasAvailableLocalFlags}}
Flags:
{{.LocalFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasAvailableInheritedFlags}}

Global Flags:
{{.InheritedFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasHelpSubCommands}}

Additional help topics:{{range .Commands}}{{if .IsAdditionalHelpTopicCommand}}
  {{rpad .Name .NamePadding}} {{.Short}}{{end}}{{end}}{{end}}{{if .HasAvailableSubCommands}}

Use "{{.CommandPath}} [command] --help" for more information about a command.{{end}}
`
