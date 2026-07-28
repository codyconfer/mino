package plugin

import (
	"context"
	"fmt"
	"sort"
	"sync"
)

type ActionFunc func(ctx context.Context, params map[string]string) error

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

func LookupAction(signal, name string) (ActionSpec, bool) {
	actionMu.RLock()
	defer actionMu.RUnlock()
	a, ok := actions[actionKey(signal, name)]
	return a, ok
}

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

func SetSignalEnabledFunc(fn func(signal string) bool) {
	signalEnabledFn = fn
}

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
