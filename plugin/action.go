package plugin

import (
	"context"
	"fmt"
	"sort"
	"sync"
)

// ActionFunc is a named write/side-effect capability implementation.
type ActionFunc func(ctx context.Context, params map[string]string) error

// ActionSpec describes a registered action under a signal.
type ActionSpec struct {
	Signal string
	Name   string
	Run    ActionFunc
}

var (
	actionMu sync.RWMutex
	actions  = map[string]ActionSpec{}
)

func actionKey(signal, name string) string { return signal + "\x00" + name }

// RegisterAction registers a CapAction implementation for signal/name and
// links a KindAction companion descriptor (Ref = "signal/name") once the
// KindSignal plugin is registered. Optional [Option] values
// (e.g. [WithServiceOnly]) configure the companion descriptor.
// Panics on duplicate.
func RegisterAction(signal, name string, run ActionFunc, opts ...Option) {
	if signal == "" || name == "" || run == nil {
		panic("plugin: RegisterAction requires signal, name, and run")
	}
	d := Descriptor{}
	applyOptions(&d, opts)
	actionMu.Lock()
	k := actionKey(signal, name)
	if _, ok := actions[k]; ok {
		actionMu.Unlock()
		panic(fmt.Sprintf("plugin: duplicate action %s/%s", signal, name))
	}
	actions[k] = ActionSpec{Signal: signal, Name: name, Run: run}
	actionMu.Unlock()

	mu.Lock()
	queueActionKindLocked(signal, name, d.ServiceOnly)
	mu.Unlock()
}

// LookupAction returns a registered action.
func LookupAction(signal, name string) (ActionSpec, bool) {
	actionMu.RLock()
	defer actionMu.RUnlock()
	a, ok := actions[actionKey(signal, name)]
	return a, ok
}

// ActionsFor lists actions registered for signal, sorted by name.
func ActionsFor(signal string) []ActionSpec {
	actionMu.RLock()
	defer actionMu.RUnlock()
	var out []ActionSpec
	for _, a := range actions {
		if a.Signal == signal {
			out = append(out, a)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

var signalEnabledFn func(signal string) bool

// SetSignalEnabledFunc wires host enablement checks for [RunAction].
// Stock munin calls this from internal/plugin init; overlays rarely need it.
func SetSignalEnabledFunc(fn func(signal string) bool) {
	signalEnabledFn = fn
}

// RunAction executes a registered action.
func RunAction(ctx context.Context, signal, name string, params map[string]string) error {
	a, ok := LookupAction(signal, name)
	if !ok {
		return fmt.Errorf("unknown action %s/%s", signal, name)
	}
	if signalEnabledFn != nil && !signalEnabledFn(signal) {
		return fmt.Errorf("signal %q is disabled", signal)
	}
	if !HasCapability(signal, CapAction) {
		return fmt.Errorf("signal %q does not advertise CapAction", signal)
	}
	if params == nil {
		params = map[string]string{}
	}
	return a.Run(ctx, params)
}
