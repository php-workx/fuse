package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/php-workx/fuse/internal/config"
	"github.com/php-workx/fuse/internal/core"
	"github.com/php-workx/fuse/internal/db"
)

type replayEventsOptions struct {
	dbPath string
	limit  int
	json   bool
	top    int
}

type replayEventsReport struct {
	Total                     int                         `json:"total"`
	HistoricalByDecision      map[string]int              `json:"historical_by_decision"`
	CurrentByDecision         map[string]int              `json:"current_by_decision"`
	DecisionMatrix            map[string]map[string]int   `json:"decision_matrix"`
	HistoricalApprovalEvents  int                         `json:"historical_approval_events"`
	CurrentApprovalEvents     int                         `json:"current_approval_events"`
	DedupedApprovalPromptKeys int                         `json:"deduped_approval_prompt_keys"`
	RemainingClusters         []replayEventsCluster       `json:"remaining_clusters"`
	ClassificationErrors      []replayClassificationError `json:"classification_errors,omitempty"`
}

type replayEventsCluster struct {
	Count                int            `json:"count"`
	Decision             string         `json:"decision"`
	Reason               string         `json:"reason"`
	RuleID               string         `json:"rule_id,omitempty"`
	PromptKeys           int            `json:"prompt_keys"`
	HistoricalByDecision map[string]int `json:"historical_by_decision"`
	Example              string         `json:"example"`
	Source               string         `json:"source,omitempty"`
	Agent                string         `json:"agent,omitempty"`
	Workspace            string         `json:"workspace,omitempty"`

	promptKeySet map[string]bool
}

type replayClassificationError struct {
	EventID int64  `json:"event_id"`
	Error   string `json:"error"`
}

var replayEventsOpts replayEventsOptions

var testReplayEventsCmd = &cobra.Command{
	Use:   "replay-events",
	Short: "Replay historical events through the current classifier",
	Long:  "Replay historical Fuse event commands through the current classifier without executing them.",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runReplayEvents(&replayEventsOpts)
	},
}

func init() {
	testReplayEventsCmd.Flags().StringVar(&replayEventsOpts.dbPath, "db", "", "Path to events database (default: ~/.fuse/state/fuse.db)")
	testReplayEventsCmd.Flags().IntVar(&replayEventsOpts.limit, "limit", 0, "Maximum number of events to replay (0 = all)")
	testReplayEventsCmd.Flags().BoolVar(&replayEventsOpts.json, "json", false, "Emit JSON")
	testReplayEventsCmd.Flags().IntVar(&replayEventsOpts.top, "top", 20, "Number of remaining approval/caution clusters to show (0 = all)")
	testCmd.AddCommand(testReplayEventsCmd)
}

func runReplayEvents(opts *replayEventsOptions) error {
	dbPath := opts.dbPath
	if dbPath == "" {
		dbPath = config.DBPath()
	}
	if _, err := os.Stat(dbPath); err != nil {
		if os.IsNotExist(err) {
			if opts.json {
				fmt.Println("{}")
			} else {
				fmt.Println("No fuse events database found.")
			}
			return nil
		}
		return err
	}

	database, err := db.OpenDB(dbPath)
	if err != nil {
		return err
	}
	defer func() { _ = database.Close() }()

	report, err := buildReplayEventsReport(database, loadPolicyEvaluator(), opts.limit, opts.top)
	if err != nil {
		return err
	}

	if opts.json {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(report)
	}
	printReplayEventsReport(report)
	return nil
}

func buildReplayEventsReport(database *db.DB, evaluator core.PolicyEvaluator, limit, top int) (*replayEventsReport, error) {
	events, err := database.ListEventsForReplay(limit)
	if err != nil {
		return nil, err
	}

	report := &replayEventsReport{
		HistoricalByDecision: make(map[string]int),
		CurrentByDecision:    make(map[string]int),
		DecisionMatrix:       make(map[string]map[string]int),
	}
	clusters := make(map[string]*replayEventsCluster)
	approvalPromptKeys := make(map[string]bool)

	for i := range events {
		event := &events[i]
		oldDecision := normalizeReplayDecision(event.Decision)
		report.Total++
		report.HistoricalByDecision[oldDecision]++
		if oldDecision == string(core.DecisionApproval) {
			report.HistoricalApprovalEvents++
		}

		result, err := core.Classify(core.ShellRequest{
			RawCommand: event.Command,
			Cwd:        event.Cwd,
			Source:     event.Source,
			SessionID:  event.SessionID,
		}, evaluator)
		if err != nil {
			report.ClassificationErrors = append(report.ClassificationErrors, replayClassificationError{
				EventID: event.ID,
				Error:   err.Error(),
			})
			continue
		}

		newDecision := string(result.Decision)
		report.CurrentByDecision[newDecision]++
		if report.DecisionMatrix[oldDecision] == nil {
			report.DecisionMatrix[oldDecision] = make(map[string]int)
		}
		report.DecisionMatrix[oldDecision][newDecision]++

		if result.Decision == core.DecisionApproval {
			report.CurrentApprovalEvents++
			approvalPromptKeys[result.DecisionKey] = true
		}
		if result.Decision == core.DecisionApproval || result.Decision == core.DecisionCaution {
			addReplayCluster(clusters, event, result, oldDecision)
		}
	}

	report.DedupedApprovalPromptKeys = len(approvalPromptKeys)
	report.RemainingClusters = sortedReplayClusters(clusters, top)
	return report, nil
}

func addReplayCluster(clusters map[string]*replayEventsCluster, event *db.EventRecord, result *core.ClassifyResult, oldDecision string) {
	key := strings.Join([]string{
		string(result.Decision),
		result.Reason,
		result.RuleID,
		result.DecisionKey,
	}, "\x00")
	cluster := clusters[key]
	if cluster == nil {
		cluster = &replayEventsCluster{
			Decision:             string(result.Decision),
			Reason:               result.Reason,
			RuleID:               result.RuleID,
			HistoricalByDecision: make(map[string]int),
			Example:              event.Command,
			Source:               event.Source,
			Agent:                event.Agent,
			Workspace:            event.WorkspaceRoot,
			promptKeySet:         make(map[string]bool),
		}
		clusters[key] = cluster
	}
	cluster.Count++
	cluster.HistoricalByDecision[oldDecision]++
	if result.DecisionKey != "" {
		cluster.promptKeySet[result.DecisionKey] = true
	}
	cluster.PromptKeys = len(cluster.promptKeySet)
}

func sortedReplayClusters(clusters map[string]*replayEventsCluster, top int) []replayEventsCluster {
	out := make([]replayEventsCluster, 0, len(clusters))
	for _, cluster := range clusters {
		c := *cluster
		c.promptKeySet = nil
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		if core.DecisionSeverity(core.Decision(out[i].Decision)) != core.DecisionSeverity(core.Decision(out[j].Decision)) {
			return core.DecisionSeverity(core.Decision(out[i].Decision)) > core.DecisionSeverity(core.Decision(out[j].Decision))
		}
		if out[i].Reason != out[j].Reason {
			return out[i].Reason < out[j].Reason
		}
		return out[i].Example < out[j].Example
	})
	if top > 0 && len(out) > top {
		out = out[:top]
	}
	return out
}

func printReplayEventsReport(report *replayEventsReport) {
	fmt.Println("Classifier replay audit")
	fmt.Printf("Events replayed: %d\n", report.Total)
	fmt.Printf("Historical APPROVAL events: %d\n", report.HistoricalApprovalEvents)
	fmt.Printf("Current APPROVAL events: %d\n", report.CurrentApprovalEvents)
	fmt.Printf("Deduped approval prompt keys: %d\n", report.DedupedApprovalPromptKeys)
	if len(report.ClassificationErrors) > 0 {
		fmt.Printf("Classification errors: %d\n", len(report.ClassificationErrors))
	}

	fmt.Println("\nOld -> current decision matrix")
	w := tabwriter.NewWriter(os.Stdout, 0, 8, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "OLD\tCURRENT\tCOUNT")
	for _, oldDecision := range sortedDecisionKeys(report.DecisionMatrix) {
		for _, newDecision := range sortedCountKeys(report.DecisionMatrix[oldDecision]) {
			_, _ = fmt.Fprintf(w, "%s\t%s\t%d\n", oldDecision, newDecision, report.DecisionMatrix[oldDecision][newDecision])
		}
	}
	_ = w.Flush()

	if len(report.RemainingClusters) == 0 {
		fmt.Println("\nNo remaining APPROVAL or CAUTION clusters.")
		return
	}

	fmt.Println("\nTop remaining APPROVAL/CAUTION clusters")
	w = tabwriter.NewWriter(os.Stdout, 0, 8, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "COUNT\tDECISION\tKEYS\tREASON\tEXAMPLE")
	for i := range report.RemainingClusters {
		cluster := &report.RemainingClusters[i]
		_, _ = fmt.Fprintf(w, "%d\t%s\t%d\t%s\t%s\n",
			cluster.Count,
			cluster.Decision,
			cluster.PromptKeys,
			shorten(cluster.Reason, 80),
			shorten(cluster.Example, 96),
		)
	}
	_ = w.Flush()
}

func normalizeReplayDecision(decision string) string {
	decision = strings.ToUpper(strings.TrimSpace(decision))
	if decision == "" {
		return "UNKNOWN"
	}
	return decision
}

func sortedDecisionKeys(matrix map[string]map[string]int) []string {
	keys := make([]string, 0, len(matrix))
	for key := range matrix {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		severityI := core.DecisionSeverity(core.Decision(keys[i]))
		severityJ := core.DecisionSeverity(core.Decision(keys[j]))
		if severityI != severityJ {
			return severityI > severityJ
		}
		return keys[i] < keys[j]
	})
	return keys
}

func sortedCountKeys(counts map[string]int) []string {
	keys := make([]string, 0, len(counts))
	for key := range counts {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if counts[keys[i]] != counts[keys[j]] {
			return counts[keys[i]] > counts[keys[j]]
		}
		return keys[i] < keys[j]
	})
	return keys
}
