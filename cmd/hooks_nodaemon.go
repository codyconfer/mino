//go:build nodaemon

package cmd

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/codyconfer/munin/internal/app/statusstrip"
)

func daemonCommands() []*cobra.Command { return nil }

func ensureServeProvider(context.Context, string) (stop func()) { return func() {} }

func daemonStatusChip() statusstrip.DaemonStatus { return nil }

func serveSocketTaken() bool { return false }
