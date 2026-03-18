package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/runger/fuse/internal/config"
	"github.com/runger/fuse/internal/db"
)

type eventsOptions struct {
	limit     int
	json      bool
	source    string
	agent     string
	decision  string
	session   string
	workspace string
}

var eventsOpts eventsOptions

var statsJSON bool

var eventsCmd = &cobra.Command{
	Use:   "events",
	Short: "Show recent local fuse events",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runEvents(&eventsOpts)
	},
}

var statsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Summarize local fuse activity",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runStats()
	},
}

func init() {
	eventsCmd.Flags().IntVar(&eventsOpts.limit, "limit", 20, "Maximum number of events to show")
	eventsCmd.Flags().BoolVar(&eventsOpts.json, "json", false, "Emit JSON")
	eventsCmd.Flags().StringVar(&eventsOpts.source, "source", "", "Filter by source (hook, run, codex-shell)")
	eventsCmd.Flags().StringVar(&eventsOpts.agent, "agent", "", "Filter by agent (claude, codex, manual)")
	eventsCmd.Flags().StringVar(&eventsOpts.decision, "decision", "", "Filter by decision")
	eventsCmd.Flags().StringVar(&eventsOpts.session, "session", "", "Filter by session ID")
	eventsCmd.Flags().StringVar(&eventsOpts.workspace, "workspace", "", "Filter by workspace root")

	statsCmd.Flags().BoolVar(&statsJSON, "json", false, "Emit JSON")

	rootCmd.AddCommand(eventsCmd)
	rootCmd.AddCommand(statsCmd)
}

func runEvents(opts *eventsOptions) error {
	database, exists, err := openEventsDB()
	if err != nil {
		return err
	}
	if !exists {
		if opts.json {
			fmt.Println("[]")
		} else {
			fmt.Println("No fuse events recorded yet.")
		}
		return nil
	}
	defer func() { _ = database.Close() }()

	events, err := database.ListEvents(&db.EventFilter{
		Limit:         opts.limit,
		Source:        opts.source,
		Agent:         opts.agent,
		Decision:      strings.ToUpper(opts.decision),
		Session:       opts.session,
		WorkspaceRoot: opts.workspace,
	})
	if err != nil {
		return err
	}

	if opts.json {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(events)
	}

	if len(events) == 0 {
		fmt.Println("No matching fuse events.")
		return nil
	}

	fmt.Println("Recent fuse events")
	w := tabwriter.NewWriter(os.Stdout, 0, 8, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "TIME\tAGENT\tSOURCE\tDECISION\tWORKSPACE\tCOMMAND")
	for i := range events {
		e := &events[i]
		_, _ = fmt.Fprintf(
			w,
			"%s\t%s\t%s\t%s\t%s\t%s\n",
			e.Timestamp,
			fallbackValue(e.Agent),
			fallbackValue(e.Source),
			fallbackValue(e.Decision),
			fallbackValue(e.WorkspaceRoot),
			shorten(e.Command, 96),
		)
	}
	return w.Flush()
}

func runStats() error {
	database, exists, err := openEventsDB()
	if err != nil {
		return err
	}
	if !exists {
		if statsJSON {
			fmt.Println("{}")
		} else {
			fmt.Println("No fuse events recorded yet.")
		}
		return nil
	}
	defer func() { _ = database.Close() }()

	summary, err := database.SummarizeEvents()
	if err != nil {
		return err
	}

	if statsJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(summary)
	}

	fmt.Printf("Total events: %d\n", summary.Total)
	printCounts("By decision", summary.ByDecision)
	printCounts("By agent", summary.ByAgent)
	printCounts("By source", summary.BySource)
	printCounts("By workspace", summary.ByWorkspace)
	return nil
}

func openEventsDB() (*db.DB, bool, error) {
	if _, err := os.Stat(config.DBPath()); err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, err
	}
	database, err := db.OpenDB(config.DBPath())
	if err != nil {
		return nil, false, err
	}
	return database, true, nil
}

func printCounts(title string, counts map[string]int) {
	fmt.Printf("\n%s\n", title)
	for _, pair := range db.SortedCounts(counts) {
		fmt.Printf("  %s: %d\n", pair.Key, pair.Count)
	}
}

func fallbackValue(value string) string {
	if value == "" {
		return "-"
	}
	return value
}

func shorten(value string, maxLen int) string {
	if len(value) <= maxLen {
		return value
	}
	if maxLen <= 3 {
		return value[:maxLen]
	}
	return value[:maxLen-3] + "..."
}
