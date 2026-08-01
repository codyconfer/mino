package main

import (
	"context"
	"embed"
	"io/fs"
	"os"

	"github.com/codyconfer/sisyphus/daemon"

	minoapp "github.com/codyconfer/mino/app"
	"github.com/codyconfer/mino/cmd"
	plugins "github.com/codyconfer/mino/external/plugins"
)

//go:embed all:defaults
var defaultsRoot embed.FS

func main() {
	defer cmd.Shutdown()

	ctx, stop := daemon.SignalContext(context.Background())
	defer stop()

	defaultsFS, err := fs.Sub(defaultsRoot, "defaults")
	if err != nil {
		panic(err)
	}

	err = minoapp.Run(minoapp.Options{
		Defaults:        defaultsFS,
		RegisterPlugins: plugins.Register,
		CLI: func(_ context.Context, args []string) error {
			root := cmd.Root()
			root.SetArgs(args)
			return root.ExecuteContext(ctx)
		},
	})
	if err != nil {
		cmd.Shutdown()
		os.Exit(1)
	}
}
