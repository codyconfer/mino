//go:build !nodaemon

package daemon

import (
	"context"

	sysdaemon "github.com/codyconfer/sisyphus/daemon"
	"github.com/codyconfer/sisyphus/daemon/ui"

	"github.com/codyconfer/mino/cmd"
	"github.com/codyconfer/mino/internal/app/serve"
	"github.com/codyconfer/mino/internal/log"
	"github.com/codyconfer/mino/internal/render/icons"
)

func watch(ctx context.Context, opt options) error {
	if opt.Desktop || opt.Tray {
		icons.LoadStateIcons(cmd.App().Cfg.Home, opt.Theme)
	}
	if opt.Tray {
		return runTray(ctx, opt)
	}
	return server().Run(ctx, serve.RunOptions{
		Flight:   opt.Flight,
		Interval: opt.Interval,
		Bell:     opt.Bell,
		Desktop:  opt.Desktop,
		Terminal: true,
	})
}

func runTray(parent context.Context, opt options) error {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	errCh := make(chan error, 1)
	ready := make(chan struct{})
	stateIcons := sysdaemon.DefaultStateIcons()
	if missing := stateIcons.Missing(); len(missing) > 0 {
		log.Warnf("daemon: tray icons missing for %v (icon may be blank)", missing)
	}
	var tray *ui.Tray
	tray = ui.NewTray(ui.TrayConfig{
		Title:   "mino",
		Tooltip: "mino",
		Icons:   stateIcons,
		OnQuit:  cancel,
		OnReady: func() {
			close(ready)
			go func() {
				tray.SetState(sysdaemon.StateRunning)
				errCh <- server().Run(ctx, serve.RunOptions{
					Flight:   opt.Flight,
					Interval: opt.Interval,
					Bell:     opt.Bell,
					Desktop:  opt.Desktop,
					OnState:  tray.SetState,
				})
				cancel()
				tray.Stop()
			}()
		},
	})
	go func() {
		<-ctx.Done()
		tray.Stop()
	}()
	tray.Run()
	select {
	case <-ready:
		return <-errCh
	default:
		return nil
	}
}
