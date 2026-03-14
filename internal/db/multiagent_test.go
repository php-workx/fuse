package db

import (
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// testEvent creates an EventRecord with the given fields for testing.
func testEvent(sessionID, command, decision, source string) EventRecord {
	return EventRecord{
		SessionID: sessionID,
		Command:   command,
		Decision:  decision,
		Source:    source,
	}
}

// --- fu-xio: Test concurrent DB writes under WAL + busy_timeout ---

// initSharedDB pre-creates and migrates the database so concurrent
// openers don't all race through schema creation.
func initSharedDB(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "shared.db")
	d, err := OpenDB(dbPath)
	if err != nil {
		t.Fatalf("init shared DB: %v", err)
	}
	d.Close()
	return dbPath
}

func TestConcurrentEventWrites(t *testing.T) {
	dbPath := initSharedDB(t)

	const numWriters = 10
	const eventsPerWriter = 50

	var wg sync.WaitGroup
	var failures atomic.Int64

	for w := 0; w < numWriters; w++ {
		wg.Add(1)
		go func(writerID int) {
			defer wg.Done()
			d, err := OpenDB(dbPath)
			if err != nil {
				t.Errorf("writer %d: OpenDB: %v", writerID, err)
				failures.Add(1)
				return
			}
			defer d.Close()

			sessionID := fmt.Sprintf("session-%d", writerID)
			for i := 0; i < eventsPerWriter; i++ {
				cmd := fmt.Sprintf("command-%d-%d", writerID, i)
				if err := d.LogEvent(testEvent(sessionID, cmd, "SAFE", "test")); err != nil {
					t.Errorf("writer %d event %d: LogEvent: %v", writerID, i, err)
					failures.Add(1)
					return
				}
			}
		}(w)
	}

	wg.Wait()

	if f := failures.Load(); f > 0 {
		t.Fatalf("%d writer(s) failed", f)
	}

	// Verify all events were written.
	d, err := OpenDB(dbPath)
	if err != nil {
		t.Fatalf("verify OpenDB: %v", err)
	}
	defer d.Close()

	var count int
	if err := d.db.QueryRow("SELECT COUNT(*) FROM events").Scan(&count); err != nil {
		t.Fatalf("count events: %v", err)
	}
	expected := numWriters * eventsPerWriter
	if count != expected {
		t.Fatalf("event count = %d, want %d (lost %d writes)", count, expected, expected-count)
	}
}

func TestConcurrentMixedOperations(t *testing.T) {
	dbPath := initSharedDB(t)

	const numWorkers = 8
	var wg sync.WaitGroup
	var failures atomic.Int64

	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			d, err := OpenDB(dbPath)
			if err != nil {
				t.Errorf("worker %d: OpenDB: %v", workerID, err)
				failures.Add(1)
				return
			}
			defer d.Close()

			sessionID := fmt.Sprintf("worker-%d", workerID)
			expires := time.Now().Add(1 * time.Hour)

			// Mix event writes, approval creates, and approval consumes.
			for i := 0; i < 20; i++ {
				// Log an event.
				if err := d.LogEvent(testEvent(sessionID, fmt.Sprintf("cmd-%d", i), "SAFE", "mixed")); err != nil {
					t.Errorf("worker %d: LogEvent: %v", workerID, err)
					failures.Add(1)
					return
				}

				// Create an approval.
				approvalID := fmt.Sprintf("a-%d-%d", workerID, i)
				key := fmt.Sprintf("key-%d-%d", workerID, i)
				if err := d.CreateApproval(approvalID, key, "APPROVAL", "once", sessionID, "hmac", &expires); err != nil {
					t.Errorf("worker %d: CreateApproval: %v", workerID, err)
					failures.Add(1)
					return
				}

				// Consume our own approval.
				a, err := d.ConsumeApproval(key, sessionID)
				if err != nil {
					t.Errorf("worker %d: ConsumeApproval: %v", workerID, err)
					failures.Add(1)
					return
				}
				if a == nil {
					t.Errorf("worker %d: ConsumeApproval returned nil for own approval", workerID)
					failures.Add(1)
					return
				}
			}
		}(w)
	}

	wg.Wait()

	if f := failures.Load(); f > 0 {
		t.Fatalf("%d worker(s) had failures", f)
	}

	// Verify integrity.
	d, err := OpenDB(dbPath)
	if err != nil {
		t.Fatalf("verify OpenDB: %v", err)
	}
	defer d.Close()

	var eventCount int
	if err := d.db.QueryRow("SELECT COUNT(*) FROM events").Scan(&eventCount); err != nil {
		t.Fatalf("count events: %v", err)
	}
	if eventCount != numWorkers*20 {
		t.Errorf("events = %d, want %d", eventCount, numWorkers*20)
	}

	var approvalCount int
	if err := d.db.QueryRow("SELECT COUNT(*) FROM approvals").Scan(&approvalCount); err != nil {
		t.Fatalf("count approvals: %v", err)
	}
	if approvalCount != numWorkers*20 {
		t.Errorf("approvals = %d, want %d", approvalCount, numWorkers*20)
	}
}

// --- fu-wfy: Test ListEvents/SummarizeEvents session filter correctness ---

func TestListEvents_SessionFilter(t *testing.T) {
	d := openTestDB(t)

	// Log events from 3 sessions and 2 sources.
	for i := 0; i < 5; i++ {
		_ = d.LogEvent(testEvent("sess-A", fmt.Sprintf("cmd-a-%d", i), "SAFE", "hook"))
	}
	for i := 0; i < 3; i++ {
		_ = d.LogEvent(testEvent("sess-B", fmt.Sprintf("cmd-b-%d", i), "BLOCKED", "codex"))
	}
	for i := 0; i < 2; i++ {
		_ = d.LogEvent(testEvent("sess-C", fmt.Sprintf("cmd-c-%d", i), "APPROVAL", "hook"))
	}

	// Filter by session-A: should get exactly 5 events.
	eventsA, err := d.ListEvents(EventFilter{Session: "sess-A"})
	if err != nil {
		t.Fatalf("ListEvents sess-A: %v", err)
	}
	if len(eventsA) != 5 {
		t.Fatalf("sess-A events = %d, want 5", len(eventsA))
	}
	for _, e := range eventsA {
		if e.SessionID != "sess-A" {
			t.Errorf("expected sess-A, got session_id=%q", e.SessionID)
		}
	}

	// Filter by session-B: should get exactly 3 events.
	eventsB, err := d.ListEvents(EventFilter{Session: "sess-B"})
	if err != nil {
		t.Fatalf("ListEvents sess-B: %v", err)
	}
	if len(eventsB) != 3 {
		t.Fatalf("sess-B events = %d, want 3", len(eventsB))
	}

	// Filter by nonexistent session: should get 0.
	eventsNone, err := d.ListEvents(EventFilter{Session: "sess-Z"})
	if err != nil {
		t.Fatalf("ListEvents sess-Z: %v", err)
	}
	if len(eventsNone) != 0 {
		t.Fatalf("sess-Z events = %d, want 0", len(eventsNone))
	}

	// No filter: should get all 10.
	all, err := d.ListEvents(EventFilter{})
	if err != nil {
		t.Fatalf("ListEvents all: %v", err)
	}
	if len(all) != 10 {
		t.Fatalf("all events = %d, want 10", len(all))
	}
}

func TestListEvents_CombinedFilters(t *testing.T) {
	d := openTestDB(t)

	_ = d.LogEvent(testEvent("sess-A", "safe-hook", "SAFE", "hook"))
	_ = d.LogEvent(testEvent("sess-A", "blocked-hook", "BLOCKED", "hook"))
	_ = d.LogEvent(testEvent("sess-A", "safe-codex", "SAFE", "codex"))
	_ = d.LogEvent(testEvent("sess-B", "safe-hook", "SAFE", "hook"))

	// Session=A + Decision=SAFE.
	events, err := d.ListEvents(EventFilter{Session: "sess-A", Decision: "SAFE"})
	if err != nil {
		t.Fatalf("ListEvents: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("events = %d, want 2 (SAFE events in sess-A)", len(events))
	}

	// Session=A + Source=hook.
	events, err = d.ListEvents(EventFilter{Session: "sess-A", Source: "hook"})
	if err != nil {
		t.Fatalf("ListEvents: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("events = %d, want 2 (hook events in sess-A)", len(events))
	}

	// Session=A + Source=hook + Decision=SAFE.
	events, err = d.ListEvents(EventFilter{Session: "sess-A", Source: "hook", Decision: "SAFE"})
	if err != nil {
		t.Fatalf("ListEvents: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("events = %d, want 1", len(events))
	}
}

func TestSummarizeEvents_AggregatesAll(t *testing.T) {
	d := openTestDB(t)

	// 10 SAFE hook events + 5 BLOCKED codex events.
	for i := 0; i < 10; i++ {
		_ = d.LogEvent(testEvent("sess-A", fmt.Sprintf("cmd-%d", i), "SAFE", "hook"))
	}
	for i := 0; i < 5; i++ {
		_ = d.LogEvent(testEvent("sess-B", fmt.Sprintf("cmd-%d", i), "BLOCKED", "codex"))
	}

	summary, err := d.SummarizeEvents()
	if err != nil {
		t.Fatalf("SummarizeEvents: %v", err)
	}
	if summary.Total != 15 {
		t.Fatalf("total = %d, want 15", summary.Total)
	}
	if summary.ByDecision["SAFE"] != 10 {
		t.Errorf("ByDecision[SAFE] = %d, want 10", summary.ByDecision["SAFE"])
	}
	if summary.ByDecision["BLOCKED"] != 5 {
		t.Errorf("ByDecision[BLOCKED] = %d, want 5", summary.ByDecision["BLOCKED"])
	}
	if summary.BySource["hook"] != 10 {
		t.Errorf("BySource[hook] = %d, want 10", summary.BySource["hook"])
	}
	if summary.BySource["codex"] != 5 {
		t.Errorf("BySource[codex] = %d, want 5", summary.BySource["codex"])
	}
}

func TestSummarizeEvents_FullTable(t *testing.T) {
	d := openTestDB(t)

	// Insert more events than any reasonable limit.
	for i := 0; i < 200; i++ {
		_ = d.LogEvent(testEvent("s", fmt.Sprintf("cmd-%d", i), "SAFE", "hook"))
	}

	// SummarizeEvents must aggregate ALL events, not a capped subset.
	summary, err := d.SummarizeEvents()
	if err != nil {
		t.Fatalf("SummarizeEvents: %v", err)
	}
	if summary.Total != 200 {
		t.Fatalf("total = %d, want 200 (aggregation must cover full table)", summary.Total)
	}
}

// --- fu-uvi: Test session-scoped approval isolation ---

func TestSessionApprovalIsolation(t *testing.T) {
	d := openTestDB(t)
	expires := time.Now().Add(1 * time.Hour)

	// Create session-scoped approvals for 3 different sessions, same decision_key.
	for i, sess := range []string{"sess-1", "sess-2", "sess-3"} {
		id := fmt.Sprintf("a-%d", i)
		if err := d.CreateApproval(id, "shared-key", "APPROVAL", "session", sess, "hmac", &expires); err != nil {
			t.Fatalf("CreateApproval %s: %v", sess, err)
		}
	}

	// Each session should only see its own approval.
	for _, sess := range []string{"sess-1", "sess-2", "sess-3"} {
		a, err := d.ConsumeApproval("shared-key", sess)
		if err != nil {
			t.Fatalf("ConsumeApproval %s: %v", sess, err)
		}
		if a == nil {
			t.Fatalf("session %s got nil approval for shared-key", sess)
		}
		if a.SessionID != sess {
			t.Errorf("session %s got approval for session %s", sess, a.SessionID)
		}
	}

	// An unknown session should NOT get any approval.
	a, err := d.ConsumeApproval("shared-key", "sess-unknown")
	if err != nil {
		t.Fatalf("ConsumeApproval unknown: %v", err)
	}
	if a != nil {
		t.Error("unknown session got an approval, expected nil")
	}
}

func TestSessionApprovalConcurrentIsolation(t *testing.T) {
	dbPath := initSharedDB(t)

	const numSessions = 20
	expires := time.Now().Add(1 * time.Hour)

	// Seed the DB with one session-scoped approval per session.
	d, err := OpenDB(dbPath)
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	for i := 0; i < numSessions; i++ {
		sess := fmt.Sprintf("sess-%d", i)
		id := fmt.Sprintf("a-%d", i)
		if err := d.CreateApproval(id, "deploy-key", "APPROVAL", "session", sess, "hmac", &expires); err != nil {
			t.Fatalf("CreateApproval %s: %v", sess, err)
		}
	}
	d.Close()

	// Concurrently consume from all sessions.
	var wg sync.WaitGroup
	results := make([]*Approval, numSessions)
	errors := make([]error, numSessions)

	for i := 0; i < numSessions; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			conn, err := OpenDB(dbPath)
			if err != nil {
				errors[idx] = err
				return
			}
			defer conn.Close()

			sess := fmt.Sprintf("sess-%d", idx)
			results[idx], errors[idx] = conn.ConsumeApproval("deploy-key", sess)
		}(i)
	}

	wg.Wait()

	for i := 0; i < numSessions; i++ {
		if errors[i] != nil {
			t.Fatalf("session %d: %v", i, errors[i])
		}
		if results[i] == nil {
			t.Fatalf("session %d: got nil approval", i)
		}
		expected := fmt.Sprintf("sess-%d", i)
		if results[i].SessionID != expected {
			t.Errorf("session %d: got approval for session %s", i, results[i].SessionID)
		}
	}
}

// --- fu-zmg: Test once-scope approval race: only one consumer wins ---

func TestOnceApprovalRace(t *testing.T) {
	dbPath := initSharedDB(t)

	expires := time.Now().Add(1 * time.Hour)

	// Create a single once-scope approval.
	d, err := OpenDB(dbPath)
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	if err := d.CreateApproval("race-1", "race-key", "APPROVAL", "once", "", "hmac", &expires); err != nil {
		t.Fatalf("CreateApproval: %v", err)
	}
	d.Close()

	const numRacers = 20
	var wg sync.WaitGroup
	var wins atomic.Int64

	for i := 0; i < numRacers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			conn, err := OpenDB(dbPath)
			if err != nil {
				t.Errorf("racer OpenDB: %v", err)
				return
			}
			defer conn.Close()

			a, err := conn.ConsumeApproval("race-key", "")
			if err != nil {
				t.Errorf("racer ConsumeApproval: %v", err)
				return
			}
			if a != nil {
				wins.Add(1)
			}
		}()
	}

	wg.Wait()

	winCount := wins.Load()
	if winCount != 1 {
		t.Fatalf("once-scope approval consumed by %d racers, want exactly 1", winCount)
	}
}

func TestOnceApprovalRace_Repeated(t *testing.T) {
	// Run the race multiple times to increase confidence.
	for trial := 0; trial < 10; trial++ {
		t.Run(fmt.Sprintf("trial-%d", trial), func(t *testing.T) {
			dbPath := initSharedDB(t)

			expires := time.Now().Add(1 * time.Hour)
			d, err := OpenDB(dbPath)
			if err != nil {
				t.Fatalf("OpenDB: %v", err)
			}
			if err := d.CreateApproval("r", "rk", "APPROVAL", "once", "", "h", &expires); err != nil {
				t.Fatalf("CreateApproval: %v", err)
			}
			d.Close()

			const numRacers = 10
			var wg sync.WaitGroup
			var wins atomic.Int64

			for i := 0; i < numRacers; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					conn, err := OpenDB(dbPath)
					if err != nil {
						return
					}
					defer conn.Close()
					a, _ := conn.ConsumeApproval("rk", "")
					if a != nil {
						wins.Add(1)
					}
				}()
			}

			wg.Wait()

			if w := wins.Load(); w != 1 {
				t.Fatalf("trial %d: %d winners, want 1", trial, w)
			}
		})
	}
}
