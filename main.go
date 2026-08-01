package main

import (
	"context"
	"fmt"
	"os"

	"github.com/codyconfer/sisyphus/daemon"
	"github.com/codyconfer/viewkit/theme"

	minoapp "github.com/codyconfer/mino/app"
	"github.com/codyconfer/mino/cmd"
	"github.com/codyconfer/mino/internal/config"
	"github.com/codyconfer/mino/internal/errs"
	"github.com/codyconfer/mino/internal/keymap"
	"github.com/codyconfer/mino/internal/log"
	"github.com/codyconfer/mino/internal/render"
	"github.com/codyconfer/mino/internal/render/glyph"
)

func main() {
	applyTheme()
	glyph.Resolve()
	keymap.Install()
	applyLogLevel()
	applyLogColor()
	defer cmd.Shutdown()

	ctx, stop := daemon.Context(context.Background())
	defer stop()

	err := minoapp.Run(minoapp.Options{
		CLI: func(_ context.Context, args []string) error {
			root := cmd.Root()
			root.SetArgs(args)
			return root.ExecuteContext(ctx)
		},
	})
	if err != nil {
		fmt.Fprint(os.Stderr, errs.Render(err))
		cmd.Shutdown()
		os.Exit(1)
	}
}

func applyLogLevel() {
	lvl := os.Getenv("MINO_LOG_LEVEL")
	if lvl == "" {
		lvl = config.LoadGlobalSettings().LogLevel
	}
	if l, ok := log.ParseLevel(lvl); ok {
		log.SetLevel(l)
	}
}

func applyLogColor() {
	c := os.Getenv("MINO_LOG_COLOR")
	if c == "" {
		c = config.LoadGlobalSettings().LogColor
	}
	if m, ok := log.ParseColorMode(c); ok {
		log.SetColorMode(m)
	}
}

func applyTheme() {
	render.InstallDefaultTheme()
	key := os.Getenv("MINO_THEME")
	if key == "" {
		key = config.LoadGlobalSettings().Theme
	}
	if key == "" || key == render.DefaultThemeKey {
		return
	}
	if t, ok := theme.Named(key); ok {
		theme.Use(t)
	}
}
