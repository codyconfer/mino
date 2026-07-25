package plugin

import (
	"context"
	"fmt"
	"sync"
)

// BuildContext is the host-provided environment for Query/Stream builders.
// Overlay plugins should depend only on these methods. Host implementations
// may offer additional optional interfaces (for example [TokenSource]).
type BuildContext interface {
	Params() map[string]string
	Home() string
	Role() string
}

// TokenSource is an optional [BuildContext] extension for sealed credentials.
type TokenSource interface {
	GetToken(ctx context.Context, service string) (accessToken, scope string, ok bool, err error)
}

// QueryFunc constructs a [Query] for a signal.
type QueryFunc func(ctx BuildContext) (Query, error)

// StreamFunc constructs a [Stream] for a signal.
type StreamFunc func(ctx BuildContext) (Stream, error)

// Builders wires construction for a config signal name.
// Registering builders is what makes verify/host construction succeed —
// descriptors alone are not enough.
type Builders struct {
	Query  QueryFunc
	Stream StreamFunc
}

var (
	buildMu  sync.RWMutex
	builders = map[string]Builders{}
)

// RegisterBuilders associates Query/Stream constructors with a config signal
// name. Panics on duplicate signal or empty builders. Call from plugin init
// alongside [Register], or use [RegisterSignal].
func RegisterBuilders(signal string, b Builders) {
	if signal == "" {
		panic("plugin: RegisterBuilders requires signal")
	}
	if b.Query == nil && b.Stream == nil {
		panic("plugin: RegisterBuilders requires Query and/or Stream")
	}
	buildMu.Lock()
	defer buildMu.Unlock()
	if _, ok := builders[signal]; ok {
		panic(fmt.Sprintf("plugin: duplicate builders for signal %q", signal))
	}
	builders[signal] = b
}

// RegisterSignal registers a descriptor and its builders together.
func RegisterSignal(d Descriptor, b Builders) {
	Register(d)
	if d.Signal == "" {
		panic("plugin: RegisterSignal requires Descriptor.Signal")
	}
	RegisterBuilders(d.Signal, b)
}

// LookupBuilders returns builders for signal.
func LookupBuilders(signal string) (Builders, bool) {
	buildMu.RLock()
	defer buildMu.RUnlock()
	b, ok := builders[signal]
	return b, ok
}

// HasBuilder reports whether signal has a Query builder.
func HasBuilder(signal string) bool {
	b, ok := LookupBuilders(signal)
	return ok && b.Query != nil
}

// HasStreamBuilder reports whether signal has a Stream builder.
func HasStreamBuilder(signal string) bool {
	b, ok := LookupBuilders(signal)
	return ok && b.Stream != nil
}

// BuilderSignals returns signal names with any registered builders.
func BuilderSignals() map[string]bool {
	buildMu.RLock()
	defer buildMu.RUnlock()
	out := make(map[string]bool, len(builders))
	for name := range builders {
		out[name] = true
	}
	return out
}

// BuildQuery constructs the Query for signal using bc.
func BuildQuery(signal string, bc BuildContext) (Query, error) {
	b, ok := LookupBuilders(signal)
	if !ok || b.Query == nil {
		return nil, fmt.Errorf("plugin: no query builder for signal %q", signal)
	}
	return b.Query(bc)
}

// BuildStream constructs the Stream for signal using bc.
func BuildStream(signal string, bc BuildContext) (Stream, error) {
	b, ok := LookupBuilders(signal)
	if !ok || b.Stream == nil {
		return nil, fmt.Errorf("plugin: no stream builder for signal %q", signal)
	}
	return b.Stream(bc)
}
