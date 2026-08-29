//go:build darwin

package attention

import (
	"context"
	"errors"
	"io"
	"sync"
	"time"
)

const requestTimeout = 5 * time.Second
const shutdownTimeout = 10 * time.Second

type ownerLock interface {
	TryAcquire() (bool, error)
	PreviousHelperPID() int
	SetHelperPID(int) error
	Release() error
}

type helperProcess interface {
	PID() int
	Wait() error
}

type helperStarter interface {
	Start(context.Context, TerminalTargetMetadata) (io.ReadWriteCloser, helperProcess, error)
}

type serviceDependencies struct {
	lock ownerLock
	clientVersion string
	starter helperStarter
	fuseHome string
	defaultFuseHome string
	allowCustomHome bool
	terminal TerminalTargetMetadata
	restartDelay func(context.Context, time.Duration) error
	verifyHelperPID func(int) bool
	signalPID func(int) error
	waitForExit func(context.Context, int, time.Duration) error
}

type darwinService struct {
	dependencies serviceDependencies
	mu sync.RWMutex
	status Status
	client *protocolClient
	process helperProcess
	submitted map[string]struct{}
	delivered map[string]struct{}
	failures int
	closing bool
	closed bool
	events chan Event
}

func newController(dependencies serviceDependencies) *darwinService {
	service := &darwinService{
		dependencies: dependencies,
		status: Status{Ownership: OwnershipDisabled, Authorization: AuthorizationUnknown, AlertSetting: SettingUnknown, SoundSetting: SettingUnknown, Target: TargetUnknown, Transport: TransportNone},
		submitted: make(map[string]struct{}),
		delivered: make(map[string]struct{}),
		events: make(chan Event, 128),
	}
	if dependencies.fuseHome != dependencies.defaultFuseHome && !dependencies.allowCustomHome {
		service.setDisabled("native notifications are disabled for custom FUSE_HOME")
		return service
	}
	_ = service.acquire(context.Background())
	return service
}

func (service *darwinService) acquire(context.Context) error {
	if service.dependencies.lock == nil || service.dependencies.starter == nil {
		service.setDisabled("notifier dependencies unavailable")
		return errors.New("notifier dependencies unavailable")
	}
	acquired, err := service.dependencies.lock.TryAcquire()
	if err != nil { service.setDisabled(err.Error()); return err }
	if !acquired {
		service.setStatus(func(status *Status) { status.Ownership = OwnershipSecondary; status.DegradedReason = "another fuse monitor owns notifications" })
		return nil
	}
	return service.startOwned(context.Background())
}

func (service *darwinService) retryOwnership(ctx context.Context) error {
	service.mu.RLock()
	ownership := service.status.Ownership
	closed := service.closed
	service.mu.RUnlock()
	if closed || ownership != OwnershipSecondary { return nil }
	return service.acquire(ctx)
}

func (service *darwinService) startOwned(ctx context.Context) error {
	previousPID := service.dependencies.lock.PreviousHelperPID()
	if previousPID != 0 {
		if service.dependencies.verifyHelperPID == nil || !service.dependencies.verifyHelperPID(previousPID) {
			service.setDisabled("refusing to signal unverified stale notifier process")
			_ = service.dependencies.lock.Release()
			return errors.New("unverified stale notifier process")
		}
		if service.dependencies.signalPID != nil {
			if err := service.dependencies.signalPID(previousPID); err != nil { service.setDisabled(err.Error()); return err }
		}
		if service.dependencies.waitForExit != nil {
			if err := service.dependencies.waitForExit(ctx, previousPID, 3*time.Second); err != nil { service.setDisabled(err.Error()); return err }
		}
	}
	connection, process, err := service.dependencies.starter.Start(ctx, service.dependencies.terminal)
	if err != nil { service.setDisabled(err.Error()); return err }
	client := newProtocolClient(connection, service.handleMessage, service.handleFailure)
	service.mu.Lock()
	if service.closed || service.closing { service.mu.Unlock(); _ = client.close(); return errProtocolClosed }
	service.client, service.process = client, process
	service.status.Ownership = OwnershipPrimary
	service.status.Transport = TransportLaunchServices
	service.status.DegradedReason = ""
	service.mu.Unlock()
	request, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	ready, err := client.call(request, protocolMessage{Type: "hello", ClientVersion: service.dependencies.clientVersion, Terminal: terminalTargetWire(service.dependencies.terminal)})
	if err != nil { return err }
	if ready.Type != "ready" { return errors.New("notifier did not return ready") }
	service.setStatus(func(status *Status) {
		status.HelperVersion = ready.HelperVersion
		status.ProtocolVersion = ready.ProtocolMax
		status.Authorization = AuthorizationStatus(ready.Authorization)
		status.AlertSetting = SettingStatus(ready.AlertSetting)
		status.SoundSetting = SettingStatus(ready.SoundSetting)
		status.Target = TargetStatus(ready.Target)
	})
	if ready.PID != 0 { _ = service.dependencies.lock.SetHelperPID(ready.PID) }
	return nil
}

func (service *darwinService) Post(ctx context.Context, notice Notice) error {
	if notice.ID == "" { return errors.New("notification ID is required") }
	service.mu.Lock()
	if service.status.Ownership != OwnershipPrimary || service.client == nil { service.mu.Unlock(); return nil }
	service.submitted[notice.ID] = struct{}{}
	client := service.client
	service.mu.Unlock()
	service.emit(Event{Kind: EventSubmitted, PendingID: notice.ID, Status: service.Status()})
	request, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	response, err := client.call(request, protocolMessage{Type: "post", ID: notice.ID, Title: trimDisplay(notice.Title), Subtitle: trimDisplay(notice.Subtitle), Body: trimDisplay(notice.Body), Sound: notice.Sound})
	if err != nil { service.emit(Event{Kind: EventFailed, PendingID: notice.ID, Status: service.Status(), Err: err}); return err }
	if response.Type != "result" || response.Operation != "post" || response.Status != "accepted" {
		err := errors.New("notifier rejected post")
		service.emit(Event{Kind: EventFailed, PendingID: notice.ID, Status: service.Status(), Err: err})
		return err
	}
	return nil
}

func (service *darwinService) Remove(ctx context.Context, id string) error {
	service.mu.RLock()
	client := service.client
	primary := service.status.Ownership == OwnershipPrimary
	service.mu.RUnlock()
	if !primary || client == nil { return nil }
	request, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	response, err := client.call(request, protocolMessage{Type: "remove", ID: id})
	if err != nil { return err }
	if response.Type != "result" || response.Operation != "remove" || response.Status != "ok" { return errors.New("notifier rejected remove") }
	service.mu.Lock(); delete(service.submitted, id); delete(service.delivered, id); service.mu.Unlock()
	service.emit(Event{Kind: EventRemoved, PendingID: id, Status: service.Status()})
	return nil
}

func (service *darwinService) Reconcile(ctx context.Context, currentIDs []string) (ReconcileResult, error) {
	service.mu.RLock(); client := service.client; primary := service.status.Ownership == OwnershipPrimary; service.mu.RUnlock()
	if !primary || client == nil { return ReconcileResult{}, nil }
	request, cancel := context.WithTimeout(ctx, requestTimeout); defer cancel()
	response, err := client.call(request, protocolMessage{Type: "reconcile", CurrentIDs: currentIDs})
	if err != nil { return ReconcileResult{}, err }
	if response.Type != "reconciled" { return ReconcileResult{}, errors.New("notifier rejected reconciliation") }
	delivered := append([]string(nil), response.DeliveredCurrentIDs...)
	deliveredSet := make(map[string]struct{}, len(delivered))
	for _, id := range delivered { deliveredSet[id] = struct{}{} }
	removed := make([]string, 0)
	service.mu.Lock()
	for id := range service.delivered { if _, ok := deliveredSet[id]; !ok { removed = append(removed, id) } }
	service.delivered = deliveredSet
	service.mu.Unlock()
	return ReconcileResult{DeliveredIDs: delivered, RemovedIDs: removed}, nil
}

func (service *darwinService) Focus(ctx context.Context) (FocusResult, error) {
	service.mu.RLock(); client := service.client; primary := service.status.Ownership == OwnershipPrimary; service.mu.RUnlock()
	if !primary || client == nil { return FocusResult{Mode: FocusApplicationFallback, Detail: "native notifier unavailable"}, nil }
	request, cancel := context.WithTimeout(ctx, requestTimeout); defer cancel()
	response, err := client.call(request, protocolMessage{Type: "focus"})
	if err != nil { return FocusResult{}, err }
	if response.Type != "focus_result" { return FocusResult{}, errors.New("notifier rejected focus") }
	mode := FocusMode(response.Mode)
	if mode == "" { mode = FocusApplicationFallback }
	return FocusResult{Mode: mode, Detail: response.Detail}, nil
}

func (service *darwinService) Events() <-chan Event { return service.events }

func (service *darwinService) Status() Status { service.mu.RLock(); defer service.mu.RUnlock(); return service.status }

func (service *darwinService) Close(ctx context.Context) error {
	service.mu.Lock()
	if service.closed { service.mu.Unlock(); return nil }
	service.closing = true
	client, process := service.client, service.process
	service.mu.Unlock()
	var first error
	if client != nil {
		request, cancel := context.WithTimeout(ctx, requestTimeout)
		_, err := client.call(request, protocolMessage{Type: "remove_all"})
		cancel()
		if err != nil && !errors.Is(err, errProtocolClosed) { first = err }
		shutdown, cancel := context.WithTimeout(ctx, shutdownTimeout)
		if err := client.shutdownRequest(shutdown); err != nil && first == nil && !errors.Is(err, errProtocolClosed) { first = err }
		cancel()
		if err := client.close(); err != nil && first == nil { first = err }
	}
	if process != nil { _ = process.Wait() }
	if service.dependencies.lock != nil { if err := service.dependencies.lock.Release(); err != nil && first == nil { first = err } }
	service.mu.Lock()
	if !service.closed { service.closed = true; close(service.events) }
	service.mu.Unlock()
	return first
}

func (service *darwinService) handleMessage(message protocolMessage) {
	switch message.Type {
	case "posted":
		service.mu.Lock(); service.delivered[message.ID] = struct{}{}; service.mu.Unlock()
		service.emit(Event{Kind: EventDelivered, PendingID: message.ID, Status: service.Status()})
	case "activated": service.emit(Event{Kind: EventActivated, PendingID: message.ID, Status: service.Status()})
	case "settings_changed":
		service.setStatus(func(status *Status) { status.Authorization = AuthorizationStatus(message.Authorization); status.AlertSetting = SettingStatus(message.AlertSetting); status.SoundSetting = SettingStatus(message.SoundSetting) })
	}
}

func (service *darwinService) handleFailure(error) {
	service.mu.Lock()
	if service.closing || service.closed || service.status.Ownership != OwnershipPrimary { service.mu.Unlock(); return }
	service.failures++
	failures := service.failures
	service.mu.Unlock()
	if failures > 1 { service.setDisabled("helper exited twice"); return }
	go func() {
		if service.dependencies.restartDelay != nil { _ = service.dependencies.restartDelay(context.Background(), time.Second) }
		service.mu.RLock(); stopped := service.closing || service.closed; service.mu.RUnlock()
		if stopped { return }
		if err := service.startOwned(context.Background()); err != nil {
			service.mu.RLock()
			alreadyDisabled := service.status.Ownership == OwnershipDisabled
			service.mu.RUnlock()
			if !alreadyDisabled { service.setDisabled(err.Error()) }
		}
	}()
}

func (service *darwinService) setDisabled(reason string) { service.setStatus(func(status *Status) { status.Ownership = OwnershipDisabled; status.Transport = TransportNone; status.DegradedReason = reason }) }
func (service *darwinService) setStatus(change func(*Status)) { service.mu.Lock(); change(&service.status); status := service.status; closed := service.closed; service.mu.Unlock(); if !closed { service.emit(Event{Kind: EventStatusChanged, Status: status}) } }
func (service *darwinService) emit(event Event) {
	service.mu.RLock()
	defer service.mu.RUnlock()
	if service.closed { return }
	select { case service.events <- event: default: }
}
func (service *darwinService) isSubmitted(id string) bool { service.mu.RLock(); defer service.mu.RUnlock(); _, ok := service.submitted[id]; return ok }
func (service *darwinService) isDelivered(id string) bool { service.mu.RLock(); defer service.mu.RUnlock(); _, ok := service.delivered[id]; return ok }
func boolString(value bool) string { if value { return "true" }; return "false" }
func terminalTargetWire(metadata TerminalTargetMetadata) *terminalWire {
	return &terminalWire{
		Kind: metadata.TerminalProgram,
		ProgramVersion: metadata.TerminalProgramVersion,
		ITermSessionID: metadata.ITermSessionID,
		TTY: metadata.ControllingTTY,
		Marker: metadata.GhosttyTitleMarker,
		Degraded: metadata.Degradation.ExactTargetingDegraded(),
	}
}
