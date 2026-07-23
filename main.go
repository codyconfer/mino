package main

import (
	"context"
	"fmt"
	"os"

	"github.com/codyconfer/sisyphus/daemon"
	"github.com/codyconfer/viewkit/theme"

	"github.com/codyconfer/munin/cmd"
	"github.com/codyconfer/munin/internal/config"
	"github.com/codyconfer/munin/internal/errs"
	"github.com/codyconfer/munin/internal/keymap"
	"github.com/codyconfer/munin/internal/log"
	"github.com/codyconfer/munin/internal/render"
)

func main() {
	applyTheme()
	keymap.Install()
	applyLogLevel()
	applyLogColor()
	defer cmd.Shutdown()
	ctx, stop := daemon.Context(context.Background())
	defer stop()
	if err := cmd.Root().ExecuteContext(ctx); err != nil {
		fmt.Fprint(os.Stderr, errs.Render(err))
		cmd.Shutdown()
		os.Exit(1)
	}
}

func applyLogLevel() {
	lvl := os.Getenv("MUNIN_LOG_LEVEL")
	if lvl == "" {
		lvl = config.LoadGlobalSettings().LogLevel
	}
	if l, ok := log.ParseLevel(lvl); ok {
		log.SetLevel(l)
	}
}

func applyLogColor() {
	c := os.Getenv("MUNIN_LOG_COLOR")
	if c == "" {
		c = config.LoadGlobalSettings().LogColor
	}
	if m, ok := log.ParseColorMode(c); ok {
		log.SetColorMode(m)
	}
}

func applyTheme() {
	render.InstallDefaultTheme()
	key := os.Getenv("MUNIN_THEME")
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
