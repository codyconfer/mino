package plugin

import (
	"sync"

	"github.com/codyconfer/munin/internal/config"
	"github.com/codyconfer/munin/internal/errs"
)

var (
	enableMu sync.RWMutex
	// disabled overrides; empty means all registered plugins are enabled.
	disabled = map[string]bool{}
	loaded   bool
)

// LoadEnabled reads the disabled-plugin set from global settings (ADR-13).
func LoadEnabled() {
	gs := config.LoadGlobalSettings()
	enableMu.Lock()
	defer enableMu.Unlock()
	disabled = map[string]bool{}
	for _, id := range gs.DisabledPlugins {
		disabled[id] = true
	}
	loaded = true
}

func ensureLoaded() {
	enableMu.RLock()
	ok := loaded
	enableMu.RUnlock()
	if !ok {
		LoadEnabled()
	}
}

// Enabled reports whether plugin id is active. Unknown ids are treated as
// disabled for verify clarity.
func Enabled(id string) bool {
	ensureLoaded()
	if _, ok := Lookup(id); !ok {
		return false
	}
	enableMu.RLock()
	defer enableMu.RUnlock()
	return !disabled[id]
}

// SignalEnabled reports whether the plugin backing signal is enabled.
func SignalEnabled(signal string) bool {
	d, ok := BySignal(signal)
	if !ok {
		return false
	}
	return Enabled(d.ID)
}

// SetEnabled persists enable/disable for a registered plugin.
func SetEnabled(id string, on bool) error {
	if _, ok := Lookup(id); !ok {
		return errs.Newf(errs.KindConfig, "unknown plugin %q", id)
	}
	gs := config.LoadGlobalSettings()
	var next []string
	for _, d := range gs.DisabledPlugins {
		if d != id {
			next = append(next, d)
		}
	}
	if !on {
		next = append(next, id)
	}
	gs.DisabledPlugins = next
	if err := config.SaveGlobalSettings(gs); err != nil {
		return err
	}
	LoadEnabled()
	return nil
}

// ListEnabled returns ids and whether each is enabled.
func ListEnabled() []struct {
	ID      string
	Enabled bool
} {
	ensureLoaded()
	all := All()
	out := make([]struct {
		ID      string
		Enabled bool
	}, 0, len(all))
	for _, d := range all {
		out = append(out, struct {
			ID      string
			Enabled bool
		}{ID: d.ID, Enabled: Enabled(d.ID)})
	}
	return out
}
