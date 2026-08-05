package cmd

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/spf13/cobra"

	sysdaemon "github.com/codyconfer/sisyphus/daemon"

	"github.com/codyconfer/mino/internal/app/serve"
	"github.com/codyconfer/mino/internal/app/serve/httpapi"
	"github.com/codyconfer/mino/internal/app/suggest"
	"github.com/codyconfer/mino/internal/config"
	"github.com/codyconfer/mino/internal/errs"
	"github.com/codyconfer/mino/internal/render/icons"
	"github.com/codyconfer/mino/internal/signals"
)

func serveServer() *serve.Server {
	return serve.NewServer(shared)
}

func newServeCmd() *cobra.Command {
	var interval time.Duration
	var bell, desktop, httpAPI bool
	var httpPort int
	var httpHost string
	var theme string
	c := &cobra.Command{
		Use:   "serve [flight]",
		Short: "Run mino in the foreground, watching a flight's signals in realtime and notifying",
		Long: "Runs mino in the CURRENT shell as a long-running process that opens each of the\n" +
			"flight's realtime-capable signals, fans their events into one loop, and emits a\n" +
			"notification for each new item. Ctrl-C exits. Logs stream to the shell and the\n" +
			"log dir. serve does NOT install an OS service or own the system tray — use\n" +
			"`mino daemon` for the installed service (and daemon.tray for a tray icon).\n" +
			"--desktop sends OS notifications using per-state icons from <home>/icons/ or\n" +
			"embedded themes. Only Slack is a true websocket; the rest are polled at\n" +
			"--interval; unsupported signals are skipped.\n" +
			"--http exposes a bearer-token HTTP API that triggers flights, queries and\n" +
			"actions, and mirrors the event stream over SSE. It is off unless asked for,\n" +
			"binds loopback unless --http-host/daemon.http.host says otherwise, and fails\n" +
			"to start if the address is taken.",
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: completeFlightNames,
		Annotations:       map[string]string{annoGateMode: modeServe},
		RunE: func(cmd *cobra.Command, args []string) error {
			f := cmd.Flags()
			if !f.Changed("interval") {
				interval = configServeInterval()
			}
			if err := checkServeInterval(f.Changed("interval"), interval); err != nil {
				return err
			}
			if !f.Changed("bell") {
				bell = shared.Cfg.Daemon.Bell
			}
			if !f.Changed("desktop") {
				desktop = shared.Cfg.Daemon.Desktop
			}
			if !f.Changed("theme") {
				theme = configServeTheme()
			}
			if !f.Changed("http") {
				httpAPI = shared.Cfg.Daemon.HTTP.Enabled
			}
			if !f.Changed("http-port") {
				httpPort = configServeHTTPPort()
			}
			if !f.Changed("http-host") {
				httpHost = configServeHTTPHost()
			}
			var httpToken, httpTokenSource string
			var httpIdentity serve.HTTPIdentityOptions
			if httpAPI {
				if err := checkServeHTTPPort(f.Changed("http-port"), httpPort); err != nil {
					return err
				}
				if err := checkServeHTTPHost(f.Changed("http-host"), httpHost); err != nil {
					return err
				}
				tok, src, err := httpapi.ResolveToken(shared.Cfg.Home, shared.Cfg.Daemon.HTTP.Token)
				if err != nil {
					return err
				}
				httpToken, httpTokenSource = tok, src
				id, err := resolveServeHTTPIdentity()
				if err != nil {
					return err
				}
				httpIdentity = id
			}
			name, err := resolveServeFlight(args)
			if err != nil {
				return err
			}
			if desktop {
				icons.LoadStateIcons(shared.Cfg.Home, theme)
			}
			ctx, cancelLifeline := serve.BindDeckLifeline(cmd.Context())
			defer cancelLifeline()
			err = serveServer().Run(ctx, serve.RunOptions{
				Flight:          name,
				Interval:        interval,
				Bell:            bell,
				Desktop:         desktop,
				Terminal:        true,
				HTTP:            httpAPI,
				HTTPHost:        httpHost,
				HTTPPort:        httpPort,
				HTTPToken:       httpToken,
				HTTPTokenSource: httpTokenSource,
				HTTPIdentity:    httpIdentity,
			})
			// Run only errors when startup failed, in which case nothing was
			// serving and "shutting down" is a confusing thing to read above the
			// real error.
			if err == nil {
				fmt.Fprintln(cmd.ErrOrStderr(), "serve: shutting down")
			}
			return err
		},
	}
	c.Flags().DurationVar(&interval, "interval", 60*time.Second, "poll interval floor for polled (non-websocket) signals")
	c.Flags().BoolVar(&bell, "bell", true, "ring the terminal bell on each notification")
	c.Flags().BoolVar(&desktop, "desktop", false, "send OS desktop notifications (uses per-state icons from <home>/icons/)")
	c.Flags().StringVar(&theme, "theme", "dark", "icon theme for desktop notifications: dark or light")
	c.Flags().BoolVar(&httpAPI, "http", false, "expose a bearer-token HTTP trigger API while serving")
	c.Flags().IntVar(&httpPort, "http-port", config.DefaultHTTPPort, "port for the HTTP trigger API")
	c.Flags().StringVar(&httpHost, "http-host", config.DefaultHTTPHost,
		"bind address for the HTTP trigger API; anything but loopback exposes it off-box")
	bindFlagCompletion(c, "theme", completeFlagValues(suggest.Themes))
	return c
}

func ensureServeProvider(ctx context.Context, flight string) (stop func()) {
	return serveServer().EnsureLiveProvider(ctx, flight, selfServeArgs()...)
}

func selfServeArgs() []string {
	// The deck-owned provider exists to feed the socket; it must never try to
	// own the HTTP port. Without this, `daemon.http.enabled: true` in config
	// would make every `mino deck` launch a provider that fails to bind
	// whenever a foreground serve already holds the port.
	args := []string{"--http=false"}
	if flagHome != "" {
		args = append(args, "--home", flagHome)
	}
	if flagConfigFile != "" {
		args = append(args, "--config", flagConfigFile)
	}
	if flagRole != "" {
		args = append(args, "--role", flagRole)
	}
	return args
}

func serveSocketTaken() bool {
	return sysdaemon.Attached(config.SocketPrefix, serveServer().SocketPath())
}

func configServeInterval() time.Duration {
	if d, err := time.ParseDuration(shared.Cfg.Daemon.Interval); err == nil && d > 0 {
		return d
	}
	return 60 * time.Second
}

func checkServeInterval(fromFlag bool, d time.Duration) error {
	where := "daemon.interval"
	if fromFlag {
		where = "--interval"
	}
	return signals.CheckPollInterval(where, d)
}

func configServeTheme() string {
	if t := shared.Cfg.Daemon.Theme; t != "" {
		return t
	}
	return "dark"
}

func configServeHTTPPort() int {
	if p := shared.Cfg.Daemon.HTTP.Port; p > 0 {
		return p
	}
	return config.DefaultHTTPPort
}

func configServeHTTPHost() string {
	if h := strings.TrimSpace(shared.Cfg.Daemon.HTTP.Host); h != "" {
		return h
	}
	return config.DefaultHTTPHost
}

func checkServeHTTPHost(fromFlag bool, host string) error {
	where := "daemon.http.host"
	if fromFlag {
		where = "--http-host"
	}
	h := strings.TrimSpace(host)
	if h == "" {
		return errs.Newf(errs.KindUsage, "%s is empty", where).
			WithHint("use 127.0.0.1 for loopback only, or 0.0.0.0 to accept off-box connections")
	}
	if net.ParseIP(strings.Trim(h, "[]")) != nil || strings.EqualFold(h, "localhost") {
		return nil
	}
	if strings.ContainsAny(h, "/\\ \t:") {
		return errs.Newf(errs.KindUsage, "%s %q is not a bind address", where, h).
			WithHint("pass a bare IP or hostname with no scheme, port or path")
	}
	return nil
}

func checkServeHTTPPort(fromFlag bool, port int) error {
	where := "daemon.http.port"
	if fromFlag {
		where = "--http-port"
	}
	if port < 1024 || port > 65535 {
		return errs.Newf(errs.KindUsage, "%s %d is out of range", where, port).
			WithHint("use an unprivileged port between 1024 and 65535")
	}
	return nil
}

func resolveServeFlight(args []string) (string, error) {
	name := defaultFlightName()
	if len(args) == 1 {
		name = args[0]
	}
	if _, ok := shared.Directives.Flights[name]; !ok {
		return "", errs.Newf(errs.KindUsage, "no flight named %q%s", name, availableFlightSuffix()).
			WithHint("run `mino fly` with no argument to list available flights")
	}
	if !access().FlightVisible(name) {
		return "", notInRoleError("flight", name)
	}
	return name, nil
}
