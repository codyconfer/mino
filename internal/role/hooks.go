package role

import (
	"github.com/codyconfer/sisyphus/lifecycle"

	"github.com/codyconfer/munin/internal/config"
)

type RunFunc = lifecycle.RunFunc

var Run = lifecycle.DefaultRun

func scripts(h config.RoleShellHooks) lifecycle.Scripts {
	return lifecycle.Scripts{Bash: h.Bash, PowerShell: h.PowerShell}
}

func Select(hooks config.RoleShellHooks) (kind, script string, ok bool) {
	return lifecycle.Select(scripts(hooks))
}

func RunHooks(hooks config.RoleShellHooks) error {
	kind, script, ok := Select(hooks)
	if !ok {
		return nil
	}
	return Run(kind, script)
}

func RunEnter(rd config.RoleDef) error {
	return RunHooks(rd.Hooks.Enter)
}

func RunExit(rd config.RoleDef) error {
	return RunHooks(rd.Hooks.Exit)
}
