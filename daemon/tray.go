//go:build !nodaemon

package daemon

import (
	"context"

	systray "github.com/codyconfer/sisyphus/tray"

	"github.com/codyconfer/mino/cmd"
	"github.com/codyconfer/mino/internal/app/serve"
	"github.com/codyconfer/mino/internal/log"
	"github.com/codyconfer/mino/internal/render/icons"
)

func watch(ctx context.Context, opt options) error {
	if opt.Desktop || opt.Tray {
		icons.LoadStateIcons(cmd.App().Cfg.Home, opt.Theme)
	}
	runOpts, err := runOptions(opt)
	if err != nil {
		return err
	}
	if opt.Tray {
		return runTray(ctx, runOpts)
	}
	runOpts.Terminal = true
	return server().Run(ctx, runOpts)
}

// runOptions builds the shared serve options, resolving the API token when the
// HTTP trigger API is on.
func runOptions(opt options) (serve.RunOptions, error) {
	out := serve.RunOptions{
		Flight:   opt.Flight,
		Interval: opt.Interval,
		Bell:     opt.Bell,
		Desktop:  opt.Desktop,
		HTTP:     opt.HTTP,
		HTTPHost: opt.HTTPHost,
		HTTPPort: opt.HTTPPort,
	}
	if !opt.HTTP {
		return out, nil
	}
	tok, src, err := cmd.ResolveServeHTTPToken()
	if err != nil {
		return serve.RunOptions{}, err
	}
	out.HTTPToken, out.HTTPTokenSource = tok, src
	id, err := cmd.ResolveServeHTTPIdentity()
	if err != nil {
		return serve.RunOptions{}, err
	}
	out.HTTPIdentity = id
	return out, nil
}

func runTray(parent context.Context, runOpts serve.RunOptions) error {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	errCh := make(chan error, 1)
	ready := make(chan struct{})
	stateIcons := systray.DefaultIcons()
	if missing := stateIcons.Missing(); len(missing) > 0 {
		log.Warnf("daemon: tray icons missing for %v (icon may be blank)", missing)
	}
	var tray *systray.Tray
	tray = systray.NewTray(systray.Config{
		Title:   "mino",
		Tooltip: "mino",
		Icons:   stateIcons,
		OnQuit:  cancel,
		OnReady: func() {
			close(ready)
			go func() {
				tray.SetState(systray.StateRunning)
				runOpts.OnState = tray.SetState
				errCh <- server().Run(ctx, runOpts)
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
