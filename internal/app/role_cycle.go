package app

import (
	"os/exec"
	"sync"
	"time"

	"github.com/codyconfer/viewkit/layout"

	"github.com/codyconfer/munin/internal/log"
	"github.com/codyconfer/munin/internal/role"
)

const RoleCycleDebounce = 400 * time.Millisecond

type roleDebounce struct {
	mu      sync.Mutex
	gen     uint64
	pending bool
}

const NoRole = ""

func NextRole(names []string, current string, delta int) (next string, ok bool) {
	if len(names) == 0 || delta == 0 {
		return NoRole, false
	}
	ring := make(layout.Ring, 0, len(names)+1)
	ring = append(ring, NoRole)
	ring = append(ring, names...)

	idx := -1
	for i, name := range ring {
		if name == current {
			idx = i
			break
		}
	}
	if idx < 0 {
		if delta > 0 {
			return names[0], true
		}
		return names[len(names)-1], true
	}
	next = ring.At(ring.Step(idx, delta))
	if next == current {
		return NoRole, false
	}
	return next, true
}

const (
	RoleHookEnter = "enter"
	RoleHookExit  = "exit"
)

type RoleHookStep struct {
	Role   string
	Phase  string
	Kind   string
	Script string
}

func (s RoleHookStep) Label() string {
	return s.Phase + " hook for role " + s.Role + " (" + s.Kind + ")"
}

func (s RoleHookStep) Command() (*exec.Cmd, error) {
	return role.Command(s.Kind, s.Script)
}

type RoleSettle struct {
	Steps []RoleHookStep
	plan  rolePlan
}

type rolePlan struct {
	prev    string
	next    string
	changed bool
	steps   []RoleHookStep
}

func (a *App) planRoleLifecycle() rolePlan {
	var p rolePlan
	if a == nil || a.Cfg == nil || a.thin {
		return p
	}
	p.prev, p.next = role.LoadActive(a.home()), a.Role()
	p.changed = p.prev != p.next
	if !p.changed {
		return p
	}
	p.steps = append(a.roleHookSteps(p.prev, RoleHookExit), a.roleHookSteps(p.next, RoleHookEnter)...)
	return p
}

func (a *App) roleHookSteps(name, phase string) []RoleHookStep {
	if name == "" {
		return nil
	}
	rd, ok := a.RoleDef(name)
	if !ok {
		log.Warnf("role %s: %q not defined; skipping hooks", phase, name)
		return nil
	}
	hooks := rd.Hooks.Enter
	if phase == RoleHookExit {
		hooks = rd.Hooks.Exit
	}
	kind, script, ok := role.Select(hooks)
	if !ok {
		return nil
	}
	return []RoleHookStep{{Role: name, Phase: phase, Kind: kind, Script: script}}
}

func (a *App) runRolePlan(p rolePlan) {
	for _, s := range p.steps {
		if err := role.Run(s.Kind, s.Script); err != nil {
			log.Warnf("role %q %s hooks: %v", s.Role, s.Phase, err)
		}
	}
}

func (a *App) commitRolePlan(p rolePlan) {
	if a == nil || a.Cfg == nil || a.thin {
		return
	}
	a.refreshRoleStatus(p.next)
	if p.changed {
		if err := role.SaveActive(a.home(), p.next); err != nil {
			log.Warnf("role state: %v", err)
		}
	}
	a.applyRoleContexts()
}

func (a *App) BeginRoleCycle(name string) (gen uint64, changed bool) {
	if a == nil || a.Cfg == nil {
		return 0, false
	}
	if name == a.Role() {
		return 0, false
	}
	a.setRole(name)
	a.roleDebounce.mu.Lock()
	a.roleDebounce.gen++
	gen = a.roleDebounce.gen
	a.roleDebounce.pending = true
	a.roleDebounce.mu.Unlock()
	return gen, true
}

func (a *App) BeginRoleSettle(gen uint64) (*RoleSettle, bool) {
	if a == nil {
		return nil, false
	}
	a.roleDebounce.mu.Lock()
	if gen == 0 || gen != a.roleDebounce.gen {
		a.roleDebounce.mu.Unlock()
		return nil, false
	}
	a.roleDebounce.pending = false
	a.roleDebounce.gen++
	a.roleDebounce.mu.Unlock()
	p := a.planRoleLifecycle()
	return &RoleSettle{Steps: p.steps, plan: p}, true
}

func (a *App) FinishRoleSettle(s *RoleSettle) {
	if a == nil || s == nil {
		return
	}
	a.commitActivatedRole(s.plan)
}

func (a *App) commitActivatedRole(p rolePlan) {
	if p.changed {
		if err := a.persistRole(p.next); err != nil {
			log.Warnf("persisting role %q: %v", p.next, err)
		}
	}
	a.commitRolePlan(p)
}

func (a *App) SettleRoleCycle(gen uint64) bool {
	s, ok := a.BeginRoleSettle(gen)
	if !ok {
		return false
	}
	a.runRolePlan(s.plan)
	a.FinishRoleSettle(s)
	return true
}

func (a *App) FlushRoleLifecycle() {
	if a == nil {
		return
	}
	a.roleDebounce.mu.Lock()
	pending := a.roleDebounce.pending
	a.roleDebounce.pending = false
	a.roleDebounce.gen++
	a.roleDebounce.mu.Unlock()
	if !pending {
		return
	}
	p := a.planRoleLifecycle()
	a.runRolePlan(p)
	a.commitActivatedRole(p)
}

func (a *App) invalidateRoleDebounce() {
	if a == nil {
		return
	}
	a.roleDebounce.mu.Lock()
	a.roleDebounce.gen++
	a.roleDebounce.pending = false
	a.roleDebounce.mu.Unlock()
}
