package plugin

import (
	"context"
	"time"
)

// Kind identifies a registered contribution surface.
// Host routing indexes Descriptor.Ref (or Signal for KindSignal) and verify
// checks each Kind against its host registry:
//
//	KindSignal  → builders (+ Cap* bindings)
//	KindFilter  → RegisterFilter / RegisterFilterEngine (Ref = filter name)
//	KindAction  → RegisterAction (Ref = "signal/name")
//	KindView    → RegisterView / viewkit deck (Ref = view id)
//	KindTheme   → RegisterTheme / viewkit theme (Ref = theme key)
//	KindContext → RegisterContext (Ref = tool name)
type Kind string

const (
	KindSignal  Kind = "signal"
	KindFilter  Kind = "filter"
	KindAction  Kind = "action"
	KindView    Kind = "view"
	KindTheme   Kind = "theme"
	KindContext Kind = "context"
)

// Capability names the signal capability model.
type Capability string

const (
	CapQuery     Capability = "query"
	CapAction    Capability = "action"
	CapStream    Capability = "stream"
	CapScheduled Capability = "scheduled"
)

// Query is the passive fetch capability.
type Query interface {
	Name() string
	Fetch(ctx context.Context) ([]Section, error)
}

// Stream is the active streaming capability.
type Stream interface {
	Name() string
	Stream(ctx context.Context) (<-chan Event, error)
	LatencyFloor() time.Duration
}

// Action is a write/side-effect capability plugins may expose.
type Action interface {
	Name() string
	Run(ctx context.Context, params map[string]string) error
}

// Scheduled is the time-triggered capability.
type Scheduled interface {
	Name() string
	Next(ctx context.Context, now time.Time) (due time.Time, ready bool, err error)
	Fetch(ctx context.Context) ([]Section, error)
}

// Descriptor describes a compile-time registered plugin contribution.
type Descriptor struct {
	ID           string
	Kind         Kind
	Capabilities []Capability
	// Signal is the config signal name when Kind is KindSignal.
	Signal string
	// Ref is the host contribution key for non-signal kinds
	// (filter name, "signal/action", view id, theme key, context tool).
	Ref string
	// Parent is the owning primary plugin id for companion contributions
	// (views, actions, contexts, …). Empty means this descriptor is primary
	// for enable/disable listing.
	Parent string
	// ServiceOnly marks contributions that belong to serve/daemon mode.
	// They remain registered for host routing and verification, but
	// interactive UI lists omit them unless a live service is attached
	// (see [UIVisible], [WithServiceOnly]).
	ServiceOnly bool
}

// KnownKinds lists every Kind the host routes and verifies.
func KnownKinds() []Kind {
	return []Kind{KindSignal, KindFilter, KindAction, KindView, KindTheme, KindContext}
}

// ValidKind reports whether k is a routed contribution surface.
func ValidKind(k Kind) bool {
	switch k {
	case KindSignal, KindFilter, KindAction, KindView, KindTheme, KindContext:
		return true
	default:
		return false
	}
}
