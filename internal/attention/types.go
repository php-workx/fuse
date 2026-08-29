package attention

import "context"

type Service interface {
	Post(context.Context, Notice) error
	Remove(context.Context, string) error
	Reconcile(context.Context, []string) (ReconcileResult, error)
	Focus(context.Context) (FocusResult, error)
	Events() <-chan Event
	Status() Status
	Close(context.Context) error
}

type Notice struct {
	ID string
	Title string
	Subtitle string
	Body string
	Sound bool
}

type EventKind string

const (
	EventActivated EventKind = "activated"
	EventSubmitted EventKind = "submitted"
	EventDelivered EventKind = "delivered"
	EventFailed EventKind = "failed"
	EventRemoved EventKind = "removed"
	EventStatusChanged EventKind = "status_changed"
)

type Event struct {
	Kind EventKind
	PendingID string
	Status Status
	Err error
}

type OwnershipStatus string

const (
	OwnershipPrimary OwnershipStatus = "primary"
	OwnershipSecondary OwnershipStatus = "secondary"
	OwnershipDisabled OwnershipStatus = "disabled"
	OwnershipUnsupported OwnershipStatus = "unsupported"
)

type AuthorizationStatus string
type SettingStatus string
type TargetStatus string
type TransportKind string
type FocusMode string

const (
	AuthorizationUnknown AuthorizationStatus = "unknown"
	SettingUnknown SettingStatus = "unknown"
	TargetUnknown TargetStatus = "unknown"
	TransportNone TransportKind = "none"
	TransportLaunchServices TransportKind = "launchservices_socket"
	FocusExact FocusMode = "exact"
	FocusDispatched FocusMode = "dispatched"
	FocusApplicationFallback FocusMode = "application_fallback"
)

type Status struct {
	Ownership OwnershipStatus
	HelperVersion string
	ProtocolVersion int
	Authorization AuthorizationStatus
	AlertSetting SettingStatus
	SoundSetting SettingStatus
	Target TargetStatus
	Transport TransportKind
	DegradedReason string
}

type FocusResult struct {
	Mode FocusMode
	Detail string
}

type ReconcileResult struct {
	DeliveredIDs []string
	RemovedIDs []string
}

type Options struct {
	ClientVersion string
	Development bool
}
