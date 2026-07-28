package plugin

import (
	"sync"

	"github.com/codyconfer/munin/internal/config"
	"github.com/codyconfer/munin/internal/errs"
)

var (
	enableMu  sync.RWMutex
	disabled  = map[string]bool{}
	installed = map[string]bool{}
	loaded    bool
)

func LoadEnabled() {
	gs := config.LoadGlobalSettings()
	enableMu.Lock()
	defer enableMu.Unlock()
	disabled = map[string]bool{}
	for _, id := range gs.DisabledPlugins {
		disabled[id] = true
	}
	installed = map[string]bool{}
	for _, id := range gs.InstalledPlugins {
		installed[id] = true
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

func Enabled(id string) bool {
	ensureLoaded()
	d, ok := Lookup(id)
	if !ok {
		return false
	}
	owner := OwnerID(d)
	enableMu.RLock()
	defer enableMu.RUnlock()
	return !disabled[owner]
}

func Installed(id string) bool {
	ensureLoaded()
	d, ok := Lookup(id)
	if !ok {
		return false
	}
	owner := OwnerID(d)
	if IsInternal(owner) {
		return true
	}
	enableMu.RLock()
	defer enableMu.RUnlock()
	return installed[owner]
}

func SignalEnabled(signal string) bool {
	d, ok := BySignal(signal)
	if !ok {
		return false
	}
	return Enabled(d.ID)
}

func primaryDescriptor(id string) (Descriptor, error) {
	d, ok := Lookup(id)
	if !ok {
		return Descriptor{}, errs.Newf(errs.KindConfig, "unknown plugin %q", id)
	}
	if d.Parent != "" {
		return Descriptor{}, errs.Newf(errs.KindConfig, "plugin %q is a %s companion of %q; enable/disable the parent", id, d.Kind, d.Parent)
	}
	return d, nil
}

func savePluginSets(gs config.GlobalSettings) error {
	if err := config.SaveGlobalSettings(gs); err != nil {
		return err
	}
	LoadEnabled()
	return nil
}

func withDisabled(gs config.GlobalSettings, id string, on bool) config.GlobalSettings {
	var next []string
	for _, x := range gs.DisabledPlugins {
		if x != id {
			next = append(next, x)
		}
	}
	if !on {
		next = append(next, id)
	}
	gs.DisabledPlugins = next
	return gs
}

func withInstalled(gs config.GlobalSettings, id string, want bool) config.GlobalSettings {
	var next []string
	for _, x := range gs.InstalledPlugins {
		if x != id {
			next = append(next, x)
		}
	}
	if want {
		next = append(next, id)
	}
	gs.InstalledPlugins = next
	return gs
}

func SetEnabled(id string, on bool) error {
	if _, err := primaryDescriptor(id); err != nil {
		return err
	}
	gs := withDisabled(config.LoadGlobalSettings(), id, on)
	return savePluginSets(gs)
}

func MarkInstalled(id string) error {
	if _, err := primaryDescriptor(id); err != nil {
		return err
	}
	gs := withInstalled(config.LoadGlobalSettings(), id, true)
	return savePluginSets(gs)
}

func ClearInstalled(id string) error {
	if _, err := primaryDescriptor(id); err != nil {
		return err
	}
	gs := withInstalled(config.LoadGlobalSettings(), id, false)
	return savePluginSets(gs)
}

func setInstalledEnabled(id string, installedOn, enabledOn bool) error {
	if _, err := primaryDescriptor(id); err != nil {
		return err
	}
	gs := config.LoadGlobalSettings()
	gs = withInstalled(gs, id, installedOn)
	gs = withDisabled(gs, id, enabledOn)
	return savePluginSets(gs)
}

func ListEnabled() []struct {
	ID      string
	Enabled bool
} {
	ensureLoaded()
	all := Primaries()
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

func ListInstalled() []struct {
	ID      string
	Enabled bool
} {
	ensureLoaded()
	all := Primaries()
	out := make([]struct {
		ID      string
		Enabled bool
	}, 0, len(all))
	for _, d := range all {
		if !Installed(d.ID) {
			continue
		}
		out = append(out, struct {
			ID      string
			Enabled bool
		}{ID: d.ID, Enabled: Enabled(d.ID)})
	}
	return out
}
