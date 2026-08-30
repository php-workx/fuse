package tui

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/php-workx/fuse/internal/attention"
	"github.com/php-workx/fuse/internal/db"
)

func openTestDB(t *testing.T) *db.DB {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	d, err := db.OpenDB(dbPath)
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	return d
}

func seedEvents(t *testing.T, d *db.DB, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		_ = d.LogEvent(&db.EventRecord{
			Command:  "echo test",
			Decision: "SAFE",
			Source:   "hook",
			Agent:    "claude",
		})
	}
}

func TestNewModelRetainsNotificationOptions(t *testing.T) {
	d := openTestDB(t)
	options := NewNotificationOptions(false, true)

	m := NewModel(d, "enabled", nil, options, nil)

	if m.notifications.Enabled() {
		t.Fatal("notification options enabled = true, want false")
	}
	if !m.notifications.Sound() {
		t.Fatal("notification options sound = false, want true")
	}
}

func TestModel_InitReturnsTick(t *testing.T) {
	d := openTestDB(t)
	m := NewModel(d, "dryrun", nil, NotificationOptions{}, nil)
	cmd := m.Init()
	if cmd == nil {
		t.Fatal("Init() should return a tick command, got nil")
	}
}

func TestModel_TickFetchesData(t *testing.T) {
	d := openTestDB(t)
	seedEvents(t, d, 3)

	m := NewModel(d, "enabled", nil, NotificationOptions{}, nil)
	m.width = 120
	m.height = 40
	m.events.SetSize(120, 37)

	// Simulate tick.
	updated, cmd := m.Update(tickMsg{view: viewEvents})
	m = updated.(Model)

	if !m.fetching {
		t.Error("after tick, fetching should be true")
	}
	if cmd == nil {
		t.Fatal("tick should return fetchData command")
	}

	// Execute the fetch command synchronously.
	msg := cmd()
	updated, cmd = m.Update(msg)
	m = updated.(Model)

	if m.fetching {
		t.Error("after dataMsg, fetching should be false")
	}
	if len(m.events.events) != 3 {
		t.Errorf("events: got %d, want 3", len(m.events.events))
	}
	if cmd == nil {
		t.Fatal("dataMsg should schedule next tick")
	}
}

func TestModel_StaleResultDiscarded(t *testing.T) {
	d := openTestDB(t)
	seedEvents(t, d, 1)

	m := NewModel(d, "enabled", nil, NotificationOptions{}, nil)
	m.width = 120
	m.height = 40
	m.events.SetSize(120, 37)

	// Simulate: fetchGen=0, start fetch.
	updated, cmd := m.Update(tickMsg{view: viewEvents})
	m = updated.(Model)

	// User switches view → fetchGen increments.
	m.fetchGen++

	// Stale result arrives with reqGen=0.
	staleMsg := dataMsg{reqGen: 0, view: viewEvents, events: []db.EventRecord{{ID: 999}}}
	updated, _ = m.Update(staleMsg)
	m = updated.(Model)

	// Events should NOT be updated (stale).
	if len(m.events.events) != 0 {
		t.Errorf("stale result should be discarded, got %d events", len(m.events.events))
	}
	_ = cmd
}

func TestModel_ViewSwitching(t *testing.T) {
	d := openTestDB(t)
	m := NewModel(d, "dryrun", nil, NotificationOptions{}, nil)
	m.width = 80
	m.height = 24

	if m.activeView != viewEvents {
		t.Fatal("initial view should be events")
	}

	// Press Tab.
	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	m = updated.(Model)
	if m.activeView != viewStats {
		t.Errorf("after Tab: got %d, want viewStats", m.activeView)
	}

	// Press Tab again.
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	m = updated.(Model)
	if m.activeView != viewApprovals {
		t.Errorf("after Tab: got %d, want viewApprovals", m.activeView)
	}

	// Tab on approvals view cycles to events (no longer trapped).
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	m = updated.(Model)
	if m.activeView != viewEvents {
		t.Errorf("Tab on approvals should cycle to events: got %d, want viewEvents", m.activeView)
	}

	// Number key '1' also goes to events.
	updated, _ = m.Update(tea.KeyPressMsg{Code: '1'})
	m = updated.(Model)
	if m.activeView != viewEvents {
		t.Errorf("after key '1': got %d, want viewEvents", m.activeView)
	}
}

func TestModel_ErrorPreservesData(t *testing.T) {
	d := openTestDB(t)
	seedEvents(t, d, 2)

	m := NewModel(d, "enabled", nil, NotificationOptions{}, nil)
	m.width = 120
	m.height = 40
	m.events.SetSize(120, 37)

	// First successful fetch.
	updated, cmd := m.Update(tickMsg{view: viewEvents})
	m = updated.(Model)
	msg := cmd()
	updated, _ = m.Update(msg)
	m = updated.(Model)

	if len(m.events.events) != 2 {
		t.Fatalf("initial fetch: got %d events, want 2", len(m.events.events))
	}

	// Simulate error fetch.
	errMsg := dataMsg{reqGen: m.fetchGen, view: viewEvents, err: os.ErrClosed}
	updated, _ = m.Update(errMsg)
	m = updated.(Model)

	// Data should be preserved.
	if len(m.events.events) != 2 {
		t.Errorf("after error, events should be preserved: got %d", len(m.events.events))
	}
	if m.lastErr == nil {
		t.Error("lastErr should be set after error fetch")
	}
}

func TestModel_DetailPausesPolling(t *testing.T) {
	d := openTestDB(t)
	m := NewModel(d, "dryrun", nil, NotificationOptions{}, nil)
	m.width = 80
	m.height = 24
	m.events.SetSize(80, 21)
	m.events.events = []db.EventRecord{{ID: 1, Decision: "SAFE", Command: "test"}}
	m.events.applyFilters()

	// Open detail panel.
	m.events.showDetail = true

	// Tick should NOT trigger a fetch.
	updated, cmd := m.Update(tickMsg{view: viewEvents})
	m = updated.(Model)

	if m.fetching {
		t.Error("tick while detail open should not set fetching=true")
	}
	if cmd == nil {
		t.Fatal("tick while detail open should still reschedule")
	}
}

func TestModel_ResizePropagates(t *testing.T) {
	d := openTestDB(t)
	m := NewModel(d, "dryrun", nil, NotificationOptions{}, nil)

	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 50})
	m = updated.(Model)

	if m.width != 100 || m.height != 50 {
		t.Errorf("resize: got %dx%d, want 100x50", m.width, m.height)
	}
	if m.events.width != 100 {
		t.Errorf("events width: got %d, want 100", m.events.width)
	}
}

// --- tickCmd tests ---

func TestTickCmd_EventsInterval(t *testing.T) {
	cmd := tickCmd(viewEvents)
	if cmd == nil {
		t.Fatal("tickCmd(viewEvents) should return a non-nil Cmd")
	}
}

func TestTickCmd_ApprovalsInterval(t *testing.T) {
	cmd := tickCmd(viewApprovals)
	if cmd == nil {
		t.Fatal("tickCmd(viewApprovals) should return a non-nil Cmd")
	}
}

func TestTickCmd_StatsInterval(t *testing.T) {
	cmd := tickCmd(viewStats)
	if cmd == nil {
		t.Fatal("tickCmd(viewStats) should return a non-nil Cmd")
	}
}

// --- Stale tick discard tests ---

func TestModel_StaleTickReschedules(t *testing.T) {
	d := openTestDB(t)
	m := NewModel(d, "dryrun", nil, NotificationOptions{}, nil)
	m.width = 80
	m.height = 24
	m.events.SetSize(80, 21)

	// Switch to stats view.
	m.activeView = viewStats

	// Send a tick for events (stale since we're on stats).
	updated, cmd := m.Update(tickMsg{view: viewEvents})
	m = updated.(Model)

	// Should NOT have started a fetch (stale tick).
	if m.fetching {
		t.Error("stale tick should not trigger a fetch")
	}

	// Should reschedule a tick (non-nil cmd).
	if cmd == nil {
		t.Fatal("stale tick should reschedule, got nil cmd")
	}
}

func TestModel_StaleTickDoesNotFetch(t *testing.T) {
	d := openTestDB(t)
	seedEvents(t, d, 2)

	m := NewModel(d, "dryrun", nil, NotificationOptions{}, nil)
	m.width = 80
	m.height = 24

	// Switch to approvals view.
	m.activeView = viewApprovals

	// Send tick for events view (wrong view).
	updated, _ := m.Update(tickMsg{view: viewEvents})
	m = updated.(Model)

	// Events should remain empty since we never fetched for events.
	if len(m.events.events) != 0 {
		t.Errorf("stale tick should not fetch events data, got %d events", len(m.events.events))
	}
}

type fakeAttentionService struct {
	events       chan attention.Event
	status       attention.Status
	postErr      error
	reconcileErr error
	focusErr     error
	closeErr     error
	operations   []string
	notices      []attention.Notice
}

func newFakeAttentionService() *fakeAttentionService {
	return &fakeAttentionService{
		events: make(chan attention.Event),
		status: attention.Status{Ownership: attention.OwnershipPrimary},
	}
}

func (s *fakeAttentionService) Post(_ context.Context, notice attention.Notice) error {
	s.operations = append(s.operations, "post:"+notice.ID)
	s.notices = append(s.notices, notice)
	return s.postErr
}

func (s *fakeAttentionService) Remove(_ context.Context, id string) error {
	s.operations = append(s.operations, "remove:"+id)
	return nil
}

func (s *fakeAttentionService) Reconcile(_ context.Context, _ []string) (attention.ReconcileResult, error) {
	s.operations = append(s.operations, "reconcile")
	return attention.ReconcileResult{}, s.reconcileErr
}

func (s *fakeAttentionService) Focus(context.Context) (attention.FocusResult, error) {
	s.operations = append(s.operations, "focus")
	return attention.FocusResult{}, s.focusErr
}

func (s *fakeAttentionService) Events() <-chan attention.Event { return s.events }
func (s *fakeAttentionService) Status() attention.Status     { return s.status }

func (s *fakeAttentionService) Close(context.Context) error {
	s.operations = append(s.operations, "close")
	return s.closeErr
}

func TestModel_GlobalPendingPollUpdatesApprovalsOutsideApprovalsView(t *testing.T) {
	d := openTestDB(t)
	request := db.PendingRequest{ID: "pending-1", DecisionKey: "decision-1", Command: "rm -rf", Source: "hook", WorkspaceRoot: "/work/repo", CreatedAt: "2026-03-20T12:00:00Z"}
	if err := d.InsertPendingRequest(request); err != nil {
		t.Fatalf("InsertPendingRequest: %v", err)
	}
	notifier := newFakeAttentionService()
	m := NewModel(d, "enabled", nil, NewNotificationOptions(true, true), notifier)
	m.activeView = viewStats

	updated, cmd := m.Update(cleanupMsg{})
	m = updated.(Model)
	if !m.pendingFetching {
		t.Fatal("cleanup completion should start the first global pending fetch")
	}
	if cmd == nil {
		t.Fatal("cleanup completion should return global pending fetch command")
	}

	updated, cmd = m.Update(pendingFetchMsg{reqGen: m.pendingGen, pending: []db.PendingRequest{request}})
	m = updated.(Model)
	if len(m.pending) != 1 || m.pending[0].ID != request.ID {
		t.Fatalf("global pending snapshot = %#v, want %q", m.pending, request.ID)
	}
	if len(m.approvals.pending) != 1 || m.approvals.pending[0].ID != request.ID {
		t.Fatal("approvals view did not receive global pending snapshot")
	}
	if cmd == nil {
		t.Fatal("successful pending fetch should schedule notification work and next poll")
	}
}

func TestModel_GlobalPendingErrorPreservesSnapshotAndDoesNotRemove(t *testing.T) {
	d := openTestDB(t)
	notifier := newFakeAttentionService()
	m := NewModel(d, "enabled", nil, NewNotificationOptions(true, true), notifier)
	m.pending = []db.PendingRequest{{ID: "still-pending"}}
	m.approvals.SetPending(m.pending)
	m.pendingGen = 7
	m.pendingFetching = true

	updated, _ := m.Update(pendingFetchMsg{reqGen: 7, err: os.ErrClosed})
	m = updated.(Model)
	if len(m.pending) != 1 || m.pending[0].ID != "still-pending" {
		t.Fatalf("pending snapshot after DB error = %#v, want preserved request", m.pending)
	}
	if m.pendingErr == nil {
		t.Fatal("pendingErr = nil after DB error")
	}
	if len(notifier.operations) != 0 {
		t.Fatalf("notification operations after failed DB fetch = %v, want none", notifier.operations)
	}
}

func TestModel_ActivationSelectsPendingByIDAndFocuses(t *testing.T) {
	d := openTestDB(t)
	request := db.PendingRequest{ID: "pending-1", DecisionKey: "decision-1", Command: "rm -rf", Source: "hook", CreatedAt: "2026-03-20T12:00:00Z"}
	if err := d.InsertPendingRequest(request); err != nil {
		t.Fatalf("InsertPendingRequest: %v", err)
	}
	notifier := newFakeAttentionService()
	m := NewModel(d, "enabled", nil, NewNotificationOptions(true, true), notifier)
	m.pending = []db.PendingRequest{request}
	m.approvals.SetPending(m.pending)

	updated, cmd := m.Update(attentionEventMsg{event: attention.Event{Kind: attention.EventActivated, PendingID: request.ID}, ok: true})
	m = updated.(Model)
	msg := activationLookupMessage(t, cmd)
	updated, cmd = m.Update(msg)
	m = updated.(Model)
	if m.activeView != viewApprovals || m.approvals.pendingID != request.ID {
		t.Fatalf("activation selected %q in view %d, want %q in approvals", m.approvals.pendingID, m.activeView, request.ID)
	}
	if cmd == nil {
		t.Fatal("resolved activation should schedule terminal focus")
	}
}

func TestModel_ResolvedActivationClosesPendingModal(t *testing.T) {
	d := openTestDB(t)
	notifier := newFakeAttentionService()
	m := NewModel(d, "enabled", nil, NewNotificationOptions(true, true), notifier)
	m.approvals.scopeSelect = true

	updated, cmd := m.Update(attentionEventMsg{event: attention.Event{Kind: attention.EventActivated, PendingID: "resolved"}, ok: true})
	m = updated.(Model)
	updated, _ = m.Update(activationLookupMessage(t, cmd))
	m = updated.(Model)

	if m.approvals.scopeSelect {
		t.Error("scopeSelect = true after activation for resolved request")
	}
	if m.approvals.statusMsg != "Request already resolved." {
		t.Errorf("statusMsg = %q, want resolved message", m.approvals.statusMsg)
	}
}

func TestModel_AttentionFailureDoesNotSetDatabaseError(t *testing.T) {
	d := openTestDB(t)
	notifier := newFakeAttentionService()
	m := NewModel(d, "enabled", nil, NewNotificationOptions(true, true), notifier)

	updated, _ := m.Update(attentionResultMsg{err: os.ErrPermission, status: notifier.status})
	m = updated.(Model)
	if m.notificationErr == nil {
		t.Fatal("notificationErr = nil after notification failure")
	}
	if m.lastErr != nil || m.pendingErr != nil {
		t.Fatalf("notification failure contaminated database errors: last=%v pending=%v", m.lastErr, m.pendingErr)
	}
}

func TestModel_ClosedAttentionEventsAreNotRearmed(t *testing.T) {
	d := openTestDB(t)
	m := NewModel(d, "enabled", nil, NewNotificationOptions(true, true), newFakeAttentionService())

	updated, cmd := m.Update(attentionEventMsg{})
	m = updated.(Model)
	if !m.attentionEventsClosed {
		t.Fatal("closed attention event channel was not recorded")
	}
	if cmd != nil {
		t.Fatal("closed attention event channel should not be re-armed")
	}
}

func activationLookupMessage(t *testing.T, cmd tea.Cmd) tea.Msg {
	t.Helper()
	msg := cmd()
	batch, ok := msg.(tea.BatchMsg)
	if !ok {
		return msg
	}
	if len(batch) != 2 {
		t.Fatalf("activation command batch has %d commands, want event re-arm and lookup", len(batch))
	}
	return batch[1]()
}

func TestModel_GlobalPendingDiffPostsRemovesAndReconcilesInOrder(t *testing.T) {
	d := openTestDB(t)
	notifier := newFakeAttentionService()
	m := NewModel(d, "enabled", nil, NewNotificationOptions(true, true), notifier)
	m.pending = []db.PendingRequest{{ID: "removed"}}
	m.pendingGen = 1
	m.pendingFetching = true
	current := db.PendingRequest{
		ID:            "added",
		Command:       "sensitive command",
		Reason:        "sensitive reason",
		Source:        "hook",
		WorkspaceRoot: "/Users/test/workspace",
	}

	updated, cmd := m.Update(pendingFetchMsg{reqGen: 1, pending: []db.PendingRequest{current}})
	m = updated.(Model)
	batch, ok := cmd().(tea.BatchMsg)
	if !ok || len(batch) != 2 {
		t.Fatalf("pending update command = %T with %d commands, want notification sync and next poll", cmd(), len(batch))
	}
	updated, _ = m.Update(batch[0]())
	m = updated.(Model)

	if got, want := notifier.operations, []string{"post:added", "remove:removed", "reconcile"}; !reflect.DeepEqual(got, want) {
		t.Errorf("notification operations = %v, want %v", got, want)
	}
	if len(notifier.notices) != 1 {
		t.Fatalf("notices = %d, want 1", len(notifier.notices))
	}
	notice := notifier.notices[0]
	if notice.Title != "Fuse approval required" || notice.Subtitle != "workspace" || notice.Body != "Source: hook" || !notice.Sound {
		t.Errorf("notice = %#v, want generic title, workspace label, source, and sound", notice)
	}
	if containsStr(notice.Body, current.Command) || containsStr(notice.Body, current.Reason) {
		t.Errorf("notice leaks sensitive pending data: %#v", notice)
	}
}

func TestModel_RepeatedActivationKeepsSelectionByID(t *testing.T) {
	d := openTestDB(t)
	request := db.PendingRequest{ID: "pending-1", DecisionKey: "decision-1", Command: "rm -rf", Source: "hook", CreatedAt: "2026-03-20T12:00:00Z"}
	if err := d.InsertPendingRequest(request); err != nil {
		t.Fatalf("InsertPendingRequest: %v", err)
	}
	m := NewModel(d, "enabled", nil, NewNotificationOptions(true, true), newFakeAttentionService())
	m.pending = []db.PendingRequest{request}
	m.approvals.SetPending(m.pending)

	for range 2 {
		updated, cmd := m.Update(attentionEventMsg{event: attention.Event{Kind: attention.EventActivated, PendingID: request.ID}, ok: true})
		m = updated.(Model)
		updated, _ = m.Update(activationLookupMessage(t, cmd))
		m = updated.(Model)
	}
	if m.approvals.pendingID != request.ID || m.approvals.pendingIdx != 0 {
		t.Errorf("repeated activation selection = %q at %d, want %q at 0", m.approvals.pendingID, m.approvals.pendingIdx, request.ID)
	}
}

func TestModel_QuitClosesAttentionService(t *testing.T) {
	d := openTestDB(t)
	notifier := newFakeAttentionService()
	program := tea.NewProgram(
		NewModel(d, "enabled", nil, NewNotificationOptions(true, true), notifier),
		tea.WithInput(nil),
		tea.WithOutput(io.Discard),
		tea.WithoutRenderer(),
		tea.WithoutSignalHandler(),
	)
	done := make(chan error, 1)
	go func() {
		_, err := program.Run()
		done <- err
	}()

	program.Send(tea.KeyPressMsg{Code: 'q'})
	if err := <-done; err != nil {
		t.Fatalf("Program.Run: %v", err)
	}
	if got, want := notifier.operations, []string{"close"}; !reflect.DeepEqual(got, want) {
		t.Errorf("notification operations = %v, want %v", got, want)
	}
}

func TestModel_DisabledNotificationsDoNotArmAttentionEvents(t *testing.T) {
	d := openTestDB(t)
	notifier := newFakeAttentionService()
	m := NewModel(d, "enabled", nil, NewNotificationOptions(false, true), notifier)

	updated, cmd := m.Update(cleanupMsg{})
	m = updated.(Model)
	if m.attention != nil {
		t.Fatal("disabled notifications retained an attention service")
	}
	batch, ok := cmd().(tea.BatchMsg)
	if !ok || len(batch) != 2 {
		t.Fatalf("disabled notifications scheduled %d startup commands, want only pending and events fetches", len(batch))
	}
	if _, ok := batch[0]().(pendingFetchMsg); !ok {
		t.Fatalf("first disabled startup command returned %T, want pendingFetchMsg", batch[0]())
	}
	if _, ok := batch[1]().(dataMsg); !ok {
		t.Fatalf("second disabled startup command returned %T, want dataMsg", batch[1]())
	}
}

func TestModel_CleanupStartsDefaultEventsFetchAlongsideGlobalPendingFetch(t *testing.T) {
	d := openTestDB(t)
	seedEvents(t, d, 2)
	m := NewModel(d, "enabled", nil, NewNotificationOptions(false, true), nil)

	updated, cmd := m.Update(cleanupMsg{})
	m = updated.(Model)
	batch, ok := cmd().(tea.BatchMsg)
	if !ok || len(batch) != 2 {
		t.Fatalf("cleanup command = %T with %d commands, want pending and events fetches", cmd(), len(batch))
	}
	msg := batch[1]()
	updated, _ = m.Update(msg)
	m = updated.(Model)
	if len(m.events.events) != 2 {
		t.Fatalf("events after cleanup = %d, want 2", len(m.events.events))
	}
}
