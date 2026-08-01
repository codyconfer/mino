package main

import (
	"context"
	"fmt"
	"os"

	"github.com/codyconfer/sisyphus/daemon"

	minoapp "github.com/codyconfer/mino/app"
	"github.com/codyconfer/mino/cmd"
	plugins "github.com/codyconfer/mino/external/plugins"
)

func main() {
	defer cmd.Shutdown()

	ctx, stop := daemon.Context(context.Background())
	defer stop()

	err := minoapp.Run(minoapp.Options{
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
