package plugin

import (
	"context"

	"github.com/codyconfer/munin/internal/signals"
)

// Kind identifies a registered contribution surface (ADR-7).
// Host routing today uses KindSignal (+ Cap* for signal capabilities) and
// ContextProvider registration for tool contexts. KindFilter/Action/View/Theme
// are reserved descriptors for future contribution surfaces.
type Kind string

const (
	KindSignal  Kind = "signal"
	KindFilter  Kind = "filter"
	KindAction  Kind = "action"
	KindView    Kind = "view"
	KindTheme   Kind = "theme"
	KindContext Kind = "context"
)

// Capability names the signal capability model (ADR-6 / ADR-10).
type Capability string

const (
	CapQuery     Capability = "query"
	CapAction    Capability = "action"
	CapStream    Capability = "stream"
	CapScheduled Capability = "scheduled"
)

// Query is the passive fetch capability (existing signals.Signal).
type Query = signals.Signal

// Stream is the active streaming capability (existing signals.ActiveSignal).
type Stream = signals.ActiveSignal

// Action is a write/side-effect capability plugins may expose.
type Action interface {
	Name() string
	Run(ctx context.Context, params map[string]string) error
}

// Scheduled is the time-triggered capability (ADR-10).
type Scheduled = signals.Scheduled

// Descriptor describes a compile-time registered plugin contribution.
type Descriptor struct {
	ID           string
	Kind         Kind
	Capabilities []Capability
	// Signal is the config signal name when Kind is KindSignal.
	Signal string
}
