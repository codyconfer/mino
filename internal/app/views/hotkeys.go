package views

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"

	vkdeck "github.com/codyconfer/viewkit/deck"
	"github.com/codyconfer/viewkit/keys"

	"github.com/codyconfer/mino/internal/app"
	"github.com/codyconfer/mino/internal/config"
	"github.com/codyconfer/mino/internal/keymap"
	"github.com/codyconfer/mino/internal/log"
	"github.com/codyconfer/mino/internal/plugin/ntr"
)

type roleLifecycleSettleMsg struct{ gen uint64 }

type roleHookStepMsg struct {
	settle *app.RoleSettle
	next   int
}

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
		switch t := msg.(type) {
		case roleLifecycleSettleMsg:
			if k.d.App == nil {
				return nil, true
			}
			settle, ok := k.d.App.BeginRoleSettle(t.gen)
			if !ok {
				return nil, true
			}
			return k.runRoleHookStep(m, settle, 0), true
		case roleHookStepMsg:
			return k.runRoleHookStep(m, t.settle, t.next), true
		case storeTickMsg:
			return tea.Batch(k.onStoreTick(m), k.probeHistory()), true
		case historyProbedMsg:
			k.histKnown, k.histHas = true, t.has
			return nil, true
		}
		return nil, false
	}
}

func (k *Kit) onStoreTick(m *vkdeck.Model) tea.Cmd {
	if !k.d.App.HasStore() {
		return StoreTick()
	}
	if !k.storeChanged() {
		return StoreTick()
	}
	if err := k.d.App.RefreshDirectives(config.ReconcileIgnore); err != nil {
		log.Warnf("reloading directives after an external change: %v", err)
		return StoreTick()
	}
	return tea.Batch(m.RefreshStatus(), reloadCmd(), StoreTick())
}

func (k *Kit) runRoleHookStep(m *vkdeck.Model, settle *app.RoleSettle, i int) tea.Cmd {
	settle.MarkDone(i)
	for ; settle != nil && i < len(settle.Steps); i++ {
		step := settle.Steps[i]
		cmd, err := step.Command()
		if err != nil {
			log.Warnf("role %q %s hooks: %v", step.Role, step.Phase, err)
			continue
		}
		resume := roleHookStepMsg{settle: settle, next: i + 1}
		return tea.ExecProcess(cmd, func(err error) tea.Msg {
			if err != nil {
				log.Warnf("role %q %s hooks: %v", step.Role, step.Phase, err)
			}
			return resume
		})
	}
	k.d.App.FinishRoleSettle(settle)
	return tea.Batch(m.RefreshStatus(), reloadCmd())
}

func reloadCmd() tea.Cmd {
	return func() tea.Msg { return vkdeck.ReloadMsg{} }
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
		return m.Push(ntr.NewNoteBuilder(home, role))
	case keymap.TargetTaskNew:
		return m.Push(ntr.NewTaskBuilder(home, role))
	case keymap.TargetRemindNew:
		if !ntr.RemindersUIVisible() {
			return nil
		}
		return m.Push(ntr.NewRemindBuilder(home, role))
	case keymap.TargetRoleNext:
		return k.cycleRoleCmd(1)
	case keymap.TargetRolePrev:
		return k.cycleRoleCmd(-1)
	case keymap.TargetPaneInbox, keymap.TargetPanePop, keymap.TargetPaneShell, keymap.TargetPaneClose:
		return k.paneCmd(m, target)
	}
	name, ok := keymap.FlightTarget(target)
	if !ok || k.d.App == nil {
		return nil
	}
	d := k.d.App.Dirs()
	if d == nil {
		return nil
	}
	if _, exists := d.Flights[name]; !exists {
		return nil
	}
	return m.Push(k.FlightResults(name))
}

func (k *Kit) paneCmd(m *vkdeck.Model, target string) tea.Cmd {
	if k.d.Panes == nil {
		return nil
	}
	panes := k.d.Panes
	switch target {
	case keymap.TargetPaneInbox:
		return paneAction(panes.OpenInbox)
	case keymap.TargetPaneShell:
		return paneAction(panes.OpenShell)
	case keymap.TargetPaneClose:
		return paneAction(panes.CloseLast)
	case keymap.TargetPanePop:
		src, ok := m.Top().(PaneSource)
		if !ok {
			return nil
		}
		snap, ok := src.PaneSnapshot()
		if !ok {
			return nil
		}
		return paneAction(func() error { return panes.OpenSnapshot(snap) })
	}
	return nil
}

func paneAction(fn func() error) tea.Cmd {
	return func() tea.Msg {
		if err := fn(); err != nil {
			log.Warnf("pane: %v", err)
		}
		return nil
	}
}

func (k *Kit) cycleRoleCmd(delta int) tea.Cmd {
	if k.d.App == nil {
		return nil
	}
	d := k.d.App.Dirs()
	if d == nil {
		return nil
	}
	next, ok := app.NextRole(d.RoleNames(), k.d.App.Role(), delta)
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
		role = k.d.App.Role()
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
