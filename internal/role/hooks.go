package role

import (
	"fmt"
	"os/exec"

	"github.com/codyconfer/sisyphus/lifecycle"

	"github.com/codyconfer/mino/internal/config"
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

func Command(kind, script string) (*exec.Cmd, error) {
	switch kind {
	case "bash":
		bin, err := lifecycle.LookBash()
		if err != nil {
			return nil, err
		}
		return exec.Command(bin, "-c", script), nil
	case "powershell":
		bin, err := lifecycle.LookPowerShell()
		if err != nil {
			return nil, err
		}
		return exec.Command(bin, "-NoProfile", "-Command", script), nil
	default:
		return nil, fmt.Errorf("unknown shell kind %q", kind)
	}
}

func RunEnter(rd config.RoleDef) error {
	return RunHooks(rd.Hooks.Enter)
}

func RunExit(rd config.RoleDef) error {
	return RunHooks(rd.Hooks.Exit)
}
