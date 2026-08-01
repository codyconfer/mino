//go:build !nodaemon

package daemon

import (
	"github.com/codyconfer/sisyphus/daemon/service"
	"github.com/codyconfer/sisyphus/ipc"
	vkglyph "github.com/codyconfer/viewkit/glyph"

	"github.com/codyconfer/mino/cmd"
	"github.com/codyconfer/mino/internal/config"
	"github.com/codyconfer/mino/internal/deck"
)

type InstallState int

const (
	InstallNotInstalled InstallState = iota
	InstallStopped
	InstallRunning
)

func classifyInstall(opt options) InstallState {
	if ipc.IsListening(config.SocketPrefix, socketPath()) {
		return InstallRunning
	}
	svc, err := newService(opt, true)
	if err != nil {
		return InstallNotInstalled
	}
	st, sErr := svc.Status()
	switch {
	case sErr != nil:
		return InstallNotInstalled
	case st == service.StateRunning:
		return InstallRunning
	default:
		return InstallStopped
	}
}

func statusChip() (deck.ServiceStatus, bool) {
	if cmd.App() == nil || cmd.App().Cfg == nil {
		return deck.ServiceStatus{}, false
	}
	switch classifyInstall(configOptions(cmd.DefaultFlight())) {
	case InstallRunning:
		return deck.ServiceStatus{Name: "daemon", Detail: "running", Severity: vkglyph.SeverityPositive}, true
	case InstallStopped:
		return deck.ServiceStatus{Name: "daemon", Detail: "stopped", Severity: vkglyph.SeverityWarning}, true
	default:
		return deck.ServiceStatus{}, false
	}
}
