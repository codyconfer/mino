package app

import (
	"sync"
	"time"

	"github.com/codyconfer/viewkit/layout"
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

func (a *App) FlushRoleLifecycle() {
	if a == nil {
		return
	}
	a.roleDebounce.mu.Lock()
	pending := a.roleDebounce.pending
	a.roleDebounce.pending = false
	a.roleDebounce.gen++
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
