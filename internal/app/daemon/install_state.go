//go:build !nodaemon

package daemon

import (
	"time"

	sysdaemon "github.com/codyconfer/sisyphus/daemon"

	"github.com/codyconfer/munin/internal/deck"
)

type InstallState int

const (
	InstallNotInstalled InstallState = iota
	InstallStopped
	InstallRunning
)

func (s *Server) ClassifyInstall(flight string, interval time.Duration, bell, desktop, tray bool, theme string) InstallState {
	if sysdaemon.IsListening(daemonName, s.SocketPath()) {
		return InstallRunning
	}
	svc, err := s.Service(flight, interval, bell, desktop, tray, theme, true)
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

func (s *Server) ServiceStatusChip(flight string, interval time.Duration, bell, desktop, tray bool, theme string) deck.ServiceStatus {
	switch s.ClassifyInstall(flight, interval, bell, desktop, tray, theme) {
	case InstallRunning:
		return deck.ServiceStatus{Name: "daemon", Detail: "running", Level: deck.StatusOK}
	case InstallStopped:
		return deck.ServiceStatus{Name: "daemon", Detail: "stopped", Level: deck.StatusWarn}
	default:
		return deck.ServiceStatus{Name: "daemon", Detail: "not installed", Level: deck.StatusMuted}
	}
}
