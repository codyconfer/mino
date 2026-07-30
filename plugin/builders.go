package plugin

import (
	"context"
	"fmt"
	"sort"
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

type ScheduledFunc func(ctx BuildContext) (Scheduled, error)

type Builders struct {
	Query     QueryFunc
	Stream    StreamFunc
	Scheduled ScheduledFunc
}

var (
	buildMu       sync.RWMutex
	builders      = map[string]Builders{}
	builderOwners = map[string]string{}
)

// RegisterBuilders is the standalone route: the caller registered its
// Descriptor separately, so the owning plugin is resolved from the signal.
func RegisterBuilders(signal string, b Builders) {
	registerBuilders(descriptorOwner(signal), signal, b)
}

func descriptorOwner(signal string) string {
	if d, ok := BySignal(signal); ok {
		return d.ID
	}
	return ""
}

func registerBuilders(ownerID, signal string, b Builders) bool {
	noteRegistrationCheckpoint(ownerID)
	if signal == "" {
		noteDiagnostic(Diagnostic{
			PluginID: ownerID,
			Message:  "RegisterBuilders requires a signal name; builders skipped",
		})
		return false
	}
	if b.Query == nil && b.Stream == nil && b.Scheduled == nil {
		noteDiagnosticf(ownerID, KindSignal, signal,
			"RegisterBuilders(%q) supplied no Query, Stream, or Scheduled builder; builders skipped", signal)
		return false
	}
	buildMu.Lock()
	_, dup := builders[signal]
	if !dup {
		builders[signal] = b
		builderOwners[signal] = ownerID
	}
	buildMu.Unlock()
	if dup {
		noteDiagnosticf(ownerID, KindSignal, signal,
			"builders for signal %q are already registered by %q; later builders skipped", signal, builderOwner(signal))
		return false
	}
	return true
}

// builderOwner names whoever registered the *incumbent* builders, which is not
// necessarily the descriptor owner for the signal.
func builderOwner(signal string) string {
	buildMu.RLock()
	owner := builderOwners[signal]
	buildMu.RUnlock()
	if owner == "" {
		return "an earlier registration"
	}
	return owner
}

func RegisterSignal(d Descriptor, b Builders) {
	if d.Signal == "" {
		noteDiagnostic(Diagnostic{
			PluginID: d.ID,
			Kind:     d.Kind,
			Message:  "RegisterSignal requires Descriptor.Signal; contribution skipped",
		})
		return
	}
	if !registerDescriptor(d) {
		return
	}
	registerBuilders(d.ID, d.Signal, b)
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

func HasScheduledBuilder(signal string) bool {
	b, ok := LookupBuilders(signal)
	return ok && b.Scheduled != nil
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

func ScheduledSignals() []string {
	buildMu.RLock()
	out := make([]string, 0, len(builders))
	for name, b := range builders {
		if b.Scheduled != nil {
			out = append(out, name)
		}
	}
	buildMu.RUnlock()
	sort.Strings(out)
	return out
}

func BuildQuery(signal string, bc BuildContext) (Query, error) {
	b, ok := LookupBuilders(signal)
	if !ok || b.Query == nil {
		return nil, fmt.Errorf("plugin: no query builder for signal %q", signal)
	}
	q, err := b.Query(bc)
	if err != nil {
		return nil, err
	}
	if isNilRef(q) {
		return nil, fmt.Errorf("plugin: builder for %q returned no query", signal)
	}
	return q, nil
}

func BuildStream(signal string, bc BuildContext) (Stream, error) {
	b, ok := LookupBuilders(signal)
	if !ok || b.Stream == nil {
		return nil, fmt.Errorf("plugin: no stream builder for signal %q", signal)
	}
	s, err := b.Stream(bc)
	if err != nil {
		return nil, err
	}
	if isNilRef(s) {
		return nil, fmt.Errorf("plugin: builder for %q returned no stream", signal)
	}
	return s, nil
}

func BuildScheduled(signal string, bc BuildContext) (Scheduled, error) {
	b, ok := LookupBuilders(signal)
	if !ok || b.Scheduled == nil {
		return nil, fmt.Errorf("plugin: no scheduled builder for signal %q", signal)
	}
	j, err := b.Scheduled(bc)
	if err != nil {
		return nil, err
	}
	if isNilRef(j) {
		return nil, fmt.Errorf("plugin: builder for %q returned no scheduled job", signal)
	}
	return j, nil
}
