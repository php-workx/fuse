package db

import "fmt"

// JudgeSummary holds aggregated judge accuracy statistics.
type JudgeSummary struct {
	Evaluated      int
	Agreed         int // judge decision == original decision
	WouldUpgrade   int // judge severity > original severity
	WouldDowngrade int // judge severity < original severity
	Errors         int // judge_error != ''
	AvgLatencyMs   float64
}

// SummarizeJudgeAccuracy computes judge accuracy metrics from events in the last 7 days.
func (d *DB) SummarizeJudgeAccuracy() (JudgeSummary, error) {
	var s JudgeSummary

	row := d.db.QueryRow(`
		SELECT
			COUNT(*) AS evaluated,
			COALESCE(SUM(CASE WHEN judge_decision = decision THEN 1 ELSE 0 END), 0) AS agreed,
			COALESCE(SUM(CASE
				WHEN judge_error != '' THEN 0
				WHEN (judge_decision = 'APPROVAL' AND decision IN ('SAFE','CAUTION'))
				  OR (judge_decision = 'CAUTION' AND decision = 'SAFE')
				THEN 1 ELSE 0 END), 0) AS would_upgrade,
			COALESCE(SUM(CASE
				WHEN judge_error != '' THEN 0
				WHEN (judge_decision = 'SAFE' AND decision IN ('CAUTION','APPROVAL'))
				  OR (judge_decision = 'CAUTION' AND decision = 'APPROVAL')
				THEN 1 ELSE 0 END), 0) AS would_downgrade,
			COALESCE(SUM(CASE WHEN judge_error != '' THEN 1 ELSE 0 END), 0) AS errors,
			COALESCE(AVG(CASE WHEN judge_latency_ms > 0 THEN judge_latency_ms END), 0) AS avg_latency
		FROM events
		WHERE (judge_decision != '' OR judge_error != '')
		  AND timestamp > datetime('now', '-7 days')
	`)

	if err := row.Scan(
		&s.Evaluated,
		&s.Agreed,
		&s.WouldUpgrade,
		&s.WouldDowngrade,
		&s.Errors,
		&s.AvgLatencyMs,
	); err != nil {
		return s, fmt.Errorf("summarize judge accuracy: %w", err)
	}
	return s, nil
}
