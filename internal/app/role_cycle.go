package app

import (
	"sync"
	"time"
)

// RoleCycleDebounce is the quiet period before enter/exit hooks, status chips,
// and role contexts run after a hotkey role cycle. Intermediate roles skipped
// during a burst never receive lifecycle side effects.
const RoleCycleDebounce = 400 * time.Millisecond

type roleDebounce struct {
	mu      sync.Mutex
	gen     uint64
	pending bool
}

// NextRole steps delta (±1) through names (typically Directives.RoleNames order).
// ok is false when there are no roles or the step would not change the active role.
func NextRole(names []string, current string, delta int) (next string, ok bool) {
	n := len(names)
	if n == 0 || delta == 0 {
		return "", false
	}
	idx := -1
	for i, name := range names {
		if name == current {
			idx = i
			break
		}
	}
	if idx < 0 {
		if delta > 0 {
			return names[0], true
		}
		return names[n-1], true
	}
	nextIdx := (idx + delta) % n
	if nextIdx < 0 {
		nextIdx += n
	}
	next = names[nextIdx]
	if next == current {
		return "", false
	}
	return next, true
}

// BeginRoleCycle updates the in-memory active role immediately (so visibility
// scopes) and bumps the debounce generation. Lifecycle hooks/status/contexts
// wait for SettleRoleCycle with the returned gen.
func (a *App) BeginRoleCycle(name string) (gen uint64, changed bool) {
	if a == nil || a.Cfg == nil {
		return 0, false
	}
	if name == a.Cfg.Role {
		return 0, false
	}
	a.Cfg.Role = name
	a.roleDebounce.mu.Lock()
	a.roleDebounce.gen++
	gen = a.roleDebounce.gen
	a.roleDebounce.pending = true
	a.roleDebounce.mu.Unlock()
	return gen, true
}

// SettleRoleCycle runs syncRoleLifecycle when gen is still current (final
// settle after debounce). Returns false if a newer cycle superseded this gen.
func (a *App) SettleRoleCycle(gen uint64) bool {
	if a == nil {
		return false
	}
	a.roleDebounce.mu.Lock()
	if gen == 0 || gen != a.roleDebounce.gen {
		a.roleDebounce.mu.Unlock()
		return false
	}
	a.roleDebounce.pending = false
	a.roleDebounce.mu.Unlock()
	a.syncRoleLifecycle()
	return true
}

// FlushRoleLifecycle applies any pending debounced role lifecycle immediately
// (e.g. on app shutdown so quit mid-cycle does not leave hooks unrun).
func (a *App) FlushRoleLifecycle() {
	if a == nil {
		return
	}
	a.roleDebounce.mu.Lock()
	pending := a.roleDebounce.pending
	a.roleDebounce.pending = false
	a.roleDebounce.gen++ // invalidate in-flight settle msgs
	a.roleDebounce.mu.Unlock()
	if pending {
		a.syncRoleLifecycle()
	}
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
