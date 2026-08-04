package app

import (
	"context"
	"os/exec"
	"sync"
	"time"

	"github.com/codyconfer/mino/internal/log"
	"github.com/codyconfer/mino/internal/role"
)

const RoleCycleDebounce = 400 * time.Millisecond

type roleDebounce struct {
	mu      sync.Mutex
	gen     uint64
	pending bool
	active  *RoleSettle
}

const NoRole = ""

func NextRole(names []string, current string, delta int) (next string, ok bool) {
	if len(names) == 0 || delta == 0 {
		return NoRole, false
	}
	ring := make([]string, 0, len(names)+1)
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
	n := len(ring)
	next = ring[((idx+delta)%n+n)%n]
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

	mu   sync.Mutex
	done int
	plan rolePlan
}

func (s *RoleSettle) MarkDone(n int) {
	if s == nil {
		return
	}
	s.mu.Lock()
	if n > s.done {
		s.done = min(n, len(s.Steps))
	}
	s.mu.Unlock()
}

func (s *RoleSettle) Remaining() []RoleHookStep {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.Steps[s.done:]
}

type rolePlan struct {
	prev    string
	next    string
	changed bool
	steps   []RoleHookStep
}

func (a *App) planRoleLifecycle() rolePlan {
	if a == nil || a.Cfg == nil || a.thin {
		return rolePlan{}
	}
	next := a.Role()
	prev, _ := a.persistedRole()
	return a.planRoleChange(prev, next)
}

func (a *App) planRoleChange(prev, next string) rolePlan {
	if a == nil || a.Cfg == nil || a.thin {
		return rolePlan{}
	}
	p := rolePlan{prev: prev, next: next, changed: prev != next}
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

func (a *App) runRolePlan(p rolePlan) { a.runRoleHookSteps(p.steps) }

func (a *App) runRoleHookSteps(steps []RoleHookStep) {
	for _, s := range steps {
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
		if err := a.stateStore().SetActiveRole(context.Background(), p.next); err != nil {
			log.Warnf("role state: %v", err)
		}
		a.clearTransientRole()
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
	s := &RoleSettle{Steps: p.steps, plan: p}

	a.roleDebounce.mu.Lock()
	a.roleDebounce.active = s
	a.roleDebounce.mu.Unlock()
	return s, true
}

func (a *App) FinishRoleSettle(s *RoleSettle) {
	if a == nil || s == nil {
		return
	}
	a.roleDebounce.mu.Lock()
	if a.roleDebounce.active == s {
		a.roleDebounce.active = nil
	}
	a.roleDebounce.mu.Unlock()
	s.MarkDone(len(s.Steps))
	a.commitRolePlan(s.plan)
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
	abandoned := a.roleDebounce.active
	a.roleDebounce.active = nil
	a.roleDebounce.pending = false
	a.roleDebounce.gen++
	a.roleDebounce.mu.Unlock()
	if pending {
		p := a.planRoleLifecycle()
		a.runRolePlan(p)
		a.commitRolePlan(p)
		return
	}
	if abandoned == nil {
		return
	}
	remaining := abandoned.Remaining()
	abandoned.MarkDone(len(abandoned.Steps))
	a.runRoleHookSteps(remaining)
	a.commitRolePlan(abandoned.plan)
}

func (a *App) invalidateRoleDebounce() {
	if a == nil {
		return
	}
	a.roleDebounce.mu.Lock()
	a.roleDebounce.gen++
	a.roleDebounce.pending = false
	a.roleDebounce.active = nil
	a.roleDebounce.mu.Unlock()
}
