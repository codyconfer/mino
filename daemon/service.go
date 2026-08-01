//go:build !nodaemon

package daemon

import (
	"context"
	"time"

	"github.com/codyconfer/sisyphus/daemon/service"

	"github.com/codyconfer/mino/cmd"
	"github.com/codyconfer/mino/internal/app/serve"
	"github.com/codyconfer/mino/internal/config"
)

const daemonName = "mino"

type options struct {
	Flight   string
	Interval time.Duration
	Bell     bool
	Desktop  bool
	Tray     bool
	Theme    string
}

func server() *serve.Server { return serve.NewServer(cmd.App()) }

func configOptions(flight string) options {
	c := cmd.App().Cfg
	return options{
		Flight:   flight,
		Interval: cmd.ServeInterval(),
		Bell:     c.Daemon.Bell,
		Desktop:  c.Daemon.Desktop,
		Tray:     c.Daemon.Tray,
		Theme:    cmd.ServeTheme(),
	}
}

func daemonService(flight string, userService bool) (*service.Service, error) {
	return newService(configOptions(flight), userService)
}

func newService(opt options, userService bool) (*service.Service, error) {
	return service.New(service.Config{
		Name:        daemonName,
		DisplayName: "mino",
		Description: "mino realtime signal watcher",
		Arguments:   RunArgs(opt.Flight, opt.Interval, opt.Bell, opt.Desktop, opt.Theme),
		UserService: userService,
	}, func(ctx context.Context) error {
		return watch(ctx, opt)
	})
}

func RunArgs(flight string, interval time.Duration, bell, desktop bool, theme string) []string {
	args := []string{"daemon", "run", flight, "--interval", interval.String()}
	if !bell {
		args = append(args, "--bell=false")
	}
	if desktop {
		args = append(args, "--desktop")
	}
	if theme != "" && theme != "dark" {
		args = append(args, "--theme", theme)
	}
	return args
}

func socketPath() string { return config.ServeSocketPath(cmd.App().Cfg.Home) }
