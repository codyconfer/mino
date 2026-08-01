//go:build !nodaemon

package daemon

import (
	sysdaemon "github.com/codyconfer/sisyphus/daemon"

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
	if sysdaemon.IsListening(config.SocketPrefix, socketPath()) {
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
	case st == "running":
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
		return deck.ServiceStatus{Name: "daemon", Detail: "running", Level: deck.StatusOK}, true
	case InstallStopped:
		return deck.ServiceStatus{Name: "daemon", Detail: "stopped", Level: deck.StatusWarn}, true
	default:
		return deck.ServiceStatus{}, false
	}
}
