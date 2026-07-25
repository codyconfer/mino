//go:build !nodaemon

package cmd

import (
	"context"

	"github.com/spf13/cobra"

	sysdaemon "github.com/codyconfer/sisyphus/daemon"

	"github.com/codyconfer/munin/internal/app/statusstrip"
	"github.com/codyconfer/munin/internal/deck"
)

// daemonCommands are the serve/daemon mode entrypoints the root command
// registers. See hooks_nodaemon.go for the build without daemon mode.
func daemonCommands() []*cobra.Command {
	return []*cobra.Command{newServeCmd(), newDaemonCmd()}
}

// ensureServeProvider gives deck a live event provider, reusing a listening
// daemon or starting a session-owned background serve.
func ensureServeProvider(ctx context.Context, flight string) (stop func()) {
	return serveServer().EnsureLiveProvider(ctx, flight)
}

// daemonStatusChip reports the installed/running daemon chip for deck chrome.
func daemonStatusChip() statusstrip.DaemonStatus {
	return func() deck.ServiceStatus {
		return serveServer().ServiceStatusChip(
			defaultFlightName(),
			configServeInterval(),
			shared.Cfg.Daemon.Bell,
			shared.Cfg.Daemon.Desktop,
			shared.Cfg.Daemon.Tray,
			configServeTheme(),
		)
	}
}

// serveSocketTaken reports whether another munin daemon already owns the socket
// this process would expose.
func serveSocketTaken() bool {
	return sysdaemon.Attached("munin", serveServer().SocketPath())
}
