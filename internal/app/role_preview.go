package app

import (
	"bytes"
	"strings"
	"time"

	"github.com/codyconfer/mino/internal/config"
	"github.com/codyconfer/mino/internal/role"
)

const (
	RolePreviewHold = 5 * time.Second

	previewOutputLimit = 400
)

type RolePreviewStep struct {
	Label  string
	Detail string
	Err    error
}

var previewHook = capturedHook

func capturedHook(kind, script string) (string, error) {
	cmd, err := role.Command(kind, script)
	if err != nil {
		return "", err
	}
	var out bytes.Buffer
	cmd.Stdin = nil
	cmd.Stdout = &out
	cmd.Stderr = &out
	err = cmd.Run()
	return trimPreviewOutput(out.String()), err
}

func trimPreviewOutput(s string) string {
	s = strings.TrimSpace(strings.ReplaceAll(s, "\r\n", "\n"))
	s = strings.Join(strings.Fields(strings.ReplaceAll(s, "\n", " ")), " ")
	if len(s) > previewOutputLimit {
		return s[:previewOutputLimit] + "…"
	}
	return s
}

func previewHookStep(label, kind, script string) RolePreviewStep {
	out, err := previewHook(kind, script)
	return RolePreviewStep{Label: label, Detail: out, Err: err}
}

func (a *App) PreviewRole(rd config.RoleDef, hold time.Duration, body func() RolePreviewStep) []RolePreviewStep {
	if body == nil {
		body = func() RolePreviewStep { return RolePreviewStep{} }
	}
	if a == nil || a.Cfg == nil || a.thin {
		return []RolePreviewStep{body()}
	}

	enterKind, enterScript, hasEnter := role.Select(rd.Hooks.Enter)
	exitKind, exitScript, hasExit := role.Select(rd.Hooks.Exit)

	var steps []RolePreviewStep
	if hasEnter {
		steps = append(steps, previewHookStep("enter hook ("+enterKind+")", enterKind, enterScript))
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
		steps = append(steps, previewHookStep("exit hook ("+exitKind+")", exitKind, exitScript))
	}
	return append(steps, a.restoreRole())
}

func (a *App) restoreRole() RolePreviewStep {
	name := a.Role()
	if name == "" {
		role.ClearStatusChips()
		return RolePreviewStep{Label: "restored: no active role"}
	}
	step := RolePreviewStep{Label: "restored role: " + name}
	rd, ok := a.RoleDef(name)
	if !ok {
		role.ClearStatusChips()
		return step
	}
	if kind, script, has := role.Select(rd.Hooks.Enter); has {
		step.Detail, step.Err = previewHook(kind, script)
	}
	a.applyRoleStatus(rd)
	a.applyRoleContexts()
	return step
}
