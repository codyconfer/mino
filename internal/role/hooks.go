// Package role runs role enter/exit shell hooks and tracks the last activated role.
package role

import (
	"github.com/codyconfer/sisyphus/lifecycle"

	"github.com/codyconfer/munin/internal/config"
)

// RunFunc executes a shell script with the given interpreter kind ("bash" or "powershell").
type RunFunc = lifecycle.RunFunc

// Run is the process-level shell runner; tests may replace it.
var Run = lifecycle.DefaultRun

// scripts adapts munin config hooks to the sisyphus Scripts type.
func scripts(h config.RoleShellHooks) lifecycle.Scripts {
	return lifecycle.Scripts{Bash: h.Bash, PowerShell: h.PowerShell}
}

// Select picks which script to run for the current platform.
func Select(hooks config.RoleShellHooks) (kind, script string, ok bool) {
	return lifecycle.Select(scripts(hooks))
}

// RunHooks runs the selected script for hooks, or nil when nothing to run.
// Missing interpreters return an error; callers typically warn and continue.
func RunHooks(hooks config.RoleShellHooks) error {
	kind, script, ok := Select(hooks)
	if !ok {
		return nil
	}
	return Run(kind, script)
}

// RunEnter runs rd.Hooks.Enter.
func RunEnter(rd config.RoleDef) error {
	return RunHooks(rd.Hooks.Enter)
}

// RunExit runs rd.Hooks.Exit.
func RunExit(rd config.RoleDef) error {
	return RunHooks(rd.Hooks.Exit)
}
