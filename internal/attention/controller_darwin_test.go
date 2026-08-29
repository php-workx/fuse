//go:build darwin

package attention

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net"
	"sync"
	"testing"
	"time"
)

func TestDarwinServiceOwnerCompetitionAndHandoff(t *testing.T) {
	lock := &memoryOwnerLock{}
	firstStarter := newScriptedStarter()
	first := newServiceForTest(testDependencies(lock, firstStarter))
	t.Cleanup(func() { _ = first.Close(context.Background()) })
	if got := first.Status().Ownership; got != OwnershipPrimary { t.Fatalf("first ownership = %q, want primary", got) }
	secondStarter := newScriptedStarter()
	second := newServiceForTest(testDependencies(lock, secondStarter))
	t.Cleanup(func() { _ = second.Close(context.Background()) })
	if got := second.Status().Ownership; got != OwnershipSecondary { t.Fatalf("second ownership = %q, want secondary", got) }
	if secondStarter.Starts() != 0 { t.Fatalf("secondary started helper %d times", secondStarter.Starts()) }
	if err := first.Close(context.Background()); err != nil { t.Fatalf("first close: %v", err) }
	if err := second.retryOwnership(context.Background()); err != nil { t.Fatalf("handoff: %v", err) }
	if got := second.Status().Ownership; got != OwnershipPrimary { t.Fatalf("handoff ownership = %q, want primary", got) }
	if secondStarter.Starts() != 1 { t.Fatalf("handoff starts = %d, want 1", secondStarter.Starts()) }
}

func TestDarwinServiceRetainsSubmittedIDAcrossPreAcknowledgementCrash(t *testing.T) {
	starter := newScriptedStarter(func(connection *scriptedTransport) { connection.closeAfter("post") })
	service := newServiceForTest(testDependencies(&memoryOwnerLock{}, starter))
	t.Cleanup(func() { _ = service.Close(context.Background()) })
	if err := service.Post(context.Background(), Notice{ID: "pre-ack", Title: "title"}); err == nil { t.Fatal("post after transport crash = nil, want error") }
	waitFor(t, func() bool { return starter.Starts() == 2 })
	if !service.isSubmitted("pre-ack") { t.Fatal("submitted ID lost after helper restart") }
}

func TestDarwinServiceRecordsAcceptedThenDeliveredAfterCrash(t *testing.T) {
	starter := newScriptedStarter(func(connection *scriptedTransport) {
		connection.respond("post", "accepted")
		connection.emitAfter("post", map[string]any{"version": 1, "type": "posted", "request_id": 2, "id": "post-ack"})
		connection.closeAfter("post")
	})
	service := newServiceForTest(testDependencies(&memoryOwnerLock{}, starter))
	t.Cleanup(func() { _ = service.Close(context.Background()) })
	if err := service.Post(context.Background(), Notice{ID: "post-ack", Title: "title"}); err != nil { t.Fatalf("post: %v", err) }
	waitFor(t, func() bool { return service.isDelivered("post-ack") && starter.Starts() == 2 })
	if !service.isSubmitted("post-ack") { t.Fatal("accepted ID was removed after helper crash") }
}

func TestDarwinServiceDisablesAfterSecondTransportFailure(t *testing.T) {
	starter := newScriptedStarter(func(connection *scriptedTransport) { connection.closeAfter("post") }, func(connection *scriptedTransport) { connection.closeAfter("hello") })
	service := newServiceForTest(testDependencies(&memoryOwnerLock{}, starter))
	t.Cleanup(func() { _ = service.Close(context.Background()) })
	if err := service.Post(context.Background(), Notice{ID: "broken", Title: "title"}); err == nil { t.Fatal("post on broken transport = nil, want error") }
	waitFor(t, func() bool { return service.Status().Ownership == OwnershipDisabled })
	if got := service.Status().DegradedReason; got != "helper exited twice" { t.Fatalf("degraded reason = %q", got) }
}

func TestDarwinServiceDoesNotSignalUnverifiedStalePID(t *testing.T) {
	lock := &memoryOwnerLock{previousPID: 91}
	var signals int
	dependencies := testDependencies(lock, newScriptedStarter())
	dependencies.verifyHelperPID = func(int) bool { return false }
	dependencies.signalPID = func(int) error { signals++; return nil }
	service := newServiceForTest(dependencies)
	t.Cleanup(func() { _ = service.Close(context.Background()) })
	if signals != 0 { t.Fatalf("signalled unverified PID %d times", signals) }
	if got := service.Status().Ownership; got != OwnershipDisabled { t.Fatalf("ownership = %q, want disabled", got) }
}

func TestDarwinServiceReturnsFailedReconciliation(t *testing.T) {
	starter := newScriptedStarter(func(connection *scriptedTransport) { connection.respondError("reconcile", "notification failure") })
	service := newServiceForTest(testDependencies(&memoryOwnerLock{}, starter))
	t.Cleanup(func() { _ = service.Close(context.Background()) })
	if _, err := service.Reconcile(context.Background(), []string{"pending"}); err == nil { t.Fatal("reconcile = nil error, want helper failure") }
}

func TestDarwinServiceShutdownOrdersCleanupBeforeUnlock(t *testing.T) {
	lock := &memoryOwnerLock{}
	starter := newScriptedStarter()
	service := newServiceForTest(testDependencies(lock, starter))
	if err := service.Close(context.Background()); err != nil { t.Fatalf("close: %v", err) }
	if got, want := starter.Commands(), []string{"hello", "remove_all", "shutdown"}; !sameStrings(got, want) { t.Fatalf("commands = %#v, want %#v", got, want) }
	if lock.releases != 1 { t.Fatalf("lock released %d times, want 1", lock.releases) }
}

func TestDarwinServiceSuppressesCustomFuseHome(t *testing.T) {
	starter := newScriptedStarter()
	dependencies := testDependencies(&memoryOwnerLock{}, starter)
	dependencies.fuseHome, dependencies.defaultFuseHome = "/tmp/other-fuse", "/Users/test/.fuse"
	service := newServiceForTest(dependencies)
	t.Cleanup(func() { _ = service.Close(context.Background()) })
	if got := service.Status().Ownership; got != OwnershipDisabled { t.Fatalf("ownership = %q, want disabled", got) }
	if starter.Starts() != 0 { t.Fatalf("custom home started helper %d times", starter.Starts()) }
}

func TestDarwinServiceClosesEvents(t *testing.T) {
	service := newServiceForTest(testDependencies(&memoryOwnerLock{}, newScriptedStarter()))
	events := service.Events()
	for {
		select { case <-events: continue; default: }
		break
	}
	if err := service.Close(context.Background()); err != nil { t.Fatalf("close: %v", err) }
	for {
		select {
		case _, ok := <-events:
			if !ok { return }
		case <-time.After(time.Second): t.Fatal("events did not close")
		}
	}
}

func TestNewSuppressesCustomFuseHomeBeforeHelperDiscovery(t *testing.T) {
	t.Setenv("FUSE_HOME", t.TempDir())
	t.Setenv("FUSE_NOTIFIER_ALLOW_CUSTOM_HOME", "")
	service, err := New(context.Background(), Options{})
	if err != nil { t.Fatalf("New: %v", err) }
	t.Cleanup(func() { _ = service.Close(context.Background()) })
	if got := service.Status().Ownership; got != OwnershipDisabled { t.Fatalf("ownership = %q, want disabled", got) }
}

func TestDarwinServiceSerializesConcurrentPosts(t *testing.T) {
	service := newServiceForTest(testDependencies(&memoryOwnerLock{}, newScriptedStarter()))
	t.Cleanup(func() { _ = service.Close(context.Background()) })
	var group sync.WaitGroup
	for index := range 16 {
		group.Add(1)
		go func() {
			defer group.Done()
			if err := service.Post(context.Background(), Notice{ID: string(rune('a' + index)), Title: "title"}); err != nil { t.Errorf("post: %v", err) }
		}()
	}
	group.Wait()
}

func testDependencies(lock ownerLock, starter *scriptedStarter) serviceDependencies {
	return serviceDependencies{lock: lock, starter: starter, fuseHome: "/Users/test/.fuse", defaultFuseHome: "/Users/test/.fuse", restartDelay: func(context.Context, time.Duration) error { return nil }, verifyHelperPID: func(int) bool { return true }, signalPID: func(int) error { return nil }, waitForExit: func(context.Context, int, time.Duration) error { return nil }}
}

func waitFor(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) { if condition() { return }; time.Sleep(time.Millisecond) }
	t.Fatal("condition was not met")
}

func sameStrings(got, want []string) bool {
	if len(got) != len(want) { return false }
	for index := range got { if got[index] != want[index] { return false } }
	return true
}

type memoryOwnerLock struct {
	mu sync.Mutex
	held bool
	previousPID int
	releases int
}

func (lock *memoryOwnerLock) TryAcquire() (bool, error) {
	lock.mu.Lock()
	defer lock.mu.Unlock()
	if lock.held { return false, nil }
	lock.held = true
	return true, nil
}

func (lock *memoryOwnerLock) PreviousHelperPID() int {
	lock.mu.Lock()
	defer lock.mu.Unlock()
	return lock.previousPID
}

func (lock *memoryOwnerLock) SetHelperPID(pid int) error {
	lock.mu.Lock()
	defer lock.mu.Unlock()
	lock.previousPID = pid
	return nil
}

func (lock *memoryOwnerLock) Release() error {
	lock.mu.Lock()
	defer lock.mu.Unlock()
	if lock.held { lock.held = false; lock.releases++ }
	return nil
}

type scriptedStarter struct {
	mu sync.Mutex
	scripts []func(*scriptedTransport)
	starts int
	transports []*scriptedTransport
}

func newScriptedStarter(scripts ...func(*scriptedTransport)) *scriptedStarter {
	return &scriptedStarter{scripts: scripts}
}

func (starter *scriptedStarter) Start(context.Context, TerminalTargetMetadata) (io.ReadWriteCloser, helperProcess, error) {
	starter.mu.Lock()
	index := starter.starts
	starter.starts++
	starter.mu.Unlock()
	transport := newScriptedTransport()
	starter.mu.Lock()
	starter.transports = append(starter.transports, transport)
	starter.mu.Unlock()
	if index < len(starter.scripts) { starter.scripts[index](transport) }
	transport.start()
	return transport, scriptedProcess{pid: 100 + index}, nil
}

func (starter *scriptedStarter) Starts() int { starter.mu.Lock(); defer starter.mu.Unlock(); return starter.starts }
func (starter *scriptedStarter) Commands() []string {
	starter.mu.Lock()
	transports := append([]*scriptedTransport(nil), starter.transports...)
	starter.mu.Unlock()
	var commands []string
	for _, transport := range transports { commands = append(commands, transport.commandsSnapshot()...) }
	return commands
}

type scriptedProcess struct { pid int }
func (process scriptedProcess) PID() int { return process.pid }
func (process scriptedProcess) Wait() error { return nil }

type scriptedTransport struct {
	client net.Conn
	server net.Conn
	mu sync.Mutex
	commands []string
	responses map[string]string
	errors map[string]string
	after map[string][]map[string]any
	closeAfterOperation string
}

func newScriptedTransport() *scriptedTransport {
	client, server := net.Pipe()
	return &scriptedTransport{client: client, server: server, responses: make(map[string]string), errors: make(map[string]string), after: make(map[string][]map[string]any)}
}

func (transport *scriptedTransport) Read(data []byte) (int, error) { return transport.client.Read(data) }
func (transport *scriptedTransport) Write(data []byte) (int, error) { return transport.client.Write(data) }
func (transport *scriptedTransport) Close() error { _ = transport.server.Close(); return transport.client.Close() }
func (transport *scriptedTransport) closeRead() { _ = transport.server.Close() }
func (transport *scriptedTransport) closeAfter(operation string) { transport.closeAfterOperation = operation }
func (transport *scriptedTransport) respond(operation, status string) { transport.responses[operation] = status }
func (transport *scriptedTransport) respondError(operation, message string) { transport.errors[operation] = message }
func (transport *scriptedTransport) emitAfter(operation string, message map[string]any) { transport.after[operation] = append(transport.after[operation], message) }
func (transport *scriptedTransport) emit(message map[string]any) { contents, _ := json.Marshal(message); _, _ = transport.server.Write(append(contents, '\n')) }

func (transport *scriptedTransport) start() {
	go func() {
		scanner := bufio.NewScanner(transport.server)
		for scanner.Scan() {
			var message protocolMessage
			if err := json.Unmarshal(scanner.Bytes(), &message); err != nil { return }
			transport.mu.Lock()
			transport.commands = append(transport.commands, message.Type)
			status, overridden := transport.responses[message.Type]
			failure := transport.errors[message.Type]
			after := append([]map[string]any(nil), transport.after[message.Type]...)
			closeAfter := transport.closeAfterOperation == message.Type
			transport.mu.Unlock()
			if failure != "" {
				transport.emit(map[string]any{"version": 1, "type": "error", "request_id": message.RequestID, "operation": message.Type, "id": message.ID, "code": "test_failure", "message": failure})
			} else if !closeAfter || overridden {
				if !overridden { status = defaultStatus(message.Type) }
				transport.respondTo(message, status)
			}
			for _, event := range after { transport.emit(event) }
			if closeAfter { transport.closeRead(); return }
		}
	}()
}

func (transport *scriptedTransport) respondTo(message protocolMessage, status string) {
	switch message.Type {
	case "hello":
		transport.emit(map[string]any{"version": 1, "type": "ready", "request_id": message.RequestID, "helper_version": "test", "protocol_min": 1, "protocol_max": 1, "pid": 100, "authorization": "authorized", "alert_setting": "enabled", "sound_setting": "enabled", "target": "exact"})
	case "reconcile":
		transport.emit(map[string]any{"version": 1, "type": "reconciled", "request_id": message.RequestID, "delivered_current_ids": message.CurrentIDs})
	case "focus":
		transport.emit(map[string]any{"version": 1, "type": "focus_result", "request_id": message.RequestID, "mode": "exact", "detail": "focused"})
	case "shutdown":
		transport.emit(map[string]any{"version": 1, "type": "shutdown_complete"})
	default:
		transport.emit(map[string]any{"version": 1, "type": "result", "request_id": message.RequestID, "operation": message.Type, "id": message.ID, "status": status})
	}
}

func (transport *scriptedTransport) commandsSnapshot() []string { transport.mu.Lock(); defer transport.mu.Unlock(); return append([]string(nil), transport.commands...) }
func defaultStatus(operation string) string { if operation == "post" { return "accepted" }; return "ok" }
func newServiceForTest(dependencies serviceDependencies) *darwinService { dependencies.clientVersion = "test"; return newController(dependencies) }
