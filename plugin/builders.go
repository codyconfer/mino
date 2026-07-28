package plugin

import (
	"context"
	"fmt"
	"sync"
)

type BuildContext interface {
	Params() map[string]string
	Home() string
	Role() string
}

type TokenSource interface {
	GetToken(ctx context.Context, service string) (accessToken, scope string, ok bool, err error)
}

type QueryFunc func(ctx BuildContext) (Query, error)

type StreamFunc func(ctx BuildContext) (Stream, error)

type Builders struct {
	Query  QueryFunc
	Stream StreamFunc
}

var (
	buildMu  sync.RWMutex
	builders = map[string]Builders{}
)

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

func RegisterSignal(d Descriptor, b Builders) {
	Register(d)
	if d.Signal == "" {
		panic("plugin: RegisterSignal requires Descriptor.Signal")
	}
	RegisterBuilders(d.Signal, b)
}

func LookupBuilders(signal string) (Builders, bool) {
	buildMu.RLock()
	defer buildMu.RUnlock()
	b, ok := builders[signal]
	return b, ok
}

func HasBuilder(signal string) bool {
	b, ok := LookupBuilders(signal)
	return ok && b.Query != nil
}

func HasStreamBuilder(signal string) bool {
	b, ok := LookupBuilders(signal)
	return ok && b.Stream != nil
}

func BuilderSignals() map[string]bool {
	buildMu.RLock()
	defer buildMu.RUnlock()
	out := make(map[string]bool, len(builders))
	for name := range builders {
		out[name] = true
	}
	return out
}

func BuildQuery(signal string, bc BuildContext) (Query, error) {
	b, ok := LookupBuilders(signal)
	if !ok || b.Query == nil {
		return nil, fmt.Errorf("plugin: no query builder for signal %q", signal)
	}
	return b.Query(bc)
}

func BuildStream(signal string, bc BuildContext) (Stream, error) {
	b, ok := LookupBuilders(signal)
	if !ok || b.Stream == nil {
		return nil, fmt.Errorf("plugin: no stream builder for signal %q", signal)
	}
	return b.Stream(bc)
}
