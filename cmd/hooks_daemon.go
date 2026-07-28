//go:build !nodaemon

package cmd

import (
	"context"

	"github.com/spf13/cobra"

	sysdaemon "github.com/codyconfer/sisyphus/daemon"

	"github.com/codyconfer/munin/internal/app/statusstrip"
	"github.com/codyconfer/munin/internal/deck"
)

func daemonCommands() []*cobra.Command {
	return []*cobra.Command{newServeCmd(), newDaemonCmd()}
}

func ensureServeProvider(ctx context.Context, flight string) (stop func()) {
	return serveServer().EnsureLiveProvider(ctx, flight)
}

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

func serveSocketTaken() bool {
	return sysdaemon.Attached("munin", serveServer().SocketPath())
}
