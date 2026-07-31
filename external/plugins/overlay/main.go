package main

import (
	"context"
	"fmt"
	"os"

	"github.com/codyconfer/sisyphus/daemon"

	muninapp "github.com/codyconfer/munin/app"
	"github.com/codyconfer/munin/cmd"
	plugins "github.com/codyconfer/munin/external/plugins"
)

func main() {
	defer cmd.Shutdown()

	ctx, stop := daemon.Context(context.Background())
	defer stop()

	err := muninapp.Run(muninapp.Options{
		RegisterPlugins: plugins.Register,
		CLI: func(_ context.Context, args []string) error {
			root := cmd.Root()
			root.SetArgs(args)
			return root.ExecuteContext(ctx)
		},
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		cmd.Shutdown()
		os.Exit(1)
	}
}
