//go:build !nodaemon

package daemon

import (
	"context"
	"strconv"
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
	HTTP     bool
	HTTPHost string
	HTTPPort int
}

func server() *serve.Server { return serve.NewServer(cmd.App()) }

func configOptions(flight string) options {
	c := cmd.App().Cfg
	httpAPI, httpHost, httpPort := cmd.ServeHTTP()
	return options{
		Flight:   flight,
		Interval: cmd.ServeInterval(),
		Bell:     c.Daemon.Bell,
		Desktop:  c.Daemon.Desktop,
		Tray:     c.Daemon.Tray,
		Theme:    cmd.ServeTheme(),
		HTTP:     httpAPI,
		HTTPHost: httpHost,
		HTTPPort: httpPort,
	}
}

func daemonService(flight string, userService bool) (*service.Service, error) {
	return newService(configOptions(flight), userService)
}

func newService(opt options, userService bool) (*service.Service, error) {
	scope := service.ScopeSystem
	if userService {
		scope = service.ScopeUser
	}
	return service.New(service.Config{
		Name:        daemonName,
		DisplayName: "mino",
		Description: "mino realtime signal watcher",
		Arguments:   RunArgs(opt),
		Scope:       scope,
	}, func(ctx context.Context) error {
		return watch(ctx, opt)
	})
}

// RunArgs is the argv baked into the service unit at install time, so editing
// config later cannot silently change an installed service's behavior.
func RunArgs(opt options) []string {
	args := []string{"daemon", "run", opt.Flight, "--interval", opt.Interval.String()}
	if !opt.Bell {
		args = append(args, "--bell=false")
	}
	if opt.Desktop {
		args = append(args, "--desktop")
	}
	if opt.Theme != "" && opt.Theme != "dark" {
		args = append(args, "--theme", opt.Theme)
	}
	if opt.HTTP {
		args = append(args, "--http", "--http-port", strconv.Itoa(opt.HTTPPort))
		if opt.HTTPHost != "" {
			args = append(args, "--http-host", opt.HTTPHost)
		}
	}
	return args
}

func socketPath() string { return config.ServeSocketPath(cmd.App().Cfg.Home) }
