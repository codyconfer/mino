package plugin

import (
	"context"
	"time"
)

type Kind string

const (
	KindSignal  Kind = "signal"
	KindFilter  Kind = "filter"
	KindAction  Kind = "action"
	KindView    Kind = "view"
	KindTheme   Kind = "theme"
	KindContext Kind = "context"
)

type Capability string

const (
	CapQuery     Capability = "query"
	CapAction    Capability = "action"
	CapStream    Capability = "stream"
	CapScheduled Capability = "scheduled"
	// CapCacheable marks a signal whose Fetch results are worth caching between runs.
	// Signals reading local state should omit it so writes show up immediately.
	CapCacheable Capability = "cacheable"
	CapDetail    Capability = "detail"
)

type Query interface {
	Name() string
	Fetch(ctx context.Context) ([]Section, error)
}

type Stream interface {
	Name() string
	Stream(ctx context.Context) (<-chan Event, error)
	LatencyFloor() time.Duration
}

type Action interface {
	Name() string
	Run(ctx context.Context, params map[string]string) error
}

type Scheduled interface {
	Name() string
	Next(ctx context.Context, now time.Time) (due time.Time, ready bool, err error)
	Fetch(ctx context.Context) ([]Section, error)
}

type Detailer interface {
	Name() string
	Detail(ctx context.Context, item Item) (ItemDetail, error)
}

type Descriptor struct {
	ID                 string
	Kind               Kind
	Capabilities       []Capability
	Signal             string
	Ref                string
	Parent             string
	ServiceOnly        bool
	Credentials        []string
	SettingsNamespaces []string
}

func KnownKinds() []Kind {
	return []Kind{KindSignal, KindFilter, KindAction, KindView, KindTheme, KindContext}
}

func ValidKind(k Kind) bool {
	switch k {
	case KindSignal, KindFilter, KindAction, KindView, KindTheme, KindContext:
		return true
	default:
		return false
	}
}
