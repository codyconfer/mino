package views

import (
	tea "github.com/charmbracelet/bubbletea"

	vkdeck "github.com/codyconfer/viewkit/deck"

	"github.com/codyconfer/munin/internal/keymap"
	"github.com/codyconfer/munin/internal/plugin/ntr"
)

// KeyHook returns a deck global key interceptor that opens keybind targets
// (NTR create forms or named flights). Wired via deck.WithKeyHook.
func (k *Kit) KeyHook() vkdeck.KeyHook {
	return func(m *vkdeck.Model, key tea.KeyMsg) (tea.Cmd, bool) {
		binds := k.keybinds()
		target, ok := keymap.ResolveHotkey(binds, key.String())
		if !ok {
			return nil, false
		}
		cmd := k.openHotkeyTarget(m, target)
		return cmd, cmd != nil
	}
}

func (k *Kit) keybinds() map[string]string {
	if k.d.App == nil || k.d.App.Cfg == nil {
		return nil
	}
	return k.d.App.Cfg.Keybinds
}

func (k *Kit) openHotkeyTarget(m *vkdeck.Model, target string) tea.Cmd {
	home, role := k.ntrHomeRole()
	switch target {
	case keymap.TargetNoteNew:
		return m.Push(ntr.NewNoteForm(home, role))
	case keymap.TargetTaskNew:
		return m.Push(ntr.NewTaskForm(home, role))
	case keymap.TargetRemindNew:
		if !ntr.RemindersUIVisible() {
			return nil
		}
		return m.Push(ntr.NewRemindForm(home, role))
	}
	name, ok := keymap.FlightTarget(target)
	if !ok || k.d.App == nil || k.d.App.Directives == nil {
		return nil
	}
	if _, exists := k.d.App.Directives.Flights[name]; !exists {
		return nil
	}
	return m.Push(k.FlightResults(name))
}

func (k *Kit) ntrHomeRole() (home, role string) {
	if k.d.App != nil && k.d.App.Cfg != nil {
		home = k.d.App.Cfg.Home
		role = k.d.App.Cfg.Role
	}
	if role == "" {
		role = "default"
	}
	return home, role
}

// hotkeyHints returns footer hints for configured NTR create binds.
func (k *Kit) hotkeyHints() [][2]string {
	binds := k.keybinds()
	if len(binds) == 0 {
		return nil
	}
	order := []struct {
		target, label string
	}{
		{keymap.TargetNoteNew, "new note"},
		{keymap.TargetTaskNew, "new task"},
		{keymap.TargetRemindNew, "new reminder"},
	}
	var out [][2]string
	for _, o := range order {
		if o.target == keymap.TargetRemindNew && !ntr.RemindersUIVisible() {
			continue
		}
		for key, target := range binds {
			if target == o.target {
				out = append(out, [2]string{keymap.NormalizeKey(key), o.label})
				break
			}
		}
	}
	return out
}
