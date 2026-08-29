//go:build !darwin

package attention

import "context"

type unsupportedService struct { events chan Event }

func New(context.Context, Options) (Service, error) {
	events := make(chan Event)
	close(events)
	return unsupportedService{events: events}, nil
}

func (service unsupportedService) Post(context.Context, Notice) error { return nil }
func (service unsupportedService) Remove(context.Context, string) error { return nil }
func (service unsupportedService) Reconcile(context.Context, []string) (ReconcileResult, error) { return ReconcileResult{}, nil }
func (service unsupportedService) Focus(context.Context) (FocusResult, error) { return FocusResult{Mode: FocusApplicationFallback, Detail: "native notifications are unsupported on this platform"}, nil }
func (service unsupportedService) Events() <-chan Event { return service.events }
func (service unsupportedService) Status() Status { return Status{Ownership: OwnershipUnsupported, Authorization: AuthorizationUnknown, AlertSetting: SettingUnknown, SoundSetting: SettingUnknown, Target: TargetUnknown, Transport: TransportNone, DegradedReason: "native notifications are unsupported on this platform"} }
func (service unsupportedService) Close(context.Context) error { return nil }
