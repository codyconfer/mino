package app

import (
	"time"

	"github.com/codyconfer/munin/internal/config"
	"github.com/codyconfer/munin/internal/role"
)

const RolePreviewHold = 5 * time.Second

type RolePreviewStep struct {
	Label  string
	Detail string
	Err    error
}

func (a *App) PreviewRole(rd config.RoleDef, hold time.Duration, body func() RolePreviewStep) []RolePreviewStep {
	if body == nil {
		body = func() RolePreviewStep { return RolePreviewStep{} }
	}
	if a == nil || a.Cfg == nil || a.thin {
		return []RolePreviewStep{body()}
	}

	enterKind, _, hasEnter := role.Select(rd.Hooks.Enter)
	exitKind, _, hasExit := role.Select(rd.Hooks.Exit)

	var steps []RolePreviewStep
	if hasEnter {
		steps = append(steps, RolePreviewStep{Label: "enter hook (" + enterKind + ")", Err: role.RunEnter(rd)})
	}

	steps = append(steps, body())

	if !hasEnter && !hasExit {
		return steps
	}

	if hold > 0 {
		time.Sleep(hold)
		steps = append(steps, RolePreviewStep{Label: "held " + hold.String()})
	}
	if hasExit {
		steps = append(steps, RolePreviewStep{Label: "exit hook (" + exitKind + ")", Err: role.RunExit(rd)})
	}
	return append(steps, RolePreviewStep{Label: a.restoreRole()})
}

func (a *App) restoreRole() string {
	name := a.Role()
	if name == "" {
		role.ClearStatusChips()
		return "restored: no active role"
	}
	a.runRoleEnter(name)
	a.applyRoleContexts()
	return "restored role: " + name
}
