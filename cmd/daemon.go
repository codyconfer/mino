package cmd

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	mdaemon "github.com/codyconfer/munin/internal/app/daemon"
	"github.com/codyconfer/munin/internal/errs"
	"github.com/codyconfer/munin/internal/render/icons"
)

func serveServer() *mdaemon.Server {
	return mdaemon.NewServer(shared)
}

func newServeCmd() *cobra.Command {
	var interval time.Duration
	var bell, desktop bool
	var theme string
	c := &cobra.Command{
		Use:   "serve [flight]",
		Short: "Run munin in the foreground, watching a flight's signals in realtime and notifying",
		Long: "Runs munin in the CURRENT shell as a long-running process that opens each of the\n" +
			"flight's realtime-capable signals, fans their events into one loop, and emits a\n" +
			"notification for each new item. Ctrl-C exits. Logs stream to the shell and the\n" +
			"log dir. serve does NOT install an OS service or own the system tray — use\n" +
			"`munin daemon` for the installed service (and daemon.tray for a tray icon).\n" +
			"--desktop sends OS notifications using per-state icons from <home>/icons/ or\n" +
			"embedded themes. Only Slack is a true websocket; the rest are polled at\n" +
			"--interval; unsupported signals are skipped.",
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: completeFlightNames,
		Annotations:       map[string]string{annoGateMode: modeServe},
		RunE: func(cmd *cobra.Command, args []string) error {
			f := cmd.Flags()
			if !f.Changed("interval") {
				interval = configServeInterval()
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
			name, err := resolveServeFlight(args)
			if err != nil {
				return err
			}
			if desktop {
				icons.LoadStateIcons(shared.Cfg.Home, theme)
			}
			ctx, cancelLifeline := mdaemon.BindDeckLifeline(cmd.Context())
			defer cancelLifeline()
			err = serveServer().Run(ctx, name, interval, bell, desktop, nil)
			fmt.Fprintln(cmd.ErrOrStderr(), "serve: shutting down")
			return err
		},
	}
	c.Flags().DurationVar(&interval, "interval", 60*time.Second, "poll interval floor for polled (non-websocket) signals")
	c.Flags().BoolVar(&bell, "bell", true, "ring the terminal bell on each notification")
	c.Flags().BoolVar(&desktop, "desktop", false, "send OS desktop notifications (uses per-state icons from <home>/icons/)")
	c.Flags().StringVar(&theme, "theme", "dark", "icon theme for desktop notifications: dark or light")
	return c
}

func configServeInterval() time.Duration {
	if d, err := time.ParseDuration(shared.Cfg.Daemon.Interval); err == nil && d > 0 {
		return d
	}
	return 60 * time.Second
}

func configServeTheme() string {
	if t := shared.Cfg.Daemon.Theme; t != "" {
		return t
	}
	return "dark"
}

func resolveServeFlight(args []string) (string, error) {
	name := defaultFlightName()
	if len(args) == 1 {
		name = args[0]
	}
	if _, ok := shared.Directives.Flights[name]; !ok {
		return "", errs.Newf(errs.KindUsage, "no flight named %q%s", name, availableFlightSuffix()).
			WithHint("run `munin fly` with no argument to list available flights")
	}
	if !access().FlightVisible(name) {
		return "", notInRoleError("flight", name)
	}
	return name, nil
}
