//go:build nodaemon

package cmd

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/codyconfer/munin/internal/app/statusstrip"
)

// daemonCommands is empty: `nodaemon` builds ship no serve or daemon commands.
func daemonCommands() []*cobra.Command { return nil }

// ensureServeProvider is a no-op; deck runs without a realtime event provider.
func ensureServeProvider(context.Context, string) (stop func()) { return func() {} }

// daemonStatusChip is nil so deck chrome omits the daemon chip entirely.
func daemonStatusChip() statusstrip.DaemonStatus { return nil }

// serveSocketTaken is always false: nothing here can own a serve socket.
func serveSocketTaken() bool { return false }
