package cli

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/runger/fuse/internal/config"
	"github.com/runger/fuse/internal/db"
)

var (
	eventsSource   string
	eventsAgent    string
	eventsDecision string
	eventsSession  string
	eventsLimit    int
)

var eventsCmd = &cobra.Command{
	Use:   "events",
	Short: "List recent fuse events",
	RunE: func(cmd *cobra.Command, args []string) error {
		database, err := db.OpenDB(config.DBPath())
		if err != nil {
			return fmt.Errorf("open database: %w", err)
		}
		defer func() { _ = database.Close() }()

		filter := db.EventFilter{
			Source:   eventsSource,
			Agent:    eventsAgent,
			Decision: eventsDecision,
			Session:  eventsSession,
			Limit:    eventsLimit,
		}

		events, err := database.ListEvents(filter)
		if err != nil {
			return err
		}

		if len(events) == 0 {
			fmt.Fprintln(os.Stderr, "No events found.")
			return nil
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
		fmt.Fprintln(w, "ID\tTIMESTAMP\tDECISION\tSOURCE\tSESSION\tCOMMAND")
		for _, e := range events {
			cmd := e.Command
			if len(cmd) > 60 {
				cmd = cmd[:57] + "..."
			}
			session := e.SessionID
			if len(session) > 16 {
				session = session[:16]
			}
			fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%s\t%s\n",
				e.ID, truncTimestamp(e.Timestamp), e.Decision, e.Source, session, cmd)
		}
		return w.Flush()
	},
}

var statsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Show aggregated fuse event statistics",
	RunE: func(cmd *cobra.Command, args []string) error {
		database, err := db.OpenDB(config.DBPath())
		if err != nil {
			return fmt.Errorf("open database: %w", err)
		}
		defer func() { _ = database.Close() }()

		filter := db.EventFilter{
			Source:   eventsSource,
			Agent:    eventsAgent,
			Decision: eventsDecision,
			Session:  eventsSession,
		}

		summaries, total, err := database.SummarizeEvents(filter)
		if err != nil {
			return err
		}

		fmt.Printf("Total events: %d\n\n", total)
		if len(summaries) == 0 {
			return nil
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
		fmt.Fprintln(w, "DECISION\tSOURCE\tCOUNT")
		for _, s := range summaries {
			fmt.Fprintf(w, "%s\t%s\t%d\n", s.Decision, s.Source, s.Count)
		}
		return w.Flush()
	},
}

func truncTimestamp(ts string) string {
	if len(ts) > 19 {
		return ts[:19]
	}
	return ts
}

func init() {
	for _, cmd := range []*cobra.Command{eventsCmd, statsCmd} {
		cmd.Flags().StringVar(&eventsSource, "source", "", "Filter by source (hook, codex, shell)")
		cmd.Flags().StringVar(&eventsAgent, "agent", "", "Filter by agent")
		cmd.Flags().StringVar(&eventsDecision, "decision", "", "Filter by decision (SAFE, CAUTION, APPROVAL, BLOCKED)")
		cmd.Flags().StringVar(&eventsSession, "session", "", "Filter by session ID")
	}
	eventsCmd.Flags().IntVar(&eventsLimit, "limit", 50, "Maximum number of events to show")

	rootCmd.AddCommand(eventsCmd)
	rootCmd.AddCommand(statsCmd)
}
