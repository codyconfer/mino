package views

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"

	vkdeck "github.com/codyconfer/viewkit/deck"
	"github.com/codyconfer/viewkit/keys"

	"github.com/codyconfer/munin/internal/app"
	"github.com/codyconfer/munin/internal/keymap"
	"github.com/codyconfer/munin/internal/plugin/ntr"
)

type roleLifecycleSettleMsg struct{ gen uint64 }

func (k *Kit) KeyHook() vkdeck.KeyHook {
	return func(m *vkdeck.Model, key tea.KeyMsg) (tea.Cmd, bool) {
		binds := k.keybinds()
		target, ok := keys.Resolve(binds, key.String())
		if !ok {
			return nil, false
		}
		cmd := k.openHotkeyTarget(m, target)
		return cmd, cmd != nil || isRoleCycleTarget(target)
	}
}

func (k *Kit) MsgHook() vkdeck.MsgHook {
	return func(m *vkdeck.Model, msg tea.Msg) (tea.Cmd, bool) {
		s, ok := msg.(roleLifecycleSettleMsg)
		if !ok {
			return nil, false
		}
		if k.d.App == nil || !k.d.App.SettleRoleCycle(s.gen) {
			return nil, true
		}
		return m.RefreshStatus(), true
	}
}

func isRoleCycleTarget(target string) bool {
	return target == keymap.TargetRoleNext || target == keymap.TargetRolePrev
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
	case keymap.TargetRoleNext:
		return k.cycleRoleCmd(1)
	case keymap.TargetRolePrev:
		return k.cycleRoleCmd(-1)
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

func (k *Kit) cycleRoleCmd(delta int) tea.Cmd {
	if k.d.App == nil || k.d.App.Directives == nil {
		return nil
	}
	next, ok := app.NextRole(k.d.App.Directives.RoleNames(), k.d.App.Cfg.Role, delta)
	if !ok {
		return nil
	}
	gen, changed := k.d.App.BeginRoleCycle(next)
	if !changed {
		return nil
	}
	return tea.Tick(app.RoleCycleDebounce, func(time.Time) tea.Msg {
		return roleLifecycleSettleMsg{gen: gen}
	})
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
		{keymap.TargetRolePrev, "prev role"},
		{keymap.TargetRoleNext, "next role"},
	}
	var out [][2]string
	for _, o := range order {
		if o.target == keymap.TargetRemindNew && !ntr.RemindersUIVisible() {
			continue
		}
		for key, target := range binds {
			if target == o.target {
				out = append(out, [2]string{keys.Normalize(key), o.label})
				break
			}
		}
	}
	return out
}
