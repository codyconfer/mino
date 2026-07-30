package plugin

import (
	"fmt"
	"sort"
	"sync"
)

type Diagnostic struct {
	PluginID string
	Kind     Kind
	Ref      string
	Message  string
}

func (d Diagnostic) String() string {
	who := d.PluginID
	if who == "" {
		who = "<unidentified plugin>"
	}
	if d.Kind != "" && d.Ref != "" {
		return fmt.Sprintf("%s (%s %q): %s", who, d.Kind, d.Ref, d.Message)
	}
	return fmt.Sprintf("%s: %s", who, d.Message)
}

var (
	diagMu sync.RWMutex
	diags  []Diagnostic
)

func NoteDiagnostic(pluginID string, kind Kind, ref, message string) {
	noteDiagnostic(Diagnostic{PluginID: pluginID, Kind: kind, Ref: ref, Message: message})
}

func noteDiagnosticf(pluginID string, kind Kind, ref, format string, args ...any) {
	noteDiagnostic(Diagnostic{
		PluginID: pluginID,
		Kind:     kind,
		Ref:      ref,
		Message:  fmt.Sprintf(format, args...),
	})
}

func noteDiagnostic(d Diagnostic) {
	diagMu.Lock()
	defer diagMu.Unlock()
	for _, have := range diags {
		if have == d {
			return
		}
	}
	diags = append(diags, d)
}

func Diagnostics() []Diagnostic {
	diagMu.RLock()
	out := make([]Diagnostic, len(diags))
	copy(out, diags)
	diagMu.RUnlock()
	out = append(out, capabilityDiagnostics()...)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].PluginID != out[j].PluginID {
			return lessMenuID(out[i].PluginID, out[j].PluginID)
		}
		return out[i].Message < out[j].Message
	})
	return out
}

func DiagnosticsFor(pluginID string) []Diagnostic {
	var out []Diagnostic
	for _, d := range Diagnostics() {
		if d.PluginID == pluginID {
			out = append(out, d)
		}
	}
	return out
}

func HasDiagnostics() bool { return len(Diagnostics()) > 0 }

// Guarded runs one plugin's registration behind its own recover so a panic in
// that plugin's code becomes a diagnostic instead of truncating every plugin
// that would have registered after it. Compose a host's RegisterPlugins hook out
// of Guarded calls to get per-plugin containment.
func Guarded(pluginID string, register func()) {
	if register == nil {
		return
	}
	defer func() {
		if r := recover(); r != nil {
			noteDiagnosticf(pluginID, "", "",
				"registration panicked: %v; this plugin's registration was truncated, so its remaining contributions were skipped", r)
		}
	}()
	register()
}

func capabilityDiagnostics() []Diagnostic {
	var out []Diagnostic
	for _, d := range AllOfKind(KindSignal) {
		if d.Signal == "" || IsInternal(OwnerID(d)) {
			continue
		}
		if descriptorHasCapability(d, CapScheduled) && !HasScheduledBuilder(d.Signal) {
			out = append(out, Diagnostic{
				PluginID: d.ID,
				Kind:     KindSignal,
				Ref:      d.Signal,
				Message:  "declares CapScheduled but registered no Builders.Scheduled, so the job can never run",
			})
		}
	}
	// The inverse mistake is equally likely and equally fatal: both
	// serve's scheduledJobs and build.ScheduledSignals gate on the
	// capability, so a builder without CapScheduled is dead code.
	for _, signal := range ScheduledSignals() {
		d, ok := BySignal(signal)
		if !ok || IsInternal(OwnerID(d)) {
			continue
		}
		if !descriptorHasCapability(d, CapScheduled) {
			out = append(out, Diagnostic{
				PluginID: d.ID,
				Kind:     KindSignal,
				Ref:      signal,
				Message:  "registered Builders.Scheduled but the descriptor omits CapScheduled, so the job can never run",
			})
		}
	}
	return out
}
