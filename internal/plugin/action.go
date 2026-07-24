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
	actions  = map[string]ActionSpec{} // key: signal + "\x00" + name
)

func actionKey(signal, name string) string { return signal + "\x00" + name }

// RegisterAction registers a CapAction implementation for signal/name.
// Panics on duplicate. Call from plugin init alongside Descriptor registration.
func RegisterAction(signal, name string, run ActionFunc) {
	if signal == "" || name == "" || run == nil {
		panic("plugin: RegisterAction requires signal, name, and run")
	}
	actionMu.Lock()
	defer actionMu.Unlock()
	k := actionKey(signal, name)
	if _, ok := actions[k]; ok {
		panic(fmt.Sprintf("plugin: duplicate action %s/%s", signal, name))
	}
	actions[k] = ActionSpec{Signal: signal, Name: name, Run: run}
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

// RunAction executes a registered action.
func RunAction(ctx context.Context, signal, name string, params map[string]string) error {
	a, ok := LookupAction(signal, name)
	if !ok {
		return fmt.Errorf("unknown action %s/%s", signal, name)
	}
	if !SignalEnabled(signal) {
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
